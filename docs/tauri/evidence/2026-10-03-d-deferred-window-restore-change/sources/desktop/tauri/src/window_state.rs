use std::{
    fs, io,
    io::Write,
    path::PathBuf,
    sync::{
        atomic::{AtomicBool, Ordering},
        Mutex,
    },
};

use serde::{Deserialize, Serialize};
use tauri::{Manager, WebviewWindow};

const STATE_FILE: &str = "window-state.json";
const MIN_WIDTH: u32 = 900;
const MIN_HEIGHT: u32 = 620;
const MAX_DIMENSION: u32 = 16_384;

/// Host state stays outside REASONIX_HOME. Keep normal geometry in memory so
/// maximizing, minimizing or hiding cannot replace it with transient bounds.
pub struct PreviewWindowState {
    path: PathBuf,
    normal: Mutex<Option<SavedWindowState>>,
    restore_pending: AtomicBool,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
struct SavedWindowState {
    width: u32,
    height: u32,
    maximized: bool,
    #[serde(default)]
    x: Option<i32>,
    #[serde(default)]
    y: Option<i32>,
    #[serde(default)]
    scale_factor: Option<f64>,
}

impl SavedWindowState {
    fn is_valid(&self) -> bool {
        (MIN_WIDTH..=MAX_DIMENSION).contains(&self.width)
            && (MIN_HEIGHT..=MAX_DIMENSION).contains(&self.height)
            && self
                .scale_factor
                .is_none_or(|scale| scale.is_finite() && (0.5..=8.0).contains(&scale))
    }
}

struct WorkArea {
    x: i32,
    y: i32,
    width: u32,
    height: u32,
    scale: f64,
}

/// Require a reachable title bar on a connected monitor. Missing monitors and
/// old size-only files fall back to the primary monitor, with bounded position.
fn restored_bounds(state: &SavedWindowState, areas: &[WorkArea]) -> Option<(i32, i32, u32, u32)> {
    let saved_position = state.x.zip(state.y);
    let target = saved_position
        .and_then(|(x, y)| {
            areas
                .iter()
                .filter_map(|area| {
                    let left = i64::from(area.x);
                    let right = left + i64::from(area.width);
                    let bottom = i64::from(area.y) + i64::from(area.height);
                    let saved_left = i64::from(x);
                    let saved_right = saved_left + i64::from(state.width);
                    let overlap = saved_right.min(right) - saved_left.max(left);
                    (overlap >= 96
                        && i64::from(y) >= i64::from(area.y)
                        && i64::from(y) + 32 <= bottom)
                        .then_some((area, overlap))
                })
                // A straddling window must not move to the primary display
                // merely because a small part of its title bar touches it.
                // min_by_key retains the first (primary) area on equal scores.
                .min_by_key(|(_, overlap)| std::cmp::Reverse(*overlap))
                .map(|(area, _)| area)
        })
        .or_else(|| areas.first())?;
    let ratio = state.scale_factor.map_or(1.0, |scale| target.scale / scale);
    let width = ((f64::from(state.width) * ratio).round() as u32)
        .min(target.width)
        .max((f64::from(MIN_WIDTH) * target.scale).round() as u32);
    let height = ((f64::from(state.height) * ratio).round() as u32)
        .min(target.height)
        .max((f64::from(MIN_HEIGHT) * target.scale).round() as u32);
    let (x, y) = saved_position.unwrap_or((target.x, target.y));
    // Use i64: monitor coordinates can be negative, and saved positions are untrusted.
    let max_x = i64::from(target.x) + i64::from(target.width.saturating_sub(width));
    let max_y = i64::from(target.y) + i64::from(target.height.saturating_sub(height));
    Some((
        i64::from(x).clamp(i64::from(target.x), max_x) as i32,
        i64::from(y).clamp(i64::from(target.y), max_y) as i32,
        width,
        height,
    ))
}

impl PreviewWindowState {
    pub fn for_app(app: &tauri::App) -> Result<Self, String> {
        let directory = app
            .path()
            .app_data_dir()
            .map_err(|error| format!("resolve Tauri window state directory: {error}"))?;
        fs::create_dir_all(&directory).map_err(|error| {
            format!(
                "create Tauri window state directory {}: {error}",
                directory.display()
            )
        })?;
        let path = directory.join(STATE_FILE);
        let saved = read_state(&path);
        let restore_pending = AtomicBool::new(saved.is_some());
        let normal = Mutex::new(saved);
        Ok(Self {
            path,
            normal,
            restore_pending,
        })
    }

    pub fn restore(&self, window: &WebviewWindow) {
        if !self.restore_pending.load(Ordering::SeqCst) {
            return;
        }
        let state = self
            .normal
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .clone();
        let Some(state) = state else {
            return;
        };
        let Ok(mut monitors) = window.available_monitors() else {
            return;
        };
        // CoreGraphics can temporarily report no active displays while AppKit
        // retains cached screens. A cached primary is not proof that a saved
        // secondary was unplugged. Keep the original until enumeration resumes.
        if monitors.is_empty() {
            return;
        }
        if let Ok(Some(primary)) = window.primary_monitor() {
            monitors.retain(|monitor| monitor.position() != primary.position());
            monitors.insert(0, primary);
        }
        let areas: Vec<_> = monitors
            .iter()
            .map(|monitor| {
                let area = monitor.work_area();
                WorkArea {
                    x: area.position.x,
                    y: area.position.y,
                    width: area.size.width,
                    height: area.size.height,
                    scale: monitor.scale_factor(),
                }
            })
            .collect();
        if let Some((x, y, width, height)) = restored_bounds(&state, &areas) {
            if window
                .set_size(tauri::PhysicalSize::new(width, height))
                .is_err()
                || window
                    .set_position(tauri::PhysicalPosition::new(x, y))
                    .is_err()
            {
                return;
            }
        }
        if state.maximized && window.maximize().is_err() {
            return;
        }
        self.restore_pending.store(false, Ordering::SeqCst);
    }

    pub fn capture(&self, window: &WebviewWindow) {
        if self.restore_pending.load(Ordering::SeqCst)
            || window
                .available_monitors()
                .map_or(true, |monitors| monitors.is_empty())
        {
            return;
        }
        if window.is_minimized().unwrap_or(true) || window.is_fullscreen().unwrap_or(true) {
            return;
        }
        let Ok(maximized) = window.is_maximized() else {
            return;
        };
        let mut normal = self.normal.lock().unwrap_or_else(|e| e.into_inner());
        if maximized {
            if let Some(state) = normal.as_mut() {
                state.maximized = true;
            }
            return;
        }
        let (Ok(size), Ok(position), Ok(scale)) = (
            window.inner_size(),
            window.outer_position(),
            window.scale_factor(),
        ) else {
            return;
        };
        let state = SavedWindowState {
            width: size.width,
            height: size.height,
            maximized,
            x: Some(position.x),
            y: Some(position.y),
            scale_factor: Some(scale),
        };
        if state.is_valid() {
            *normal = Some(state);
        }
    }

    pub fn save(&self, window: &WebviewWindow) -> Result<(), String> {
        self.capture(window);
        let state = self
            .normal
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .clone();
        let Some(state) = state else {
            return Ok(());
        };
        let encoded = serde_json::to_vec(&state)
            .map_err(|error| format!("encode main window state: {error}"))?;
        write_state(&self.path, &encoded)
            .map_err(|error| format!("write main window state {}: {error}", self.path.display()))
    }
}

fn read_state(path: &std::path::Path) -> Option<SavedWindowState> {
    serde_json::from_slice::<SavedWindowState>(&fs::read(path).ok()?)
        .ok()
        .filter(SavedWindowState::is_valid)
}

fn write_state(path: &std::path::Path, encoded: &[u8]) -> io::Result<()> {
    let parent = path
        .parent()
        .ok_or_else(|| io::Error::other("window state has no parent"))?;
    let mut file = tempfile::NamedTempFile::new_in(parent)?;
    file.write_all(encoded)?;
    file.as_file().sync_all()?;
    file.persist(path).map_err(|error| error.error)?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    fn state() -> SavedWindowState {
        SavedWindowState {
            width: 1280,
            height: 820,
            maximized: false,
            x: None,
            y: None,
            scale_factor: None,
        }
    }
    fn area(x: i32, y: i32, width: u32, height: u32, scale: f64) -> WorkArea {
        WorkArea {
            x,
            y,
            width,
            height,
            scale,
        }
    }

    #[test]
    fn accepts_only_sensible_window_sizes_and_scales() {
        assert!(state().is_valid());
        assert!(!SavedWindowState {
            width: MIN_WIDTH - 1,
            ..state()
        }
        .is_valid());
        assert!(!SavedWindowState {
            height: MAX_DIMENSION + 1,
            ..state()
        }
        .is_valid());
        assert!(!SavedWindowState {
            scale_factor: Some(f64::NAN),
            ..state()
        }
        .is_valid());
    }

    #[test]
    fn persists_and_reads_old_size_only_state() {
        let root = tempfile::tempdir().unwrap();
        let path = root.path().join(STATE_FILE);
        let expected = SavedWindowState {
            x: Some(-1200),
            y: Some(50),
            scale_factor: Some(2.0),
            ..state()
        };
        write_state(&path, &serde_json::to_vec(&expected).unwrap()).unwrap();
        assert_eq!(read_state(&path), Some(expected));
        fs::write(&path, r#"{"width":1440,"height":900,"maximized":true}"#).unwrap();
        assert_eq!(
            read_state(&path),
            Some(SavedWindowState {
                width: 1440,
                height: 900,
                maximized: true,
                ..state()
            })
        );
    }

    #[test]
    fn ignores_corrupt_or_unreasonable_state() {
        let root = tempfile::tempdir().unwrap();
        let path = root.path().join(STATE_FILE);
        fs::write(&path, "not json").unwrap();
        assert_eq!(read_state(&path), None);
        fs::write(&path, r#"{"width":1,"height":1,"maximized":false}"#).unwrap();
        assert_eq!(read_state(&path), None);
    }

    #[test]
    fn restores_negative_coordinates_on_a_connected_secondary_monitor() {
        let state = SavedWindowState {
            x: Some(-1400),
            y: Some(70),
            ..state()
        };
        assert_eq!(
            restored_bounds(
                &state,
                &[
                    area(0, 25, 1920, 1055, 1.0),
                    area(-1920, 25, 1920, 1055, 1.0)
                ]
            ),
            Some((-1400, 70, 1280, 820))
        );
    }

    #[test]
    fn restores_straddling_window_to_display_with_most_visible_title_bar() {
        let saved = SavedWindowState {
            x: Some(-800),
            y: Some(70),
            scale_factor: Some(1.0),
            ..state()
        };
        assert_eq!(
            restored_bounds(
                &saved,
                &[
                    area(0, 25, 1920, 1055, 1.0),
                    area(-1920, 25, 1920, 1055, 1.0),
                ]
            ),
            Some((-1280, 70, 1280, 820))
        );
        let saved = SavedWindowState {
            x: Some(1800),
            ..saved
        };
        assert_eq!(
            restored_bounds(
                &saved,
                &[
                    area(0, 25, 1920, 1055, 1.0),
                    area(1920, 25, 3840, 2110, 2.0),
                ]
            ),
            Some((1920, 70, 2560, 1640))
        );
    }

    #[test]
    fn equal_title_bar_overlap_prefers_primary_display() {
        let saved = SavedWindowState {
            x: Some(1280),
            y: Some(70),
            ..state()
        };
        assert_eq!(
            restored_bounds(
                &saved,
                &[
                    area(0, 25, 1920, 1055, 1.0),
                    area(1920, 25, 1920, 1055, 1.0),
                ]
            ),
            Some((640, 70, 1280, 820))
        );
    }

    #[test]
    fn disconnected_monitor_and_extreme_coordinates_return_to_primary_work_area() {
        for (x, y) in [(-2500, 50), (i32::MAX, i32::MIN)] {
            let state = SavedWindowState {
                x: Some(x),
                y: Some(y),
                ..state()
            };
            let (x, y, width, height) =
                restored_bounds(&state, &[area(0, 25, 1920, 1055, 1.0)]).unwrap();
            assert!((0..=640).contains(&x) && (25..=260).contains(&y));
            assert_eq!((width, height), (1280, 820));
        }
    }

    #[test]
    fn keeps_logical_size_when_restoring_on_a_different_display_scale() {
        let state = SavedWindowState {
            width: 2560,
            height: 1640,
            scale_factor: Some(2.0),
            ..state()
        };
        assert_eq!(
            restored_bounds(&state, &[area(0, 25, 1920, 1055, 1.0)]),
            Some((0, 25, 1280, 820))
        );
        assert_eq!(restored_bounds(&state, &[]), None);
    }
}
