use crate::request::Target;

#[cfg(any(target_os = "linux", test))]
mod linux;

#[cfg(target_os = "linux")]
pub fn verify(target: &Target) -> Result<(), String> {
    let (identity, ticks) = linux::resolve(target.pid)?;
    if target.bundle_id != identity || target.launch_time != ticks as f64 {
        return Err("approved Linux app incarnation changed".into());
    }
    Ok(())
}

// Recheck the approved native process incarnation before entering the SDK.
#[cfg(target_os = "macos")]
pub fn verify(target: &Target) -> Result<(), String> {
    use objc2::{class, msg_send, rc::autoreleasepool, runtime::AnyObject};
    use std::ffi::{c_char, CStr};
    autoreleasepool(|_| unsafe {
        let app: *mut AnyObject = msg_send![class!(NSRunningApplication), runningApplicationWithProcessIdentifier: target.pid];
        if app.is_null() {
            return Err("approved app exited".into());
        }
        let bundle: *mut AnyObject = msg_send![app, bundleIdentifier];
        let date: *mut AnyObject = msg_send![app, launchDate];
        if bundle.is_null() || date.is_null() {
            return Err("approved app identity unavailable".into());
        }
        let bytes: *const c_char = msg_send![bundle, UTF8String];
        let launch: f64 = msg_send![date, timeIntervalSince1970];
        if bytes.is_null()
            || CStr::from_ptr(bytes).to_bytes() != target.bundle_id.as_bytes()
            || !launch.is_finite()
            || (launch - target.launch_time).abs() > 0.000001
        {
            return Err("approved app incarnation changed".into());
        }
        Ok(())
    })
}

#[cfg(not(any(target_os = "macos", target_os = "linux")))]
pub fn verify(_: &Target) -> Result<(), String> {
    Err("this Reasonix Cua adapter requires macOS or GNU/Linux".into())
}
