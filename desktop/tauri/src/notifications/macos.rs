use super::{NotificationState, Permission, DELIVERY_ERROR};
use block2::{DynBlock, RcBlock};
use objc2::{
    define_class, msg_send,
    rc::Retained,
    runtime::{Bool, ProtocolObject},
    AnyThread, DefinedClass,
};
use objc2_foundation::{NSBundle, NSError, NSObject, NSObjectProtocol, NSString};
use objc2_user_notifications::{
    UNAuthorizationOptions, UNAuthorizationStatus, UNMutableNotificationContent, UNNotification,
    UNNotificationDefaultActionIdentifier, UNNotificationPresentationOptions,
    UNNotificationRequest, UNNotificationResponse, UNNotificationSettings,
    UNUserNotificationCenter, UNUserNotificationCenterDelegate,
};
use std::{
    cell::RefCell,
    ptr::NonNull,
    sync::{mpsc, Arc},
    time::Duration,
};

struct DelegateIvars {
    app: tauri::AppHandle,
    state: Arc<NotificationState>,
}
define_class!(
    // NSObject permits subclassing. Every callback uses only the thread-safe
    // AppHandle and Arc state; no Cocoa UI object is held in the Rust ivars.
    #[unsafe(super = NSObject)]
    #[name = "ReasonixPreviewNotificationDelegate"]
    #[ivars = DelegateIvars]
    struct Delegate;
    unsafe impl NSObjectProtocol for Delegate {}
    unsafe impl UNUserNotificationCenterDelegate for Delegate {
        #[unsafe(method(userNotificationCenter:willPresentNotification:withCompletionHandler:))]
        fn will_present(
            &self,
            _center: &UNUserNotificationCenter,
            _notification: &UNNotification,
            completion: &DynBlock<dyn Fn(UNNotificationPresentationOptions)>,
        ) {
            // Frontend owns sound preferences; avoid a duplicate system chime.
            completion
                .call((UNNotificationPresentationOptions::Banner
                    | UNNotificationPresentationOptions::List,));
        }
        #[unsafe(method(userNotificationCenter:didReceiveNotificationResponse:withCompletionHandler:))]
        fn did_receive(
            &self,
            _center: &UNUserNotificationCenter,
            response: &UNNotificationResponse,
            completion: &DynBlock<dyn Fn()>,
        ) {
            // Only the default click navigates. Dismissal or an unknown action
            // must never answer questions or authorize an agent tool.
            // SAFETY: Apple's exported NSString constant lives for the process.
            if &*response.actionIdentifier() == unsafe { UNNotificationDefaultActionIdentifier } {
                let token = response.notification().request().identifier().to_string();
                self.ivars().state.activate(&self.ivars().app, &token);
            }
            completion.call(());
        }
    }
);
thread_local! { static DELEGATE: RefCell<Option<Retained<Delegate>>> = const { RefCell::new(None) }; }

fn bundled(identifier: &str) -> bool {
    let bundle = NSBundle::mainBundle();
    bundle
        .bundleIdentifier()
        .is_some_and(|value| value.to_string() == identifier)
        && bundle.bundlePath().to_string().ends_with(".app")
}
fn center(identifier: &str) -> Result<Retained<UNUserNotificationCenter>, String> {
    // currentNotificationCenter can abort an unbundled process. Verify the
    // actual bundle first instead of adopting another app's identity.
    if !bundled(identifier) {
        return Err(DELIVERY_ERROR.into());
    }
    Ok(UNUserNotificationCenter::currentNotificationCenter())
}
pub(super) fn install(app: &tauri::AppHandle, state: Arc<NotificationState>) -> Result<(), String> {
    let center = center(&app.config().identifier)?;
    let object = Delegate::alloc().set_ivars(DelegateIvars {
        app: app.clone(),
        state,
    });
    // SAFETY: NSObject's init signature is inherited unchanged.
    let delegate: Retained<Delegate> = unsafe { msg_send![super(object), init] };
    center.setDelegate(Some(ProtocolObject::from_ref(&*delegate)));
    // Apple's delegate is weak; retain it on the application's main thread
    // for the complete process lifetime, before the WebView becomes ready.
    DELEGATE.with(|slot| *slot.borrow_mut() = Some(delegate));
    Ok(())
}
fn settings(center: &UNUserNotificationCenter) -> Result<Permission, String> {
    let (sender, receiver) = mpsc::channel();
    let block = RcBlock::new(move |settings: NonNull<UNNotificationSettings>| {
        // SAFETY: Apple supplies a valid non-null settings object throughout
        // the callback. Copy the numeric status; retain no Objective-C pointer.
        let status = unsafe { settings.as_ref() }.authorizationStatus();
        let permission = match status {
            UNAuthorizationStatus::NotDetermined => Permission::NotDetermined,
            UNAuthorizationStatus::Denied => Permission::Denied,
            UNAuthorizationStatus::Authorized => Permission::Granted,
            UNAuthorizationStatus::Provisional => Permission::Provisional,
            _ => Permission::Unavailable,
        };
        let _ = sender.send(permission);
    });
    center.getNotificationSettingsWithCompletionHandler(&block);
    receiver
        .recv_timeout(Duration::from_secs(5))
        .map_err(|_| DELIVERY_ERROR.into())
}
pub(super) fn permission(identifier: &str, request: bool) -> Result<Permission, String> {
    if !bundled(identifier) {
        return Ok(Permission::Unavailable);
    }
    let center = center(identifier)?;
    let permission = settings(&center)?;
    if !request || permission != Permission::NotDetermined {
        return Ok(permission);
    }
    let (sender, receiver) = mpsc::channel();
    let block = RcBlock::new(move |granted: Bool, error: *mut NSError| {
        let _ = sender.send(if error.is_null() {
            Ok(granted.as_bool())
        } else {
            Err(DELIVERY_ERROR.to_string())
        });
    });
    center.requestAuthorizationWithOptions_completionHandler(UNAuthorizationOptions::Alert, &block);
    receiver
        .recv_timeout(Duration::from_secs(60))
        .map_err(|_| DELIVERY_ERROR)??;
    settings(&center)
}
pub(super) fn send(identifier: &str, token: &str, body: &str) -> Result<(), String> {
    let center = center(identifier)?;
    let content = UNMutableNotificationContent::new();
    content.setTitle(&NSString::from_str("Reasonix"));
    content.setBody(&NSString::from_str(body));
    let request = UNNotificationRequest::requestWithIdentifier_content_trigger(
        &NSString::from_str(token),
        &content,
        None,
    );
    let (sender, receiver) = mpsc::channel();
    let block = RcBlock::new(move |error: *mut NSError| {
        let _ = sender.send(error.is_null());
    });
    center.addNotificationRequest_withCompletionHandler(&request, Some(&block));
    match receiver.recv_timeout(Duration::from_secs(5)) {
        Ok(true) => Ok(()),
        _ => Err(DELIVERY_ERROR.into()),
    }
}

// Only the opt-in, private-profile package gate owns these receipts. Neither
// querying nor cleanup is a renderer command; cleanup never removes all items.
pub(super) struct DeliveryReceipt {
    center: Retained<UNUserNotificationCenter>,
    token: String,
}
impl DeliveryReceipt {
    pub(super) fn new(identifier: &str, token: String) -> Result<Self, String> {
        Ok(Self {
            center: center(identifier)?,
            token,
        })
    }
    pub(super) fn delivered(&self, expected: Option<&'static str>) -> Result<bool, String> {
        use objc2_foundation::NSArray;
        let (sender, receiver) = mpsc::sync_channel(1);
        let token = self.token.clone();
        let block = RcBlock::new(move |items: NonNull<NSArray<UNNotification>>| {
            // SAFETY: Apple's array and its notifications remain valid for
            // the callback. Only a boolean/fixed error crosses the channel.
            let items = unsafe { items.as_ref() };
            let result = (|| {
                if items.count() > 2048 {
                    return Err(
                        "native delivered notification query exceeds acceptance bounds".into(),
                    );
                }
                let mut found = false;
                for item in items {
                    let request = item.request();
                    if request.identifier().to_string() != token {
                        continue;
                    }
                    if found {
                        return Err("native delivered notification identifier is duplicated".into());
                    }
                    found = true;
                    if let Some(body) = expected {
                        let content = request.content();
                        if content.title().to_string() != "Reasonix"
                            || content.body().to_string() != body
                        {
                            return Err("native delivered notification caption differs from the fixed contract".into());
                        }
                    }
                }
                Ok(found)
            })();
            let _ = sender.try_send(result);
        });
        self.center
            .getDeliveredNotificationsWithCompletionHandler(&block);
        receiver
            .recv_timeout(Duration::from_secs(5))
            .map_err(|_| "native delivered notification query timed out")?
    }
    pub(super) fn remove(&self) {
        let identifiers =
            objc2_foundation::NSArray::from_retained_slice(&[NSString::from_str(&self.token)]);
        self.center
            .removePendingNotificationRequestsWithIdentifiers(&identifiers);
        self.center
            .removeDeliveredNotificationsWithIdentifiers(&identifiers);
    }
}
impl Drop for DeliveryReceipt {
    fn drop(&mut self) {
        self.remove();
    }
}

#[cfg(test)]
mod tests {
    #[test]
    fn unbundled_process_reports_unavailable_without_touching_notification_center() {
        assert_eq!(
            super::permission("io.reasonix.desktop.preview", false).unwrap(),
            crate::notifications::Permission::Unavailable
        );
        assert!(super::send("io.reasonix.desktop.preview", "test", "test").is_err());
    }
}
