use super::*;
use tauri::Emitter;

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct UnsubscribeRequest {
    pub subscription_id: String,
}

#[derive(Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
struct StateNotice<'a> {
    protocol_version: u64,
    subscription: &'a SubscriptionIdentity,
    state: &'static str,
}
#[derive(Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
struct SessionEvent<'a> {
    protocol_version: u64,
    subscription: &'a SubscriptionIdentity,
    frame: &'a BridgeRemoteControllerSessionEvent,
}

pub(crate) fn run_subscription(
    app: tauri::AppHandle,
    client: RemoteControllerClient,
    operation: SubscriptionOperation,
) {
    run_with(
        client,
        operation,
        |identity, state| {
            app.emit_to(
                "main",
                "bridge:remote-session-state",
                StateNotice {
                    protocol_version: 1,
                    subscription: identity,
                    state,
                },
            )
            .map_err(|_| ())
        },
        |identity, frame| {
            app.emit_to(
                "main",
                "bridge:remote-session-event",
                SessionEvent {
                    protocol_version: 1,
                    subscription: identity,
                    frame,
                },
            )
            .map_err(|_| ())
        },
    );
}

fn run_with(
    client: RemoteControllerClient,
    operation: SubscriptionOperation,
    state: impl Fn(&SubscriptionIdentity, &'static str) -> Result<(), ()>,
    emit: impl Fn(&SubscriptionIdentity, &BridgeRemoteControllerSessionEvent) -> Result<(), ()>,
) {
    let identity = operation.identity();
    if !matches!(
        operation.with_current(|identity| state(identity, "opening")),
        Ok(Some(Ok(())))
    ) {
        return;
    }
    let stream = client.session_events_cancellable(
        SessionViewRequest {
            controller_id: identity.controller_id,
            session_path: identity.session_path,
        },
        operation.cancellation(),
    );
    let Ok(mut stream) = stream else {
        let _ = operation.with_terminal(|identity| state(identity, "ended"));
        return;
    };
    if !matches!(
        operation.with_current(|identity| state(identity, "ready")),
        Ok(Some(Ok(())))
    ) {
        return;
    }
    while let Ok(frame) = stream.next() {
        if !matches!(
            operation.with_current(|identity| emit(identity, &frame)),
            Ok(Some(Ok(())))
        ) {
            return;
        }
    }
    // No decoder/free-text diagnostic or auto-reconnect/replay grant.
    let _ = operation.with_terminal(|identity| state(identity, "ended"));
}

#[cfg(test)]
#[path = "remote_controller_subscription_worker_tests.rs"]
mod tests;
