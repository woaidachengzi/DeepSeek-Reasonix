//! Private diagnostic baseline, never part of the production app entry point.
//! Same locked Tauri/Tao/Wry, fixed local HTML, no sidecar or product settings.
#[cfg(target_os = "macos")]
thread_local! {
    static COCOA_WINDOW: std::cell::RefCell<Option<objc2::rc::Retained<objc2_app_kit::NSWindow>>> = const { std::cell::RefCell::new(None) };
}

#[cfg(target_os = "macos")]
fn main() {
    use std::{
        path::PathBuf,
        sync::{
            atomic::{AtomicBool, AtomicI32, Ordering},
            mpsc, Arc,
        },
        time::Duration,
    };
    use tauri::{Manager, WebviewUrl};

    let directory = PathBuf::from(
        std::env::var_os("REASONIX_WINDOW_BASELINE_DIR")
            .expect("explicit private baseline directory is required"),
    );
    assert!(directory.is_absolute() && directory.is_dir());
    let overlay = std::env::var("REASONIX_WINDOW_BASELINE_STYLE").as_deref() == Ok("overlay");
    let cocoa = std::env::var("REASONIX_WINDOW_BASELINE_KIND").as_deref() == Ok("cocoa");
    let mut context = tauri::generate_context!();
    context.config_mut().identifier = "io.reasonix.desktop.window-baseline".into();
    context.config_mut().product_name = Some("Reasonix Window Baseline".into());
    context.config_mut().app.windows.clear();
    let app = tauri::Builder::default()
        .manage(AtomicBool::new(false))
        .on_page_load(|webview, payload| {
            if payload.event() == tauri::webview::PageLoadEvent::Finished {
                webview.state::<AtomicBool>().store(true, Ordering::SeqCst);
            }
        })
        .register_uri_scheme_protocol("baseline", |_, _| {
            tauri::http::Response::builder().header("Content-Type", "text/html")
                .body(b"<!doctype html><title>Tauri window baseline</title><h1>Private window baseline</h1><p>No product data or network requests.</p>".to_vec()).unwrap()
        })
        .setup(move |app| {
            let mut builder = tauri::WebviewWindowBuilder::new(app, "baseline",
                WebviewUrl::CustomProtocol("baseline://localhost/".parse().unwrap()))
                .title("Tauri window baseline").inner_size(1280.0, 820.0).visible(!cocoa);
            if overlay {
                builder = builder.title_bar_style(tauri::TitleBarStyle::Overlay).hidden_title(true);
            }
            builder.build()?;
            if cocoa {
                use objc2::MainThreadOnly;
                use objc2_app_kit::{NSBackingStoreType, NSWindow, NSWindowStyleMask};
                use objc2_foundation::{NSPoint, NSRect, NSSize, NSString};
                let mtm = objc2::MainThreadMarker::new().unwrap();
                // SAFETY: Construct and retain a standalone AppKit window on
                // the UI thread. No Tauri window/delegate is replaced.
                let native = unsafe { NSWindow::initWithContentRect_styleMask_backing_defer(
                    NSWindow::alloc(mtm),
                    NSRect::new(NSPoint::new(320.0, 160.0), NSSize::new(1280.0, 820.0)),
                    NSWindowStyleMask::Titled | NSWindowStyleMask::Closable | NSWindowStyleMask::Miniaturizable | NSWindowStyleMask::Resizable | NSWindowStyleMask::FullSizeContentView,
                    NSBackingStoreType::Buffered, false) };
                // SAFETY: The thread-local Retained owns the window through
                // event-loop shutdown, including any close operation.
                unsafe { native.setReleasedWhenClosed(false); }
                native.setTitle(&NSString::from_str("Plain Cocoa in Tauri event loop"));
                native.makeKeyAndOrderFront(None);
                COCOA_WINDOW.with(|slot| *slot.borrow_mut() = Some(native));
            }
            Ok(())
        })
        .build(context).expect("build private baseline");
    let exit_code = Arc::new(AtomicI32::new(2));
    let reported_exit = Arc::clone(&exit_code);
    app.run_return(move |app, event| {
        if !matches!(event, tauri::RunEvent::Ready) { return; }
        let handle = app.clone();
        let directory = directory.clone();
        let reported_exit = Arc::clone(&reported_exit);
        std::thread::spawn(move || {
            let mut records = Vec::new();
            for step in 0..24 {
                std::thread::sleep(Duration::from_millis(250));
                let (send, receive) = mpsc::sync_channel(1);
                let app = handle.clone();
                handle.run_on_main_thread(move || {
                    use objc2_app_kit::{NSApplication, NSWindow};
                    let window = app.get_webview_window("baseline").unwrap();
                    let pointer = window.ns_window().unwrap();
                    // SAFETY: Live Tauri window on the main thread. The pointer
                    // is not retained or sent across threads.
                    let native = unsafe { &*pointer.cast::<NSWindow>() };
                    COCOA_WINDOW.with(|slot| {
                    let retained = slot.borrow();
                    let native = retained.as_deref().unwrap_or(native);
                    let application = NSApplication::sharedApplication(objc2::MainThreadMarker::new().unwrap());
                    if cocoa && step == 0 {
                        native.makeKeyAndOrderFront(None);
                        #[allow(deprecated)]
                        application.activateIgnoringOtherApps(true);
                    }
                    if step == 4 { native.miniaturize(Some(native)); }
                    if step == 14 { native.deminiaturize(Some(native)); }
                    let _ = send.send(serde_json::json!({"step": step, "overlay": overlay, "cocoa": cocoa,
                        "minimized": if cocoa { native.isMiniaturized() } else { window.is_minimized().unwrap() }, "nativeMiniaturized": native.isMiniaturized(),
                        "nativeKey": native.isKeyWindow(), "applicationActive": application.isActive(),
                        "nativeVisible": native.isVisible(), "nativeStyleMask": native.styleMask().bits(),
                        "pageFinished": app.state::<AtomicBool>().load(Ordering::SeqCst)}));
                    });
                }).unwrap();
                records.push(receive.recv_timeout(Duration::from_secs(5)).expect("baseline UI response"));
            }
            let minimized = records[5..14].iter().any(|r| r["nativeMiniaturized"] == true);
            let restored = minimized && records[15..].iter().any(|r| r["nativeMiniaturized"] == false);
            let precondition = ["nativeKey", "applicationActive", "nativeVisible", "pageFinished"]
                .iter().all(|key| records[3][key] == true);
            let result = serde_json::json!({"precondition": precondition, "minimized": minimized, "restored": restored, "records": records});
            std::fs::write(directory.join("result.json"), result.to_string()).unwrap();
            reported_exit.store(if restored && precondition { 0 } else { 2 }, Ordering::SeqCst);
            handle.exit(0);
        });
    });
    // The standalone fixture explicitly reports diagnostic failure after the
    // event loop has cleaned up. Keep it distinct from normal app Quit status.
    std::process::exit(exit_code.load(Ordering::SeqCst));
}

#[cfg(not(target_os = "macos"))]
fn main() {
    eprintln!("This diagnostic baseline is macOS-only.");
    std::process::exit(1);
}
