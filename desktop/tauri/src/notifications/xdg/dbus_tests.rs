//! An isolated real broker plus a controlled notification service. Never
//! connects to the user's session bus or displays a desktop notification.
use super::*;
use std::{
    io::{BufRead, BufReader},
    process::{Child, Command, Stdio},
    sync::atomic::{AtomicU32, AtomicUsize},
};

struct Bus {
    child: Child,
    _root: tempfile::TempDir,
    address: String,
}
impl Bus {
    fn start() -> Self {
        // Keep the Unix socket below Darwin's socket-path length limit too.
        let root = tempfile::Builder::new()
            .prefix("reasonix-dbus-")
            .tempdir_in("/tmp")
            .unwrap();
        let config = root.path().join("bus.conf");
        std::fs::write(
            &config,
            format!(
                "<busconfig><type>session</type><listen>unix:tmpdir={}</listen>\
                 <auth>EXTERNAL</auth><policy context=\"default\">\
                 <allow user=\"*\"/><allow own=\"*\"/>\
                 <allow send_destination=\"*\"/><allow receive_sender=\"*\"/>\
                 </policy></busconfig>",
                root.path().display()
            ),
        )
        .unwrap();
        let binary =
            std::env::var_os("REASONIX_XDG_DBUS_TEST_BIN").unwrap_or_else(|| "dbus-daemon".into());
        let child = Command::new(binary)
            .arg("--nofork")
            .arg("--nopidfile")
            .arg("--print-address=1")
            .arg(format!("--config-file={}", config.display()))
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::inherit())
            .spawn()
            .expect("requires dbus-daemon or REASONIX_XDG_DBUS_TEST_BIN");
        let mut bus = Self {
            child,
            _root: root,
            address: String::new(),
        };
        let stdout = bus.child.stdout.take().unwrap();
        let (ready, receive) = mpsc::sync_channel(1);
        let reader = thread::spawn(move || {
            let mut address = String::new();
            let result = BufReader::new(stdout)
                .read_line(&mut address)
                .map(|_| address.trim().to_string());
            let _ = ready.send(result);
        });
        bus.address = receive
            .recv_timeout(Duration::from_secs(3))
            .expect("private bus must become ready within three seconds")
            .unwrap();
        reader.join().unwrap();
        assert!(bus.address.starts_with("unix:"));
        bus
    }
}
impl Drop for Bus {
    fn drop(&mut self) {
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}

const NORMAL: usize = 0;
const EARLY_CLICK: usize = 1;
const FAIL: usize = 2;
const HANG: usize = 3;
#[derive(Debug)]
struct Submission {
    app: String,
    replaces: u32,
    icon: String,
    title: String,
    body: String,
    actions: Vec<String>,
    silent: bool,
    hint_count: usize,
    timeout: i32,
}
struct Service {
    capabilities: Mutex<Vec<String>>,
    fail_capabilities: AtomicBool,
    mode: AtomicUsize,
    next: AtomicU32,
    submissions: Mutex<Vec<Submission>>,
    hanging: mpsc::SyncSender<()>,
    resumed: async_channel::Receiver<()>,
}
struct MockService(Arc<Service>);
#[zbus::interface(name = "org.freedesktop.Notifications")]
impl MockService {
    fn get_capabilities(&self) -> zbus::fdo::Result<Vec<String>> {
        if self.0.fail_capabilities.load(Ordering::SeqCst) {
            Err(zbus::fdo::Error::Failed(
                "private-service-diagnostic".into(),
            ))
        } else {
            Ok(self.0.capabilities.lock().unwrap().clone())
        }
    }
    // The eight wire arguments are prescribed by the XDG Notify protocol.
    #[allow(clippy::too_many_arguments)]
    async fn notify(
        &self,
        app_name: &str,
        replaces_id: u32,
        app_icon: &str,
        summary: &str,
        body: &str,
        actions: Vec<String>,
        hints: std::collections::HashMap<String, zbus::zvariant::OwnedValue>,
        expire_timeout: i32,
        #[zbus(connection)] connection: &Connection,
    ) -> zbus::fdo::Result<u32> {
        self.0.submissions.lock().unwrap().push(Submission {
            app: app_name.into(),
            replaces: replaces_id,
            icon: app_icon.into(),
            title: summary.into(),
            body: body.into(),
            actions,
            silent: hints
                .get("suppress-sound")
                .and_then(|value| bool::try_from(value).ok())
                .unwrap_or(false),
            hint_count: hints.len(),
            timeout: expire_timeout,
        });
        match self.0.mode.load(Ordering::SeqCst) {
            FAIL => Err(zbus::fdo::Error::Failed(
                "private-service-diagnostic".into(),
            )),
            HANG => {
                let _ = self.0.hanging.try_send(());
                // Test teardown closes this gate so the mock's outstanding
                // method tasks release their connection references too.
                let _ = self.0.resumed.recv().await;
                Err(zbus::fdo::Error::Failed("test request released".into()))
            }
            mode => {
                let id = self.0.next.fetch_add(1, Ordering::SeqCst);
                if mode == EARLY_CLICK {
                    connection
                        .emit_signal(None::<&str>, PATH, NAME, "ActionInvoked", &(id, "default"))
                        .await?;
                }
                Ok(id)
            }
        }
    }
}
fn connect(address: &str, service: Option<Arc<Service>>) -> Connection {
    future::block_on(async {
        let builder = zbus::connection::Builder::address(address)
            .unwrap()
            .method_timeout(CALL_TIMEOUT);
        let builder = if let Some(service) = service {
            builder
                .serve_at(PATH, MockService(service))
                .unwrap()
                .name(NAME)
                .unwrap()
        } else {
            builder
        };
        bounded(builder.build()).await.unwrap()
    })
}
fn emit<T: serde::Serialize + zbus::zvariant::DynamicType>(
    connection: &Connection,
    member: &str,
    body: &T,
) {
    future::block_on(bounded(connection.emit_signal(
        None::<&str>,
        PATH,
        NAME,
        member,
        body,
    )))
    .unwrap();
}
fn token(id: u32) -> String {
    format!("reasonix-preview-{id:032x}")
}
#[test]
#[ignore = "requires an isolated dbus-daemon; Linux CI runs this explicitly"]
fn real_broker_covers_delivery_actions_owner_restart_failure_and_shutdown() {
    let bus = Bus::start();
    let (hanging, hung) = mpsc::sync_channel(1);
    let (resume, resumed) = async_channel::bounded::<()>(1);
    let service = Arc::new(Service {
        capabilities: Mutex::new(vec!["body".into()]),
        fail_capabilities: AtomicBool::new(false),
        mode: AtomicUsize::new(NORMAL),
        next: AtomicU32::new(1),
        submissions: Mutex::new(vec![]),
        hanging,
        resumed,
    });
    let backend = Arc::new(XdgBackend::with_address(
        "io.reasonix.desktop.preview",
        Address::Explicit(bus.address.clone()),
    ));
    let (activated, activations) = mpsc::channel();
    backend
        .set_callback(Arc::new(move |token| {
            activated.send(token.to_string()).unwrap();
        }))
        .unwrap();
    assert_eq!(backend.status().permission, Permission::Unavailable);
    let daemon = connect(&bus.address, Some(service.clone()));
    let status = backend.status();
    assert_eq!(status.permission, Permission::Unknown);
    assert!(!status.click_supported);
    backend.send(&token(1), "Reply complete.").unwrap();
    {
        let sent = service.submissions.lock().unwrap();
        assert_eq!(sent.len(), 1);
        let sent = &sent[0];
        assert_eq!(sent.app, "Reasonix");
        assert_eq!(sent.replaces, 0);
        assert_eq!(sent.icon, "io.reasonix.desktop.preview");
        assert_eq!(sent.title, "Reasonix");
        assert_eq!(sent.body, "Reply complete.");
        assert!(sent.actions.is_empty());
        assert!(sent.silent);
        assert_eq!(sent.hint_count, 1);
        assert_eq!(sent.timeout, -1);
        assert!(!format!("{sent:?}").contains(&token(1)));
    }
    *service.capabilities.lock().unwrap() = vec!["actions".into(), "body".into()];
    assert!(backend.status().click_supported);
    service.mode.store(EARLY_CLICK, Ordering::SeqCst);
    backend.send(&token(2), "Reply complete.").unwrap();
    assert_eq!(activations.recv_timeout(CALL_TIMEOUT).unwrap(), token(2));
    assert_eq!(
        service.submissions.lock().unwrap()[1].actions,
        ["default", "Reasonix"]
    );
    emit(&daemon, "ActionInvoked", &(2u32, "default"));
    service.mode.store(NORMAL, Ordering::SeqCst);
    backend.send(&token(3), "Reply complete.").unwrap();
    let attacker = connect(&bus.address, None);
    emit(&attacker, "ActionInvoked", &(3u32, "default"));
    emit(&daemon, "ActionInvoked", &(3u32, "other-button"));
    emit(&daemon, "NotificationClosed", &(3u32, 2u32));
    emit(&daemon, "ActionInvoked", &(3u32, "default"));
    // A genuine later click acts as a barrier for earlier ordered service
    // signals. Any duplicate, forged, non-default or closed click fails here.
    service.mode.store(EARLY_CLICK, Ordering::SeqCst);
    backend.send(&token(4), "Reply complete.").unwrap();
    assert_eq!(activations.recv_timeout(CALL_TIMEOUT).unwrap(), token(4));
    service.mode.store(NORMAL, Ordering::SeqCst);
    backend.send(&token(5), "Reply complete.").unwrap();
    let old_owner = daemon.unique_name().unwrap().to_string();
    future::block_on(bounded(daemon.close())).unwrap();
    assert_eq!(backend.status().permission, Permission::Unavailable);
    // A restarted daemon may reuse its IDs; old bindings must be discarded.
    service.next.store(5, Ordering::SeqCst);
    let daemon = connect(&bus.address, Some(service.clone()));
    assert_ne!(daemon.unique_name().unwrap().as_str(), old_owner);
    assert!(backend.status().click_supported);
    backend.send(&token(6), "Reply complete.").unwrap();
    service.fail_capabilities.store(true, Ordering::SeqCst);
    let status = backend.status();
    assert_eq!(status.permission, Permission::Unavailable);
    assert!(!status.click_supported);
    service.fail_capabilities.store(false, Ordering::SeqCst);
    service.mode.store(FAIL, Ordering::SeqCst);
    assert_eq!(
        backend.send(&token(7), "Reply complete.").unwrap_err(),
        DELIVERY_ERROR
    );
    // A failed later delivery/capability query must preserve an earlier
    // banner's binding, including a daemon-reused ID after a restart.
    emit(&daemon, "ActionInvoked", &(5u32, "default"));
    assert_eq!(activations.recv_timeout(CALL_TIMEOUT).unwrap(), token(6));
    service.mode.store(EARLY_CLICK, Ordering::SeqCst);
    backend.send(&token(8), "Reply complete.").unwrap();
    assert_eq!(activations.recv_timeout(CALL_TIMEOUT).unwrap(), token(8));
    assert!(activations.recv_timeout(Duration::from_millis(80)).is_err());
    service.mode.store(HANG, Ordering::SeqCst);
    let before = Instant::now();
    assert_eq!(
        backend.send(&token(9), "Reply complete.").unwrap_err(),
        DELIVERY_ERROR
    );
    assert!(before.elapsed() < Duration::from_secs(3));
    hung.recv_timeout(CALL_TIMEOUT).unwrap();
    service.mode.store(EARLY_CLICK, Ordering::SeqCst);
    backend.send(&token(10), "Reply complete.").unwrap();
    assert_eq!(activations.recv_timeout(CALL_TIMEOUT).unwrap(), token(10));
    service.mode.store(HANG, Ordering::SeqCst);
    let sender = backend.clone();
    let sending = thread::spawn(move || sender.send(&token(11), "Reply complete."));
    hung.recv_timeout(CALL_TIMEOUT).unwrap();
    let before = Instant::now();
    backend.shutdown();
    assert!(before.elapsed() < Duration::from_secs(3));
    assert_eq!(sending.join().unwrap().unwrap_err(), DELIVERY_ERROR);
    assert!(backend.worker.lock().unwrap().is_none());
    assert!(backend.callback.lock().unwrap().is_none());
    assert_eq!(backend.status().permission, Permission::Unavailable);
    resume.close();
    future::block_on(bounded(attacker.close())).unwrap();
    future::block_on(bounded(daemon.close())).unwrap();
}
