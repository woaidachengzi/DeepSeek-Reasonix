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
    close_behavior: Mutex<CloseBehavior>,
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct SavedPreferences {
    close_behavior: CloseBehavior,
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
        let close_behavior = read_preferences(&path).unwrap_or_default();
        Ok(Self {
            path,
            close_behavior: Mutex::new(close_behavior),
        })
    }

    pub fn close_behavior(&self) -> CloseBehavior {
        *self
            .close_behavior
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
    }

    pub fn set_close_behavior(&self, behavior: CloseBehavior) -> Result<(), String> {
        let mut current = self
            .close_behavior
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        if *current == behavior {
            return Ok(());
        }
        let data = serde_json::to_vec(&SavedPreferences {
            close_behavior: behavior,
        })
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
        *current = behavior;
        Ok(())
    }
}

fn read_preferences(path: &std::path::Path) -> Option<CloseBehavior> {
    let data = fs::read(path).ok()?;
    serde_json::from_slice::<SavedPreferences>(&data)
        .ok()
        .map(|saved| saved.close_behavior)
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
            close_behavior: Mutex::new(CloseBehavior::KeepRunning),
        };
        store
            .set_close_behavior(CloseBehavior::Quit)
            .expect("save close behavior");
        assert_eq!(store.close_behavior(), CloseBehavior::Quit);
        assert_eq!(read_preferences(&path), Some(CloseBehavior::Quit));
        fs::write(path.as_path(), b"invalid json").expect("corrupt preference file");
        assert_eq!(read_preferences(&path), None);
    }

    #[test]
    fn failed_save_keeps_the_previous_runtime_choice() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let store = HostPreferences {
            path: directory.path().join("missing").join(FILE_NAME),
            close_behavior: Mutex::new(CloseBehavior::KeepRunning),
        };
        assert!(store.set_close_behavior(CloseBehavior::Quit).is_err());
        assert_eq!(store.close_behavior(), CloseBehavior::KeepRunning);
    }
}
