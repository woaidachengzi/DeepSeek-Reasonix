use super::super::tests::ID;
use super::*;

fn request(surface: &str, generation: u64) -> SubscribeRequest {
    SubscribeRequest {
        controller_id: ID.into(),
        session_path: "/remote/selected.jsonl".into(),
        surface_id: surface.into(),
        generation,
    }
}
fn registry() -> RemoteSubscriptions {
    let registry = RemoteSubscriptions::default();
    registry.set_owner(Some("owned-sidecar"));
    registry
}

#[test]
fn terminal_owner_revocation_cancels_only_its_current_owner() {
    let registry = registry();
    let operation = registry
        .reserve("main", "owned-sidecar", request("surface", 1))
        .unwrap();
    let cancel = operation.cancellation();
    registry.owner_revocation("owned-sidecar").revoke();
    assert!(cancel.is_closed());
    assert_eq!(
        operation
            .with_terminal(|_| panic!("dead owner publication"))
            .unwrap(),
        None::<()>
    );
    assert!(registry
        .reserve("main", "owned-sidecar", request("surface", 2))
        .is_err());

    registry.set_owner(Some("replacement"));
    let replacement = registry
        .reserve("main", "replacement", request("surface", 1))
        .unwrap();
    registry.owner_revocation("owned-sidecar").revoke();
    assert!(!replacement.cancellation().is_closed());
    assert_eq!(replacement.with_current(|_| 1).unwrap(), Some(1));
    let weak = registry.owner_revocation("replacement");
    drop(registry);
    weak.revoke(); // No registry is retained by a terminal observer.
    assert!(replacement.cancellation().is_closed());
}

#[test]
fn subscriptions_replace_before_publication_and_old_finish_cannot_remove_new() {
    let registry = registry();
    let old = registry
        .reserve("main", "owned-sidecar", request("surface", 1))
        .unwrap();
    let old_cancel = old.cancellation();
    assert_eq!(old.with_current(|_| 1).unwrap(), Some(1));
    let new = registry
        .reserve("main", "owned-sidecar", request("surface", 2))
        .unwrap();
    assert_ne!(
        old.identity().subscription_id,
        new.identity().subscription_id
    );
    assert!(old_cancel.is_closed());
    assert_eq!(
        old.with_current(|_| panic!("stale publication")).unwrap(),
        None::<()>
    );
    drop(old);
    assert_eq!(
        new.with_current(|identity| identity.generation).unwrap(),
        Some(2)
    );
    let cancel = new.cancellation();
    drop(new);
    assert!(cancel.is_closed());
    assert_eq!(registry.live.load(Ordering::Acquire), 0);
    assert!(registry
        .reserve("main", "owned-sidecar", request("surface", 2))
        .is_err());
}

#[test]
fn subscriptions_native_owner_clear_drop_and_cancel_fences() {
    let registry = registry();
    let operation = registry
        .reserve("main", "owned-sidecar", request("surface", 1))
        .unwrap();
    let cancel = operation.cancellation();
    registry.clear();
    assert!(cancel.is_closed());
    assert_eq!(
        operation
            .with_current(|_| panic!("cleared publication"))
            .unwrap(),
        None::<()>
    );
    assert!(registry
        .reserve("main", "owned-sidecar", request("surface", 1))
        .is_err());
    assert!(registry
        .reserve("main", "owned-sidecar", request("surface", 2))
        .is_err());
    registry.set_owner(Some("owned-sidecar"));
    let operation = registry
        .reserve("main", "owned-sidecar", request("surface", 2))
        .unwrap();
    registry.set_owner(Some("replacement-sidecar"));
    assert!(operation.cancellation().is_closed());
    assert!(registry
        .reserve("main", "owned-sidecar", request("surface", 3))
        .is_err());
    let replacement = registry
        .reserve("main", "replacement-sidecar", request("surface", 1))
        .unwrap();
    drop(operation);
    assert_eq!(replacement.with_current(|_| true).unwrap(), Some(true));
    replacement.cancellation().close();
    assert_eq!(
        replacement
            .with_current(|_| panic!("cancelled publication"))
            .unwrap(),
        None::<()>
    );
    drop(registry);
    assert_eq!(
        replacement
            .with_current(|_| panic!("dropped registry"))
            .unwrap(),
        None::<()>
    );
}

#[test]
fn subscriptions_admission_bounds_retiring_workers_and_surface_tombstones() {
    let registry = registry();
    let mut old = Vec::new();
    let mut new = Vec::new();
    for index in 0..CURRENT_MAX {
        old.push(
            registry
                .reserve(
                    "main",
                    "owned-sidecar",
                    request(&format!("surface-{index}"), 1),
                )
                .unwrap(),
        );
    }
    assert_eq!(
        registry
            .reserve("main", "owned-sidecar", request("overflow", 1))
            .err()
            .as_deref(),
        Some(BUSY)
    );
    for index in 0..CURRENT_MAX {
        new.push(
            registry
                .reserve(
                    "main",
                    "owned-sidecar",
                    request(&format!("surface-{index}"), 2),
                )
                .unwrap(),
        );
    }
    assert_eq!(registry.live.load(Ordering::Acquire), LIVE_MAX);
    assert_eq!(
        registry
            .reserve("main", "owned-sidecar", request("surface-0", 3))
            .err()
            .as_deref(),
        Some(BUSY)
    );
    assert_eq!(new[0].with_current(|_| true).unwrap(), Some(true));
    drop(old);
    let next = registry
        .reserve("main", "owned-sidecar", request("surface-0", 3))
        .unwrap();
    drop(next);
    drop(new);
    for index in CURRENT_MAX..SURFACE_MAX {
        drop(
            registry
                .reserve(
                    "main",
                    "owned-sidecar",
                    request(&format!("surface-{index}"), 1),
                )
                .unwrap(),
        );
    }
    assert_eq!(
        registry
            .reserve("main", "owned-sidecar", request("overflow", 1))
            .err()
            .as_deref(),
        Some("remote surface limit reached; reuse an existing surface or restart the Preview")
    );
    assert_eq!(registry.live.load(Ordering::Acquire), 0);
}

#[test]
fn subscriptions_reject_wide_scope_old_generations_and_secondary_windows() {
    let registry = registry();
    for label in ["", "settings", "main-2"] {
        assert_eq!(
            registry
                .reserve(label, "owned-sidecar", request("surface", 1))
                .err()
                .as_deref(),
            Some(INVALID)
        );
    }
    for extra in [
        "url",
        "token",
        "workspace",
        "afterSequence",
        "sidecarInstanceId",
    ] {
        let mut body = json!({"controllerId":ID,"sessionPath":"/remote/selected.jsonl","surfaceId":"surface","generation":1});
        body[extra] = json!("private");
        assert!(serde_json::from_value::<SubscribeRequest>(body).is_err());
    }
    for (surface, generation) in [
        ("", 1),
        ("bad\n", 1),
        ("surface", 0),
        ("surface", MAX_JS + 1),
    ] {
        assert!(registry
            .reserve("main", "owned-sidecar", request(surface, generation))
            .is_err());
    }
    let current = registry
        .reserve("main", "owned-sidecar", request("surface", 7))
        .unwrap();
    for generation in [1, 6, 7] {
        assert!(registry
            .reserve("main", "owned-sidecar", request("surface", generation))
            .is_err());
    }
    assert_eq!(
        current
            .with_current(|identity| (
                identity.surface_id.clone(),
                identity.controller_id.clone(),
                identity.session_path.clone(),
                identity.sidecar_instance_id.clone()
            ))
            .unwrap(),
        Some((
            "surface".into(),
            ID.into(),
            "/remote/selected.jsonl".into(),
            "owned-sidecar".into()
        ))
    );
}
