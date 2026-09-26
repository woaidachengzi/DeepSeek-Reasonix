use std::{
    env, fs,
    io::{Read, Write},
    path::{Path, PathBuf},
    time::{SystemTime, UNIX_EPOCH},
};

use serde::Serialize;
use tauri::Manager;

const CORE_PROFILE_DIR: &str = "reasonix-core";
const PREVIEW_SQLITE_EVENTS_ENV: &str = "REASONIX_PREVIEW_SQLITE_EVENTS";
const CONFIG_FILE: &str = "config.toml";
const PROJECTS_FILE: &str = "desktop-projects.json";
const MAX_PROJECTS_FILE: u64 = 4 * 1024 * 1024;
const MAX_PROJECT_COUNT: usize = 10_000;
const MAX_PROJECT_TITLE_CHARS: usize = 1024;

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
    pub project_folders_import_available: bool,
    pub project_folders_file_exists: bool,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProfileImportResult {
    pub imported_config: String,
    pub backup_config: String,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProjectFoldersImportResult {
    pub imported_file: String,
    pub project_count: usize,
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
        disable_managed_event_store();
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
    std::env::set_var(PREVIEW_SQLITE_EVENTS_ENV, "1");
}

fn disable_managed_event_store() {
    std::env::remove_var(PREVIEW_SQLITE_EVENTS_ENV);
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
            project_folders_import_available: self.managed_profile
                && self
                    .stable_projects_path()
                    .is_some_and(|path| path.is_file())
                && !self.project_folders_path().exists(),
            project_folders_file_exists: self.project_folders_path().is_file(),
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

    /// Imports only saved project folder roots and optional labels. Topic IDs,
    /// session metadata, ordering, and every other stable-profile field stay
    /// out of the Preview copy.
    pub fn import_stable_project_folders(&self) -> Result<ProjectFoldersImportResult, String> {
        if !self.managed_profile {
            return Err(
                "project folder import is unavailable when REASONIX_HOME was explicitly supplied"
                    .into(),
            );
        }
        let source = self
            .stable_projects_path()
            .ok_or("the stable project folder location is unavailable")?;
        let metadata = fs::symlink_metadata(&source).map_err(|error| {
            format!(
                "inspect saved project folders {}: {error}",
                source.display()
            )
        })?;
        if metadata.file_type().is_symlink()
            || !metadata.is_file()
            || metadata.len() > MAX_PROJECTS_FILE
        {
            return Err(
                "the stable project folder file must be a regular file no larger than 4 MiB".into(),
            );
        }
        let file = fs::File::open(&source)
            .map_err(|error| format!("read saved project folders {}: {error}", source.display()))?;
        let opened_metadata = file.metadata().map_err(|error| {
            format!(
                "inspect opened project folders {}: {error}",
                source.display()
            )
        })?;
        let current_metadata = fs::symlink_metadata(&source).map_err(|error| {
            format!(
                "reinspect saved project folders {}: {error}",
                source.display()
            )
        })?;
        if current_metadata.file_type().is_symlink()
            || !current_metadata.is_file()
            || !opened_metadata.is_file()
            || opened_metadata.len() > MAX_PROJECTS_FILE
            || !crate::workbench_projects::same_file_as_path(&source, &file).map_err(|error| {
                format!(
                    "verify opened project folders {}: {error}",
                    source.display()
                )
            })?
        {
            return Err(
                "the stable project folder file changed while opening or is not a regular file no larger than 4 MiB".into(),
            );
        }
        let bytes = read_bounded_project_folders(file, MAX_PROJECTS_FILE)
            .map_err(|error| format!("read saved project folders {}: {error}", source.display()))?;
        #[derive(serde::Deserialize)]
        struct StoredProject {
            #[serde(default)]
            root: String,
            #[serde(default)]
            title: String,
        }
        #[derive(serde::Deserialize)]
        struct StoredProjects {
            #[serde(default)]
            projects: Vec<StoredProject>,
        }
        let stored: StoredProjects = serde_json::from_slice(&bytes)
            .map_err(|error| format!("parse saved project folders: {error}"))?;
        if stored.projects.len() > MAX_PROJECT_COUNT {
            return Err("the stable project folder file contains too many entries".into());
        }
        let mut projects = Vec::with_capacity(stored.projects.len());
        let mut seen = std::collections::HashSet::new();
        for project in stored.projects {
            let root = project.root;
            if root.trim().is_empty()
                || root.len() > 4096
                || root.chars().any(char::is_control)
                || !Path::new(&root).is_absolute()
            {
                continue;
            }
            let key = crate::workbench_projects::normalized_project_key(&root);
            if !seen.insert(key) {
                continue;
            }
            let title_without_controls: String = project
                .title
                .chars()
                .filter(|character| !character.is_control())
                .collect();
            let title: String = title_without_controls
                .trim()
                .chars()
                .take(MAX_PROJECT_TITLE_CHARS)
                .collect();
            projects.push(serde_json::json!({ "root": root, "title": title }));
        }
        let project_count = projects.len();
        let output = serde_json::to_vec_pretty(&serde_json::json!({ "projects": projects }))
            .map_err(|error| format!("encode Preview project folders: {error}"))?;
        fs::create_dir_all(&self.home)
            .map_err(|error| format!("create Preview profile {}: {error}", self.home.display()))?;
        let destination = self.project_folders_path();
        let mut temporary = tempfile::Builder::new()
            .prefix(".desktop-project-folders-import-")
            .tempfile_in(&self.home)
            .map_err(|error| {
                format!(
                    "create temporary Preview project folder file in {}: {error}",
                    self.home.display()
                )
            })?;
        temporary
            .write_all(&output)
            .map_err(|error| format!("write temporary Preview project folders: {error}"))?;
        restrict_config_permissions(temporary.path())?;
        temporary
            .as_file()
            .sync_all()
            .map_err(|error| format!("sync temporary Preview project folders: {error}"))?;
        temporary.persist_noclobber(&destination).map_err(|error| {
            format!(
                "create Preview project folder file {} without overwriting: {error}",
                destination.display()
            )
        })?;
        Ok(ProjectFoldersImportResult {
            imported_file: destination.display().to_string(),
            project_count,
        })
    }

    fn config_path(&self) -> PathBuf {
        self.home.join(CONFIG_FILE)
    }

    fn stable_projects_path(&self) -> Option<PathBuf> {
        self.stable_config
            .as_ref()
            .and_then(|path| path.parent())
            .map(|directory| directory.join(PROJECTS_FILE))
    }

    fn project_folders_path(&self) -> PathBuf {
        self.home.join(PROJECTS_FILE)
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

fn read_bounded_project_folders(mut reader: impl Read, max_bytes: u64) -> std::io::Result<Vec<u8>> {
    let mut bytes = Vec::new();
    reader
        .by_ref()
        .take(max_bytes.saturating_add(1))
        .read_to_end(&mut bytes)?;
    if bytes.len() as u64 > max_bytes {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidData,
            "project folder file exceeds the size limit",
        ));
    }
    Ok(bytes)
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
    fn bounded_project_folder_reader_rejects_growth_past_limit() {
        assert_eq!(
            read_bounded_project_folders(std::io::Cursor::new(b"1234"), 4)
                .expect("exact limit is accepted"),
            b"1234"
        );
        let error = read_bounded_project_folders(std::io::Cursor::new(b"12345"), 4)
            .expect_err("one byte above limit is rejected");
        assert_eq!(error.kind(), std::io::ErrorKind::InvalidData);
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

    #[test]
    fn project_folder_import_copies_only_roots_and_never_overwrites_preview() {
        let root = tempfile::tempdir().expect("temp root");
        let stable = root.path().join("stable/config.toml");
        let stable_projects = root.path().join("stable/desktop-projects.json");
        fs::create_dir_all(stable.parent().expect("stable parent")).expect("create stable home");
        let source = br#"{"projects":[{"root":"/work/alpha","title":"Alpha","topics":["private-topic"],"pinnedTopics":["private-pin"]},{"root":"/work/empty","title":"Empty"},{"root":"/work/alpha","title":"Duplicate"}],"globalTopics":["private-global"]}"#;
        fs::write(&stable_projects, source).expect("write stable project folders");
        let profile = managed_profile(root.path().join("preview"), stable);

        let result = profile
            .import_stable_project_folders()
            .expect("import stable project folders");
        assert_eq!(result.project_count, 2);
        let imported = fs::read(&result.imported_file).expect("read imported folders");
        let imported_text = String::from_utf8(imported.clone()).expect("UTF-8 JSON");
        assert!(imported_text.contains("/work/alpha"));
        assert!(imported_text.contains("Alpha"));
        assert!(imported_text.contains("/work/empty"));
        assert!(!imported_text.contains("private-topic"));
        assert!(!imported_text.contains("private-pin"));
        assert!(!imported_text.contains("private-global"));
        assert_eq!(
            fs::read(&stable_projects).expect("stable source unchanged"),
            source
        );

        assert!(profile.import_stable_project_folders().is_err());
        assert_eq!(
            fs::read(&result.imported_file).expect("preview file remains unchanged"),
            imported
        );
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
        assert_eq!(
            env::var(PREVIEW_SQLITE_EVENTS_ENV).as_deref(),
            Ok("1"),
            "managed Preview must opt into SQLite event storage"
        );
    }

    #[test]
    fn explicit_profile_disables_managed_event_store_capability() {
        let _env = crate::test_env::guard();
        env::set_var(PREVIEW_SQLITE_EVENTS_ENV, "1");
        disable_managed_event_store();
        assert_eq!(env::var_os(PREVIEW_SQLITE_EVENTS_ENV), None);
    }
}
