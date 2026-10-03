//! Private diagnostic baseline, never part of the production app entry point.
//! Same locked Tauri/Tao/Wry, fixed local HTML, no sidecar or product settings.
#[cfg(target_os = "macos")]
thread_local! {
    static COCOA_WINDOW: std::cell::RefCell<Option<objc2::rc::Retained<objc2_app_kit::NSWindow>>> = const { std::cell::RefCell::new(None) };
    static COCOA_WEB: std::cell::RefCell<Option<objc2::rc::Retained<objc2_web_kit::WKWebView>>> = const { std::cell::RefCell::new(None) };
    static OBSERVERS: std::cell::RefCell<Vec<objc2::rc::Retained<objc2::runtime::ProtocolObject<dyn objc2_foundation::NSObjectProtocol>>>> = const { std::cell::RefCell::new(Vec::new()) };
    static PHASE_TIMER: std::cell::RefCell<Option<objc2::rc::Retained<objc2_foundation::NSTimer>>> = const { std::cell::RefCell::new(None) };
    static PHASE_RECORDS: std::cell::RefCell<Vec<serde_json::Value>> = const { std::cell::RefCell::new(Vec::new()) };
    static NATIVE_EVENTS: std::cell::RefCell<Vec<&'static str>> = const { std::cell::RefCell::new(Vec::new()) };
}

#[cfg(target_os = "macos")]
fn queued_action(window: &tauri::WebviewWindow, minimize: bool) {
    assert!(objc2::MainThreadMarker::new().is_some());
    let pointer = window.ns_window().unwrap();
    // SAFETY: The Tauri wrapper retains its live window throughout this
    // main-queue operation; no raw pointer crosses threads.
    let native = unsafe { &*pointer.cast::<objc2_app_kit::NSWindow>() };
    COCOA_WINDOW.with(|slot| {
        let retained = slot.borrow();
        let native = retained.as_deref().unwrap_or(native);
        if minimize {
            native.miniaturize(Some(native));
        } else {
            native.deminiaturize(Some(native));
        }
    });
}

#[cfg(target_os = "macos")]
fn start_phase_timer(
    window: tauri::WebviewWindow,
    state: std::sync::Arc<[std::sync::atomic::AtomicUsize; 4]>,
    executions: std::sync::Arc<[std::sync::atomic::AtomicUsize; 2]>,
    cocoa_web: bool,
    loaded_reorder: bool,
    reordered: std::sync::Arc<std::sync::atomic::AtomicBool>,
) {
    use std::sync::atomic::Ordering;
    use tauri::Manager;
    assert!(objc2::MainThreadMarker::new().is_some());
    let start = std::time::Instant::now();
    let callback = block2::RcBlock::new(move |timer: std::ptr::NonNull<objc2_foundation::NSTimer>| {
        assert!(objc2::MainThreadMarker::new().is_some());
        let pointer = window.ns_window().expect("live baseline window");
        // SAFETY: The timer runs on the main run loop and retains the live
        // window wrapper. The raw native pointer never crosses a thread.
        let tauri_native = unsafe { &*pointer.cast::<objc2_app_kit::NSWindow>() };
        COCOA_WINDOW.with(|slot| {
            let retained = slot.borrow();
            let native = retained.as_deref().unwrap_or(tauri_native);
            let application = objc2_app_kit::NSApplication::sharedApplication(objc2::MainThreadMarker::new().unwrap());
            let page = window.state::<std::sync::atomic::AtomicBool>().load(Ordering::SeqCst);
            let web_complete = COCOA_WEB.with(|slot| {
                // SAFETY: Query the retained WKWebView on the main thread.
                slot.borrow().as_ref().is_some_and(|web| unsafe { !web.isLoading() && web.estimatedProgress() == 1.0 })
            });
            let trace = |stage| {
                PHASE_RECORDS.with(|records| records.borrow_mut().push(serde_json::json!({
                    "stage": stage, "elapsedMs": start.elapsed().as_millis(),
                    "active": application.isActive(), "key": native.isKeyWindow(), "visible": native.isVisible(),
                    "pageFinished": page, "cocoaPageComplete": web_complete,
                    "minimized": native.isMiniaturized(),
                    "events": NATIVE_EVENTS.with(|events| events.borrow().clone())
                })));
            };
            match state[0].load(Ordering::SeqCst) {
                0 if page && (!cocoa_web || web_complete) => {
                    trace("loaded");
                    if loaded_reorder {
                        native.orderOut(None);
                        native.makeKeyAndOrderFront(None);
                        #[allow(deprecated)]
                        application.activateIgnoringOtherApps(true);
                        reordered.store(true, Ordering::SeqCst);
                    }
                    state[0].store(1, Ordering::SeqCst);
                }
                1 if page && (!cocoa_web || web_complete) && application.isActive() && native.isKeyWindow() && native.isVisible() => {
                    trace("minimize-ready");
                    state[1].store(1, Ordering::SeqCst);
                    state[0].store(2, Ordering::SeqCst);
                    executions[0].fetch_add(1, Ordering::SeqCst);
                    native.miniaturize(Some(native));
                    trace("minimize-requested");
                }
                2 if native.isMiniaturized() && NATIVE_EVENTS.with(|events| {
                    let events = events.borrow(); events.contains(&"will-mini") && events.contains(&"did-mini")
                }) => {
                    trace("minimized");
                    state[2].store(1, Ordering::SeqCst);
                    state[0].store(3, Ordering::SeqCst);
                    executions[1].fetch_add(1, Ordering::SeqCst);
                    native.deminiaturize(Some(native));
                }
                3 if !native.isMiniaturized() && NATIVE_EVENTS.with(|events| events.borrow().contains(&"did-demini")) => {
                    trace("restored");
                    state[3].store(1, Ordering::SeqCst);
                    state[0].store(4, Ordering::SeqCst);
                    // SAFETY: The callback receives its live NSTimer and is
                    // executing on the run loop that owns this timer.
                    unsafe { timer.as_ref().invalidate(); }
                }
                _ => {}
            }
        });
    });
    // SAFETY: Captures own Send window/state handles. AppKit retains a copied
    // block and invokes it on the current main run loop. The retained timer is
    // invalidated after sampling or success, before diagnostic teardown.
    let timer = unsafe { objc2_foundation::NSTimer::scheduledTimerWithTimeInterval_repeats_block(0.05, true, &callback) };
    PHASE_TIMER.with(|slot| *slot.borrow_mut() = Some(timer));
}

#[cfg(target_os = "macos")]
fn main() {
    use std::{
        path::PathBuf,
        sync::{
            atomic::{AtomicBool, AtomicI32, AtomicUsize, Ordering},
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
    let main_queue =
        std::env::var("REASONIX_WINDOW_BASELINE_ACTION").as_deref() == Ok("main-queue");
    let run_loop_timer =
        std::env::var("REASONIX_WINDOW_BASELINE_ACTION").as_deref() == Ok("run-loop-timer");
    let phase_timer =
        std::env::var("REASONIX_WINDOW_BASELINE_ACTION").as_deref() == Ok("phase-timer");
    let plain_application =
        std::env::var("REASONIX_WINDOW_BASELINE_APP").as_deref() == Ok("plain");
    let detached_delegate =
        std::env::var("REASONIX_WINDOW_BASELINE_DELEGATE").as_deref() == Ok("none");
    let detached_window_delegate =
        std::env::var("REASONIX_WINDOW_BASELINE_WINDOW_DELEGATE").as_deref() == Ok("none");
    let manual_activation =
        std::env::var("REASONIX_WINDOW_BASELINE_MANUAL").as_deref() == Ok("1");
    let nil_sender = std::env::var("REASONIX_WINDOW_BASELINE_SENDER").as_deref() == Ok("nil");
    let cocoa_web = std::env::var("REASONIX_WINDOW_BASELINE_CONTENT").as_deref() == Ok("web");
    let loaded_reorder = std::env::var("REASONIX_WINDOW_BASELINE_REORDER").as_deref() == Ok("loaded");
    if plain_application {
        // Diagnostic only: create the shared AppKit application before Tao
        // installs its subclass. Keep Tao's delegate and event loop unchanged.
        // This is not a supported production initialization strategy.
        let _ = objc2_app_kit::NSApplication::sharedApplication(
            objc2::MainThreadMarker::new().expect("baseline main thread"),
        );
    }
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
                if cocoa_web {
                    use objc2_web_kit::{WKWebView, WKWebViewConfiguration, WKWebsiteDataStore};
                    let content = native.contentView().expect("Cocoa content view");
                    // SAFETY: AppKit/WebKit allocation, view ownership and
                    // configuration are confined to the main thread. The
                    // fixed inline HTML uses an ephemeral data store.
                    let web = unsafe {
                        let config = WKWebViewConfiguration::new(mtm);
                        config.setWebsiteDataStore(&WKWebsiteDataStore::nonPersistentDataStore(mtm));
                        WKWebView::initWithFrame_configuration(WKWebView::alloc(mtm), content.bounds(), &config)
                    };
                    content.addSubview(&web);
                    unsafe { web.loadHTMLString_baseURL(&NSString::from_str("<!doctype html><title>Private Cocoa WebKit control</title><p>Fixed local content.</p>"), None); }
                    COCOA_WEB.with(|slot| *slot.borrow_mut() = Some(web));
                }
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
            if manual_activation {
                std::fs::write(directory.join("ready.json"), b"{\"manualActivation\":true}").unwrap();
                let deadline = std::time::Instant::now() + Duration::from_secs(60);
                while !directory.join("start.json").is_file() {
                    if std::time::Instant::now() >= deadline {
                        std::fs::write(directory.join("result.json"), b"{\"manualActivationTimeout\":true,\"precondition\":false,\"minimized\":false,\"restored\":false,\"records\":[]}").unwrap();
                        handle.exit(0);
                        return;
                    }
                    std::thread::sleep(Duration::from_millis(50));
                }
            }
            let mut records = Vec::new();
            let executions = Arc::new([AtomicUsize::new(0), AtomicUsize::new(0)]);
            let reordered = Arc::new(AtomicBool::new(false));
            let phase_state = Arc::new(std::array::from_fn::<_, 4, _>(|_| AtomicUsize::new(0)));
            for step in 0..24 {
                std::thread::sleep(Duration::from_millis(250));
                let (send, receive) = mpsc::sync_channel(1);
                let app = handle.clone();
                let executions = Arc::clone(&executions);
                let reordered = Arc::clone(&reordered);
                let phase_state = Arc::clone(&phase_state);
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
                    if step == 0 {
                        use objc2_app_kit::{NSWindowWillMiniaturizeNotification, NSWindowDidMiniaturizeNotification, NSWindowDidDeminiaturizeNotification, NSWindowDidBecomeKeyNotification, NSWindowDidResignKeyNotification, NSWindowDidChangeOcclusionStateNotification};
                        let center = objc2_foundation::NSNotificationCenter::defaultCenter();
                        // SAFETY: AppKit names identify NSWindow notifications;
                        // the retained live native window is the object filter.
                        // Callbacks touch thread-local state only on the main
                        // operation queue. Tokens are removed after run_return.
                        unsafe {
                            for (name, label) in [(NSWindowWillMiniaturizeNotification, "will-mini"), (NSWindowDidMiniaturizeNotification, "did-mini"), (NSWindowDidDeminiaturizeNotification, "did-demini"), (NSWindowDidBecomeKeyNotification, "key"), (NSWindowDidResignKeyNotification, "resign-key"), (NSWindowDidChangeOcclusionStateNotification, "occlusion")] {
                                let callback = block2::RcBlock::new(move |_: std::ptr::NonNull<objc2_foundation::NSNotification>| {
                                    assert!(objc2::MainThreadMarker::new().is_some());
                                    NATIVE_EVENTS.with(|events| events.borrow_mut().push(label));
                                });
                                let token = center.addObserverForName_object_queue_usingBlock(Some(name), Some(native), Some(&objc2_foundation::NSOperationQueue::mainQueue()), &callback);
                                OBSERVERS.with(|tokens| tokens.borrow_mut().push(token));
                            }
                        }
                    }
                    if detached_window_delegate && step == 0 {
                        // Diagnostic only: isolate the NSWindow delegate while
                        // keeping the application delegate and event loop.
                        native.setDelegate(None);
                    }
                    if detached_delegate && step == 0 {
                        // Diagnostic only, after Ready: Tao retains its
                        // delegate, but AppKit no longer dispatches to it.
                        application.setDelegate(None);
                    }
                    let cocoa_page_complete = COCOA_WEB.with(|slot| {
                        // SAFETY: Live WKWebView queried on the main thread.
                        slot.borrow().as_ref().is_some_and(|web| unsafe { !web.isLoading() && web.estimatedProgress() == 1.0 })
                    });
                    if !phase_timer && cocoa && loaded_reorder && cocoa_page_complete && !reordered.swap(true, Ordering::SeqCst) {
                        native.orderOut(None);
                        native.makeKeyAndOrderFront(None);
                        #[allow(deprecated)]
                        application.activateIgnoringOtherApps(true);
                    }
                    if cocoa && step == 0 {
                        native.makeKeyAndOrderFront(None);
                        #[allow(deprecated)]
                        application.activateIgnoringOtherApps(true);
                    }
                    if phase_timer && step == 0 {
                        start_phase_timer(window.clone(), Arc::clone(&phase_state), Arc::clone(&executions), cocoa_web, loaded_reorder, Arc::clone(&reordered));
                    }
                    if phase_timer && step == 23 {
                        PHASE_TIMER.with(|slot| {
                            if let Some(timer) = slot.borrow_mut().take() { timer.invalidate(); }
                        });
                    }
                    if !phase_timer && (step == 4 || step == 14) {
                        let index = usize::from(step == 14);
                        if run_loop_timer {
                            let target = window.clone();
                            let completed = Arc::clone(&executions);
                            let callback = block2::RcBlock::new(move |_: std::ptr::NonNull<objc2_foundation::NSTimer>| {
                                queued_action(&target, step == 4);
                                completed[index].fetch_add(1, Ordering::SeqCst);
                            });
                            // SAFETY: Schedule a single-shot AppKit timer on
                            // the current main run loop. The run loop retains
                            // the timer and copied block until it fires.
                            unsafe { objc2_foundation::NSTimer::scheduledTimerWithTimeInterval_repeats_block(0.05, false, &callback); }
                        } else if main_queue {
                            let target = window.clone();
                            let completed = Arc::clone(&executions);
                            let operation = block2::RcBlock::new(move || {
                                queued_action(&target, step == 4);
                                completed[index].fetch_add(1, Ordering::SeqCst);
                            });
                            // SAFETY: AppKit's main operation queue executes
                            // this block on the UI thread and copies it.
                            unsafe { objc2_foundation::NSOperationQueue::mainQueue().addOperationWithBlock(&operation); }
                        } else {
                            let sender: Option<&objc2::runtime::AnyObject> = if nil_sender { None } else { Some(native) };
                            if step == 4 { native.miniaturize(sender); }
                            else { native.deminiaturize(sender); }
                            executions[index].fetch_add(1, Ordering::SeqCst);
                        }
                    }
                    let properties = serde_json::json!({
                        "nativeClass": native.class().name().to_string_lossy(),
                        "windowDelegateDetached": detached_window_delegate,
                        "windowDelegatePresent": native.delegate().is_some(),
                        "nativeCollectionBehavior": native.collectionBehavior().bits(),
                        "nativeAnimationBehavior": native.animationBehavior().0,
                        "nativeLevel": native.level(),
                        "nativeCanBecomeMain": native.canBecomeMainWindow(),
                        "nativeCanBecomeKey": native.canBecomeKeyWindow(),
                        "nativeHidesOnDeactivate": native.hidesOnDeactivate(),
                        "nativeExcludedFromWindowsMenu": native.isExcludedFromWindowsMenu()
                    });
                    let _ = send.send(serde_json::json!({"step": step, "overlay": overlay, "cocoa": cocoa, "mainQueue": main_queue,
                        "runLoopTimer": run_loop_timer, "phaseTimer": phase_timer,
                        "phaseState": phase_state[0].load(Ordering::SeqCst),
                        "phasePrecondition": phase_state[1].load(Ordering::SeqCst) == 1,
                        "phaseMinimized": phase_state[2].load(Ordering::SeqCst) == 1,
                        "phaseRestored": phase_state[3].load(Ordering::SeqCst) == 1,
                        "phaseRecords": PHASE_RECORDS.with(|records| records.borrow().clone()),
                        "plainApplicationRequested": plain_application,
                        "applicationClass": application.class().name().to_string_lossy(),
                        "nativeProperties": properties,
                        "delegateDetached": detached_delegate,
                        "delegatePresent": application.delegate().is_some(),
                        "applicationActivationPolicy": application.activationPolicy().0,
                        "manualActivation": manual_activation,
                        "nativeEvents": NATIVE_EVENTS.with(|events| events.borrow().clone()),
                        "nilSender": nil_sender, "cocoaWeb": cocoa_web, "cocoaPageComplete": cocoa_page_complete, "loadedReorderRequested": loaded_reorder, "loadedReorderExecuted": reordered.load(Ordering::SeqCst),
                        "minimizeExecutions": executions[0].load(Ordering::SeqCst), "restoreExecutions": executions[1].load(Ordering::SeqCst),
                        "minimized": if cocoa { native.isMiniaturized() } else { window.is_minimized().unwrap() }, "nativeMiniaturized": native.isMiniaturized(),
                        "nativeKey": native.isKeyWindow(), "applicationActive": application.isActive(),
                        "nativeVisible": native.isVisible(), "nativeStyleMask": native.styleMask().bits(),
                        "pageFinished": app.state::<AtomicBool>().load(Ordering::SeqCst)}));
                    });
                }).unwrap();
                records.push(receive.recv_timeout(Duration::from_secs(5)).expect("baseline UI response"));
            }
            let minimized = if phase_timer { phase_state[2].load(Ordering::SeqCst) == 1 } else { records[5..14].iter().any(|r| r["nativeMiniaturized"] == true) };
            let restored = if phase_timer { phase_state[3].load(Ordering::SeqCst) == 1 } else { minimized && records[15..].iter().any(|r| r["nativeMiniaturized"] == false) };
            let content_precondition = !cocoa_web || records[3]["cocoaPageComplete"] == true;
            let precondition = if phase_timer { phase_state[1].load(Ordering::SeqCst) == 1 } else { ["nativeKey", "applicationActive", "nativeVisible", "pageFinished"]
                .iter().all(|key| records[3][key] == true) && content_precondition };
            let result = serde_json::json!({"precondition": precondition, "minimized": minimized, "restored": restored, "records": records});
            std::fs::write(directory.join("result.json"), result.to_string()).unwrap();
            reported_exit.store(if restored && precondition { 0 } else { 2 }, Ordering::SeqCst);
            handle.exit(0);
        });
    });
    PHASE_TIMER.with(|slot| {
        if let Some(timer) = slot.borrow_mut().take() { timer.invalidate(); }
    });
    OBSERVERS.with(|tokens| {
        let center = objc2_foundation::NSNotificationCenter::defaultCenter();
        for token in tokens.borrow_mut().drain(..) {
            // SAFETY: Each token belongs to this center; removal occurs on the
            // same main thread after sampling and before window teardown.
            unsafe { center.removeObserver((*token).as_ref()); }
        }
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
