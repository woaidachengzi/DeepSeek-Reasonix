use std::{
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

struct ProjectFolderState {
    folders: Vec<BridgeProjectFolder>,
    load_error: Option<String>,
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
        let (folders, load_error) = match read_folders(&path) {
            Ok(folders) => (folders, None),
            Err(error) => (Vec::new(), Some(error)),
        };
        Self {
            path,
            folders: Mutex::new(ProjectFolderState {
                folders,
                load_error,
            }),
        }
    }

    pub fn list(&self) -> Result<Vec<BridgeProjectFolder>, String> {
        let state = self
            .folders
            .lock()
            .map_err(|_| "project folder catalog lock is unavailable".to_string())?;
        state
            .load_error
            .clone()
            .map_or_else(|| Ok(state.folders.clone()), Err)
    }

    pub fn remember(&self, root: &str) -> Result<Vec<BridgeProjectFolder>, String> {
        let folder = BridgeProjectFolder {
            root: normalize_root(root)?,
            title: None,
        };
        let mut state = self
            .folders
            .lock()
            .map_err(|_| "project folder catalog lock is unavailable".to_string())?;
        if let Some(error) = &state.load_error {
            return Err(error.clone());
        }
        if state
            .folders
            .iter()
            .any(|existing| project_key(&existing.root) == project_key(&folder.root))
        {
            return Ok(state.folders.clone());
        }
        if state.folders.len() >= MAX_PROJECTS {
            return Err("project folder catalog is full".to_string());
        }
        let mut updated = state.folders.clone();
        updated.push(folder);
        write_folders(&self.path, &updated)?;
        state.folders = updated.clone();
        Ok(updated)
    }
}

fn normalize_root(root: &str) -> Result<String, String> {
    let root = root.trim();
    if root.is_empty()
        || root.len() > MAX_ROOT_BYTES
        || root.chars().any(char::is_control)
        || !Path::new(root).is_absolute()
    {
        return Err("project folder path must be an absolute directory path".to_string());
    }
    let trimmed = root.trim_end_matches(['/', '\\']);
    let normalized = if trimmed.is_empty() {
        root.chars().next().unwrap_or('/').to_string()
    } else if trimmed.len() == 2 && trimmed.as_bytes()[1] == b':' {
        format!("{trimmed}\\")
    } else {
        trimmed.to_string()
    };
    Ok(normalized)
}

fn project_key(root: &str) -> String {
    let trimmed = root.trim().trim_end_matches(['/', '\\']);
    if cfg!(windows) {
        trimmed.to_ascii_lowercase()
    } else {
        trimmed.to_string()
    }
}

fn read_folders(path: &Path) -> Result<Vec<BridgeProjectFolder>, String> {
    let metadata = match fs::symlink_metadata(path) {
        Ok(metadata) => metadata,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(Vec::new()),
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
    if !opened_metadata.is_file() || opened_metadata.len() > MAX_CATALOG_BYTES {
        return Err(
            "project folder catalog is not a regular file within the size limit".to_string(),
        );
    }
    let bytes = read_bounded_catalog(file, MAX_CATALOG_BYTES)
        .map_err(|error| format!("read project folder catalog failed: {error}"))?;
    let decoded = serde_json::from_slice::<Vec<BridgeProjectFolder>>(&bytes)
        .map_err(|_| "project folder catalog is not valid JSON".to_string())?;
    if decoded.len() > MAX_PROJECTS {
        return Err("project folder catalog contains too many entries".to_string());
    }
    let mut folders = Vec::with_capacity(decoded.len().min(MAX_PROJECTS));
    for folder in decoded {
        let root = normalize_root(&folder.root)?;
        if let Some(title) = folder.title.as_ref() {
            if title.chars().count() > MAX_TITLE_CHARS {
                return Err("project folder title is too long".to_string());
            }
        }
        if folders
            .iter()
            .any(|existing: &BridgeProjectFolder| project_key(&existing.root) == project_key(&root))
        {
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
    Ok(folders)
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

fn write_folders(path: &Path, folders: &[BridgeProjectFolder]) -> Result<(), String> {
    let encoded = serde_json::to_vec(folders)
        .map_err(|error| format!("encode project folder catalog: {error}"))?;
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
