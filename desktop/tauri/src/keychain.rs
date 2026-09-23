use keyring::{Entry, Error as KeyringError};
use serde_json::from_str;
use std::collections::HashMap;
use std::fs;
use std::path::Path;
use std::sync::{Arc, Mutex};
use tauri::Manager;

const SERVICE_NAME: &str = "com.reasonix.desktop";
const LEGACY_FILE_NAME: &str = "keychain.dat";

trait CredentialBackend: Send + Sync {
    fn save(&self, key: &str, value: &str) -> Result<(), String>;
    fn load(&self, key: &str) -> Result<Option<String>, String>;
    fn delete(&self, key: &str) -> Result<bool, String>;
}

struct PlatformCredentialBackend;

impl PlatformCredentialBackend {
    fn entry(key: &str) -> Result<Entry, String> {
        if key.trim().is_empty() {
            return Err("Keychain key must not be empty".to_string());
        }

        Entry::new(SERVICE_NAME, key).map_err(|e| format!("Failed to open keychain entry: {e}"))
    }
}

impl CredentialBackend for PlatformCredentialBackend {
    fn save(&self, key: &str, value: &str) -> Result<(), String> {
        Self::entry(key)?
            .set_password(value)
            .map_err(|e| format!("Failed to save keychain entry: {e}"))
    }

    fn load(&self, key: &str) -> Result<Option<String>, String> {
        match Self::entry(key)?.get_password() {
            Ok(value) => Ok(Some(value)),
            Err(KeyringError::NoEntry) => Ok(None),
            Err(e) => Err(format!("Failed to load keychain entry: {e}")),
        }
    }

    fn delete(&self, key: &str) -> Result<bool, String> {
        match Self::entry(key)?.delete_credential() {
            Ok(()) => Ok(true),
            Err(KeyringError::NoEntry) => Ok(false),
            Err(e) => Err(format!("Failed to delete keychain entry: {e}")),
        }
    }
}

/// Access to the operating system's secure credential store.
///
/// Values are deliberately not cached or copied into an app-owned file. The
/// platform backend is responsible for protecting them at rest:
/// macOS Keychain, Windows Credential Manager, or Linux Secret Service.
pub struct KeychainStore {
    backend: Arc<dyn CredentialBackend>,
}

impl KeychainStore {
    pub fn new() -> Self {
        Self {
            backend: Arc::new(PlatformCredentialBackend),
        }
    }

    #[cfg(test)]
    fn with_backend(backend: Arc<dyn CredentialBackend>) -> Self {
        Self { backend }
    }

    pub fn initialize(&self, app: &tauri::AppHandle) -> Result<(), String> {
        let data_dir = app
            .path()
            .app_data_dir()
            .map_err(|e| format!("Failed to get app data dir: {e}"))?;
        let legacy_path = data_dir.join(LEGACY_FILE_NAME);

        if legacy_path.is_file() {
            self.migrate_legacy_file(&legacy_path)?;
            fs::remove_file(&legacy_path)
                .map_err(|e| format!("Failed to remove legacy keychain file: {e}"))?;
        }

        Ok(())
    }

    fn migrate_legacy_file(&self, path: &Path) -> Result<(), String> {
        let content = fs::read_to_string(path)
            .map_err(|e| format!("Failed to read legacy keychain file: {e}"))?;
        let entries: HashMap<String, String> =
            from_str(&content).map_err(|e| format!("Failed to parse legacy keychain file: {e}"))?;

        for (key, value) in entries {
            self.save_secret(&key, &value)?;
        }

        Ok(())
    }

    pub fn save_secret(&self, key: &str, value: &str) -> Result<(), String> {
        self.backend.save(key, value)
    }

    pub fn load_secret(&self, key: &str) -> Result<Option<String>, String> {
        self.backend.load(key)
    }

    pub fn delete_secret(&self, key: &str) -> Result<bool, String> {
        self.backend.delete(key)
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

    #[derive(Default)]
    struct MemoryCredentialBackend {
        values: Mutex<HashMap<String, String>>,
    }

    impl CredentialBackend for MemoryCredentialBackend {
        fn save(&self, key: &str, value: &str) -> Result<(), String> {
            if key.trim().is_empty() {
                return Err("Keychain key must not be empty".to_string());
            }
            self.values
                .lock()
                .map_err(|_| "Memory backend lock poisoned".to_string())?
                .insert(key.to_string(), value.to_string());
            Ok(())
        }

        fn load(&self, key: &str) -> Result<Option<String>, String> {
            if key.trim().is_empty() {
                return Err("Keychain key must not be empty".to_string());
            }
            Ok(self
                .values
                .lock()
                .map_err(|_| "Memory backend lock poisoned".to_string())?
                .get(key)
                .cloned())
        }

        fn delete(&self, key: &str) -> Result<bool, String> {
            if key.trim().is_empty() {
                return Err("Keychain key must not be empty".to_string());
            }
            Ok(self
                .values
                .lock()
                .map_err(|_| "Memory backend lock poisoned".to_string())?
                .remove(key)
                .is_some())
        }
    }

    fn test_store() -> KeychainStore {
        KeychainStore::with_backend(Arc::new(MemoryCredentialBackend::default()))
    }

    #[test]
    fn save_and_load_secret() {
        let store = test_store();
        store.save_secret("test-key", "test-value").expect("save");
        assert_eq!(
            store.load_secret("test-key").expect("load"),
            Some("test-value".to_string())
        );
    }

    #[test]
    fn delete_secret() {
        let store = test_store();
        store.save_secret("key", "value").expect("save");
        assert!(store.delete_secret("key").expect("delete"));
        assert_eq!(store.load_secret("key").expect("load"), None);
        assert!(!store.delete_secret("key").expect("delete missing"));
    }

    #[test]
    fn overwrite_secret() {
        let store = test_store();
        store.save_secret("key", "v1").expect("save v1");
        store.save_secret("key", "v2").expect("save v2");
        assert_eq!(
            store.load_secret("key").expect("load"),
            Some("v2".to_string())
        );
    }

    #[test]
    fn load_nonexistent_returns_none() {
        assert_eq!(test_store().load_secret("missing").expect("load"), None);
    }

    #[test]
    fn rejects_empty_keys() {
        let store = test_store();
        assert!(store.save_secret(" ", "value").is_err());
        assert!(store.load_secret("").is_err());
        assert!(store.delete_secret("").is_err());
    }

    #[test]
    fn migrates_legacy_json_entries() {
        let store = test_store();
        let dir = tempdir().expect("temp dir");
        let path = dir.path().join(LEGACY_FILE_NAME);
        fs::write(&path, r#"{"legacy-key":"legacy-value"}"#).expect("legacy file");

        store.migrate_legacy_file(&path).expect("migrate");

        assert_eq!(
            store
                .load_secret("legacy-key")
                .expect("load migrated value"),
            Some("legacy-value".to_string())
        );
    }
}
