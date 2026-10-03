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
const MAX_CONFIG_FILE: u64 = 16 * 1024 * 1024;
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
/// Explicit roots remain available for separate developer/operator profiles.
/// They cannot select the stable default tree, including through an alias.
/// Automatic importing is unavailable for an externally selected profile.
pub fn configure_preview_profile(app: &tauri::App) -> Result<PreviewProfile, String> {
    let stable_config = default_stable_config_path();
    if let Some(home) = explicit_reasonix_home() {
        let home = protect_explicit_profile(&home, stable_config.as_deref())?;
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
    let home = prepare_managed_home(&app_data)?;
    // The Go core resolves state and cache overrides before REASONIX_HOME.
    // Inherited values would route preview writes outside its private home.
    enable_managed_profile(&home);
    Ok(PreviewProfile {
        home,
        stable_config,
        managed_profile: true,
    })
}

fn prepare_managed_home(app_data: &Path) -> Result<PathBuf, String> {
    let home = preview_home(app_data);
    fs::create_dir_all(app_data)
        .map_err(|error| format!("create Tauri application data directory: {error}"))?;
    // create_dir_all follows an existing link and would silently select the
    // stable Wails profile as the managed Preview's writable storage root.
    // Create only this leaf, then inspect without following it, including a
    // dangling link or a non-directory left by an earlier failed setup.
    match create_private_directory(&home) {
        Ok(()) => {}
        Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => {}
        Err(error) => return Err(format!("create Tauri preview data directory: {error}")),
    }
    let metadata = fs::symlink_metadata(&home)
        .map_err(|_| "Preview data directory is unavailable; check its permissions and restart")?;
    if metadata.file_type().is_symlink() || !metadata.is_dir() {
        return Err("managed Preview data directory must be an ordinary directory; move the link or conflicting file aside and restart with a separate Preview directory".into());
    }
    Ok(home)
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
    if std::env::var_os("REASONIX_CACHE_HOME").is_some() {
        std::env::remove_var("REASONIX_CACHE_HOME");
    }
    std::env::set_var("REASONIX_HOME", home);
    std::env::set_var(PREVIEW_SQLITE_EVENTS_ENV, "1");
}

fn disable_managed_event_store() {
    std::env::remove_var(PREVIEW_SQLITE_EVENTS_ENV);
}

impl PreviewProfile {
    pub(crate) fn home(&self) -> &Path {
        &self.home
    }

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
        if source_metadata.file_type().is_symlink()
            || !source_metadata.is_file()
            || source_metadata.len() > MAX_CONFIG_FILE
        {
            return Err("the stable config must be a regular file no larger than 16 MiB".into());
        }

        let destination = self.config_path();
        if destination.exists() {
            return Err(format!(
                "preview config already exists at {}; import never overwrites it",
                destination.display()
            ));
        }

        let source_file = fs::File::open(source)
            .map_err(|error| format!("open stable config {}: {error}", source.display()))?;
        let opened_metadata = source_file.metadata().map_err(|error| {
            format!("inspect opened stable config {}: {error}", source.display())
        })?;
        let current_metadata = fs::symlink_metadata(source)
            .map_err(|error| format!("reinspect stable config {}: {error}", source.display()))?;
        if current_metadata.file_type().is_symlink()
            || !current_metadata.is_file()
            || !opened_metadata.is_file()
            || opened_metadata.len() > MAX_CONFIG_FILE
            || !crate::workbench_projects::same_file_as_path(source, &source_file).map_err(
                |error| format!("verify opened stable config {}: {error}", source.display()),
            )?
        {
            return Err("the stable config changed while opening or is not a regular file no larger than 16 MiB".into());
        }
        let content = read_bounded_file(source_file, MAX_CONFIG_FILE)
            .map_err(|error| format!("read stable config {}: {error}", source.display()))?;

        let backup_dir = self.create_backup_dir()?;
        let backup = backup_dir.join(CONFIG_FILE);
        let mut backup_temp = tempfile::Builder::new()
            .prefix(".stable-config-backup-")
            .tempfile_in(&backup_dir)
            .map_err(|error| format!("create private config backup: {error}"))?;
        restrict_config_permissions(backup_temp.path())?;
        backup_temp
            .write_all(&content)
            .map_err(|error| format!("write config backup {}: {error}", backup.display()))?;
        backup_temp
            .as_file()
            .sync_all()
            .map_err(|error| format!("sync config backup {}: {error}", backup.display()))?;
        backup_temp.persist_noclobber(&backup).map_err(|error| {
            format!("create private config backup {}: {error}", backup.display())
        })?;

        let mut destination_temp = tempfile::Builder::new()
            .prefix(".config-import-")
            .tempfile_in(&self.home)
            .map_err(|error| format!("create private Preview config: {error}"))?;
        restrict_config_permissions(destination_temp.path())?;
        destination_temp
            .write_all(&content)
            .map_err(|error| format!("write Preview config {}: {error}", destination.display()))?;
        destination_temp
            .as_file()
            .sync_all()
            .map_err(|error| format!("sync Preview config {}: {error}", destination.display()))?;
        destination_temp
            .persist_noclobber(&destination)
            .map_err(|error| {
                format!(
                    "create Preview config {} without overwriting: {error}",
                    destination.display()
                )
            })?;

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
        let bytes = read_bounded_file(file, MAX_PROJECTS_FILE)
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
        match create_private_directory(&root) {
            Ok(()) => {}
            Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => {}
            Err(error) => {
                return Err(format!(
                    "create preview backup directory {}: {error}",
                    root.display()
                ))
            }
        }
        let metadata = fs::symlink_metadata(&root)
            .map_err(|error| format!("inspect preview backup directory: {error}"))?;
        if metadata.file_type().is_symlink() || !metadata.is_dir() {
            return Err(
                "preview backup location must be an ordinary directory inside the profile".into(),
            );
        }
        for suffix in 0..100_u8 {
            let candidate = root.join(format!("stable-config-{millis}-{suffix}"));
            match create_private_directory(&candidate) {
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

fn create_private_directory(path: &Path) -> std::io::Result<()> {
    let mut builder = fs::DirBuilder::new();
    #[cfg(unix)]
    {
        use std::os::unix::fs::DirBuilderExt;
        builder.mode(0o700);
    }
    builder.create(path)
}

fn explicit_reasonix_home() -> Option<PathBuf> {
    env::var_os("REASONIX_HOME")
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
}

const EXPLICIT_BOUNDARY_ERROR: &str = "Preview storage overlaps the stable Reasonix data directory; unset the storage overrides or choose separate Preview home, state and cache directories";

// Resolve without creating anything. Existing aliases are canonicalized;
// missing descendants remain comparable through their nearest existing parent.
fn comparable_storage_path(path: &Path) -> Result<PathBuf, String> {
    let absolute = if path.is_absolute() { path.to_owned() } else {
        env::current_dir().map_err(|_| EXPLICIT_BOUNDARY_ERROR)?.join(path)
    };
    let mut clean = PathBuf::new();
    for component in absolute.components() {
        match component {
            std::path::Component::CurDir => {},
            std::path::Component::ParentDir => { clean.pop(); },
            part => clean.push(part.as_os_str()),
        }
    }
    let mut ancestor = clean.as_path();
    let mut missing = Vec::new();
    loop {
        match fs::symlink_metadata(ancestor) {
            Ok(_) => {
                let mut resolved = fs::canonicalize(ancestor).map_err(|_| EXPLICIT_BOUNDARY_ERROR)?;
                for leaf in missing.into_iter().rev() { resolved.push(leaf); }
                return Ok(resolved);
            },
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
                missing.push(ancestor.file_name().ok_or(EXPLICIT_BOUNDARY_ERROR)?.to_owned());
                ancestor = ancestor.parent().ok_or(EXPLICIT_BOUNDARY_ERROR)?;
            },
            Err(_) => return Err(EXPLICIT_BOUNDARY_ERROR.into()),
        }
    }
}

fn reject_stable_overlap(candidate: &Path, stable: &Path) -> Result<(), String> {
    let candidate = comparable_storage_path(candidate)?;
    let stable = comparable_storage_path(stable)?;
    if candidate.starts_with(&stable) || stable.starts_with(&candidate) {
        return Err(EXPLICIT_BOUNDARY_ERROR.into());
    }
    Ok(())
}

fn protect_explicit_profile(home: &Path, stable_config: Option<&Path>) -> Result<PathBuf, String> {
    let Some(stable) = stable_config.and_then(Path::parent) else {
        return Err("stable profile location is unavailable; set HOME before starting Preview".into());
    };
    let mut overrides = vec![("REASONIX_HOME", home.to_owned())];
    for name in ["REASONIX_STATE_HOME", "REASONIX_CACHE_HOME"] {
        if let Some(value) = env::var_os(name).filter(|v| !v.is_empty()) {
            overrides.push((name, PathBuf::from(value)));
        }
    }
    // Go expands ~/ and ${VAR} in overrides. Reject unresolved syntax rather
    // than comparing a literal Rust path against a different sidecar target.
    // No environment or filesystem mutation takes place until all checks pass.
    for (_, path) in &overrides {
        let text = path.to_str().ok_or(EXPLICIT_BOUNDARY_ERROR)?;
        if text.trim() != text || text.starts_with('~') || text.contains("${") {
            return Err("Preview storage overrides must use resolved paths; expand variables and tilde, remove surrounding whitespace, then restart".into());
        }
        reject_stable_overlap(path, stable)?;
        #[cfg(target_os = "macos")]
        if let Some(user_home) = stable.parent() {
            for legacy in [user_home.join("Library/Application Support/reasonix"), user_home.join("Library/Caches/reasonix"), user_home.join(".config/reasonix")] {
                reject_stable_overlap(path, &legacy)?;
            }
        }
    }
    // Comparison must not rewrite the operator's selected environment. The
    // sidecar and existing profile probes retain their established path form.
    Ok(home.to_owned())
}

fn read_bounded_file(mut reader: impl Read, max_bytes: u64) -> std::io::Result<Vec<u8>> {
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

    #[cfg(unix)]
    #[test]
    fn explicit_boundary_refuses_stable_aliases_ancestors_and_missing_descendants() {
        use std::os::unix::fs::symlink;
        let root = tempfile::tempdir().unwrap();
        let stable = root.path().join(".reasonix");
        fs::create_dir(&stable).unwrap();
        let original = stable.join("config.toml");
        fs::write(&original, b"stable original\n").unwrap();
        let before = fs::metadata(&original).unwrap();
        let alias = root.path().join("alias");
        symlink(&stable, &alias).unwrap();
        for candidate in [stable.clone(), alias.clone(), alias.join("new/state"), root.path().to_owned(), stable.join("../.reasonix/cache")] {
            assert_eq!(reject_stable_overlap(&candidate, &stable).unwrap_err(), EXPLICIT_BOUNDARY_ERROR);
        }
        assert!(reject_stable_overlap(&root.path().join("preview/new/state"), &stable).is_ok());
        assert!(!stable.join("new").exists());
        assert_eq!(fs::read(&original).unwrap(), b"stable original\n");
        assert_eq!(fs::metadata(&original).unwrap().modified().unwrap(), before.modified().unwrap());
        assert!(fs::symlink_metadata(alias).unwrap().file_type().is_symlink());
    }

    #[test]
    fn explicit_state_and_cache_cannot_escape_into_stable_profile() {
        let _env = crate::test_env::guard();
        let root = tempfile::tempdir().unwrap();
        let stable = root.path().join(".reasonix/config.toml");
        let preview = root.path().join("preview");
        env::set_var("REASONIX_HOME", &preview);
        for name in ["REASONIX_STATE_HOME", "REASONIX_CACHE_HOME"] {
            env::remove_var("REASONIX_STATE_HOME");
            env::remove_var("REASONIX_CACHE_HOME");
            let target = stable.parent().unwrap().join("missing");
            env::set_var(name, &target);
            assert!(protect_explicit_profile(&preview, Some(&stable)).is_err());
            assert_eq!(env::var_os(name), Some(target.into_os_string()));
            assert_eq!(env::var_os("REASONIX_HOME"), Some(preview.clone().into_os_string()));
            assert!(!preview.exists());
        }
        env::remove_var("REASONIX_STATE_HOME");
        env::remove_var("REASONIX_CACHE_HOME");
        assert_eq!(protect_explicit_profile(&preview, Some(&stable)).unwrap(), preview);
        assert!(!preview.exists());
        env::set_var("REASONIX_STATE_HOME", "${HOME}/.reasonix");
        assert!(protect_explicit_profile(&preview, Some(&stable)).unwrap_err().contains("resolved paths"));
    }

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
            read_bounded_file(std::io::Cursor::new(b"1234"), 4).expect("exact limit is accepted"),
            b"1234"
        );
        let error = read_bounded_file(std::io::Cursor::new(b"12345"), 4)
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

    #[cfg(unix)]
    #[test]
    fn managed_profile_rejects_linked_legacy_directory_without_touching_originals() {
        use std::os::unix::fs::{symlink, PermissionsExt};
        let root = tempfile::tempdir().unwrap();
        let legacy = root.path().join(".reasonix");
        fs::create_dir(&legacy).unwrap();
        let config = legacy.join(CONFIG_FILE);
        fs::write(&config, b"legacy config canary\n").unwrap();
        fs::set_permissions(&config, fs::Permissions::from_mode(0o600)).unwrap();
        let before = fs::metadata(&config).unwrap();
        let app_data = root.path().join("app-data");
        fs::create_dir(&app_data).unwrap();
        let home = preview_home(&app_data);
        symlink(&legacy, &home).unwrap();
        assert!(
            prepare_managed_home(&app_data).is_err(),
            "managed Preview must not adopt a linked Wails directory"
        );
        assert_eq!(fs::read(&config).unwrap(), b"legacy config canary\n");
        assert_eq!(
            fs::metadata(&config).unwrap().permissions().mode(),
            before.permissions().mode()
        );
        assert_eq!(
            fs::metadata(&config).unwrap().modified().unwrap(),
            before.modified().unwrap()
        );
        assert_eq!(fs::read_link(&home).unwrap(), legacy);
    }

    #[test]
    fn managed_profile_creates_private_root_and_preserves_existing_profile() {
        let root = tempfile::tempdir().unwrap();
        let app_data = root.path().join("new/app-data");
        let home = prepare_managed_home(&app_data).unwrap();
        assert_eq!(home, preview_home(&app_data));
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            assert_eq!(
                fs::metadata(&home).unwrap().permissions().mode() & 0o777,
                0o700
            );
        }
        let canary = home.join(CONFIG_FILE);
        fs::write(&canary, b"existing Preview\n").unwrap();
        assert_eq!(prepare_managed_home(&app_data).unwrap(), home);
        assert_eq!(fs::read(canary).unwrap(), b"existing Preview\n");
    }

    #[cfg(unix)]
    #[test]
    fn managed_profile_rejects_dangling_link_and_regular_file() {
        use std::os::unix::fs::symlink;
        let root = tempfile::tempdir().unwrap();
        let home = preview_home(root.path());
        let missing = root.path().join("missing-legacy");
        symlink(&missing, &home).unwrap();
        assert!(prepare_managed_home(root.path()).is_err());
        assert!(
            !missing.exists(),
            "rejecting a dangling link must not create its target"
        );
        assert_eq!(fs::read_link(&home).unwrap(), missing);
        fs::remove_file(&home).unwrap();
        fs::write(&home, b"ordinary file canary\n").unwrap();
        assert!(prepare_managed_home(root.path()).is_err());
        assert_eq!(fs::read(&home).unwrap(), b"ordinary file canary\n");
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
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            for directory in [
                profile.home.join("backups"),
                Path::new(&result.backup_config)
                    .parent()
                    .unwrap()
                    .to_path_buf(),
            ] {
                assert_eq!(
                    fs::metadata(directory).unwrap().permissions().mode() & 0o777,
                    0o700
                );
            }
            for path in [&result.imported_config, &result.backup_config] {
                let mode = fs::metadata(path)
                    .expect("private imported file")
                    .permissions()
                    .mode();
                assert_eq!(mode & 0o777, 0o600);
            }
        }
        assert!(!stable.starts_with(&profile.home));
    }

    #[cfg(unix)]
    #[test]
    fn import_rejects_redirected_backup_root_without_external_writes() {
        use std::os::unix::fs::symlink;
        let root = tempfile::tempdir().unwrap();
        let stable = root.path().join("stable.toml");
        fs::write(&stable, b"default_model = \"stable\"\n").unwrap();
        let before = fs::read(&stable).unwrap();
        let profile = managed_profile(root.path().join("preview"), stable.clone());
        fs::create_dir(&profile.home).unwrap();
        let outside = root.path().join("outside");
        fs::create_dir(&outside).unwrap();
        symlink(&outside, profile.home.join("backups")).unwrap();
        assert!(profile.import_stable_config().is_err());
        assert_eq!(fs::read(&stable).unwrap(), before);
        assert_eq!(fs::read_dir(&outside).unwrap().count(), 0);
        assert!(!profile.config_path().exists());
    }

    #[test]
    fn import_rejects_non_directory_backup_root_without_changing_files() {
        let root = tempfile::tempdir().unwrap();
        let stable = root.path().join("stable.toml");
        fs::write(&stable, b"default_model = \"stable\"\n").unwrap();
        let profile = managed_profile(root.path().join("preview"), stable.clone());
        fs::create_dir(&profile.home).unwrap();
        let backup_root = profile.home.join("backups");
        fs::write(&backup_root, b"existing original").unwrap();
        assert!(profile.import_stable_config().is_err());
        assert_eq!(fs::read(&backup_root).unwrap(), b"existing original");
        assert_eq!(fs::read(&stable).unwrap(), b"default_model = \"stable\"\n");
        assert!(!profile.config_path().exists());
    }

    #[test]
    fn returning_to_stable_keeps_original_and_backup_after_preview_edits() {
        let root = tempfile::tempdir().expect("temp root");
        let stable = root.path().join("stable/config.toml");
        fs::create_dir_all(stable.parent().unwrap()).unwrap();
        let original = "default_model = \"stable-model\"\n";
        fs::write(&stable, original).unwrap();
        let profile = managed_profile(root.path().join("preview"), stable.clone());
        fs::create_dir_all(&profile.home).unwrap();
        let imported = profile.import_stable_config().unwrap();
        fs::write(
            &imported.imported_config,
            "default_model = \"preview-model\"\n",
        )
        .unwrap();
        assert_eq!(fs::read_to_string(&stable).unwrap(), original);
        assert_eq!(
            fs::read_to_string(&imported.backup_config).unwrap(),
            original
        );
        assert!(profile.import_stable_config().is_err());
        assert_eq!(fs::read_to_string(&stable).unwrap(), original);
    }

    #[test]
    fn import_rejects_oversized_config_before_creating_a_backup() {
        let root = tempfile::tempdir().expect("temp root");
        let stable = root.path().join("stable/config.toml");
        fs::create_dir_all(stable.parent().expect("stable parent")).expect("create stable home");
        fs::File::create(&stable)
            .expect("create stable config")
            .set_len(MAX_CONFIG_FILE + 1)
            .expect("make sparse oversized config");
        let profile = managed_profile(root.path().join("preview"), stable);

        assert!(profile.import_stable_config().is_err());
        assert!(!profile.config_path().exists());
        assert!(!profile.home.join("backups").exists());
    }

    #[cfg(unix)]
    #[test]
    fn import_rejects_symlinked_stable_config() {
        use std::os::unix::fs::symlink;

        let root = tempfile::tempdir().expect("temp root");
        let stable = root.path().join("stable/config.toml");
        let outside = root.path().join("outside.toml");
        fs::create_dir_all(stable.parent().expect("stable parent")).expect("create stable home");
        fs::write(&outside, "private\n").expect("write outside file");
        symlink(&outside, &stable).expect("create stable symlink");
        let profile = managed_profile(root.path().join("preview"), stable);

        assert!(profile.import_stable_config().is_err());
        assert!(!profile.config_path().exists());
        assert!(!profile.home.join("backups").exists());
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

    /// The Go core reads state and cache overrides before REASONIX_HOME, so
    /// inherited roots would silently move preview writes outside its home.
    #[test]
    fn managed_profile_drops_inherited_state_and_cache_homes() {
        let _env = crate::test_env::guard();
        env::set_var("REASONIX_STATE_HOME", "/stable/state");
        env::set_var("REASONIX_CACHE_HOME", "/stable/cache");

        let home = PathBuf::from("/preview/home");
        enable_managed_profile(&home);

        assert_eq!(
            env::var_os("REASONIX_STATE_HOME"),
            None,
            "an inherited state home must not survive"
        );
        assert_eq!(
            env::var_os("REASONIX_CACHE_HOME"),
            None,
            "an inherited cache home must not survive"
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
