use std::{fs, io::Write, path::PathBuf, sync::Mutex};

use serde::{Deserialize, Serialize};
use tauri::Manager;

const FILE_NAME: &str = "host-preferences.json";

#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum CloseBehavior {
    #[default]
    KeepRunning,
    Quit,
}

pub struct HostPreferences {
    path: PathBuf,
    preferences: Mutex<SavedPreferences>,
}

#[derive(Clone, Copy, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct SavedPreferences {
    #[serde(default)]
    close_behavior: CloseBehavior,
    #[serde(default = "default_zoom_factor")]
    zoom_factor: f64,
}

impl Default for SavedPreferences {
    fn default() -> Self {
        Self {
            close_behavior: CloseBehavior::default(),
            zoom_factor: default_zoom_factor(),
        }
    }
}

fn default_zoom_factor() -> f64 {
    1.0
}

impl HostPreferences {
    pub fn for_app(app: &tauri::App) -> Result<Self, String> {
        let directory = app
            .path()
            .app_data_dir()
            .map_err(|error| format!("resolve Tauri preferences directory: {error}"))?;
        fs::create_dir_all(&directory)
            .map_err(|error| format!("create Tauri preferences directory: {error}"))?;
        let path = directory.join(FILE_NAME);
        let saved = read_preferences(&path).unwrap_or_default();
        Ok(Self {
            path,
            preferences: Mutex::new(saved),
        })
    }

    pub fn close_behavior(&self) -> CloseBehavior {
        self.preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .close_behavior
    }

    pub fn set_close_behavior(&self, behavior: CloseBehavior) -> Result<(), String> {
        let mut current = self
            .preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        if current.close_behavior == behavior {
            return Ok(());
        }
        let next = SavedPreferences {
            close_behavior: behavior,
            ..*current
        };
        self.write_preferences(next)?;
        *current = next;
        Ok(())
    }

    pub fn zoom_factor(&self) -> f64 {
        self.preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .zoom_factor
    }

    pub fn set_zoom_factor(&self, zoom_factor: f64) -> Result<(), String> {
        if !zoom_factor.is_finite() || !(0.5..=2.0).contains(&zoom_factor) {
            return Err("zoom factor must be between 0.5 and 2.0".to_string());
        }
        let snapped = ((zoom_factor * 20.0).round() / 20.0 * 100.0).round() / 100.0;
        let mut current = self
            .preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        if (current.zoom_factor - snapped).abs() < f64::EPSILON {
            return Ok(());
        }
        let next = SavedPreferences {
            zoom_factor: snapped,
            ..*current
        };
        self.write_preferences(next)?;
        *current = next;
        Ok(())
    }

    fn write_preferences(&self, saved: SavedPreferences) -> Result<(), String> {
        let data = serde_json::to_vec(&saved)
            .map_err(|error| format!("encode Tauri preferences: {error}"))?;
        let temporary = self.path.with_extension("json.tmp");
        let result = (|| -> std::io::Result<()> {
            let mut file = fs::File::create(&temporary)?;
            file.write_all(&data)?;
            file.sync_all()?;
            fs::rename(&temporary, &self.path)
        })();
        if let Err(error) = result {
            let _ = fs::remove_file(&temporary);
            return Err(format!("save Tauri preferences: {error}"));
        }
        Ok(())
    }
}

fn read_preferences(path: &std::path::Path) -> Option<SavedPreferences> {
    let data = fs::read(path).ok()?;
    serde_json::from_slice::<SavedPreferences>(&data).ok()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn close_preference_survives_restart_and_ignores_corrupt_data() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join(FILE_NAME);
        let store = HostPreferences {
            path: path.clone(),
            preferences: Mutex::new(SavedPreferences::default()),
        };
        store
            .set_close_behavior(CloseBehavior::Quit)
            .expect("save close behavior");
        assert_eq!(store.close_behavior(), CloseBehavior::Quit);
        assert_eq!(
            read_preferences(&path).unwrap().close_behavior,
            CloseBehavior::Quit
        );
        assert_eq!(read_preferences(&path).unwrap().zoom_factor, 1.0);
        store.set_zoom_factor(1.27).expect("save zoom factor");
        assert_eq!(store.zoom_factor(), 1.25);
        assert_eq!(read_preferences(&path).unwrap().zoom_factor, 1.25);
        fs::write(path.as_path(), b"invalid json").expect("corrupt preference file");
        assert!(read_preferences(&path).is_none());
    }

    #[test]
    fn legacy_close_only_preferences_keep_the_default_zoom() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join(FILE_NAME);
        fs::write(&path, br#"{"closeBehavior":"quit"}"#).expect("write legacy preferences");
        let saved = read_preferences(&path).expect("read legacy preferences");
        assert_eq!(saved.close_behavior, CloseBehavior::Quit);
        assert_eq!(saved.zoom_factor, 1.0);
    }

    #[test]
    fn failed_save_keeps_the_previous_runtime_choice() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let store = HostPreferences {
            path: directory.path().join("missing").join(FILE_NAME),
            preferences: Mutex::new(SavedPreferences::default()),
        };
        assert!(store.set_close_behavior(CloseBehavior::Quit).is_err());
        assert_eq!(store.close_behavior(), CloseBehavior::KeepRunning);
    }

    #[test]
    fn invalid_zoom_does_not_change_the_saved_value() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let store = HostPreferences {
            path: directory.path().join(FILE_NAME),
            preferences: Mutex::new(SavedPreferences::default()),
        };
        assert!(store.set_zoom_factor(f64::NAN).is_err());
        assert!(store.set_zoom_factor(2.1).is_err());
        assert_eq!(store.zoom_factor(), 1.0);
    }
}
