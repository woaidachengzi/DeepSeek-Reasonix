use keyring::{Entry, Error as KeyringError};
use serde_json::from_str;
use std::collections::HashMap;
use std::fs;
use std::path::Path;
use std::sync::{Arc, Mutex};
use tauri::Manager;

use crate::bridge::{BridgeStatus, BridgeSupervisor};

const SERVICE_NAME: &str = "com.reasonix.desktop";
const LEGACY_FILE_NAME: &str = "keychain.dat";
const PROVIDER_API_KEY_PREFIX: &str = "api_key_";

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
    provider_sync: Mutex<()>,
}

impl KeychainStore {
    pub fn new() -> Self {
        Self {
            backend: Arc::new(PlatformCredentialBackend),
            provider_sync: Mutex::new(()),
        }
    }

    #[cfg(test)]
    fn with_backend(backend: Arc<dyn CredentialBackend>) -> Self {
        Self {
            backend,
            provider_sync: Mutex::new(()),
        }
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

    /// Restore provider keys after the bridge starts. The native credential
    /// store remains the source of truth; the bridge retains values only for
    /// its current lifetime so core sessions can resolve them without .env.
    pub fn restore_provider_api_keys(&self, supervisor: &BridgeSupervisor) -> Result<(), String> {
        let _guard = self
            .provider_sync
            .lock()
            .map_err(|_| "provider credential lock is unavailable")?;
        self.restore_provider_api_keys_unlocked(supervisor)
    }

    pub fn restart_bridge(&self, supervisor: &BridgeSupervisor) -> Result<BridgeStatus, String> {
        let _guard = self
            .provider_sync
            .lock()
            .map_err(|_| "provider credential lock is unavailable")?;
        let status = supervisor.restart()?;
        self.restore_provider_api_keys_unlocked(supervisor)?;
        Ok(status)
    }

    fn restore_provider_api_keys_unlocked(
        &self,
        supervisor: &BridgeSupervisor,
    ) -> Result<(), String> {
        let summary = supervisor.provider_summary()?;
        for provider in summary.providers {
            if !provider.requires_key {
                continue;
            }
            let key = format!("{PROVIDER_API_KEY_PREFIX}{}", provider.name);
            if let Some(value) = self.load_secret(&key)? {
                supervisor.set_provider_key(&provider.name, Some(&value))?;
            }
        }
        Ok(())
    }

    /// The keychain is authoritative. A bridge update can fail after the
    /// sidecar accepted it but before its response reached us, so both stores
    /// are reconciled to the previous value on an uncertain outcome.
    fn mutate_secret_with_sync<F>(
        &self,
        key: &str,
        value: Option<&str>,
        sync: F,
    ) -> Result<bool, String>
    where
        F: Fn(&str, Option<&str>) -> Result<(), String>,
    {
        let provider_name = provider_name_for_key(key)?;
        if provider_name.is_some()
            && value.is_some_and(|v| v.trim().is_empty() || v.len() > 32 << 10)
        {
            return Err("provider API key is invalid".to_string());
        }
        let _guard = self
            .provider_sync
            .lock()
            .map_err(|_| "provider credential lock is unavailable")?;
        let previous = self.load_secret(key)?;
        let deleted = match value {
            Some(value) => {
                self.save_secret(key, value)?;
                false
            }
            None => self.delete_secret(key)?,
        };
        if let Some(provider_name) = provider_name {
            if let Err(error) = sync(provider_name, value) {
                restore_secret(self, key, previous.as_deref()).map_err(|_| {
                    "provider key synchronization failed and keychain rollback could not be confirmed"
                        .to_string()
                })?;
                // A timed-out response can still have applied in the bridge.
                // Reconcile its memory with the restored keychain when reachable.
                if sync(provider_name, previous.as_deref()).is_err() {
                    return Err(format!(
                        "{error}; bridge credential recovery could not be confirmed; restart the bridge"
                    ));
                }
                return Err(error);
            }
        }
        Ok(deleted)
    }

    pub fn save_and_sync_provider_key(
        &self,
        supervisor: &BridgeSupervisor,
        key: &str,
        value: &str,
    ) -> Result<(), String> {
        self.mutate_secret_with_sync(key, Some(value), |name, value| {
            supervisor.set_provider_key(name, value).map(|_| ())
        })
        .map(|_| ())
    }

    pub fn delete_and_sync_provider_key(
        &self,
        supervisor: &BridgeSupervisor,
        key: &str,
    ) -> Result<bool, String> {
        self.mutate_secret_with_sync(key, None, |name, value| {
            supervisor.set_provider_key(name, value).map(|_| ())
        })
    }
}

fn provider_name_for_key(key: &str) -> Result<Option<&str>, String> {
    let Some(provider_name) = key.strip_prefix(PROVIDER_API_KEY_PREFIX) else {
        return Ok(None);
    };
    if provider_name.is_empty() || provider_name.trim() != provider_name {
        return Err("provider key name is invalid".to_string());
    }
    Ok(Some(provider_name))
}

fn restore_secret(store: &KeychainStore, key: &str, previous: Option<&str>) -> Result<(), String> {
    match previous {
        Some(value) => store.save_secret(key, value),
        None => store.delete_secret(key).map(|_| ()),
    }
}

#[tauri::command]
pub fn keychain_save(
    state: tauri::State<'_, KeychainStore>,
    supervisor: tauri::State<'_, BridgeSupervisor>,
    key: String,
    value: String,
) -> Result<(), String> {
    state.save_and_sync_provider_key(&supervisor, &key, &value)
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
    supervisor: tauri::State<'_, BridgeSupervisor>,
    key: String,
) -> Result<bool, String> {
    state.delete_and_sync_provider_key(&supervisor, &key)
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

    #[test]
    fn recognizes_only_named_provider_api_keys() {
        assert_eq!(
            provider_name_for_key("api_key_deepseek").unwrap(),
            Some("deepseek")
        );
        assert!(provider_name_for_key("api_key_ ").is_err());
        assert_eq!(provider_name_for_key("other_key_deepseek").unwrap(), None);
    }

    #[test]
    fn failed_bridge_response_reconciles_both_copies() {
        let store = test_store();
        store
            .save_secret("api_key_deepseek", "old-secret")
            .expect("seed");
        let bridge_value = Mutex::new(Some("old-secret".to_string()));
        let calls = Mutex::new(0);
        let result =
            store.mutate_secret_with_sync("api_key_deepseek", Some("new-secret"), |name, value| {
                assert_eq!(name, "deepseek");
                *bridge_value.lock().expect("bridge lock") = value.map(str::to_string);
                let mut calls = calls.lock().expect("calls lock");
                *calls += 1;
                if *calls == 1 {
                    return Err("bridge response was lost".to_string());
                }
                Ok(())
            });
        assert!(result.is_err());
        assert_eq!(*calls.lock().expect("calls lock"), 2);
        assert_eq!(
            bridge_value.lock().expect("bridge lock").as_deref(),
            Some("old-secret")
        );
        assert_eq!(
            store.load_secret("api_key_deepseek").expect("load"),
            Some("old-secret".to_string())
        );
    }
}
