use std::{fs, io, io::Write, path::PathBuf};

use serde::{Deserialize, Serialize};
use tauri::{Manager, WebviewWindow};

const STATE_FILE: &str = "window-state.json";
const MIN_WIDTH: u32 = 900;
const MIN_HEIGHT: u32 = 620;
const MAX_DIMENSION: u32 = 16_384;

/// Non-sensitive host state. It deliberately lives outside `REASONIX_HOME`:
/// resetting or importing the Go core profile must not alter window behaviour.
pub struct PreviewWindowState {
    path: PathBuf,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Eq, Serialize)]
struct SavedWindowState {
    width: u32,
    height: u32,
    maximized: bool,
}

impl SavedWindowState {
    fn is_valid(&self) -> bool {
        (MIN_WIDTH..=MAX_DIMENSION).contains(&self.width)
            && (MIN_HEIGHT..=MAX_DIMENSION).contains(&self.height)
    }
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
        Ok(Self {
            path: directory.join(STATE_FILE),
        })
    }

    /// Restoration is best-effort: an old, corrupt, or unreasonable state must
    /// never keep the Preview from starting or produce an unusable tiny window.
    pub fn restore(&self, window: &WebviewWindow) {
        let Some(state) = self.read() else {
            return;
        };

        if state.maximized {
            let _ = window.maximize();
        } else {
            let _ = window.set_size(tauri::PhysicalSize::new(state.width, state.height));
        }
    }

    pub fn save(&self, window: &WebviewWindow) -> Result<(), String> {
        let size = window
            .inner_size()
            .map_err(|error| format!("read main window size: {error}"))?;
        let state = SavedWindowState {
            width: size.width,
            height: size.height,
            maximized: window
                .is_maximized()
                .map_err(|error| format!("read main window maximized state: {error}"))?,
        };

        if !state.is_valid() {
            return Ok(());
        }
        let encoded = serde_json::to_vec(&state)
            .map_err(|error| format!("encode main window state: {error}"))?;
        write_state(&self.path, &encoded)
            .map_err(|error| format!("write main window state {}: {error}", self.path.display()))
    }

    fn read(&self) -> Option<SavedWindowState> {
        serde_json::from_slice::<SavedWindowState>(&fs::read(&self.path).ok()?)
            .ok()
            .filter(SavedWindowState::is_valid)
    }
}

fn write_state(path: &std::path::Path, encoded: &[u8]) -> io::Result<()> {
    let mut file = fs::OpenOptions::new()
        .create(true)
        .truncate(true)
        .write(true)
        .open(path)?;
    file.write_all(encoded)?;
    file.sync_all()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepts_only_sensible_window_sizes() {
        assert!(SavedWindowState {
            width: 1280,
            height: 820,
            maximized: false,
        }
        .is_valid());
        assert!(!SavedWindowState {
            width: MIN_WIDTH - 1,
            height: MIN_HEIGHT,
            maximized: false,
        }
        .is_valid());
        assert!(!SavedWindowState {
            width: MAX_DIMENSION + 1,
            height: MIN_HEIGHT,
            maximized: false,
        }
        .is_valid());
    }

    #[test]
    fn persists_and_reloads_valid_state() {
        let root = tempfile::tempdir().expect("temp root");
        let store = PreviewWindowState {
            path: root.path().join(STATE_FILE),
        };
        let expected = SavedWindowState {
            width: 1440,
            height: 900,
            maximized: true,
        };

        write_state(
            &store.path,
            &serde_json::to_vec(&expected).expect("encode state"),
        )
        .expect("write state");

        assert_eq!(store.read(), Some(expected));
    }

    #[test]
    fn ignores_corrupt_or_unreasonable_state() {
        let root = tempfile::tempdir().expect("temp root");
        let store = PreviewWindowState {
            path: root.path().join(STATE_FILE),
        };
        fs::write(&store.path, "not json").expect("write corrupt state");
        assert_eq!(store.read(), None);

        fs::write(&store.path, r#"{"width":1,"height":1,"maximized":false}"#)
            .expect("write unreasonable state");
        assert_eq!(store.read(), None);
    }
}
