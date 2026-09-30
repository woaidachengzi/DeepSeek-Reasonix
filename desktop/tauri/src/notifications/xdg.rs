//! One bounded XDG notification worker per host, with sender-scoped signal
//! subscriptions installed before delivery. No per-notification waiter threads.
use super::{valid_token, NotificationStatus, Permission, DELIVERY_ERROR, MAX_TARGETS, TTL};
use futures_lite::{future, StreamExt};
use std::{
    collections::BTreeMap,
    sync::{
        atomic::{AtomicBool, Ordering},
        mpsc, Arc, Mutex,
    },
    thread,
    time::{Duration, Instant},
};
use zbus::{Connection, MatchRule, Message, MessageStream};

#[cfg(test)]
mod dbus_tests;

const NAME: &str = "org.freedesktop.Notifications";
const PATH: &str = "/org/freedesktop/Notifications";
const BUS: &str = "org.freedesktop.DBus";
const BUS_PATH: &str = "/org/freedesktop/DBus";
const CALL_TIMEOUT: Duration = Duration::from_secs(2);
const REQUEST_TIMEOUT: Duration = Duration::from_secs(5);
const RESPONSE_TIMEOUT: Duration = Duration::from_secs(6);
type Callback = Arc<dyn Fn(&str) + Send + Sync>;
type Reply<T> = mpsc::SyncSender<T>;

enum Request {
    Status {
        reply: Reply<NotificationStatus>,
        deadline: Instant,
    },
    Send {
        token: String,
        body: String,
        reply: Reply<Result<(), String>>,
        deadline: Instant,
    },
}
#[derive(Clone)]
enum Address {
    Session,
    #[cfg(test)]
    Explicit(String),
}
struct Worker {
    commands: async_channel::Sender<Request>,
    stop: async_channel::Sender<()>,
    thread: thread::JoinHandle<()>,
}
pub(super) struct XdgBackend {
    identifier: String,
    address: Address,
    callback: Arc<Mutex<Option<Callback>>>,
    worker: Mutex<Option<Worker>>,
    stopped: AtomicBool,
}
impl XdgBackend {
    pub fn new(identifier: &str) -> Self {
        Self::with_address(identifier, Address::Session)
    }
    fn with_address(identifier: &str, address: Address) -> Self {
        Self {
            identifier: identifier.into(),
            address,
            callback: Arc::new(Mutex::new(None)),
            worker: Mutex::new(None),
            stopped: AtomicBool::new(false),
        }
    }
    pub fn set_callback(&self, callback: Callback) -> Result<(), String> {
        let mut slot = self.callback.lock().map_err(|_| DELIVERY_ERROR)?;
        if self.stopped.load(Ordering::Acquire) {
            return Err(DELIVERY_ERROR.into());
        }
        *slot = Some(callback);
        Ok(())
    }
    fn commands(&self) -> Result<async_channel::Sender<Request>, String> {
        let mut worker = self.worker.lock().map_err(|_| DELIVERY_ERROR)?;
        if self.stopped.load(Ordering::Acquire) {
            return Err(DELIVERY_ERROR.into());
        }
        if worker.as_ref().is_some_and(|w| w.thread.is_finished()) {
            if let Some(old) = worker.take() {
                let _ = old.thread.join();
            }
        }
        if worker.is_none() {
            let (commands, requests) = async_channel::bounded(16);
            let (stop, stopped) = async_channel::bounded(1);
            let identifier = self.identifier.clone();
            let address = self.address.clone();
            let callback = self.callback.clone();
            let thread = thread::Builder::new()
                .name("reasonix-notifications".into())
                .spawn(move || {
                    future::block_on(run(identifier, address, callback, requests, stopped))
                })
                .map_err(|_| DELIVERY_ERROR)?;
            *worker = Some(Worker {
                commands,
                stop,
                thread,
            });
        }
        Ok(worker.as_ref().unwrap().commands.clone())
    }
    pub fn status(&self) -> NotificationStatus {
        let (reply, receive) = mpsc::sync_channel(1);
        let Ok(commands) = self.commands() else {
            return unavailable();
        };
        if commands
            .try_send(Request::Status {
                reply,
                deadline: Instant::now() + REQUEST_TIMEOUT,
            })
            .is_err()
        {
            return unavailable();
        }
        receive
            .recv_timeout(RESPONSE_TIMEOUT)
            .unwrap_or_else(|_| unavailable())
    }
    pub fn send(&self, token: &str, body: &str) -> Result<(), String> {
        if !valid_token(token) {
            return Err(DELIVERY_ERROR.into());
        }
        let (reply, receive) = mpsc::sync_channel(1);
        self.commands()?
            .try_send(Request::Send {
                token: token.into(),
                body: body.into(),
                reply,
                deadline: Instant::now() + REQUEST_TIMEOUT,
            })
            .map_err(|_| DELIVERY_ERROR)?;
        receive
            .recv_timeout(RESPONSE_TIMEOUT)
            .map_err(|_| DELIVERY_ERROR)?
    }
    pub fn shutdown(&self) {
        self.stopped.store(true, Ordering::Release);
        if let Ok(mut callback) = self.callback.lock() {
            *callback = None;
        }
        if let Ok(mut slot) = self.worker.lock() {
            if let Some(worker) = slot.take() {
                worker.stop.close();
                worker.commands.close();
                let _ = worker.thread.join();
            }
        }
    }
}
impl Drop for XdgBackend {
    fn drop(&mut self) {
        self.shutdown();
    }
}
fn unavailable() -> NotificationStatus {
    NotificationStatus {
        permission: Permission::Unavailable,
        click_supported: false,
    }
}
fn status(capabilities: &[String], callback: bool) -> NotificationStatus {
    // XDG has no standard permission/DND query. Server presence and action
    // support are capabilities, never proof of granted user authorization.
    NotificationStatus {
        permission: Permission::Unknown,
        click_supported: callback && capabilities.iter().any(|c| c == "actions"),
    }
}
async fn bounded<T>(
    operation: impl std::future::Future<Output = zbus::Result<T>>,
) -> zbus::Result<T> {
    future::race(operation, async {
        async_io::Timer::after(CALL_TIMEOUT).await;
        Err(zbus::Error::Failure(
            "notification request timed out".into(),
        ))
    })
    .await
}

struct Client {
    connection: Connection,
    owner: String,
    notifications: MessageStream,
    owners: MessageStream,
    targets: Tracker,
}
impl Client {
    async fn connect(address: &Address) -> zbus::Result<Self> {
        let builder = match address {
            Address::Session => zbus::connection::Builder::session()?,
            #[cfg(test)]
            Address::Explicit(value) => zbus::connection::Builder::address(value.as_str())?,
        };
        let connection = bounded(builder.method_timeout(CALL_TIMEOUT).build()).await?;
        let owner = get_owner(&connection).await?;
        let rule = MatchRule::builder()
            .msg_type(zbus::message::Type::Signal)
            .sender(owner.as_str())?
            .path(PATH)?
            .interface(NAME)?
            .build();
        let notifications =
            bounded(MessageStream::for_match_rule(rule, &connection, Some(64))).await?;
        let rule = MatchRule::builder()
            .msg_type(zbus::message::Type::Signal)
            .sender(BUS)?
            .path(BUS_PATH)?
            .interface(BUS)?
            .member("NameOwnerChanged")?
            .arg(0, NAME)?
            .build();
        let owners = bounded(MessageStream::for_match_rule(rule, &connection, Some(16))).await?;
        // Recheck after subscriptions to reject a daemon replacement during setup.
        if get_owner(&connection).await? != owner {
            return Err(zbus::Error::Failure("notification service changed".into()));
        }
        Ok(Self {
            connection,
            owner,
            notifications,
            owners,
            targets: Tracker::default(),
        })
    }
    async fn current(&self) -> bool {
        get_owner(&self.connection)
            .await
            .is_ok_and(|owner| owner == self.owner)
    }
    async fn capabilities(&self) -> zbus::Result<Vec<String>> {
        let reply = bounded(self.connection.call_method(
            Some(self.owner.as_str()),
            PATH,
            Some(NAME),
            "GetCapabilities",
            &(),
        ))
        .await?;
        reply.body().deserialize()
    }
    async fn send(
        &mut self,
        identifier: &str,
        token: &str,
        body: &str,
        actions_supported: bool,
    ) -> zbus::Result<()> {
        let actions = if actions_supported {
            vec!["default", "Reasonix"]
        } else {
            vec![]
        };
        let hints: std::collections::HashMap<&str, zbus::zvariant::Value<'_>> =
            [("suppress-sound", true.into())].into();
        let payload = (
            "Reasonix", 0u32, identifier, "Reasonix", body, actions, hints, -1i32,
        );
        // Both signal streams exist before Notify; a click emitted before the
        // method reply is buffered until its returned ID is bound to this token.
        let reply = bounded(self.connection.call_method(
            Some(self.owner.as_str()),
            PATH,
            Some(NAME),
            "Notify",
            &payload,
        ))
        .await?;
        let id: u32 = reply.body().deserialize()?;
        if !self
            .targets
            .insert(id, token, Instant::now(), actions_supported)
        {
            return Err(zbus::Error::Failure(
                "notification identity is invalid".into(),
            ));
        }
        Ok(())
    }
}
async fn get_owner(connection: &Connection) -> zbus::Result<String> {
    let reply =
        bounded(connection.call_method(Some(BUS), BUS_PATH, Some(BUS), "GetNameOwner", &(NAME,)))
            .await?;
    reply.body().deserialize()
}
#[derive(Default)]
struct Tracker {
    targets: BTreeMap<u32, (String, Instant, bool)>,
}
impl Tracker {
    fn insert(&mut self, id: u32, token: &str, now: Instant, clickable: bool) -> bool {
        self.targets
            .retain(|_, (_, created, _)| now.saturating_duration_since(*created).as_secs() <= TTL);
        if id == 0 || !valid_token(token) {
            return false;
        }
        // An unexpected ID reuse invalidates both targets; never navigate a
        // different session under an old banner's ID.
        if self.targets.remove(&id).is_some() {
            return false;
        }
        if self.targets.len() >= MAX_TARGETS {
            if let Some(oldest) = self
                .targets
                .iter()
                .min_by_key(|(_, (_, created, _))| *created)
                .map(|(id, _)| *id)
            {
                self.targets.remove(&oldest);
            }
        }
        self.targets.insert(id, (token.into(), now, clickable));
        true
    }
    fn action(&mut self, id: u32, key: &str, now: Instant) -> Option<String> {
        if key != "default" {
            return None;
        }
        let (token, created, clickable) = self.targets.remove(&id)?;
        (clickable && now.saturating_duration_since(created).as_secs() <= TTL).then_some(token)
    }
    fn close(&mut self, id: u32) {
        self.targets.remove(&id);
    }
}
fn event(message: &Message, owner: &str) -> Option<Event> {
    let header = message.header();
    if header.message_type() != zbus::message::Type::Signal {
        return None;
    }
    let sender = header.sender()?.as_str();
    let path = header.path()?.as_str();
    let interface = header.interface()?.as_str();
    if sender == BUS
        && path == BUS_PATH
        && interface == BUS
        && header.member()?.as_str() == "NameOwnerChanged"
    {
        let (name, old, new): (String, String, String) = message.body().deserialize().ok()?;
        if name == NAME && old == owner && new != owner {
            return Some(Event::ServiceChanged);
        }
        return None;
    }
    if sender != owner || path != PATH || interface != NAME {
        return None;
    }
    match header.member()?.as_str() {
        "ActionInvoked" => {
            let (id, key): (u32, String) = message.body().deserialize().ok()?;
            Some(Event::Action(id, key))
        }
        "NotificationClosed" => {
            let (id, _): (u32, u32) = message.body().deserialize().ok()?;
            Some(Event::Closed(id))
        }
        _ => None,
    }
}
enum Event {
    Action(u32, String),
    Closed(u32),
    ServiceChanged,
}
enum Step {
    Command(Request),
    Signal(zbus::Result<Message>),
    Stop,
}
async fn next(
    client: &mut Option<Client>,
    requests: &async_channel::Receiver<Request>,
    stopped: &async_channel::Receiver<()>,
) -> Step {
    future::race(
        async {
            match requests.recv().await {
                Ok(request) => Step::Command(request),
                Err(_) => Step::Stop,
            }
        },
        future::race(
            async {
                let _ = stopped.recv().await;
                Step::Stop
            },
            async {
                let Some(client) = client.as_mut() else {
                    return std::future::pending().await;
                };
                future::race(
                    async {
                        match client.notifications.try_next().await {
                            Ok(Some(message)) => Step::Signal(Ok(message)),
                            Ok(None) => Step::Signal(Err(zbus::Error::Failure(
                                "notification connection closed".into(),
                            ))),
                            Err(error) => Step::Signal(Err(error)),
                        }
                    },
                    async {
                        match client.owners.try_next().await {
                            Ok(Some(message)) => Step::Signal(Ok(message)),
                            Ok(None) => Step::Signal(Err(zbus::Error::Failure(
                                "notification connection closed".into(),
                            ))),
                            Err(error) => Step::Signal(Err(error)),
                        }
                    },
                )
                .await
            },
        ),
    )
    .await
}
async fn ensure(client: &mut Option<Client>, address: &Address) -> bool {
    if let Some(current) = client {
        if current.current().await {
            return true;
        }
    }
    *client = Client::connect(address).await.ok();
    client.is_some()
}
async fn execute<T>(
    deadline: Instant,
    stopped: &async_channel::Receiver<()>,
    operation: impl std::future::Future<Output = zbus::Result<T>>,
) -> zbus::Result<T> {
    if stopped.is_closed() || Instant::now() >= deadline {
        return Err(zbus::Error::Failure(
            "notification request cancelled".into(),
        ));
    }
    future::race(
        operation,
        future::race(
            async {
                async_io::Timer::after(deadline.saturating_duration_since(Instant::now())).await;
                Err(zbus::Error::Failure(
                    "notification request timed out".into(),
                ))
            },
            async {
                let _ = stopped.recv().await;
                Err(zbus::Error::Failure("notification worker stopped".into()))
            },
        ),
    )
    .await
}

async fn run(
    identifier: String,
    address: Address,
    callback: Arc<Mutex<Option<Callback>>>,
    requests: async_channel::Receiver<Request>,
    stopped: async_channel::Receiver<()>,
) {
    let mut client = None;
    loop {
        if stopped.is_closed() {
            break;
        }
        match next(&mut client, &requests, &stopped).await {
            Step::Stop => break,
            Step::Signal(Err(_)) => client = None,
            Step::Signal(Ok(message)) => {
                if let Some(current) = client.as_mut() {
                    match event(&message, &current.owner) {
                        Some(Event::ServiceChanged) => client = None,
                        Some(Event::Closed(id)) => current.targets.close(id),
                        Some(Event::Action(id, key)) => {
                            if let Some(token) = current.targets.action(id, &key, Instant::now()) {
                                let callback = callback.lock().ok().and_then(|value| value.clone());
                                if let Some(callback) = callback {
                                    callback(&token);
                                }
                            }
                        }
                        None => {}
                    }
                }
            }
            Step::Command(Request::Status { reply, deadline }) => {
                let result = execute(deadline, &stopped, async {
                    if !ensure(&mut client, &address).await {
                        return Err(zbus::Error::Failure(
                            "notification service unavailable".into(),
                        ));
                    }
                    let caps = client.as_ref().unwrap().capabilities().await?;
                    Ok(status(
                        &caps,
                        callback.lock().is_ok_and(|value| value.is_some()),
                    ))
                })
                .await;
                // A rejected/slow request does not invalidate IDs of other
                // delivered banners. Owner changes and stream disconnects
                // retire the client; ensure() rechecks ownership on every call.
                let _ = reply.send(result.unwrap_or_else(|_| unavailable()));
            }
            Step::Command(Request::Send {
                token,
                body,
                reply,
                deadline,
            }) => {
                let result = execute(deadline, &stopped, async {
                    if !ensure(&mut client, &address).await {
                        return Err(zbus::Error::Failure(
                            "notification service unavailable".into(),
                        ));
                    }
                    let current = client.as_mut().unwrap();
                    let caps = current.capabilities().await?;
                    let clicks = status(&caps, callback.lock().is_ok_and(|value| value.is_some()))
                        .click_supported;
                    current.send(&identifier, &token, &body, clicks).await
                })
                .await;
                let _ = reply.send(result.map_err(|_| DELIVERY_ERROR.to_string()));
            }
        }
    }
    if let Some(client) = client {
        let _ = bounded(client.connection.close()).await;
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    fn token(id: u32) -> String {
        format!("reasonix-preview-{id:032x}")
    }
    fn signal<T: serde::Serialize + zbus::zvariant::DynamicType>(
        sender: &str,
        path: &str,
        interface: &str,
        member: &str,
        body: &T,
    ) -> Message {
        Message::signal(path, interface, member)
            .unwrap()
            .sender(sender)
            .unwrap()
            .build(body)
            .unwrap()
    }
    #[test]
    fn capabilities_report_unknown_authorization_and_require_actions_and_live_callback() {
        assert_eq!(
            status(&["body".into()], true).permission,
            Permission::Unknown
        );
        assert!(!status(&["body".into()], true).click_supported);
        assert!(!status(&["actions".into()], false).click_supported);
        assert!(status(&["actions".into(), "body".into()], true).click_supported);
        assert_eq!(
            status(&["actions".into()], true).permission,
            Permission::Unknown
        );
        assert_eq!(unavailable().permission, Permission::Unavailable);
        assert!(!unavailable().click_supported);
    }
    #[test]
    fn actions_only_navigate_requested_default_once_and_close_prevents_replay() {
        let now = Instant::now();
        let mut tracker = Tracker::default();
        assert!(tracker.insert(1, &token(1), now, true));
        assert!(tracker.action(1, "approve", now).is_none());
        assert!(tracker.action(99, "default", now).is_none());
        assert_eq!(tracker.action(1, "default", now), Some(token(1)));
        assert!(tracker.action(1, "default", now).is_none());
        assert!(tracker.insert(2, &token(2), now, true));
        tracker.close(2);
        assert!(tracker.action(2, "default", now).is_none());
        assert!(tracker.insert(3, &token(3), now, false));
        assert!(tracker.action(3, "default", now).is_none());
    }
    #[test]
    fn duplicate_id_invalidates_both_targets_and_expiry_and_capacity_are_bounded() {
        let now = Instant::now();
        let mut tracker = Tracker::default();
        assert!(!tracker.insert(0, &token(1), now, true));
        assert!(!tracker.insert(1, "session-one", now, true));
        assert!(tracker.insert(1, &token(1), now, true));
        assert!(!tracker.insert(1, &token(2), now, true));
        assert!(tracker.action(1, "default", now).is_none());
        assert!(tracker.insert(2, &token(2), now, true));
        assert!(tracker
            .action(2, "default", now + Duration::from_secs(TTL + 1))
            .is_none());
        for id in 1..=(MAX_TARGETS as u32 + 1) {
            assert!(tracker.insert(id, &token(id), now + Duration::from_secs(id as u64), true));
        }
        assert_eq!(tracker.targets.len(), MAX_TARGETS);
        assert!(!tracker.targets.contains_key(&1));
        assert!(tracker.targets.contains_key(&(MAX_TARGETS as u32 + 1)));
    }
    #[test]
    fn signal_parser_rejects_forged_sender_path_interface_member_and_malformed_body() {
        let owner = ":1.10";
        let genuine = signal(owner, PATH, NAME, "ActionInvoked", &(7u32, "default"));
        assert!(matches!(event(&genuine, owner), Some(Event::Action(7, key)) if key == "default"));
        for msg in [
            signal(":1.11", PATH, NAME, "ActionInvoked", &(7u32, "default")),
            signal(owner, "/forged", NAME, "ActionInvoked", &(7u32, "default")),
            signal(
                owner,
                PATH,
                "org.example.Forged",
                "ActionInvoked",
                &(7u32, "default"),
            ),
            signal(owner, PATH, NAME, "Unknown", &(7u32, "default")),
            signal(
                owner,
                PATH,
                NAME,
                "ActionInvoked",
                &("session-private", "default"),
            ),
        ] {
            assert!(event(&msg, owner).is_none());
        }
        assert!(matches!(
            event(
                &signal(owner, PATH, NAME, "NotificationClosed", &(7u32, 2u32)),
                owner
            ),
            Some(Event::Closed(7))
        ));
    }
    #[test]
    fn service_generation_changes_only_trust_bus_owner_change_for_current_service() {
        let owner = ":1.10";
        assert!(matches!(
            event(
                &signal(
                    BUS,
                    BUS_PATH,
                    BUS,
                    "NameOwnerChanged",
                    &(NAME, owner, ":1.12")
                ),
                owner
            ),
            Some(Event::ServiceChanged)
        ));
        for msg in [
            signal(
                ":1.11",
                BUS_PATH,
                BUS,
                "NameOwnerChanged",
                &(NAME, owner, ":1.12"),
            ),
            signal(
                BUS,
                BUS_PATH,
                BUS,
                "NameOwnerChanged",
                &("org.example.Other", owner, ":1.12"),
            ),
            signal(
                BUS,
                BUS_PATH,
                BUS,
                "NameOwnerChanged",
                &(NAME, ":1.13", ":1.12"),
            ),
            signal(
                BUS,
                BUS_PATH,
                BUS,
                "NameOwnerChanged",
                &(NAME, owner, owner),
            ),
        ] {
            assert!(event(&msg, owner).is_none());
        }
    }
    #[test]
    fn unavailable_bus_is_truthful_and_single_worker_retries_then_shutdown_releases_listener() {
        let root = tempfile::tempdir().unwrap();
        let backend = XdgBackend::with_address(
            "io.reasonix.desktop.preview",
            Address::Explicit(format!(
                "unix:path={}",
                root.path().join("missing-bus").display()
            )),
        );
        backend
            .set_callback(Arc::new(|_| {
                panic!("unavailable bus must not activate a session")
            }))
            .unwrap();
        assert_eq!(backend.status().permission, Permission::Unavailable);
        let worker = backend
            .worker
            .lock()
            .unwrap()
            .as_ref()
            .unwrap()
            .thread
            .thread()
            .id();
        assert!(!backend.status().click_supported);
        assert_eq!(
            backend
                .worker
                .lock()
                .unwrap()
                .as_ref()
                .unwrap()
                .thread
                .thread()
                .id(),
            worker
        );
        assert_eq!(
            backend.send(&token(1), "Reply complete.").unwrap_err(),
            DELIVERY_ERROR
        );
        backend.shutdown();
        assert!(backend.worker.lock().unwrap().is_none());
        assert!(backend.callback.lock().unwrap().is_none());
        assert_eq!(backend.status().permission, Permission::Unavailable);
        assert!(backend.send(&token(2), "Reply complete.").is_err());
        assert!(backend.set_callback(Arc::new(|_| {})).is_err());
        assert!(backend.callback.lock().unwrap().is_none());
        assert!(backend.worker.lock().unwrap().is_none());
    }
    #[test]
    fn expired_queued_request_and_shutdown_do_not_dispatch_and_hung_call_times_out() {
        use std::sync::atomic::AtomicUsize;
        let calls = AtomicUsize::new(0);
        let (stop, stopped) = async_channel::bounded(1);
        let request = async {
            calls.fetch_add(1, Ordering::SeqCst);
            Ok(())
        };
        assert!(future::block_on(execute(
            Instant::now() - Duration::from_secs(1),
            &stopped,
            request
        ))
        .is_err());
        assert_eq!(calls.load(Ordering::SeqCst), 0);
        stop.close();
        let request = async {
            calls.fetch_add(1, Ordering::SeqCst);
            Ok(())
        };
        assert!(
            future::block_on(execute(Instant::now() + REQUEST_TIMEOUT, &stopped, request)).is_err()
        );
        assert_eq!(calls.load(Ordering::SeqCst), 0);
        let (_stop, stopped) = async_channel::bounded(1);
        let before = Instant::now();
        assert!(future::block_on(execute(
            before + Duration::from_millis(20),
            &stopped,
            std::future::pending::<zbus::Result<()>>()
        ))
        .is_err());
        assert!(before.elapsed() < Duration::from_secs(1));
    }
}
