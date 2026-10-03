use keyring::{Entry, Error as KeyringError};
use serde::de::{MapAccess, Visitor};
use serde::{Deserialize, Deserializer};
use std::collections::HashMap;
use std::fmt;
use std::fs;
use std::io::Read;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};
use tauri::Manager;

use crate::bridge::{BridgeStatus, BridgeSupervisor};

const LEGACY_SERVICE_NAME: &str = "com.reasonix.desktop";
const WAILS_SERVICE_NAME: &str = "reasonix";
const STORAGE_ERROR: &str =
    "system credential storage is unavailable; unlock it or check application access permissions";
const LEGACY_FILE_NAME: &str = "keychain.dat";
const MAX_LEGACY_FILE_BYTES: u64 = 1024 * 1024;
const MAX_LEGACY_ENTRIES: usize = 256;
const PROVIDER_API_KEY_PREFIX: &str = "api_key_";

trait CredentialBackend: Send + Sync {
    fn availability_error(&self) -> Option<&str> {
        None
    }
    fn save(&self, key: &str, value: &str) -> Result<(), String>;
    fn load(&self, key: &str) -> Result<Option<String>, String>;
    fn delete(&self, key: &str) -> Result<bool, String>;
}

struct PlatformCredentialBackend {
    service: String,
}

impl PlatformCredentialBackend {
    fn entry(&self, key: &str) -> Result<Entry, String> {
        if key.trim().is_empty() {
            return Err("Keychain key must not be empty".to_string());
        }

        Entry::new(&self.service, key).map_err(credential_error)
    }
}

impl CredentialBackend for PlatformCredentialBackend {
    fn save(&self, key: &str, value: &str) -> Result<(), String> {
        self.entry(key)?
            .set_password(value)
            .map_err(credential_error)
    }

    fn load(&self, key: &str) -> Result<Option<String>, String> {
        match self.entry(key)?.get_password() {
            Ok(value) => Ok(Some(value)),
            Err(KeyringError::NoEntry) => Ok(None),
            Err(e) => Err(credential_error(e)),
        }
    }

    fn delete(&self, key: &str) -> Result<bool, String> {
        match self.entry(key)?.delete_credential() {
            Ok(()) => Ok(true),
            Err(KeyringError::NoEntry) => Ok(false),
            Err(e) => Err(credential_error(e)),
        }
    }
}

// Never expose platform errors: Ambiguous embeds debug credentials and a
// platform failure may include sensitive attributes in its diagnostic text.
fn credential_error(error: KeyringError) -> String {
    match error {
        KeyringError::BadEncoding(_) => "credential encoding is invalid; replace the saved credential".into(),
        KeyringError::Ambiguous(_) => "credential identity is ambiguous; resolve duplicate entries in the system credential store".into(),
        KeyringError::TooLong(_, _) | KeyringError::Invalid(_, _) => "credential attributes are invalid; check the provider name and key".into(),
        _ => STORAGE_ERROR.into(),
    }
}

struct UnavailableCredentialBackend(String);
impl CredentialBackend for UnavailableCredentialBackend {
    fn availability_error(&self) -> Option<&str> {
        Some(&self.0)
    }
    fn save(&self, _: &str, _: &str) -> Result<(), String> {
        Err(self.0.clone())
    }
    fn load(&self, _: &str) -> Result<Option<String>, String> {
        Err(self.0.clone())
    }
    fn delete(&self, _: &str) -> Result<bool, String> {
        Err(self.0.clone())
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
    legacy_backend: Arc<dyn CredentialBackend>,
    legacy_path: Option<PathBuf>,
}

impl KeychainStore {
    pub fn for_profile(
        profile: &crate::data_profile::PreviewProfile,
        app: &tauri::AppHandle,
    ) -> Self {
        let backend: Arc<dyn CredentialBackend> =
            match crate::credential_namespace::service_for_profile(profile.home()) {
                Ok(service) => Arc::new(PlatformCredentialBackend { service }),
                Err(error) => Arc::new(UnavailableCredentialBackend(error)),
            };
        Self {
            backend,
            provider_sync: Mutex::new(()),
            legacy_backend: Arc::new(PlatformCredentialBackend {
                service: LEGACY_SERVICE_NAME.into(),
            }),
            legacy_path: app
                .path()
                .app_data_dir()
                .ok()
                .map(|path| path.join(LEGACY_FILE_NAME)),
        }
    }

    #[cfg(test)]
    fn with_backend(backend: Arc<dyn CredentialBackend>) -> Self {
        Self {
            backend,
            provider_sync: Mutex::new(()),
            legacy_backend: Arc::new(MemoryCredentialBackendUnavailable),
            legacy_path: None,
        }
    }

    fn read_legacy_file(path: &Path) -> Result<HashMap<String, String>, String> {
        let metadata = match fs::symlink_metadata(path) {
            Ok(metadata) => metadata,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(HashMap::new()),
            Err(_) => {
                return Err("legacy credential file is unavailable; check file permissions".into())
            }
        };
        if metadata.file_type().is_symlink()
            || !metadata.is_file()
            || metadata.len() > MAX_LEGACY_FILE_BYTES
        {
            return Err("Legacy keychain file must be a regular file no larger than 1 MiB".into());
        }
        let mut file = fs::File::open(path)
            .map_err(|_| "legacy credential file is unavailable; check file permissions")?;
        let opened_metadata = file
            .metadata()
            .map_err(|_| "legacy credential file is unavailable; check file permissions")?;
        let current_metadata = fs::symlink_metadata(path)
            .map_err(|_| "legacy credential file is unavailable; check file permissions")?;
        if current_metadata.file_type().is_symlink()
            || !current_metadata.is_file()
            || !opened_metadata.is_file()
            || opened_metadata.len() > MAX_LEGACY_FILE_BYTES
            || !crate::workbench_projects::same_file_as_path(path, &file)
                .map_err(|_| "legacy credential file is unavailable; check file permissions")?
        {
            return Err("Legacy keychain file changed while opening".into());
        }
        let mut content = Vec::new();
        file.by_ref()
            .take(MAX_LEGACY_FILE_BYTES + 1)
            .read_to_end(&mut content)
            .map_err(|_| "legacy credential file is unavailable; check file permissions")?;
        if content.len() as u64 > MAX_LEGACY_FILE_BYTES {
            return Err("Legacy keychain file exceeds 1 MiB".into());
        }
        let entries = serde_json::from_slice::<UniqueLegacyEntries>(&content)
            .map_err(|_| "legacy credential file is invalid; repair it or re-enter the key")?
            .0;
        if entries.len() > MAX_LEGACY_ENTRIES
            || entries.iter().any(|(key, value)| {
                key.trim().is_empty()
                    || key.len() > 256
                    || key.chars().any(char::is_control)
                    || value.len() > 32 << 10
                    || value.contains('\0')
            })
        {
            return Err("Legacy keychain file contains too many or oversized entries".into());
        }

        Ok(entries)
    }

    fn import_legacy_with_sync<F>(&self, key: &str, sync: F) -> Result<(), String>
    where
        F: Fn(&str, Option<&str>) -> Result<(), String>,
    {
        self.import_source_with_sync(key, || {
            let entries = self.legacy_path.as_deref().map(Self::read_legacy_file)
                .transpose()?.unwrap_or_default();
            match entries.get(key) {
                Some(value) => Ok(value.clone()),
                None => self.legacy_backend.load(key)?
                    .ok_or_else(|| "no old Preview credential was found; re-enter the provider key".into()),
            }
        }, sync)
    }

    fn import_source_with_sync<F, L>(&self, key: &str, load_source: L, sync: F) -> Result<(), String>
    where
        F: Fn(&str, Option<&str>) -> Result<(), String>,
        L: FnOnce() -> Result<String, String>,
    {
        let _guard = self
            .provider_sync
            .lock()
            .map_err(|_| "provider credential lock is unavailable")?;
        let provider =
            provider_name_for_key(key)?.ok_or("only provider API keys may be migrated")?;
        if self.load_secret(key)?.is_some() {
            return Err(
                "the profile already has a credential; migration cannot overwrite it".into(),
            );
        }
        let value = load_source()?;
        if value.trim().is_empty() || value.contains('\0') || value.len() > 32 << 10 {
            return Err("legacy provider credential is invalid; re-enter the provider key".into());
        }
        // No old store or file is modified, even after successful migration.
        self.mutate_secret_unlocked(key, Some(&value), |name, value| {
            debug_assert_eq!(name, provider);
            sync(name, value)
        })
        .map(|_| ())
    }

    pub fn import_legacy_provider_key(
        &self,
        supervisor: &BridgeSupervisor,
        provider: &str,
    ) -> Result<(), String> {
        require_provider(supervisor, provider)?;
        self.import_legacy_with_sync(
            &format!("{PROVIDER_API_KEY_PREFIX}{provider}"),
            |name, value| supervisor.set_provider_key(name, value).map(|_| ()),
        )
    }

    pub fn import_wails_provider_key(&self, supervisor: &BridgeSupervisor, provider: &str) -> Result<(), String> {
        require_provider(supervisor, provider)?;
        self.import_source_with_sync(&format!("{PROVIDER_API_KEY_PREFIX}{provider}"), || {
            let account = supervisor.provider_credential_account(provider)?;
            PlatformCredentialBackend { service: WAILS_SERVICE_NAME.into() }.load(&account)?
                .ok_or_else(|| "no Wails keyring credential was found; re-enter the provider key".into())
        }, |name, value| supervisor.set_provider_key(name, value).map(|_| ()))
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
        // A missing/corrupt profile identity disables the whole backend. Keep
        // its metadata recovery instruction instead of treating it as one
        // bad entry or making unnecessary sidecar requests.
        if let Some(error) = self.backend.availability_error() {
            return Err(error.into());
        }
        let summary = supervisor.provider_summary()?;
        let mut incomplete = false;
        for provider in summary.providers {
            if !provider.requires_key {
                continue;
            }
            let key = format!("{PROVIDER_API_KEY_PREFIX}{}", provider.name);
            match self.load_secret(&key) {
                Ok(Some(value)) => {
                    if supervisor
                        .set_provider_key(&provider.name, Some(&value))
                        .is_err()
                    {
                        incomplete = true;
                    }
                }
                Ok(None) => {}
                Err(_) => incomplete = true,
            }
        }
        if incomplete {
            // One unavailable entry must not disable other providers. Keep
            // the store authoritative and report partial restoration without
            // exposing provider names, values or backend diagnostics.
            Err("some provider credentials could not be restored; unlock credential storage or check application access, then restart the bridge".into())
        } else {
            Ok(())
        }
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
        let _guard = self
            .provider_sync
            .lock()
            .map_err(|_| "provider credential lock is unavailable")?;
        self.mutate_secret_unlocked(key, value, sync)
    }

    fn mutate_secret_unlocked<F>(
        &self,
        key: &str,
        value: Option<&str>,
        sync: F,
    ) -> Result<bool, String>
    where
        F: Fn(&str, Option<&str>) -> Result<(), String>,
    {
        let provider_name = provider_name_for_key(key)?
            .ok_or_else(|| "only provider API keys may be changed from the window".to_string())?;
        if value.is_some_and(|v| v.trim().is_empty() || v.contains('\0') || v.len() > 32 << 10) {
            return Err("provider API key is invalid".to_string());
        }
        let previous = self.load_secret(key)?;
        let written = match value {
            Some(value) => self.save_secret(key, value).map(|_| false),
            None => self.delete_secret(key),
        };
        let deleted = match written {
            Ok(deleted) => deleted,
            Err(_) => {
                restore_secret(self, key, previous.as_deref()).map_err(|_| "credential write failed and rollback could not be confirmed; unlock credential storage and retry")?;
                return Err("credential write failed; unlock credential storage or check application access".into());
            }
        };
        if sync(provider_name, value).is_err() {
            restore_secret(self, key, previous.as_deref()).map_err(|_| {
                "provider key synchronization failed and keychain rollback could not be confirmed"
                    .to_string()
            })?;
            // A timed-out response can still have applied in the bridge.
            // Reconcile its memory with the restored keychain when reachable.
            if sync(provider_name, previous.as_deref()).is_err() {
                return Err(
                    "bridge credential recovery could not be confirmed; restart the bridge".into(),
                );
            }
            return Err(
                "provider credential synchronization failed; restart the bridge and retry".into(),
            );
        }
        Ok(deleted)
    }

    pub fn save_and_sync_provider_key(
        &self,
        supervisor: &BridgeSupervisor,
        key: &str,
        value: &str,
    ) -> Result<(), String> {
        let name = provider_name_for_key(key)?.ok_or("only provider API keys may be changed")?;
        require_provider(supervisor, name)?;
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
        let name = provider_name_for_key(key)?.ok_or("only provider API keys may be changed")?;
        require_provider(supervisor, name)?;
        self.mutate_secret_with_sync(key, None, |name, value| {
            supervisor.set_provider_key(name, value).map(|_| ())
        })
    }
}

fn provider_name_for_key(key: &str) -> Result<Option<&str>, String> {
    let Some(provider_name) = key.strip_prefix(PROVIDER_API_KEY_PREFIX) else {
        return Ok(None);
    };
    if provider_name.is_empty()
        || key.len() > 256
        || provider_name.trim() != provider_name
        || provider_name.chars().any(char::is_control)
        || provider_name.contains(['/', '\\'])
    {
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
pub async fn keychain_save(
    window: tauri::WebviewWindow,
    key: String,
    value: String,
) -> Result<(), String> {
    credential_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        window.state::<KeychainStore>().save_and_sync_provider_key(
            &window.state::<BridgeSupervisor>(),
            &key,
            &value,
        )
    })
    .await
    .map_err(|_| "credential operation failed; retry saving")?
}
#[tauri::command]
pub async fn keychain_delete(window: tauri::WebviewWindow, key: String) -> Result<bool, String> {
    credential_window(window.label())?;
    tauri::async_runtime::spawn_blocking(move || {
        window
            .state::<KeychainStore>()
            .delete_and_sync_provider_key(&window.state::<BridgeSupervisor>(), &key)
    })
    .await
    .map_err(|_| "credential operation failed; retry deleting")?
}
// Public migration failures carry only fixed codes. Platform diagnostics and
// credential attributes must never become UI text or serialized error fields.
#[derive(Debug, serde::Serialize, PartialEq)]
#[serde(tag = "code", rename_all = "snake_case")]
pub enum CredentialImportError {
    ExistingCredential,
    MissingLegacyCredential,
    MissingWailsCredential,
    Unavailable,
}

impl From<String> for CredentialImportError {
    fn from(error: String) -> Self {
        match error.as_str() {
            "the profile already has a credential; migration cannot overwrite it" => {
                Self::ExistingCredential
            }
            "no old Preview credential was found; re-enter the provider key" => {
                Self::MissingLegacyCredential
            }
            "no Wails keyring credential was found; re-enter the provider key" => Self::MissingWailsCredential,
            _ => Self::Unavailable,
        }
    }
}

#[tauri::command]
pub async fn keychain_import_legacy(
    window: tauri::WebviewWindow,
    provider: String,
    source: Option<String>,
) -> Result<(), CredentialImportError> {
    credential_window(window.label()).map_err(CredentialImportError::from)?;
    tauri::async_runtime::spawn_blocking(move || {
        let store = window.state::<KeychainStore>();
        let supervisor = window.state::<BridgeSupervisor>();
        match source.as_deref() {
            None | Some("preview") => store.import_legacy_provider_key(&supervisor, &provider),
            Some("wails") => store.import_wails_provider_key(&supervisor, &provider),
            _ => Err("unsupported credential import source".into()),
        }.map_err(CredentialImportError::from)
    })
    .await
    .map_err(|_| CredentialImportError::Unavailable)?
}
fn credential_window(label: &str) -> Result<(), String> {
    if label != "main" {
        return Err("credential operations require the main window".into());
    }
    Ok(())
}
fn require_provider(supervisor: &BridgeSupervisor, provider: &str) -> Result<(), String> {
    provider_name_for_key(&format!("{PROVIDER_API_KEY_PREFIX}{provider}"))?;
    if !supervisor
        .provider_summary()?
        .providers
        .iter()
        .any(|item| item.name == provider && item.requires_key)
    {
        return Err(
            "provider is unavailable or does not require a credential; reload provider settings"
                .into(),
        );
    }
    Ok(())
}

struct UniqueLegacyEntries(HashMap<String, String>);
impl<'de> Deserialize<'de> for UniqueLegacyEntries {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        struct EntriesVisitor;
        impl<'de> Visitor<'de> for EntriesVisitor {
            type Value = UniqueLegacyEntries;
            fn expecting(&self, formatter: &mut fmt::Formatter) -> fmt::Result {
                formatter.write_str("unique bounded credential entries")
            }
            fn visit_map<A: MapAccess<'de>>(self, mut map: A) -> Result<Self::Value, A::Error> {
                let mut entries = HashMap::new();
                while let Some((key, value)) = map.next_entry::<String, String>()? {
                    if entries.len() >= MAX_LEGACY_ENTRIES || entries.insert(key, value).is_some() {
                        return Err(serde::de::Error::custom("invalid credential entries"));
                    }
                }
                Ok(UniqueLegacyEntries(entries))
            }
        }
        deserializer.deserialize_map(EntriesVisitor)
    }
}

#[cfg(test)]
struct MemoryCredentialBackendUnavailable;
#[cfg(test)]
impl CredentialBackend for MemoryCredentialBackendUnavailable {
    fn save(&self, _: &str, _: &str) -> Result<(), String> {
        unreachable!()
    }
    fn load(&self, _: &str) -> Result<Option<String>, String> {
        Ok(None)
    }
    fn delete(&self, _: &str) -> Result<bool, String> {
        unreachable!()
    }
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
    fn migration_failure_codes_are_precise_and_redacted() {
        for (message, code) in [
            (
                "the profile already has a credential; migration cannot overwrite it",
                "existing_credential",
            ),
            (
                "no old Preview credential was found; re-enter the provider key",
                "missing_legacy_credential",
            ),
            ("no Wails keyring credential was found; re-enter the provider key", "missing_wails_credential"),
            ("platform diagnostic with private-token", "unavailable"),
            (
                "credential operations require the main window",
                "unavailable",
            ),
            (
                "the profile already has a credential; migration cannot overwrite it private-token",
                "unavailable",
            ),
        ] {
            assert_eq!(
                serde_json::to_value(CredentialImportError::from(message.to_string())).unwrap(),
                serde_json::json!({"code": code})
            );
        }
    }

    #[test]
    fn explicit_source_import_refuses_existing_target_before_reading_source_and_preserves_source_on_rollback() {
        let store = test_store();
        let source = MemoryCredentialBackend::default();
        source.save("SHARED_WAILS_KEY", "fake-wails-secret").unwrap();
        store.save_secret("api_key_demo", "existing-preview").unwrap();
        assert!(store.import_source_with_sync("api_key_demo", || panic!("existing target must not read Wails"), |_, _| Ok(())).is_err());
        store.delete_secret("api_key_demo").unwrap();
        assert!(store.import_source_with_sync("api_key_demo", || source.load("SHARED_WAILS_KEY").map(|v| v.unwrap()), |_, _| Err("offline".into())).is_err());
        assert_eq!(store.load_secret("api_key_demo").unwrap(), None);
        assert_eq!(source.load("SHARED_WAILS_KEY").unwrap().as_deref(), Some("fake-wails-secret"));
        store.import_source_with_sync("api_key_demo", || source.load("SHARED_WAILS_KEY").map(|v| v.unwrap()), |name, value| {
            assert_eq!(name, "demo");
            assert_eq!(value, Some("fake-wails-secret"));
            Ok(())
        }).unwrap();
        assert_eq!(store.load_secret("api_key_demo").unwrap().as_deref(), Some("fake-wails-secret"));
        assert_eq!(source.load("SHARED_WAILS_KEY").unwrap().as_deref(), Some("fake-wails-secret"));
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
        let mut store = store;
        store.legacy_path = Some(path.clone());
        fs::write(&path, r#"{"api_key_demo":"legacy-value"}"#).unwrap();
        store
            .import_legacy_with_sync("api_key_demo", |_, _| Ok(()))
            .expect("migrate");

        assert_eq!(
            store
                .load_secret("api_key_demo")
                .expect("load migrated value"),
            Some("legacy-value".to_string())
        );
        assert!(
            path.exists(),
            "explicit migration must preserve the old profile source"
        );
    }

    #[test]
    fn missing_legacy_file_does_not_block_first_launch() {
        let dir = tempdir().expect("temp dir");
        KeychainStore::read_legacy_file(&dir.path().join(LEGACY_FILE_NAME))
            .expect("fresh Preview profile has no legacy keychain file");
    }

    #[test]
    fn rejects_oversized_legacy_file_without_importing_any_keys() {
        let dir = tempdir().expect("temp dir");
        let path = dir.path().join(LEGACY_FILE_NAME);
        fs::File::create(&path)
            .expect("create legacy file")
            .set_len(MAX_LEGACY_FILE_BYTES + 1)
            .expect("make oversized sparse file");

        assert!(KeychainStore::read_legacy_file(&path).is_err());
        assert!(path.exists(), "failed migration must preserve its source");
    }

    #[test]
    fn validates_every_legacy_entry_before_writing_credentials() {
        let store = test_store();
        let dir = tempdir().expect("temp dir");
        let path = dir.path().join(LEGACY_FILE_NAME);
        let content = serde_json::json!({
            "api_key_valid": "valid-secret",
            "api_key_oversized": "x".repeat((32 << 10) + 1),
        });
        fs::write(
            &path,
            serde_json::to_vec(&content).expect("serialize legacy file"),
        )
        .expect("write legacy file");

        assert!(KeychainStore::read_legacy_file(&path).is_err());
        assert_eq!(store.load_secret("api_key_valid").expect("read key"), None);
        assert!(path.exists(), "failed migration must preserve its source");
    }

    #[cfg(unix)]
    #[test]
    fn rejects_symlinked_legacy_file_without_importing_target() {
        use std::os::unix::fs::symlink;

        let store = test_store();
        let dir = tempdir().expect("temp dir");
        let target = dir.path().join("other-secrets.json");
        let path = dir.path().join(LEGACY_FILE_NAME);
        fs::write(&target, r#"{"api_key_other":"unrelated-secret"}"#)
            .expect("write unrelated file");
        symlink(&target, &path).expect("create legacy symlink");

        assert!(KeychainStore::read_legacy_file(&path).is_err());
        assert_eq!(store.load_secret("api_key_other").expect("read key"), None);
        assert!(target.exists(), "symlink target must remain untouched");
    }

    #[test]
    fn recognizes_only_named_provider_api_keys() {
        assert_eq!(
            provider_name_for_key("api_key_deepseek").unwrap(),
            Some("deepseek")
        );
        assert!(provider_name_for_key("api_key_ ").is_err());
        assert_eq!(provider_name_for_key("other_key_deepseek").unwrap(), None);
        for name in ["商汤", "通义千问", "Local Gateway", "9router"] {
            assert_eq!(
                provider_name_for_key(&format!("api_key_{name}")).unwrap(),
                Some(name),
                "imported Wails provider identities remain compatible"
            );
        }
    }

    #[test]
    fn window_cannot_change_unrelated_keychain_entries() {
        let store = test_store();
        let sync = |_: &str, _: Option<&str>| -> Result<(), String> {
            panic!("unrelated key must never reach bridge sync")
        };
        assert!(store
            .mutate_secret_with_sync("other_key_deepseek", Some("secret"), sync)
            .is_err());
        assert!(store
            .mutate_secret_with_sync("other_key_deepseek", None, sync)
            .is_err());
        assert_eq!(store.load_secret("other_key_deepseek").unwrap(), None);
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

    #[test]
    fn explicit_import_keeps_legacy_store_and_refuses_overwrite() {
        let old = Arc::new(MemoryCredentialBackend::default());
        old.save("api_key_demo", "old-secret").unwrap();
        old.save("api_key_other", "other-secret").unwrap();
        let mut store = test_store();
        store.legacy_backend = old.clone();
        store
            .import_legacy_with_sync("api_key_demo", |name, value| {
                assert_eq!(name, "demo");
                assert_eq!(value, Some("old-secret"));
                Ok(())
            })
            .unwrap();
        assert_eq!(
            old.load("api_key_demo").unwrap().as_deref(),
            Some("old-secret")
        );
        assert_eq!(store.load_secret("api_key_other").unwrap(), None);
        store.save_secret("api_key_demo", "user-secret").unwrap();
        assert!(store
            .import_legacy_with_sync("api_key_demo", |_, _| panic!("no overwrite"))
            .is_err());
        assert_eq!(
            store.load_secret("api_key_demo").unwrap().as_deref(),
            Some("user-secret")
        );
    }

    #[test]
    fn malformed_legacy_file_and_missing_or_denied_legacy_store_leave_target_empty() {
        let dir = tempdir().unwrap();
        let path = dir.path().join(LEGACY_FILE_NAME);
        let mut store = test_store();
        store.legacy_path = Some(path.clone());
        for json in [
            r#"{"api_key_demo":"first","api_key_demo":"second"}"#,
            r#"{"api_key_demo":"first","bad\u0000name":"second"}"#,
            r#"{"api_key_demo":"first","other":"bad\u0000value"}"#,
            r#"{"api_key_demo":"first","other":1}"#,
        ] {
            fs::write(&path, json).unwrap();
            assert!(store
                .import_legacy_with_sync("api_key_demo", |_, _| panic!("invalid source"))
                .is_err());
            assert_eq!(store.load_secret("api_key_demo").unwrap(), None);
            assert_eq!(fs::read_to_string(&path).unwrap(), json);
        }
        fs::remove_file(&path).unwrap();
        assert!(store
            .import_legacy_with_sync("api_key_demo", |_, _| panic!("missing source"))
            .is_err());
        store.legacy_backend = Arc::new(UnavailableCredentialBackend(STORAGE_ERROR.into()));
        assert!(store
            .import_legacy_with_sync("api_key_demo", |_, _| panic!("denied source"))
            .is_err());
        assert_eq!(store.load_secret("api_key_demo").unwrap(), None);
        store
            .mutate_secret_with_sync("api_key_demo", Some("manual-secret"), |_, _| Ok(()))
            .unwrap();
    }

    #[test]
    fn import_bridge_failure_removes_target_and_preserves_source() {
        let old = Arc::new(MemoryCredentialBackend::default());
        old.save("api_key_demo", "old-secret").unwrap();
        let mut store = test_store();
        store.legacy_backend = old.clone();
        let values = Mutex::new(Vec::new());
        let error = store
            .import_legacy_with_sync("api_key_demo", |_, value| {
                let mut calls = values.lock().unwrap();
                calls.push(value.map(str::to_owned));
                if calls.len() == 1 {
                    Err("secret-in-error".into())
                } else {
                    Ok(())
                }
            })
            .unwrap_err();
        assert!(!error.contains("secret-in-error"));
        assert_eq!(
            *values.lock().unwrap(),
            vec![Some("old-secret".into()), None]
        );
        assert_eq!(store.load_secret("api_key_demo").unwrap(), None);
        assert_eq!(
            old.load("api_key_demo").unwrap().as_deref(),
            Some("old-secret")
        );
    }

    struct UncertainBackend {
        memory: MemoryCredentialBackend,
        fail_next: std::sync::atomic::AtomicBool,
    }
    impl CredentialBackend for UncertainBackend {
        fn save(&self, key: &str, value: &str) -> Result<(), String> {
            self.memory.save(key, value)?;
            if self
                .fail_next
                .swap(false, std::sync::atomic::Ordering::SeqCst)
            {
                return Err("applied-secret-diagnostic".into());
            }
            Ok(())
        }
        fn load(&self, key: &str) -> Result<Option<String>, String> {
            self.memory.load(key)
        }
        fn delete(&self, key: &str) -> Result<bool, String> {
            let deleted = self.memory.delete(key)?;
            if self
                .fail_next
                .swap(false, std::sync::atomic::Ordering::SeqCst)
            {
                return Err("deleted-secret-diagnostic".into());
            }
            Ok(deleted)
        }
    }

    #[test]
    fn uncertain_native_write_or_delete_rolls_back_before_bridge_sync() {
        let backend = Arc::new(UncertainBackend {
            memory: MemoryCredentialBackend::default(),
            fail_next: std::sync::atomic::AtomicBool::new(false),
        });
        let store = KeychainStore::with_backend(backend.clone());
        for previous in [None, Some("previous")] {
            restore_secret(&store, "api_key_demo", previous).unwrap();
            for value in [Some("next"), None] {
                backend
                    .fail_next
                    .store(true, std::sync::atomic::Ordering::SeqCst);
                let error = store
                    .mutate_secret_with_sync("api_key_demo", value, |_, _| {
                        panic!("native write failed")
                    })
                    .unwrap_err();
                assert!(!error.contains("secret-diagnostic"));
                assert_eq!(
                    store.load_secret("api_key_demo").unwrap().as_deref(),
                    previous
                );
            }
        }
    }

    #[test]
    fn import_and_manual_save_are_serialized_without_lost_user_write() {
        let old = Arc::new(MemoryCredentialBackend::default());
        old.save("api_key_demo", "old-secret").unwrap();
        let mut store = test_store();
        store.legacy_backend = old;
        let (started_tx, started_rx) = std::sync::mpsc::channel();
        let (release_tx, release_rx) = std::sync::mpsc::channel();
        let release = Mutex::new(release_rx);
        std::thread::scope(|scope| {
            let import = scope.spawn(|| {
                store.import_legacy_with_sync("api_key_demo", |_, _| {
                    started_tx.send(()).unwrap();
                    release.lock().unwrap().recv().unwrap();
                    Ok(())
                })
            });
            started_rx.recv().unwrap();
            assert!(store.provider_sync.try_lock().is_err());
            let save = scope.spawn(|| {
                store.mutate_secret_with_sync("api_key_demo", Some("manual-secret"), |_, _| Ok(()))
            });
            release_tx.send(()).unwrap();
            import.join().unwrap().unwrap();
            save.join().unwrap().unwrap();
        });
        assert_eq!(
            store.load_secret("api_key_demo").unwrap().as_deref(),
            Some("manual-secret")
        );
        assert!(store
            .import_legacy_with_sync("api_key_demo", |_, _| panic!(
                "cannot overwrite manual save"
            ))
            .is_err());
    }

    #[test]
    fn error_categories_never_echo_platform_attributes_and_windows_are_scoped() {
        for error in [
            KeyringError::PlatformFailure(Box::new(std::io::Error::other("sensitive-sentinel"))),
            KeyringError::BadEncoding(b"sensitive-sentinel".to_vec()),
            KeyringError::Invalid("sensitive-sentinel".into(), "sensitive-sentinel".into()),
            KeyringError::TooLong("sensitive-sentinel".into(), 1),
            KeyringError::Ambiguous(vec![]),
        ] {
            assert!(!credential_error(error).contains("sensitive-sentinel"));
        }
        assert!(credential_window("main").is_ok());
        for label in ["settings", "remote", "", "main "] {
            assert!(credential_window(label).is_err());
        }
        for name in ["", "path/name", " bad", "bad ", "bad\0name", "bad\nname"] {
            assert!(provider_name_for_key(&format!("api_key_{name}")).is_err());
        }
    }

    #[cfg(target_os = "macos")]
    #[test]
    #[ignore = "explicit native Keychain smoke; unique temporary namespaces and dummy values only"]
    fn native_keychain_profile_isolation_and_cleanup() {
        struct Cleanup(Vec<PlatformCredentialBackend>);
        impl Drop for Cleanup {
            fn drop(&mut self) {
                for backend in &self.0 {
                    let _ = backend.delete("api_key_smoke");
                }
            }
        }
        let root = tempdir().unwrap();
        let cleanup = Cleanup(vec![
            PlatformCredentialBackend {
                service: crate::credential_namespace::service_for_profile(
                    &root.path().join("first"),
                )
                .unwrap(),
            },
            PlatformCredentialBackend {
                service: crate::credential_namespace::service_for_profile(
                    &root.path().join("second"),
                )
                .unwrap(),
            },
        ]);
        let first = &cleanup.0[0];
        let second = &cleanup.0[1];
        assert_eq!(first.load("api_key_smoke").unwrap(), None);
        first
            .save("api_key_smoke", "dummy-only-smoke-value")
            .unwrap();
        assert_eq!(second.load("api_key_smoke").unwrap(), None);
        assert_eq!(
            first.load("api_key_smoke").unwrap().as_deref(),
            Some("dummy-only-smoke-value")
        );
        second
            .save("api_key_smoke", "second-dummy-smoke-value")
            .unwrap();
        first
            .save("api_key_smoke", "replacement-dummy-smoke-value")
            .unwrap();
        assert!(first.delete("api_key_smoke").unwrap());
        assert_eq!(first.load("api_key_smoke").unwrap(), None);
        assert_eq!(
            second.load("api_key_smoke").unwrap().as_deref(),
            Some("second-dummy-smoke-value")
        );
        assert!(second.delete("api_key_smoke").unwrap());
        assert!(!second.delete("api_key_smoke").unwrap());
    }

    #[cfg(target_os = "macos")]
    #[test]
    #[ignore = "requires a fresh owned private HOME/keychain fixture; run alone in a subprocess"]
    fn native_locked_keychain_preserves_secret_and_recovers() {
        use std::os::unix::fs::MetadataExt;
        use std::process::Command;
        #[link(name = "Security", kind = "framework")]
        unsafe extern "C" {
            fn SecKeychainGetUserInteractionAllowed(state: *mut u8) -> i32;
            fn SecKeychainSetUserInteractionAllowed(state: u8) -> i32;
        }
        let home = PathBuf::from(std::env::var_os("HOME").expect("private HOME required"));
        let fixture = PathBuf::from(std::env::var_os("REASONIX_LOCKED_KEYCHAIN_FIXTURE").expect("owned fixture required"));
        assert!(home.is_absolute());
        assert_eq!(home.file_name().unwrap(), "home");
        let root = home.parent().unwrap();
        assert_eq!(root.parent().unwrap(), Path::new("/private/tmp"));
        assert!(root.file_name().unwrap().to_str().unwrap().starts_with("reasonix-locked-keychain-"));
        assert_eq!(fixture, home.join("Library/Keychains/fixture.keychain"));
        assert_eq!(home.canonicalize().unwrap(), home);
        assert_eq!(root.canonicalize().unwrap(), root);
        assert_eq!(fixture.parent().unwrap().canonicalize().unwrap(), fixture.parent().unwrap());
        let database = fixture.with_file_name("fixture.keychain-db");
        let existing = if fixture.exists() { &fixture } else { &database };
        assert!(fs::symlink_metadata(existing).unwrap().file_type().is_file());
        let metadata = fs::metadata(root).unwrap();
        assert_eq!(metadata.mode() & 0o777, 0o700);
        assert_eq!(metadata.uid(), fs::metadata(&home).unwrap().uid());
        let security = |args: &[&str]| Command::new("/usr/bin/security").args(args).output().unwrap();
        let default = security(&["default-keychain", "-d", "user"]);
        assert!(default.status.success());
        let actual = String::from_utf8(default.stdout).unwrap();
        let actual = actual.trim().trim_matches('"');
        let path = fixture.to_str().unwrap();
        assert!(actual == path || actual == format!("{path}-db"), "default must be the owned fixture");
        let backend = PlatformCredentialBackend {
            service: format!("com.reasonix.native-locked-test.{}", rand::random::<u128>()),
        };
        const KEY: &str = "api_key_smoke";
        const PASSWORD: &str = "reasonix-owned-test-fixture-only";
        struct Cleanup<'a> {
            backend: &'a PlatformCredentialBackend,
            path: &'a str,
            interaction: u8,
        }
        impl Drop for Cleanup<'_> {
            fn drop(&mut self) {
                let _ = Command::new("/usr/bin/security").args(["unlock-keychain", "-p", PASSWORD, self.path]).output();
                let _ = self.backend.delete(KEY);
                // SAFETY: Restores the process-local setting read before this
                // single-test subprocess; never changes another process.
                unsafe { SecKeychainSetUserInteractionAllowed(self.interaction); }
            }
        }
        let mut interaction = 0;
        // SAFETY: Valid one-byte output storage for Security's Boolean.
        assert_eq!(unsafe { SecKeychainGetUserInteractionAllowed(&mut interaction) }, 0);
        let _cleanup = Cleanup { backend: &backend, path, interaction };
        // Disable interaction before the first backend call as well: fixture
        // setup must never depend on a user responding to an ACL prompt.
        assert_eq!(unsafe { SecKeychainSetUserInteractionAllowed(0) }, 0);
        assert!(security(&["unlock-keychain", "-p", PASSWORD, path]).status.success());
        eprintln!("owned-keychain-stage: initial-read");
        assert_eq!(backend.load(KEY).unwrap(), None);
        eprintln!("owned-keychain-stage: seed");
        backend.save(KEY, "owned-dummy-before-lock").unwrap();
        eprintln!("owned-keychain-stage: seeded-read");
        assert_eq!(backend.load(KEY).unwrap().as_deref(), Some("owned-dummy-before-lock"));
        // SAFETY: Only this explicitly isolated test process loses optional
        // authentication UI; production keeps its normal interaction policy.
        assert_eq!(unsafe { SecKeychainSetUserInteractionAllowed(0) }, 0);
        assert!(security(&["lock-keychain", path]).status.success());
        eprintln!("owned-keychain-stage: locked-read");
        assert_eq!(backend.load(KEY).unwrap_err(), STORAGE_ERROR);
        eprintln!("owned-keychain-stage: locked-replace");
        assert_eq!(backend.save(KEY, "owned-dummy-denied-replacement").unwrap_err(), STORAGE_ERROR);
        eprintln!("owned-keychain-stage: locked-delete");
        assert_eq!(backend.delete(KEY).unwrap_err(), STORAGE_ERROR);
        eprintln!("owned-keychain-stage: unlock");
        assert!(security(&["unlock-keychain", "-p", PASSWORD, path]).status.success());
        assert_eq!(backend.load(KEY).unwrap().as_deref(), Some("owned-dummy-before-lock"));
        backend.save(KEY, "owned-dummy-after-unlock").unwrap();
        assert_eq!(backend.load(KEY).unwrap().as_deref(), Some("owned-dummy-after-unlock"));
        assert!(backend.delete(KEY).unwrap());
        assert_eq!(backend.load(KEY).unwrap(), None);
        eprintln!("owned-keychain-stage: cleaned");
    }

    #[test]
    fn unavailable_profile_preserves_recovery_instruction_without_bridge_access() {
        let recovery = "credential profile identity is unavailable; restore its metadata backup or check profile permissions";
        let store =
            KeychainStore::with_backend(Arc::new(UnavailableCredentialBackend(recovery.into())));
        let supervisor = BridgeSupervisor::with_binary(PathBuf::from("/no-test-bridge"));
        assert_eq!(
            store.restore_provider_api_keys(&supervisor).unwrap_err(),
            recovery
        );
        assert!(!supervisor.status().running);
    }

    #[cfg(target_os = "macos")]
    #[test]
    #[ignore = "requires an owned isolated HOME/keychain and freshly built bridge; run alone"]
    fn native_wails_keyring_import_uses_core_account_and_preserves_sources() {
        use std::process::Command;
        use std::os::unix::fs::MetadataExt;
        #[link(name = "Security", kind = "framework")]
        unsafe extern "C" {
            fn SecKeychainGetUserInteractionAllowed(state: *mut u8) -> i32;
            fn SecKeychainSetUserInteractionAllowed(state: u8) -> i32;
            fn SecKeychainCopyDomainDefault(domain: u32, keychain: *mut *const std::ffi::c_void) -> i32;
            fn SecKeychainGetPath(keychain: *const std::ffi::c_void, length: *mut u32, path: *mut u8) -> i32;
            fn SecKeychainFindGenericPassword(keychain: *const std::ffi::c_void, service_len: u32, service: *const u8,
                account_len: u32, account: *const u8, password_len: *mut u32, password: *mut *mut std::ffi::c_void,
                item: *mut *const std::ffi::c_void) -> i32;
            fn SecKeychainItemFreeContent(attributes: *const std::ffi::c_void, password: *mut std::ffi::c_void) -> i32;
        }
        #[link(name = "CoreFoundation", kind = "framework")]
        unsafe extern "C" { fn CFRelease(value: *const std::ffi::c_void); }
        let home = PathBuf::from(std::env::var_os("HOME").unwrap());
        let root = home.parent().unwrap();
        assert_eq!(root.parent().unwrap(), Path::new("/private/tmp"));
        assert!(root.file_name().unwrap().to_str().unwrap().starts_with("reasonix-locked-keychain-"));
        assert_eq!(home.canonicalize().unwrap(), home);
        assert_eq!(fs::metadata(root).unwrap().mode() & 0o777, 0o700);
        assert_eq!(fs::metadata(root).unwrap().uid(), fs::metadata(&home).unwrap().uid());
        let fixture = home.join("Library/Keychains/fixture.keychain");
        assert_eq!(fixture.parent().unwrap().canonicalize().unwrap(), fixture.parent().unwrap());
        let database = fixture.with_file_name("fixture.keychain-db");
        let existing = if fixture.exists() { &fixture } else { &database };
        assert!(fs::symlink_metadata(existing).unwrap().file_type().is_file());
        let output = Command::new("/usr/bin/security").args(["default-keychain", "-d", "user"]).output().unwrap();
        assert!(output.status.success());
        let selected = String::from_utf8(output.stdout).unwrap();
        let selected = selected.trim().trim_matches('"');
        assert!(selected == fixture.to_str().unwrap() || selected == format!("{}-db", fixture.display()));
        let search = Command::new("/usr/bin/security").args(["list-keychains", "-d", "user"]).output().unwrap();
        assert!(search.status.success());
        let search = String::from_utf8(search.stdout).unwrap();
        let entries: Vec<_> = search.lines().map(|line| line.trim().trim_matches('"')).filter(|line| !line.is_empty()).collect();
        assert_eq!(entries, vec![selected], "only the owned private keychain may be searched");
        // Independently check the API domain used by keyring, rather than
        // assuming that the security CLI and native API choose the same root.
        let mut native = std::ptr::null();
        let status = unsafe { SecKeychainCopyDomainDefault(0, &mut native) };
        eprintln!("owned-wails-keyring: native default status={status}");
        assert_eq!(status, 0);
        struct Release(*const std::ffi::c_void);
        impl Drop for Release { fn drop(&mut self) { unsafe { CFRelease(self.0); } } }
        let _release = Release(native);
        let mut path = [0_u8; 4096];
        let mut length = path.len() as u32;
        assert_eq!(unsafe { SecKeychainGetPath(native, &mut length, path.as_mut_ptr()) }, 0);
        let native_path = std::ffi::CStr::from_bytes_until_nul(&path).unwrap().to_str().unwrap();
        assert!(native_path == fixture.to_str().unwrap() || native_path == format!("{}-db", fixture.display()), "native API must select the owned fixture");
        let mut interaction = 0;
        assert_eq!(unsafe { SecKeychainGetUserInteractionAllowed(&mut interaction) }, 0);
        struct RestoreInteraction(u8);
        impl Drop for RestoreInteraction {
            fn drop(&mut self) { unsafe { SecKeychainSetUserInteractionAllowed(self.0); } }
        }
        let _interaction = RestoreInteraction(interaction);
        // Only this native test subprocess disallows optional authorization UI.
        assert_eq!(unsafe { SecKeychainSetUserInteractionAllowed(0) }, 0);
        let source = PlatformCredentialBackend { service: WAILS_SERVICE_NAME.into() };
        let account = format!("REASONIX_WAILS_FIXTURE_{:032X}", rand::random::<u128>());
        let core = root.join("core");
        fs::create_dir(&core).unwrap();
        fs::write(core.join("config.toml"), format!("default_model = \"demo/chat\"\n[[providers]]\nname = \"demo\"\nkind = \"openai\"\nbase_url = \"https://provider.invalid/v1\"\napi_key_env = \"{account}\"\nmodels = [\"chat\"]\ndefault = \"chat\"\n")).unwrap();
        for name in ["REASONIX_HOME", "REASONIX_STATE_HOME", "REASONIX_CACHE_HOME"] {
            std::env::set_var(name, &core);
        }
        let target = Arc::new(PlatformCredentialBackend {
            service: crate::credential_namespace::service_for_profile(&core).unwrap(),
        });
        let store = KeychainStore::with_backend(target.clone());
        let supervisor = BridgeSupervisor::with_binary(PathBuf::from(std::env::var_os("REASONIX_TAURI_BRIDGE_TEST_BIN").unwrap()));
        struct Cleanup<'a> {
            source: &'a PlatformCredentialBackend, account: &'a str,
            target: &'a PlatformCredentialBackend, supervisor: &'a BridgeSupervisor,
        }
        impl Drop for Cleanup<'_> {
            fn drop(&mut self) {
                let _ = self.supervisor.stop();
                let _ = self.target.delete("api_key_demo");
                let _ = self.source.delete(self.account);
            }
        }
        let _cleanup = Cleanup { source: &source, account: &account, target: &target, supervisor: &supervisor };
        // This fixed dummy record in the exclusive private keychain is
        // pre-authorized for the compatibility test. It cannot prove normal
        // Wails authorization/ACL prompts; production access stays unchanged.
        let seeded = Command::new("/usr/bin/security").args(["add-generic-password", "-s", WAILS_SERVICE_NAME, "-a", &account,
            "-w", "dummy-wails-source-only", "-A", fixture.to_str().unwrap()]).output().unwrap();
        assert!(seeded.status.success());
        let unlocked = Command::new("/usr/bin/security").args(["unlock-keychain", "-p", "reasonix-owned-test-fixture-only", fixture.to_str().unwrap()]).output().unwrap();
        assert!(unlocked.status.success());
        let mut password_len = 0;
        let mut password = std::ptr::null_mut();
        let read_status = unsafe { SecKeychainFindGenericPassword(native, WAILS_SERVICE_NAME.len() as u32, WAILS_SERVICE_NAME.as_ptr(),
            account.len() as u32, account.as_ptr(), &mut password_len, &mut password, std::ptr::null_mut()) };
        if !password.is_null() { unsafe { SecKeychainItemFreeContent(std::ptr::null(), password); } }
        // Only an OS status is logged; never emit source bytes or attributes.
        eprintln!("owned-wails-keyring: direct source read status={read_status}");
        eprintln!("owned-wails-keyring: source-read after explicit fixture unlock");
        assert_eq!(source.load(&account).unwrap().as_deref(), Some("dummy-wails-source-only"));
        assert_eq!(source.load("api_key_demo").unwrap(), None);
        supervisor.start().unwrap();
        eprintln!("owned-wails-keyring: core-mapping and import");
        assert_eq!(supervisor.provider_credential_account("demo").unwrap(), account);
        store.import_wails_provider_key(&supervisor, "demo").unwrap();
        assert_eq!(store.load_secret("api_key_demo").unwrap().as_deref(), Some("dummy-wails-source-only"));
        assert_eq!(source.load(&account).unwrap().as_deref(), Some("dummy-wails-source-only"));
        assert!(supervisor.provider_summary().unwrap().providers.iter().any(|p| p.name == "demo" && p.configured));
        assert_eq!(store.import_wails_provider_key(&supervisor, "demo").unwrap_err(), "the profile already has a credential; migration cannot overwrite it");
        supervisor.stop().unwrap();
        supervisor.start().unwrap();
        store.restore_provider_api_keys(&supervisor).unwrap();
        assert!(supervisor.provider_summary().unwrap().providers.iter().any(|p| p.name == "demo" && p.configured));
        store.delete_and_sync_provider_key(&supervisor, "api_key_demo").unwrap();
        assert_eq!(store.load_secret("api_key_demo").unwrap(), None);
        assert_eq!(source.load(&account).unwrap().as_deref(), Some("dummy-wails-source-only"));
        supervisor.stop().unwrap();
        eprintln!("owned-wails-keyring: mapped account, source preservation, refusal, restart and target deletion OK");
    }

    #[test]
    fn real_bridge_restore_continues_after_one_credential_read_failure() {
        let Some(binary) = std::env::var_os("REASONIX_TAURI_BRIDGE_TEST_BIN") else {
            assert!(
                std::env::var_os("CI").is_none(),
                "real bridge binary required in CI"
            );
            return;
        };
        struct FailingEntry {
            values: MemoryCredentialBackend,
            fail: std::sync::atomic::AtomicBool,
        }
        impl CredentialBackend for FailingEntry {
            fn save(&self, key: &str, value: &str) -> Result<(), String> {
                self.values.save(key, value)
            }
            fn delete(&self, key: &str) -> Result<bool, String> {
                self.values.delete(key)
            }
            fn load(&self, key: &str) -> Result<Option<String>, String> {
                if key == "api_key_first" && self.fail.load(std::sync::atomic::Ordering::SeqCst) {
                    return Err("private backend failure detail".into());
                }
                self.values.load(key)
            }
        }
        let _env = crate::test_env::guard();
        let home = tempdir().unwrap();
        for name in [
            "REASONIX_HOME",
            "REASONIX_STATE_HOME",
            "REASONIX_CACHE_HOME",
        ] {
            std::env::set_var(name, home.path());
        }
        let config = r#"default_model = "first/chat"
[[providers]]
name = "first"
kind = "openai"
base_url = "https://provider.invalid/v1"
api_key_env = "REASONIX_KEYCHAIN_PARTIAL_FIRST"
models = ["chat"]
default = "chat"
[[providers]]
name = "second"
kind = "openai"
base_url = "https://provider.invalid/v1"
api_key_env = "REASONIX_KEYCHAIN_PARTIAL_SECOND"
models = ["chat"]
default = "chat"
"#;
        fs::write(home.path().join("config.toml"), config).unwrap();
        let backend = Arc::new(FailingEntry {
            values: MemoryCredentialBackend::default(),
            fail: std::sync::atomic::AtomicBool::new(true),
        });
        backend
            .save("api_key_first", "dummy-partial-first")
            .unwrap();
        backend
            .save("api_key_second", "dummy-partial-second")
            .unwrap();
        let store = KeychainStore::with_backend(backend.clone());
        let supervisor = BridgeSupervisor::with_binary(PathBuf::from(binary));
        supervisor.start().unwrap();
        assert_eq!(supervisor.provider_credential_account("first").unwrap(), "REASONIX_KEYCHAIN_PARTIAL_FIRST");
        assert!(supervisor.provider_credential_account("unknown-account").is_err());
        let outcome = store.restore_provider_api_keys(&supervisor);
        let summary = supervisor.provider_summary().unwrap();
        supervisor.stop().unwrap();
        assert!(
            outcome.is_err(),
            "partial restoration must not report full success"
        );
        assert!(
            !summary
                .providers
                .iter()
                .find(|p| p.name == "first")
                .unwrap()
                .configured
        );
        assert!(
            summary
                .providers
                .iter()
                .find(|p| p.name == "second")
                .unwrap()
                .configured,
            "one failed entry must not prevent a later valid provider from restoring"
        );
        assert!(!outcome
            .unwrap_err()
            .contains("private backend failure detail"));
        backend
            .fail
            .store(false, std::sync::atomic::Ordering::SeqCst);
        supervisor.start().unwrap();
        store.restore_provider_api_keys(&supervisor).unwrap();
        let restored = supervisor.provider_summary().unwrap();
        backend
            .fail
            .store(true, std::sync::atomic::Ordering::SeqCst);
        let restart = store.restart_bridge(&supervisor);
        let running = supervisor.status().running;
        let partial_restart = supervisor.provider_summary().unwrap();
        supervisor.stop().unwrap();
        assert!(restored
            .providers
            .iter()
            .all(|p| !p.requires_key || p.configured));
        assert!(
            restart.is_err(),
            "partial restart restore must not claim full success"
        );
        assert!(running, "unaffected providers keep a usable sidecar");
        assert!(
            !partial_restart
                .providers
                .iter()
                .find(|p| p.name == "first")
                .unwrap()
                .configured
        );
        assert!(
            partial_restart
                .providers
                .iter()
                .find(|p| p.name == "second")
                .unwrap()
                .configured
        );
        assert_eq!(
            fs::read_to_string(home.path().join("config.toml")).unwrap(),
            config
        );
        assert_eq!(
            backend.values.load("api_key_first").unwrap().as_deref(),
            Some("dummy-partial-first")
        );
        assert_eq!(
            backend.values.load("api_key_second").unwrap().as_deref(),
            Some("dummy-partial-second")
        );
        fn check_files(directory: &Path) {
            for entry in fs::read_dir(directory).unwrap() {
                let entry = entry.unwrap();
                if entry.file_type().unwrap().is_dir() {
                    check_files(&entry.path());
                } else if entry.file_type().unwrap().is_file() {
                    assert!(
                        !String::from_utf8_lossy(&fs::read(entry.path()).unwrap())
                            .contains("dummy-partial-"),
                        "restored credentials must not be persisted into profile files"
                    );
                }
            }
        }
        check_files(home.path());
    }

    #[test]
    fn real_bridge_import_save_restart_and_delete_keep_credentials_out_of_files() {
        let Some(binary) = std::env::var_os("REASONIX_TAURI_BRIDGE_TEST_BIN") else {
            assert!(
                std::env::var_os("CI").is_none(),
                "real bridge binary required in CI"
            );
            return;
        };
        let _env = crate::test_env::guard();
        let home = tempdir().unwrap();
        std::env::set_var("REASONIX_HOME", home.path());
        std::env::set_var("REASONIX_STATE_HOME", home.path());
        std::env::set_var("REASONIX_CACHE_HOME", home.path());
        let config = r#"default_model = "keychain-test-provider/chat"
[[providers]]
name = "keychain-test-provider"
kind = "openai"
base_url = "https://provider.invalid/v1"
api_key_env = "REASONIX_KEYCHAIN_INTEGRATION_TEST_KEY"
models = ["chat"]
default = "chat"
[[providers]]
name = "旧版服务"
kind = "openai"
base_url = "https://provider.invalid/v1"
api_key_env = "REASONIX_KEYCHAIN_UNICODE_TEST_KEY"
models = ["chat"]
default = "chat"
"#;
        fs::write(home.path().join("config.toml"), config).unwrap();
        let old = Arc::new(MemoryCredentialBackend::default());
        old.save(
            "api_key_keychain-test-provider",
            "dummy-legacy-integration-key",
        )
        .unwrap();
        let mut store = test_store();
        store.legacy_backend = old.clone();
        let supervisor = BridgeSupervisor::with_binary(PathBuf::from(binary));
        supervisor.start().unwrap();
        let ready = || {
            supervisor
                .provider_summary()
                .unwrap()
                .providers
                .iter()
                .find(|p| p.name == "keychain-test-provider")
                .unwrap()
                .configured
        };
        assert!(!ready());
        store
            .save_and_sync_provider_key(
                &supervisor,
                "api_key_旧版服务",
                "dummy-unicode-integration-key",
            )
            .unwrap();
        assert!(
            supervisor
                .provider_summary()
                .unwrap()
                .providers
                .iter()
                .find(|p| p.name == "旧版服务")
                .unwrap()
                .configured
        );
        assert!(store
            .delete_and_sync_provider_key(&supervisor, "api_key_旧版服务")
            .unwrap());
        assert!(store
            .import_legacy_provider_key(&supervisor, "unknown-provider")
            .is_err());
        assert_eq!(store.load_secret("api_key_unknown-provider").unwrap(), None);
        store
            .import_legacy_provider_key(&supervisor, "keychain-test-provider")
            .unwrap();
        assert!(ready());
        assert_eq!(
            old.load("api_key_keychain-test-provider")
                .unwrap()
                .as_deref(),
            Some("dummy-legacy-integration-key")
        );
        store
            .save_and_sync_provider_key(
                &supervisor,
                "api_key_keychain-test-provider",
                "dummy-new-integration-key",
            )
            .unwrap();
        assert!(ready());
        assert!(store.restart_bridge(&supervisor).unwrap().running);
        assert!(
            ready(),
            "restarted sidecar restores keys from scoped backend"
        );
        assert!(store
            .delete_and_sync_provider_key(&supervisor, "api_key_keychain-test-provider")
            .unwrap());
        assert!(!ready());
        assert!(store.restart_bridge(&supervisor).unwrap().running);
        assert!(
            !ready(),
            "deleted key is never resurrected from legacy storage"
        );
        supervisor.stop().unwrap();
        assert_eq!(
            fs::read_to_string(home.path().join("config.toml")).unwrap(),
            config
        );
        fn check_files(dir: &Path) {
            for entry in fs::read_dir(dir).unwrap() {
                let entry = entry.unwrap();
                let kind = entry.file_type().unwrap();
                if kind.is_dir() {
                    check_files(&entry.path());
                } else if kind.is_file() {
                    let bytes = fs::read(entry.path()).unwrap();
                    assert!(!bytes
                        .windows(b"dummy-legacy-integration-key".len())
                        .any(|slice| slice == b"dummy-legacy-integration-key"));
                    assert!(!bytes
                        .windows(b"dummy-new-integration-key".len())
                        .any(|slice| slice == b"dummy-new-integration-key"));
                }
            }
        }
        check_files(home.path());
    }
}
