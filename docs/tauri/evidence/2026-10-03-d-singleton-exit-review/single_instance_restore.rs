//! macOS singleton restoration is coordinated with the short-lived launcher's
//! termination, so it cannot take activation back after the primary restores.
use block2::RcBlock;
use objc2::{rc::Retained, runtime::ProtocolObject, MainThreadMarker};
use objc2_app_kit::{
    NSRunningApplication, NSWorkspace, NSWorkspaceDidTerminateApplicationNotification,
};
use objc2_foundation::{NSNotification, NSObjectProtocol, NSOperationQueue, NSString};
use std::{cell::RefCell, ptr::NonNull};

#[derive(Default)]
struct Pending {
    peers: Vec<(i32, Retained<NSRunningApplication>)>,
    requested: bool,
    observer: Option<Retained<ProtocolObject<dyn NSObjectProtocol>>>,
}
thread_local! {
    // AppKit objects and notification tokens never cross the native UI thread.
    static PENDING: RefCell<Pending> = RefCell::new(Pending::default());
}

fn remove_token(token: Retained<ProtocolObject<dyn NSObjectProtocol>>) {
    // SAFETY: This exact token was registered with the workspace center on the
    // same main thread, and is removed before its retained reference is dropped.
    unsafe {
        NSWorkspace::sharedWorkspace()
            .notificationCenter()
            .removeObserver((*token).as_ref());
    }
}

pub fn stop() {
    if MainThreadMarker::new().is_none() {
        return;
    }
    let token = PENDING.with(|pending| {
        let mut pending = pending.borrow_mut();
        pending.peers.clear();
        pending.requested = false;
        pending.observer.take()
    });
    if let Some(token) = token {
        remove_token(token);
    }
}

fn reconcile(app: &tauri::AppHandle) {
    if MainThreadMarker::new().is_none() {
        return;
    }
    let restore = PENDING.with(|pending| {
        let mut pending = pending.borrow_mut();
        if !pending.requested {
            return false;
        }
        pending.peers.retain(|(_, peer)| !peer.isTerminated());
        if pending.peers.is_empty() {
            pending.requested = false;
            true
        } else {
            false
        }
    });
    if restore {
        crate::tray::show_main_window(app);
    }
}

pub fn install(app: &tauri::AppHandle) {
    if MainThreadMarker::new().is_none()
        || PENDING.with(|pending| pending.borrow().observer.is_some())
    {
        return;
    }
    let target = app.clone();
    let callback = RcBlock::new(move |_: NonNull<NSNotification>| reconcile(&target));
    // SAFETY: Registration and delivery use the AppKit main thread. No payload
    // PID is trusted; only retained system peers are inspected. Install before
    // the singleton socket listener can receive a secondary launch.
    let token = unsafe {
        NSWorkspace::sharedWorkspace()
            .notificationCenter()
            .addObserverForName_object_queue_usingBlock(
                Some(NSWorkspaceDidTerminateApplicationNotification),
                None,
                Some(&NSOperationQueue::mainQueue()),
                &callback,
            )
    };
    PENDING.with(|pending| pending.borrow_mut().observer = Some(token));
}

pub fn request(app: &tauri::AppHandle) {
    let handle = app.clone();
    let _ = app.run_on_main_thread(move || {
        let Some(_) = MainThreadMarker::new() else {
            return;
        };
        let peers = NSRunningApplication::runningApplicationsWithBundleIdentifier(
            &NSString::from_str(&handle.config().identifier),
        );
        PENDING.with(|pending| {
            let mut pending = pending.borrow_mut();
            pending.requested = true;
            pending.peers.retain(|(_, peer)| !peer.isTerminated());
            for peer in peers.iter() {
                let pid = peer.processIdentifier();
                if pid > 0
                    && pid != std::process::id() as i32
                    && !peer.isTerminated()
                    && peer
                        .executableURL()
                        .and_then(|url| url.path())
                        .is_some_and(|path| {
                            std::path::Path::new(&path.to_string())
                                .file_name()
                                .is_some_and(|name| name == "reasonix-tauri")
                        })
                    && !pending.peers.iter().any(|(existing, _)| *existing == pid)
                {
                    pending.peers.push((pid, peer));
                }
            }
        });
        // Terminations before this request are reflected in isTerminated;
        // those after it are covered by the lifetime notification subscription.
        reconcile(&handle);
    });
}
