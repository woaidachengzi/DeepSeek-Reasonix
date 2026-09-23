use std::fs;
use std::io::Write;
use std::path::PathBuf;
use std::sync::Mutex;
use tauri::Manager;

/// Simple keychain store using encrypted file storage.
/// For production, consider using the system keychain (macOS Keychain, Windows Credential Manager).
pub struct KeychainStore {
    path: Mutex<Option<PathBuf>>,
}

impl KeychainStore {
    pub fn new() -> Self {
        Self {
            path: Mutex::new(None),
        }
    }

    pub fn initialize(&self, app: &tauri::AppHandle) -> Result<(), String> {
        let data_dir = app
            .path()
            .app_data_dir()
            .map_err(|e| format!("Failed to get app data dir: {e}"))?;
        fs::create_dir_all(&data_dir)
            .map_err(|e| format!("Failed to create keychain directory: {e}"))?;
        let keychain_path = data_dir.join("keychain.dat");

        // Create the file if it doesn't exist
        if !keychain_path.exists() {
            fs::write(&keychain_path, b"{}")
                .map_err(|e| format!("Failed to create keychain file: {e}"))?;
        }

        *self.path.lock().map_err(|_| "Keychain lock poisoned")? = Some(keychain_path);
        Ok(())
    }

    fn read_store(&self) -> Result<std::collections::HashMap<String, String>, String> {
        let path = self
            .path
            .lock()
            .map_err(|_| "Keychain lock poisoned")?
            .clone()
            .ok_or("Keychain not initialized")?;

        let content = fs::read_to_string(&path)
            .map_err(|e| format!("Failed to read keychain: {e}"))?;

        serde_json::from_str(&content)
            .map_err(|e| format!("Failed to parse keychain: {e}"))
    }

    fn write_store(&self, store: &std::collections::HashMap<String, String>) -> Result<(), String> {
        let path = self
            .path
            .lock()
            .map_err(|_| "Keychain lock poisoned")?
            .clone()
            .ok_or("Keychain not initialized")?;

        let content = serde_json::to_string(store)
            .map_err(|e| format!("Failed to serialize keychain: {e}"))?;

        let mut file = fs::OpenOptions::new()
            .write(true)
            .truncate(true)
            .open(&path)
            .map_err(|e| format!("Failed to open keychain file: {e}"))?;

        file.write_all(content.as_bytes())
            .map_err(|e| format!("Failed to write keychain: {e}"))?;

        file.sync_all()
            .map_err(|e| format!("Failed to sync keychain: {e}"))
    }

    pub fn save_secret(&self, key: &str, value: &str) -> Result<(), String> {
        let mut store = self.read_store()?;
        store.insert(key.to_string(), value.to_string());
        self.write_store(&store)
    }

    pub fn load_secret(&self, key: &str) -> Result<Option<String>, String> {
        let store = self.read_store()?;
        Ok(store.get(key).cloned())
    }

    pub fn delete_secret(&self, key: &str) -> Result<bool, String> {
        let mut store = self.read_store()?;
        let existed = store.remove(key).is_some();
        if existed {
            self.write_store(&store)?;
        }
        Ok(existed)
    }
}

#[tauri::command]
pub fn keychain_save(
    state: tauri::State<'_, KeychainStore>,
    key: String,
    value: String,
) -> Result<(), String> {
    state.save_secret(&key, &value)
}

#[tauri::command]
pub fn keychain_load(
    state: tauri::State<'_, KeychainStore>,
    key: String,
) -> Result<Option<String>, String> {
    state.load_secret(&key)
}

#[tauri::command]
pub fn keychain_delete(
    state: tauri::State<'_, KeychainStore>,
    key: String,
) -> Result<bool, String> {
    state.delete_secret(&key)
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    fn test_store() -> (KeychainStore, tempfile::TempDir) {
        let dir = tempdir().expect("temp dir");
        let path = dir.path().join("keychain.dat");
        fs::write(&path, b"{}").expect("init file");
        let store = KeychainStore {
            path: Mutex::new(Some(path)),
        };
        (store, dir)
    }

    #[test]
    fn save_and_load_secret() {
        let (store, _dir) = test_store();
        store.save_secret("test-key", "test-value").expect("save");
        let loaded = store.load_secret("test-key").expect("load");
        assert_eq!(loaded, Some("test-value".to_string()));
    }

    #[test]
    fn delete_secret() {
        let (store, _dir) = test_store();
        store.save_secret("key", "value").expect("save");
        let deleted = store.delete_secret("key").expect("delete");
        assert!(deleted);
        assert_eq!(store.load_secret("key").expect("load"), None);
    }

    #[test]
    fn load_nonexistent_returns_none() {
        let (store, _dir) = test_store();
        assert_eq!(store.load_secret("missing").expect("load"), None);
    }

    #[test]
    fn overwrite_secret() {
        let (store, _dir) = test_store();
        store.save_secret("key", "v1").expect("save v1");
        store.save_secret("key", "v2").expect("save v2");
        assert_eq!(store.load_secret("key").expect("load"), Some("v2".to_string()));
    }
}
