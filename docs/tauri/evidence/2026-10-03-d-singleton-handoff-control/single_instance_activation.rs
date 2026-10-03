//! Hand activation back before the singleton plugin exits the new process.
//! This runs only in native plugin setup, before any profile/sidecar is opened.
use objc2::{sel, MainThreadMarker};
use objc2_app_kit::{NSApplication, NSRunningApplication};
use objc2_foundation::{NSObjectProtocol, NSString};

pub fn yield_to_existing(identifier: &str) {
    let Some(main_thread) = MainThreadMarker::new() else {
        return;
    };
    let application = NSApplication::sharedApplication(main_thread);
    // Cooperative activation arrived in macOS 14. Preserve the ordinary
    // singleton path on the host's older supported macOS versions.
    if !application.respondsToSelector(sel!(yieldActivationToApplication:)) {
        return;
    }
    let peers = NSRunningApplication::runningApplicationsWithBundleIdentifier(&NSString::from_str(
        identifier,
    ));
    let mut existing = peers.iter().filter(|peer| {
        peer.processIdentifier() != std::process::id() as i32
            && !peer.isTerminated()
            && peer
                .executableURL()
                .and_then(|url| url.path())
                .is_some_and(|path| {
                    std::path::Path::new(&path.to_string())
                        .file_name()
                        .is_some_and(|name| name == "reasonix-tauri")
                })
    });
    // Do not choose an arbitrary target when process state is ambiguous.
    if let Some(primary) = existing.next() {
        if existing.next().is_none() {
            application.yieldActivationToApplication(&primary);
        }
    }
}
