use std::{
    collections::{HashMap, HashSet},
    fs,
    io::{Read, Write},
    path::{Path, PathBuf},
    sync::Mutex,
};

use tauri::Manager;

use crate::protocol_generated::BridgeProjectFolder;

const CATALOG_FILE: &str = "workbench-project-folders.json";
const MAX_PROJECTS: usize = 10_000;
const MAX_ROOT_BYTES: usize = 4096;
const MAX_TITLE_CHARS: usize = 1024;
const MAX_CATALOG_BYTES: u64 = 4 * 1024 * 1024;

pub struct WorkbenchProjectCatalog {
    path: PathBuf,
    folders: Mutex<ProjectFolderState>,
}

#[derive(Clone, Default, serde::Serialize, serde::Deserialize)]
struct ProjectFolderDocument {
    folders: Vec<BridgeProjectFolder>,
    #[serde(default)]
    removed_roots: Vec<String>,
}

struct ProjectFolderState {
    document: ProjectFolderDocument,
    load_error: Option<String>,
}

#[derive(serde::Deserialize)]
#[serde(untagged)]
enum StoredProjectFolders {
    Legacy(Vec<BridgeProjectFolder>),
    Current(ProjectFolderDocument),
}

impl WorkbenchProjectCatalog {
    pub fn for_app(app: &tauri::App) -> Result<Self, String> {
        let directory = app
            .path()
            .app_data_dir()
            .map_err(|error| format!("resolve project folder catalog directory: {error}"))?;
        fs::create_dir_all(&directory).map_err(|error| {
            format!(
                "create project folder catalog directory {}: {error}",
                directory.display()
            )
        })?;
        Ok(Self::at(directory.join(CATALOG_FILE)))
    }

    fn at(path: PathBuf) -> Self {
        let (document, load_error) = match read_folders(&path) {
            Ok(document) => (document, None),
            Err(error) => (ProjectFolderDocument::default(), Some(error)),
        };
        Self {
            path,
            folders: Mutex::new(ProjectFolderState {
                document,
                load_error,
            }),
        }
    }

    pub fn list(&self) -> Result<Vec<BridgeProjectFolder>, String> {
        let mut state = self
            .folders
            .lock()
            .map_err(|_| "project folder catalog lock is unavailable".to_string())?;
        // A startup read failure is cached so the sidebar can report which
        // source is unavailable. A user-triggered list/recheck must retry the
        // disk read instead of returning that stale error forever; otherwise
        // repairing permissions or replacing a malformed file requires an
        // app restart before project folders can be used again.
        if state.load_error.is_some() {
            match read_folders(&self.path) {
                Ok(document) => {
                    state.document = document;
                    state.load_error = None;
                }
                Err(error) => {
                    state.load_error = Some(error.clone());
                    return Err(error);
                }
            }
        }
        state
            .load_error
            .clone()
            .map_or_else(|| Ok(state.document.folders.clone()), Err)
    }

    pub fn remember(&self, root: &str) -> Result<Vec<BridgeProjectFolder>, String> {
        let root = normalize_root(root)?;
        let folder = BridgeProjectFolder {
            root: root.clone(),
            title: None,
        };
        let mut state = self
            .folders
            .lock()
            .map_err(|_| "project folder catalog lock is unavailable".to_string())?;
        if let Some(error) = &state.load_error {
            return Err(error.clone());
        }
        if state.document.folders.iter().any(|existing| {
            normalized_project_key(&existing.root) == normalized_project_key(&folder.root)
        }) {
            return Ok(state.document.folders.clone());
        }
        if state.document.folders.len() >= MAX_PROJECTS {
            return Err("project folder catalog is full".to_string());
        }
        let mut updated = state.document.clone();
        updated
            .removed_roots
            .retain(|removed| normalized_project_key(removed) != normalized_project_key(&root));
        updated.folders.push(folder);
        write_document(&self.path, &updated)?;
        state.document = updated.clone();
        Ok(updated.folders)
    }

    pub fn set_title(&self, root: &str, title: &str) -> Result<Vec<BridgeProjectFolder>, String> {
        let root = normalize_root(root)?;
        let title = title.trim();
        if title.chars().count() > MAX_TITLE_CHARS || title.chars().any(char::is_control) {
            return Err(
                "project title must be at most 1024 characters and contain no control characters"
                    .to_string(),
            );
        }
        let mut state = self
            .folders
            .lock()
            .map_err(|_| "project folder catalog lock is unavailable".to_string())?;
        if let Some(error) = &state.load_error {
            return Err(error.clone());
        }
        let mut updated = state.document.clone();
        updated
            .removed_roots
            .retain(|removed| normalized_project_key(removed) != normalized_project_key(&root));
        if let Some(folder) = updated
            .folders
            .iter_mut()
            .find(|folder| normalized_project_key(&folder.root) == normalized_project_key(&root))
        {
            folder.title = Some(title.to_string());
        } else {
            if updated.folders.len() >= MAX_PROJECTS {
                return Err("project folder catalog is full".to_string());
            }
            updated.folders.push(BridgeProjectFolder {
                root,
                title: Some(title.to_string()),
            });
        }
        write_document(&self.path, &updated)?;
        state.document = updated.clone();
        Ok(updated.folders)
    }
    pub fn remove(&self, root: &str) -> Result<(), String> {
        let root = normalize_root(root)?;
        let key = normalized_project_key(&root);
        let mut state = self
            .folders
            .lock()
            .map_err(|_| "project folder catalog lock is unavailable".to_string())?;
        if let Some(error) = &state.load_error {
            return Err(error.clone());
        }
        let mut updated = state.document.clone();
        updated
            .folders
            .retain(|folder| normalized_project_key(&folder.root) != key);
        if !updated
            .removed_roots
            .iter()
            .any(|removed| normalized_project_key(removed) == key)
        {
            if updated.removed_roots.len() >= MAX_PROJECTS {
                return Err("removed project folder catalog is full".to_string());
            }
            updated.removed_roots.push(root);
        }
        write_document(&self.path, &updated)?;
        state.document = updated;
        Ok(())
    }

    pub fn merge_legacy(
        &self,
        mut legacy: Vec<BridgeProjectFolder>,
    ) -> Result<Vec<BridgeProjectFolder>, String> {
        // Retry a failed startup read before using exclusions. Both local folders
        // and exclusions must come from the same locked snapshot.
        self.list()?;
        let state = self
            .folders
            .lock()
            .map_err(|_| "project folder catalog lock is unavailable".to_string())?;
        let removed: HashSet<_> = state
            .document
            .removed_roots
            .iter()
            .map(|root| normalized_project_key(root))
            .collect();
        legacy.retain(|folder| !removed.contains(&normalized_project_key(&folder.root)));
        let mut indexes: HashMap<_, _> = legacy
            .iter()
            .enumerate()
            .map(|(index, folder)| (normalized_project_key(&folder.root), index))
            .collect();
        for folder in &state.document.folders {
            let key = normalized_project_key(&folder.root);
            if let Some(&index) = indexes.get(&key) {
                if folder.title.is_some() {
                    legacy[index].title = folder.title.clone();
                }
            } else {
                indexes.insert(key, legacy.len());
                legacy.push(folder.clone());
            }
        }
        Ok(legacy)
    }
}

fn normalize_root(root: &str) -> Result<String, String> {
    if root.trim().is_empty()
        || root.len() > MAX_ROOT_BYTES
        || root.chars().any(char::is_control)
        || !Path::new(root).is_absolute()
    {
        return Err("project folder path must be an absolute directory path".to_string());
    }
    let trimmed = if cfg!(windows) {
        root.trim_end_matches(['/', '\\'])
    } else {
        root.trim_end_matches('/')
    };
    let normalized = if trimmed.is_empty() {
        root.chars().next().unwrap_or('/').to_string()
    } else if cfg!(windows) && trimmed.len() == 2 && trimmed.as_bytes()[1] == b':' {
        format!("{trimmed}\\")
    } else {
        trimmed.to_string()
    };
    Ok(normalized)
}

pub(crate) fn normalized_project_key(root: &str) -> String {
    if cfg!(windows) {
        let normalized = root.replace('/', "\\");
        let has_trailing_separator = normalized.ends_with('\\');
        let trimmed = normalized.trim_end_matches('\\');
        if trimmed.is_empty() {
            root.chars()
                .next()
                .map(|separator| if separator == '/' { '\\' } else { separator })
                .unwrap_or_default()
                .to_string()
        } else {
            let is_drive_designator = trimmed.len() == 2
                && trimmed.as_bytes()[1] == b':'
                && trimmed.as_bytes()[0].is_ascii_alphabetic();
            let key = if has_trailing_separator && is_drive_designator {
                format!("{trimmed}\\")
            } else {
                trimmed.to_string()
            };
            key.to_lowercase()
        }
    } else {
        let key = root.trim_end_matches('/');
        if key.is_empty() && root.starts_with('/') {
            "/".to_string()
        } else {
            key.to_string()
        }
    }
}

fn read_folders(path: &Path) -> Result<ProjectFolderDocument, String> {
    let metadata = match fs::symlink_metadata(path) {
        Ok(metadata) => metadata,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            return Ok(ProjectFolderDocument::default())
        }
        Err(error) => return Err(format!("inspect project folder catalog failed: {error}")),
    };
    if metadata.file_type().is_symlink()
        || !metadata.is_file()
        || metadata.len() > MAX_CATALOG_BYTES
    {
        return Err(
            "project folder catalog is not a regular file within the size limit".to_string(),
        );
    }
    let file = fs::File::open(path)
        .map_err(|error| format!("read project folder catalog failed: {error}"))?;
    let opened_metadata = file
        .metadata()
        .map_err(|error| format!("inspect opened project folder catalog failed: {error}"))?;
    let current_metadata = fs::symlink_metadata(path)
        .map_err(|error| format!("reinspect project folder catalog failed: {error}"))?;
    if current_metadata.file_type().is_symlink()
        || !current_metadata.is_file()
        || !opened_metadata.is_file()
        || opened_metadata.len() > MAX_CATALOG_BYTES
        || !same_file_as_path(path, &file)
            .map_err(|error| format!("verify opened project folder catalog failed: {error}"))?
    {
        return Err(
            "project folder catalog changed while opening or is not a regular file within the size limit".to_string(),
        );
    }
    let bytes = read_bounded_catalog(file, MAX_CATALOG_BYTES)
        .map_err(|error| format!("read project folder catalog failed: {error}"))?;
    let document = match serde_json::from_slice::<StoredProjectFolders>(&bytes)
        .map_err(|_| "project folder catalog is not valid JSON".to_string())?
    {
        StoredProjectFolders::Legacy(folders) => ProjectFolderDocument {
            folders,
            removed_roots: Vec::new(),
        },
        StoredProjectFolders::Current(document) => document,
    };
    if document.removed_roots.len() > MAX_PROJECTS {
        return Err("removed project folder catalog contains too many entries".to_string());
    }
    let mut removed_roots = Vec::new();
    let mut removed_keys = HashSet::new();
    for root in document.removed_roots {
        let root = normalize_root(&root)?;
        if removed_keys.insert(normalized_project_key(&root)) {
            removed_roots.push(root);
        }
    }
    let decoded = document.folders;
    if decoded.len() > MAX_PROJECTS {
        return Err("project folder catalog contains too many entries".to_string());
    }
    let mut folders = Vec::with_capacity(decoded.len().min(MAX_PROJECTS));
    let mut seen = HashSet::with_capacity(decoded.len());
    for folder in decoded {
        let root = normalize_root(&folder.root)?;
        if let Some(title) = folder.title.as_ref() {
            if title.chars().count() > MAX_TITLE_CHARS || title.chars().any(char::is_control) {
                return Err("project folder title is invalid or too long".to_string());
            }
        }
        if !seen.insert(normalized_project_key(&root)) {
            continue;
        }
        folders.push(BridgeProjectFolder {
            root,
            title: folder.title,
        });
        if folders.len() > MAX_PROJECTS {
            return Err("project folder catalog contains too many entries".to_string());
        }
    }
    Ok(ProjectFolderDocument {
        folders,
        removed_roots,
    })
}

pub(crate) fn same_file_as_path(path: &Path, file: &fs::File) -> std::io::Result<bool> {
    let path_handle = same_file::Handle::from_path(path)?;
    let file_handle = same_file::Handle::from_file(file.try_clone()?)?;
    Ok(path_handle == file_handle)
}

fn read_bounded_catalog(mut reader: impl Read, max_bytes: u64) -> std::io::Result<Vec<u8>> {
    let mut bytes = Vec::new();
    reader
        .by_ref()
        .take(max_bytes.saturating_add(1))
        .read_to_end(&mut bytes)?;
    if bytes.len() as u64 > max_bytes {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidData,
            "project folder catalog exceeds the size limit",
        ));
    }
    Ok(bytes)
}

fn write_document(path: &Path, document: &ProjectFolderDocument) -> Result<(), String> {
    let encoded = if document.removed_roots.is_empty() {
        serde_json::to_vec(&document.folders)
    } else {
        serde_json::to_vec(document)
    }
    .map_err(|error| format!("encode project folder catalog: {error}"))?;
    if encoded.len() as u64 > MAX_CATALOG_BYTES {
        return Err("project folder catalog exceeds the size limit".to_string());
    }
    let directory = path
        .parent()
        .ok_or_else(|| "project folder catalog path has no parent directory".to_string())?;
    let mut temporary = tempfile::Builder::new()
        .prefix(".workbench-project-folders-")
        .tempfile_in(directory)
        .map_err(|error| format!("create temporary project folder catalog: {error}"))?;
    temporary
        .write_all(&encoded)
        .map_err(|error| format!("write temporary project folder catalog: {error}"))?;
    temporary
        .as_file()
        .sync_all()
        .map_err(|error| format!("sync temporary project folder catalog: {error}"))?;
    temporary
        .persist(path)
        .map_err(|error| format!("save project folder catalog {}: {error}", path.display()))?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn remembered_folders_preserve_path_spaces_and_deduplicate_trailing_separators() {
        let directory = tempfile::tempdir().expect("temp directory");
        let path = directory.path().join(CATALOG_FILE);
        let catalog = WorkbenchProjectCatalog::at(path.clone());
        let expected_root = "/work/alpha ";

        let folders = catalog
            .remember("/work/alpha /")
            .expect("remember absolute workspace");
        assert_eq!(folders.len(), 1);
        assert_eq!(folders[0].root, expected_root);
        assert_eq!(folders[0].title, None);

        let duplicate = catalog
            .remember(expected_root)
            .expect("deduplicate normalized workspace");
        assert_eq!(duplicate.len(), folders.len());
        assert_eq!(duplicate[0].root, folders[0].root);
        let reloaded = WorkbenchProjectCatalog::at(path).list().expect("reload");
        assert_eq!(reloaded.len(), 1);
        assert_eq!(reloaded[0].root, expected_root);
        assert_eq!(reloaded[0].title, None);
    }

    #[test]
    fn removal_persists_excludes_legacy_and_preserves_workspace_files() {
        let directory = tempfile::tempdir().expect("temp directory");
        let workspace = directory.path().join("workspace");
        fs::create_dir(&workspace).unwrap();
        fs::write(workspace.join("keep.txt"), "keep").unwrap();
        let root = workspace.to_str().unwrap();
        let path = directory.path().join(CATALOG_FILE);
        // Existing array catalogs still load, including customized names.
        let legacy = vec![BridgeProjectFolder {
            root: root.into(),
            title: Some("old".into()),
        }];
        fs::write(&path, serde_json::to_vec(&legacy).unwrap()).unwrap();
        let catalog = WorkbenchProjectCatalog::at(path.clone());
        assert_eq!(catalog.list().unwrap().len(), 1);
        catalog.remove(&format!("{root}/")).unwrap();
        catalog.remove(root).unwrap();
        let reloaded = WorkbenchProjectCatalog::at(path.clone());
        assert!(reloaded.list().unwrap().is_empty());
        assert!(reloaded.merge_legacy(legacy.clone()).unwrap().is_empty());
        assert_eq!(
            fs::read_to_string(workspace.join("keep.txt")).unwrap(),
            "keep"
        );
        // Explicitly choosing the folder again restores it, also after restart.
        reloaded.remember(root).unwrap();
        assert_eq!(
            WorkbenchProjectCatalog::at(path)
                .merge_legacy(legacy)
                .unwrap()
                .len(),
            1
        );
    }

    #[test]
    fn legacy_only_removal_and_local_title_override_survive_reload() {
        let directory = tempfile::tempdir().expect("temp directory");
        let path = directory.path().join(CATALOG_FILE);
        let catalog = WorkbenchProjectCatalog::at(path.clone());
        let legacy = vec![
            BridgeProjectFolder {
                root: "/work/empty/".into(),
                title: None,
            },
            BridgeProjectFolder {
                root: "/work/kept".into(),
                title: Some("legacy".into()),
            },
        ];
        catalog.remove("/work/empty").unwrap();
        catalog.set_title("/work/kept/", "custom").unwrap();
        let merged = WorkbenchProjectCatalog::at(path)
            .merge_legacy(legacy)
            .unwrap();
        assert_eq!(merged.len(), 1);
        assert_eq!(merged[0].title.as_deref(), Some("custom"));
    }

    #[test]
    fn failed_removal_keeps_catalog_and_invalid_roots_do_not_write() {
        let directory = tempfile::tempdir().expect("temp directory");
        let path = directory.path().join(CATALOG_FILE);
        let catalog = WorkbenchProjectCatalog::at(path.clone());
        assert!(catalog.remove("relative").is_err());
        assert!(!path.exists());
        catalog.remember("/work/empty").unwrap();
        fs::remove_file(&path).unwrap();
        fs::create_dir(&path).unwrap();
        assert!(catalog.remove("/work/empty").is_err());
        assert_eq!(catalog.list().unwrap().len(), 1);
    }

    #[test]
    fn malformed_catalog_fails_closed_without_overwriting_source() {
        let directory = tempfile::tempdir().expect("temp directory");
        let path = directory.path().join(CATALOG_FILE);
        let malformed = b"not valid project JSON";
        fs::write(&path, malformed).expect("write malformed catalog");
        let catalog = WorkbenchProjectCatalog::at(path.clone());

        assert!(catalog.list().is_err());
        assert!(catalog.remember("/work/new").is_err());
        assert!(catalog.remove("/work/new").is_err());
        assert_eq!(fs::read(path).expect("read preserved catalog"), malformed);
    }

    #[test]
    fn project_title_updates_are_normalized_and_persistent() {
        let directory = tempfile::tempdir().expect("temp directory");
        let path = directory.path().join(CATALOG_FILE);
        let catalog = WorkbenchProjectCatalog::at(path.clone());

        let renamed = catalog
            .set_title("/work/alpha/", "  Alpha workspace  ")
            .expect("set project title");
        assert_eq!(renamed.len(), 1);
        assert_eq!(renamed[0].root, "/work/alpha");
        assert_eq!(renamed[0].title.as_deref(), Some("Alpha workspace"));

        let cleared = catalog
            .set_title("/work/alpha", "  ")
            .expect("clear project title");
        assert_eq!(cleared.len(), 1);
        assert_eq!(cleared[0].title.as_deref(), Some(""));
        let reloaded = WorkbenchProjectCatalog::at(path).list().expect("reload");
        assert_eq!(reloaded.len(), 1);
        assert_eq!(reloaded[0].title.as_deref(), Some(""));
    }

    #[test]
    fn invalid_project_title_does_not_write_catalog() {
        let directory = tempfile::tempdir().expect("temp directory");
        let path = directory.path().join(CATALOG_FILE);
        let catalog = WorkbenchProjectCatalog::at(path.clone());

        assert!(catalog
            .set_title("/work/alpha", &"a".repeat(MAX_TITLE_CHARS + 1))
            .is_err());
        assert!(catalog.set_title("/work/alpha", "bad\ntitle").is_err());
        assert!(!path.exists());
    }

    #[test]
    fn oversized_catalog_is_rejected_before_creating_destination() {
        let directory = tempfile::tempdir().expect("temp directory");
        let path = directory.path().join(CATALOG_FILE);
        let root = format!("/{}", "a".repeat(MAX_ROOT_BYTES - 1));
        let folders = (0..1100)
            .map(|_| BridgeProjectFolder {
                root: root.clone(),
                title: None,
            })
            .collect::<Vec<_>>();

        assert!(write_document(
            &path,
            &ProjectFolderDocument {
                folders,
                removed_roots: Vec::new()
            }
        )
        .is_err());
        assert!(!path.exists());
    }

    #[test]
    fn bounded_catalog_reader_rejects_growth_past_limit() {
        assert_eq!(
            read_bounded_catalog(std::io::Cursor::new(b"1234"), 4)
                .expect("exact limit is accepted"),
            b"1234"
        );
        let error = read_bounded_catalog(std::io::Cursor::new(b"12345"), 4)
            .expect_err("one byte above limit is rejected");
        assert_eq!(error.kind(), std::io::ErrorKind::InvalidData);
    }
}
