use super::{frame, Frame};
use crate::protocol_generated::BridgeEvent;
use objc2::{rc::Retained, AnyThread, MainThreadMarker, MainThreadOnly};
use objc2_app_kit::{
    NSBackingStoreType, NSColor, NSEvent, NSImage, NSImageView, NSPanel, NSScreen, NSWindow,
    NSWindowCollectionBehavior, NSWindowSharingType, NSWindowStyleMask, NSWorkspace,
};
use objc2_foundation::{NSData, NSPoint, NSRect, NSSize};
use std::{
    cell::RefCell,
    collections::{BTreeMap, VecDeque},
    time::Duration,
};

const ART: &str = include_str!("../../resources/blue-whale-cursor.svg");

#[derive(Default)]
struct State {
    sequence: u64,
    generation: u64,
    owner: Option<(String, String)>,
    ended: VecDeque<(String, String)>,
    known_turns: BTreeMap<String, String>,
    cancelled: VecDeque<String>,
    overlay: Option<Overlay>,
}
struct Overlay {
    panel: Retained<NSPanel>,
    view: Retained<NSImageView>,
    images: Vec<Retained<NSImage>>,
}
thread_local! { static STATE: RefCell<State> = RefCell::new(State::default()); }

impl Overlay {
    fn new(mtm: MainThreadMarker) -> Option<Self> {
        let panel = NSPanel::initWithContentRect_styleMask_backing_defer(
            NSPanel::alloc(mtm),
            NSRect::new(NSPoint::new(0., 0.), NSSize::new(64., 64.)),
            NSWindowStyleMask::Borderless | NSWindowStyleMask::NonactivatingPanel,
            NSBackingStoreType::Buffered,
            false,
        );
        panel.setOpaque(false);
        panel.setBackgroundColor(Some(&NSColor::clearColor()));
        panel.setHasShadow(false);
        panel.setIgnoresMouseEvents(true);
        panel.setHidesOnDeactivate(false);
        panel.setBecomesKeyOnlyIfNeeded(true);
        panel.setLevel(24);
        panel.setSharingType(NSWindowSharingType::None);
        panel.setCollectionBehavior(
            NSWindowCollectionBehavior::CanJoinAllSpaces
                | NSWindowCollectionBehavior::FullScreenAuxiliary
                | NSWindowCollectionBehavior::IgnoresCycle,
        );
        let view = NSImageView::initWithFrame(
            NSImageView::alloc(mtm),
            NSRect::new(NSPoint::new(0., 0.), NSSize::new(64., 64.)),
        );
        panel.setContentView(Some(&view));
        let mut images = Vec::new();
        for i in 0..7 {
            let ring = if i == 0 {
                String::new()
            } else {
                format!(
                    r##"<circle cx="12" cy="12" r="{}" fill="none" stroke="#39a4ff" stroke-width="2" opacity="{}"/>"##,
                    4 + i,
                    1.0 - (i as f64) * 0.12
                )
            };
            let svg = ART.replace("</svg>", &format!("{ring}</svg>"));
            images.push(NSImage::initWithData(
                NSImage::alloc(),
                &NSData::with_bytes(svg.as_bytes()),
            )?);
        }
        Some(Self {
            panel,
            view,
            images,
        })
    }
    fn hide(&self) {
        self.panel.orderOut(None);
    }
    fn paint(&self, frame: &Frame, step: usize, mtm: MainThreadMarker) -> bool {
        let Some(app) = NSWorkspace::sharedWorkspace().frontmostApplication() else {
            return false;
        };
        if app.processIdentifier() != frame.pid {
            return false;
        }
        let Some(primary) = NSScreen::screens(mtm).firstObject() else {
            return false;
        };
        let actual = NSEvent::mouseLocation();
        let point = NSPoint::new(
            frame.cursor.screen_x,
            primary.frame().size.height - frame.cursor.screen_y,
        );
        // Never leave a marker at an obsolete position after human movement.
        if (actual.x - point.x).hypot(actual.y - point.y) > 1.0
            || NSWindow::windowNumberAtPoint_belowWindowWithWindowNumber(actual, 0, mtm)
                != frame.window as isize
        {
            return false;
        }
        let index = if frame.cursor.clicked && step < 12 {
            step / 2 + 1
        } else {
            0
        };
        self.view.setImage(Some(&self.images[index]));
        self.panel.setFrameOrigin(panel_origin(
            frame.cursor.screen_x,
            frame.cursor.screen_y,
            primary.frame().size.height,
        ));
        self.panel.setAlphaValue(if step > 25 {
            (32 - step) as f64 / 7.
        } else {
            1.
        });
        self.panel.orderFrontRegardless();
        true
    }
}

fn hide(state: &mut State) {
    state.generation = state.generation.wrapping_add(1);
    if let Some(overlay) = &state.overlay {
        overlay.hide();
    }
}

pub(super) fn observe(app: &tauri::AppHandle, event: &BridgeEvent) {
    let sample = frame(event);
    let sequence = event.sequence;
    let owner = event
        .payload
        .get("turnId")
        .and_then(|v| v.as_str())
        .map(|turn| (event.session_id.clone(), turn.to_string()));
    let terminal = event.event_kind == "turn_done";
    let turn_started = event.event_kind == "turn_started";
    let started = matches!(event.event_kind.as_str(), "tool_start" | "tool_started")
        && event.payload.get("tool").is_some_and(|t| {
            t.get("name").and_then(|v| v.as_str()) == Some("computer_use")
                || t.get("resolvedName").and_then(|v| v.as_str()) == Some("computer_use")
                || t.get("capabilityId").and_then(|v| v.as_str()) == Some("tool:computer_use")
        });
    if sample.is_none() && !terminal && !started && !turn_started {
        return;
    }
    let handle = app.clone();
    let _ = app.run_on_main_thread(move || {
        let Some(mtm) = MainThreadMarker::new() else {
            return;
        };
        STATE.with(|cell| {
            let mut state = cell.borrow_mut();
            let Some(frame) = state.apply(sequence, owner, terminal, started, turn_started, sample)
            else {
                return;
            };
            if state.overlay.is_none() {
                state.overlay = Overlay::new(mtm);
            }
            let generation = state.generation;
            drop(state);
            animate(handle, frame, generation);
        });
    });
}

fn animate(app: tauri::AppHandle, frame: Frame, generation: u64) {
    // A bounded display-only animation. All AppKit access and generation
    // checks stay on the main thread; stale completions perform zero writes.
    std::thread::spawn(move || {
        for step in 0..=32 {
            if step > 0 {
                std::thread::sleep(Duration::from_millis(40));
            }
            let frame = frame.clone();
            let _ = app.run_on_main_thread(move || {
                STATE.with(|cell| {
                    let Some(mtm) = MainThreadMarker::new() else {
                        return;
                    };
                    let mut state = cell.borrow_mut();
                    if state.generation != generation {
                        return;
                    }
                    if step == 32
                        || !state
                            .overlay
                            .as_ref()
                            .is_some_and(|o| o.paint(&frame, step, mtm))
                    {
                        state.expire(generation);
                    }
                })
            });
        }
    });
}

pub(super) fn clear_session(app: &tauri::AppHandle, session: &str) {
    let session = session.to_string();
    let _ = app.run_on_main_thread(move || {
        STATE.with(|cell| {
            let mut state = cell.borrow_mut();
            state.cancel(&session);
        })
    });
}

impl State {
    fn remember(&mut self, owner: (String, String)) {
        if self.ended.contains(&owner) {
            return;
        }
        if self.ended.len() == 64 {
            self.ended.pop_front();
        }
        self.ended.push_back(owner);
    }
    fn apply(
        &mut self,
        sequence: u64,
        owner: Option<(String, String)>,
        terminal: bool,
        started: bool,
        turn_started: bool,
        sample: Option<Frame>,
    ) -> Option<Frame> {
        if sequence <= self.sequence {
            return None;
        }
        self.sequence = sequence;
        if turn_started {
            if let Some(owner) = owner {
                if self.ended.contains(&owner) {
                    return None;
                }
                if self
                    .known_turns
                    .get(&owner.0)
                    .is_some_and(|turn| *turn != owner.1)
                {
                    self.cancelled.retain(|session| *session != owner.0);
                }
                if self
                    .owner
                    .as_ref()
                    .is_some_and(|active| active.0 == owner.0 && active.1 != owner.1)
                {
                    if let Some(previous) = self.owner.take() {
                        self.remember(previous);
                    }
                    hide(self);
                }
                self.note_turn(&owner.0, &owner.1);
            }
            return None;
        }

        if terminal {
            if let Some(owner) = owner {
                if self.owner.as_ref() == Some(&owner) {
                    hide(self);
                    self.owner = None;
                }
                if !self.known_turns.contains_key(&owner.0) {
                    self.note_turn(&owner.0, &owner.1);
                }
                self.remember(owner);
            }
            return None;
        }
        if started {
            hide(self);
            return None;
        }
        let frame = sample?;
        let owner = (frame.session.clone(), frame.turn.clone());
        if self.ended.contains(&owner)
            || self.cancelled.contains(&owner.0)
            || self
                .known_turns
                .get(&owner.0)
                .is_some_and(|turn| *turn != owner.1)
        {
            return None;
        }
        self.note_turn(&owner.0, &owner.1);
        hide(self);
        self.owner = Some(owner);
        Some(frame)
    }
    fn note_turn(&mut self, session: &str, turn: &str) {
        if self.known_turns.len() == 64 && !self.known_turns.contains_key(session) {
            self.known_turns.pop_first();
        }
        self.known_turns
            .insert(session.to_string(), turn.to_string());
    }
    fn cancel(&mut self, session: &str) {
        if !self.cancelled.iter().any(|s| s == session) {
            if self.cancelled.len() == 64 {
                self.cancelled.pop_front();
            }
            self.cancelled.push_back(session.to_string());
        }

        if self.owner.as_ref().is_some_and(|owner| owner.0 == session) {
            if let Some(owner) = self.owner.take() {
                self.remember(owner);
            }
            hide(self);
        }
    }
    fn expire(&mut self, generation: u64) -> bool {
        if self.generation != generation {
            return false;
        }
        hide(self);
        true
    }
}
fn panel_origin(x: f64, y: f64, height: f64) -> NSPoint {
    NSPoint::new(x - 12., height - y - 52.)
}

#[cfg(test)]
mod tests {
    use super::*;
    fn sample(session: &str) -> Frame {
        Frame {
            session: session.into(),
            turn: "turn".into(),
            pid: 42,
            window: 11,
            cursor: super::super::Cursor {
                x: 0.4,
                y: 0.3,
                screen_x: -1000.,
                screen_y: 200.,
                clicked: true,
            },
        }
    }
    #[test]
    fn stale_frames_and_animation_cannot_revive_stopped_tasks() {
        let mut state = State::default();
        assert!(state
            .apply(1, None, false, false, false, Some(sample("a")))
            .is_some());
        let old = state.generation;
        assert!(state
            .apply(2, None, false, false, false, Some(sample("b")))
            .is_some());
        let current = state.generation;
        state.apply(
            3,
            Some(("a".into(), "turn".into())),
            true,
            false,
            false,
            None,
        );
        assert_eq!(state.generation, current);
        assert!(!state.expire(old));
        assert_eq!(state.generation, current);
        state.cancel("a");
        assert_eq!(state.generation, current);
        state.cancel("b");
        assert!(state.owner.is_none());
        assert!(state
            .apply(4, None, false, false, false, Some(sample("b")))
            .is_none());
        assert!(state
            .apply(1, None, false, false, false, Some(sample("a")))
            .is_none());
        for n in 0..100 {
            state.remember((n.to_string(), "turn".into()));
        }
        assert_eq!(state.ended.len(), 64);
    }
    #[test]
    fn cancel_before_the_first_preview_rejects_queued_frames() {
        let mut state = State::default();
        state.cancel("a");
        state.apply(
            1,
            Some(("a".into(), "turn".into())),
            false,
            false,
            true,
            None,
        );
        assert!(state
            .apply(2, None, false, false, false, Some(sample("a")))
            .is_none());
        state.apply(
            3,
            Some(("a".into(), "new".into())),
            false,
            false,
            true,
            None,
        );
        let mut next = sample("a");
        next.turn = "new".into();
        assert!(state
            .apply(4, None, false, false, false, Some(next))
            .is_some());
        assert!(state
            .apply(5, None, false, false, false, Some(sample("a")))
            .is_none());
    }
    #[test]
    fn ending_a_cancelled_unseen_turn_allows_a_fresh_turn() {
        let mut state = State::default();
        state.cancel("a");
        state.apply(
            1,
            Some(("a".into(), "old".into())),
            true,
            false,
            false,
            None,
        );
        state.apply(
            2,
            Some(("a".into(), "new".into())),
            false,
            false,
            true,
            None,
        );
        let mut next = sample("a");
        next.turn = "new".into();
        assert!(state
            .apply(3, None, false, false, false, Some(next))
            .is_some());
    }
    #[test]
    fn overlay_hotspot_uses_primary_origin_with_negative_displays() {
        let origin = panel_origin(-1900., -300., 1080.);
        assert_eq!(origin.x + 12., -1900.);
        assert_eq!(origin.y + 52., 1380.);
    }
}

#[cfg(feature = "cursor-qa")]
pub(super) fn acceptance_fixture(output: &str) -> Result<(), String> {
    use objc2_app_kit::{NSApplication, NSApplicationActivationPolicy, NSBitmapImageFileType};
    use objc2_foundation::{NSDate, NSDictionary, NSRunLoop, NSString};
    let mtm = MainThreadMarker::new().ok_or("QA requires main thread")?;
    let app = NSApplication::sharedApplication(mtm);
    app.setActivationPolicy(NSApplicationActivationPolicy::Accessory);
    app.finishLaunching();
    let mut actual = NSEvent::mouseLocation();
    let target = unsafe {
        NSWindow::initWithContentRect_styleMask_backing_defer(
            NSWindow::alloc(mtm),
            NSRect::new(
                NSPoint::new(actual.x - 100., actual.y - 80.),
                NSSize::new(240., 180.),
            ),
            NSWindowStyleMask::Titled,
            NSBackingStoreType::Buffered,
            false,
        )
    };
    target.setTitle(&NSString::from_str("Reasonix Whale Cursor QA"));
    target.setBackgroundColor(Some(&NSColor::whiteColor()));
    target.makeKeyAndOrderFront(None);
    // The isolated fixture explicitly activates only its own window.
    #[allow(deprecated)]
    app.activateIgnoringOtherApps(true);
    NSRunLoop::currentRunLoop().runUntilDate(&NSDate::dateWithTimeIntervalSinceNow(0.2));
    actual = NSEvent::mouseLocation();
    let primary = NSScreen::screens(mtm)
        .firstObject()
        .ok_or("missing display")?;
    let cursor = super::Cursor {
        x: 0.4,
        y: 0.3,
        screen_x: actual.x,
        screen_y: primary.frame().size.height - actual.y,
        clicked: true,
    };
    let sample = Frame {
        session: "qa".into(),
        turn: "qa-turn".into(),
        pid: std::process::id() as i32,
        window: target.windowNumber() as i64,
        cursor,
    };
    let overlay = Overlay::new(mtm).ok_or("SVG failed to decode")?;
    let key = app.keyWindow().map(|w| w.windowNumber());
    if !overlay.paint(&sample, 2, mtm) {
        eprintln!(
            "fixture guards: frontmost_is_self={}, hit_is_fixture={}, pointer_moved={}",
            NSWorkspace::sharedWorkspace()
                .frontmostApplication()
                .is_some_and(|a| a.processIdentifier() == std::process::id() as i32),
            NSWindow::windowNumberAtPoint_belowWindowWithWindowNumber(
                NSEvent::mouseLocation(),
                0,
                mtm
            ) == target.windowNumber(),
            (NSEvent::mouseLocation().x - actual.x).hypot(NSEvent::mouseLocation().y - actual.y)
        );
        target.orderOut(None);
        return Err("production overlay refused fixture target".into());
    }
    NSRunLoop::currentRunLoop().runUntilDate(&NSDate::dateWithTimeIntervalSinceNow(0.1));
    let hit = NSWindow::windowNumberAtPoint_belowWindowWithWindowNumber(actual, 0, mtm);
    let focused = app.keyWindow().map(|w| w.windowNumber());
    let rect = overlay.view.bounds();
    let image = overlay
        .view
        .bitmapImageRepForCachingDisplayInRect(rect)
        .ok_or("cache view unavailable")?;
    overlay
        .view
        .cacheDisplayInRect_toBitmapImageRep(rect, &image);
    // SAFETY: PNG accepts an empty property dictionary; the cached bitmap
    // belongs to this fixture view and remains alive throughout encoding.
    let data = unsafe {
        image.representationUsingType_properties(NSBitmapImageFileType::PNG, &NSDictionary::new())
    }
    .ok_or("PNG unavailable")?;
    let wrote = data.writeToFile_atomically(&NSString::from_str(output), true);
    let valid = wrote
        && hit == target.windowNumber()
        && focused == key
        && overlay.panel.ignoresMouseEvents()
        && !overlay.panel.isOpaque()
        && overlay.panel.sharingType() == NSWindowSharingType::None;
    overlay.hide();
    target.orderOut(None);
    if !valid {
        return Err("overlay click-through/focus/appearance contract failed".into());
    }
    println!("PASS production whale overlay: SVG rendered, pointer hit passes through, focus unchanged, screenshot sharing disabled, no input sent");
    Ok(())
}
