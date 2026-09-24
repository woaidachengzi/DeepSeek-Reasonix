use std::{
    env, fs,
    io::Write,
    path::{Path, PathBuf},
    time::{SystemTime, UNIX_EPOCH},
};

use serde::Serialize;
use tauri::Manager;

const CORE_PROFILE_DIR: &str = "reasonix-core";
const CONFIG_FILE: &str = "config.toml";

/// The preview's private core profile and the stable config it may explicitly
/// import. This state is created before `REASONIX_HOME` is changed, so the
/// source resolves to the stable default rather than the preview itself.
#[derive(Clone, Debug)]
pub struct PreviewProfile {
    home: PathBuf,
    stable_config: Option<PathBuf>,
    managed_profile: bool,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PreviewProfileStatus {
    pub preview_home: String,
    pub preview_config_exists: bool,
    pub stable_config: Option<String>,
    pub stable_config_exists: bool,
    pub import_available: bool,
    pub managed_profile: bool,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProfileImportResult {
    pub imported_config: String,
    pub backup_config: String,
}

/// Selects the Go core's private storage root for the Tauri preview.
///
/// The established Wails client uses Reasonix's default user profile. A Tauri
/// preview must not silently read, upgrade, or write that profile: apart from
/// corrupting a migration experiment, doing so would let two independently
/// managed desktop hosts write the same configuration and session state.
///
/// An explicitly supplied `REASONIX_HOME` remains authoritative. That is a
/// deliberate developer/operator choice; automatic importing is unavailable
/// for that externally selected profile.
pub fn configure_preview_profile(app: &tauri::App) -> Result<PreviewProfile, String> {
    let stable_config = default_stable_config_path();
    if let Some(home) = explicit_reasonix_home() {
        return Ok(PreviewProfile {
            home,
            stable_config,
            managed_profile: false,
        });
    }

    let app_data = app
        .path()
        .app_data_dir()
        .map_err(|error| format!("resolve Tauri preview data directory: {error}"))?;
    let home = preview_home(&app_data);
    fs::create_dir_all(&home).map_err(|error| {
        format!(
            "create Tauri preview data directory {}: {error}",
            home.display()
        )
    })?;

    // The Go core resolves its state root as REASONIX_STATE_HOME first and
    // REASONIX_HOME second. An inherited REASONIX_STATE_HOME therefore wins
    // over the preview home below, which would route preview sessions, the
    // config and the identity database into a stable installation.
    enable_managed_profile(&home);
    Ok(PreviewProfile {
        home,
        stable_config,
        managed_profile: true,
    })
}

/// Points the Go core at one private state root and drops inherited overrides.
///
/// Split out from `configure_preview_profile` so the isolation rule is directly
/// testable without a Tauri `App`: the failure it prevents is silent, because
/// the sidecar would simply read and write a stable installation.
fn enable_managed_profile(home: &Path) {
    if std::env::var_os("REASONIX_STATE_HOME").is_some() {
        std::env::remove_var("REASONIX_STATE_HOME");
    }
    std::env::set_var("REASONIX_HOME", home);
}

impl PreviewProfile {
    pub fn status(&self) -> PreviewProfileStatus {
        let stable_config_exists = self
            .stable_config
            .as_ref()
            .is_some_and(|path| path.is_file());
        let preview_config_exists = self.config_path().is_file();
        PreviewProfileStatus {
            preview_home: self.home.display().to_string(),
            preview_config_exists,
            stable_config: self
                .stable_config
                .as_ref()
                .map(|path| path.display().to_string()),
            stable_config_exists,
            import_available: self.managed_profile
                && stable_config_exists
                && !preview_config_exists,
            managed_profile: self.managed_profile,
        }
    }

    /// Copies only the stable user configuration into an empty preview
    /// profile. Sessions, caches, plugins, and `.env` files stay untouched.
    /// A timestamped copy is created first, and the destination uses
    /// create-new semantics so an existing preview config is never overwritten.
    pub fn import_stable_config(&self) -> Result<ProfileImportResult, String> {
        if !self.managed_profile {
            return Err(
                "stable config import is unavailable when REASONIX_HOME was explicitly supplied"
                    .into(),
            );
        }
        let source = self
            .stable_config
            .as_ref()
            .ok_or("the stable Reasonix config location is unavailable")?;
        let source_metadata = fs::symlink_metadata(source)
            .map_err(|error| format!("read stable config {}: {error}", source.display()))?;
        if source_metadata.file_type().is_symlink() || !source_metadata.is_file() {
            return Err("the stable config must be a regular file; refusing to import a symlink or directory".into());
        }

        let destination = self.config_path();
        if destination.exists() {
            return Err(format!(
                "preview config already exists at {}; import never overwrites it",
                destination.display()
            ));
        }

        let backup_dir = self.create_backup_dir()?;
        let backup = backup_dir.join(CONFIG_FILE);
        fs::copy(source, &backup).map_err(|error| {
            format!(
                "back up stable config {} to {}: {error}",
                source.display(),
                backup.display()
            )
        })?;
        restrict_config_permissions(&backup)?;

        let content = fs::read(&backup)
            .map_err(|error| format!("read backup {}: {error}", backup.display()))?;
        let mut created = fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&destination)
            .map_err(|error| {
                format!(
                    "create preview config {} without overwriting: {error}",
                    destination.display()
                )
            })?;
        created
            .write_all(&content)
            .map_err(|error| format!("write preview config {}: {error}", destination.display()))?;
        created
            .sync_all()
            .map_err(|error| format!("sync preview config {}: {error}", destination.display()))?;
        restrict_config_permissions(&destination)?;

        Ok(ProfileImportResult {
            imported_config: destination.display().to_string(),
            backup_config: backup.display().to_string(),
        })
    }

    fn config_path(&self) -> PathBuf {
        self.home.join(CONFIG_FILE)
    }

    fn create_backup_dir(&self) -> Result<PathBuf, String> {
        let millis = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map_err(|error| format!("read system clock for config backup: {error}"))?
            .as_millis();
        let root = self.home.join("backups");
        fs::create_dir_all(&root).map_err(|error| {
            format!(
                "create preview backup directory {}: {error}",
                root.display()
            )
        })?;
        for suffix in 0..100_u8 {
            let candidate = root.join(format!("stable-config-{millis}-{suffix}"));
            match fs::create_dir(&candidate) {
                Ok(()) => return Ok(candidate),
                Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => continue,
                Err(error) => {
                    return Err(format!(
                        "create preview backup {}: {error}",
                        candidate.display()
                    ))
                }
            }
        }
        Err("could not allocate a unique preview config backup directory".into())
    }
}

fn explicit_reasonix_home() -> Option<PathBuf> {
    env::var_os("REASONIX_HOME")
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
}

fn default_stable_config_path() -> Option<PathBuf> {
    #[cfg(target_os = "windows")]
    {
        env::var_os("APPDATA")
            .or_else(|| {
                env::var_os("USERPROFILE")
                    .map(|home| PathBuf::from(home).join("AppData/Roaming").into_os_string())
            })
            .map(PathBuf::from)
            .map(|root| root.join("reasonix").join(CONFIG_FILE))
    }
    #[cfg(not(target_os = "windows"))]
    {
        env::var_os("HOME")
            .map(PathBuf::from)
            .map(|home| home.join(".reasonix").join(CONFIG_FILE))
    }
}

fn preview_home(app_data_dir: &Path) -> PathBuf {
    app_data_dir.join(CORE_PROFILE_DIR)
}

#[cfg(unix)]
fn restrict_config_permissions(path: &Path) -> Result<(), String> {
    use std::os::unix::fs::PermissionsExt;
    fs::set_permissions(path, fs::Permissions::from_mode(0o600))
        .map_err(|error| format!("set private permissions on {}: {error}", path.display()))
}

#[cfg(not(unix))]
fn restrict_config_permissions(_path: &Path) -> Result<(), String> {
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn managed_profile(home: PathBuf, stable_config: PathBuf) -> PreviewProfile {
        PreviewProfile {
            home,
            stable_config: Some(stable_config),
            managed_profile: true,
        }
    }

    #[test]
    fn preview_profile_is_nested_under_tauri_app_data() {
        let app_data = Path::new("/tmp/reasonix-preview");
        assert_eq!(
            preview_home(app_data),
            PathBuf::from("/tmp/reasonix-preview/reasonix-core")
        );
    }

    #[test]
    fn import_copies_config_after_creating_a_private_backup() {
        let root = tempfile::tempdir().expect("temp root");
        let stable = root.path().join("stable/config.toml");
        fs::create_dir_all(stable.parent().expect("stable parent")).expect("create stable home");
        fs::write(&stable, "default_model = \"preview-test\"\n").expect("write stable config");
        let profile = managed_profile(root.path().join("preview"), stable.clone());
        fs::create_dir_all(&profile.home).expect("create preview home");

        let result = profile
            .import_stable_config()
            .expect("import stable config");

        assert_eq!(
            fs::read_to_string(&result.imported_config).expect("read imported"),
            "default_model = \"preview-test\"\n"
        );
        assert_eq!(
            fs::read_to_string(&result.backup_config).expect("read backup"),
            "default_model = \"preview-test\"\n"
        );
        assert!(!stable.starts_with(&profile.home));
    }

    #[test]
    fn import_refuses_to_overwrite_a_preview_config() {
        let root = tempfile::tempdir().expect("temp root");
        let stable = root.path().join("stable/config.toml");
        fs::create_dir_all(stable.parent().expect("stable parent")).expect("create stable home");
        fs::write(&stable, "stable\n").expect("write stable config");
        let profile = managed_profile(root.path().join("preview"), stable);
        fs::create_dir_all(&profile.home).expect("create preview home");
        fs::write(profile.config_path(), "existing\n").expect("write preview config");

        let error = profile
            .import_stable_config()
            .expect_err("must not overwrite config");

        assert!(error.contains("never overwrites"));
        assert_eq!(
            fs::read_to_string(profile.config_path()).expect("read preview config"),
            "existing\n"
        );
    }

    #[test]
    fn explicit_home_disables_automatic_import() {
        let profile = PreviewProfile {
            home: PathBuf::from("/custom/profile"),
            stable_config: Some(PathBuf::from("/stable/config.toml")),
            managed_profile: false,
        };
        assert!(!profile.status().import_available);
        assert!(profile.import_stable_config().is_err());
    }

    /// The Go core reads REASONIX_STATE_HOME before REASONIX_HOME, so an
    /// inherited state home would silently move preview sessions, config and
    /// the identity database into a stable installation.
    #[test]
    fn managed_profile_drops_an_inherited_state_home() {
        let _env = crate::test_env::guard();
        env::set_var("REASONIX_STATE_HOME", "/stable/state");

        let home = PathBuf::from("/preview/home");
        enable_managed_profile(&home);

        assert_eq!(
            env::var_os("REASONIX_STATE_HOME"),
            None,
            "an inherited state home must not survive"
        );
        assert_eq!(
            env::var_os("REASONIX_HOME"),
            Some(home.into_os_string()),
            "the preview home must be the Go core's state root"
        );
    }
}
