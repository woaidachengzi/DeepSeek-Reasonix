use std::{
    fs,
    io::Write,
    path::{Path, PathBuf},
    sync::Mutex,
};

use serde::{Deserialize, Serialize};
use tauri::Manager;

const CATALOG_FILE: &str = "workbench-sessions.json";
const MAX_SESSIONS: usize = 50;
const MAX_SESSION_ID_LEN: usize = 128;
const MAX_WORKSPACE_ROOT_LEN: usize = 4096;

/// A reopenable session known to the Tauri Preview workbench. This catalog is
/// host UI state, kept outside both the stable profile and Go core session data.
#[derive(Clone, Debug, Deserialize, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkbenchSession {
    pub session_id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub workspace_root: Option<String>,
}

pub struct WorkbenchCatalog {
    path: PathBuf,
    sessions: Mutex<Vec<WorkbenchSession>>,
}

impl WorkbenchCatalog {
    pub fn for_app(app: &tauri::App) -> Result<Self, String> {
        let directory = app
            .path()
            .app_data_dir()
            .map_err(|error| format!("resolve Tauri workbench catalog directory: {error}"))?;
        fs::create_dir_all(&directory).map_err(|error| {
            format!(
                "create Tauri workbench catalog directory {}: {error}",
                directory.display()
            )
        })?;
        Ok(Self::at(directory.join(CATALOG_FILE)))
    }

    fn at(path: PathBuf) -> Self {
        let sessions = read_sessions(&path);
        Self {
            path,
            sessions: Mutex::new(sessions),
        }
    }

    pub fn list(&self) -> Result<Vec<WorkbenchSession>, String> {
        self.sessions
            .lock()
            .map(|sessions| sessions.clone())
            .map_err(|_| "workbench catalog lock is unavailable".to_string())
    }

    pub fn remember(&self, session: WorkbenchSession) -> Result<Vec<WorkbenchSession>, String> {
        validate_session(&session)?;
        let mut sessions = self
            .sessions
            .lock()
            .map_err(|_| "workbench catalog lock is unavailable".to_string())?;
        let mut updated = Vec::with_capacity(MAX_SESSIONS);
        updated.push(session.clone());
        updated.extend(
            sessions
                .iter()
                .filter(|existing| existing.session_id != session.session_id)
                .take(MAX_SESSIONS - 1)
                .cloned(),
        );
        write_sessions(&self.path, &updated)?;
        *sessions = updated.clone();
        Ok(updated)
    }
}

fn validate_session(session: &WorkbenchSession) -> Result<(), String> {
    let id = session.session_id.as_str();
    if id.is_empty()
        || id.len() > MAX_SESSION_ID_LEN
        || !id
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || byte == b'-' || byte == b'_')
    {
        return Err("workbench session identifier is invalid".to_string());
    }
    if let Some(root) = session.workspace_root.as_deref() {
        if root.len() > MAX_WORKSPACE_ROOT_LEN || root.chars().any(char::is_control) {
            return Err("workbench workspace path is invalid".to_string());
        }
    }
    Ok(())
}

fn read_sessions(path: &Path) -> Vec<WorkbenchSession> {
    let Ok(encoded) = fs::read(path) else {
        return Vec::new();
    };
    let Ok(decoded) = serde_json::from_slice::<Vec<WorkbenchSession>>(&encoded) else {
        return Vec::new();
    };

    let mut sessions = Vec::with_capacity(MAX_SESSIONS);
    for session in decoded {
        if validate_session(&session).is_err()
            || sessions
                .iter()
                .any(|existing: &WorkbenchSession| existing.session_id == session.session_id)
        {
            continue;
        }
        sessions.push(session);
        if sessions.len() == MAX_SESSIONS {
            break;
        }
    }
    sessions
}

fn write_sessions(path: &Path, sessions: &[WorkbenchSession]) -> Result<(), String> {
    let encoded = serde_json::to_vec(sessions)
        .map_err(|error| format!("encode workbench session catalog: {error}"))?;
    let directory = path
        .parent()
        .ok_or_else(|| "workbench catalog path has no parent directory".to_string())?;
    let mut temporary = tempfile::Builder::new()
        .prefix(".workbench-sessions-")
        .tempfile_in(directory)
        .map_err(|error| format!("create temporary workbench catalog: {error}"))?;
    temporary
        .write_all(&encoded)
        .map_err(|error| format!("write temporary workbench catalog: {error}"))?;
    temporary
        .as_file()
        .sync_all()
        .map_err(|error| format!("sync temporary workbench catalog: {error}"))?;
    temporary
        .persist(path)
        .map_err(|error| format!("save workbench catalog {}: {error}", path.display()))?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn session(id: &str) -> WorkbenchSession {
        WorkbenchSession {
            session_id: id.to_string(),
            workspace_root: None,
        }
    }

    #[test]
    fn remembers_recent_sessions_and_survives_reload() {
        let root = tempfile::tempdir().expect("temp root");
        let path = root.path().join(CATALOG_FILE);
        let catalog = WorkbenchCatalog::at(path.clone());
        catalog.remember(session("one")).expect("remember first");
        catalog
            .remember(WorkbenchSession {
                session_id: "two".to_string(),
                workspace_root: Some("/work/project".to_string()),
            })
            .expect("remember second");
        catalog.remember(session("one")).expect("bump first");

        let expected = vec![
            session("one"),
            WorkbenchSession {
                session_id: "two".to_string(),
                workspace_root: Some("/work/project".to_string()),
            },
        ];
        assert_eq!(catalog.list().expect("list sessions"), expected);
        assert_eq!(WorkbenchCatalog::at(path).list().expect("reload"), expected);
    }

    #[test]
    fn rejects_unsafe_session_ids_and_workspace_paths() {
        let root = tempfile::tempdir().expect("temp root");
        let catalog = WorkbenchCatalog::at(root.path().join(CATALOG_FILE));
        assert!(catalog.remember(session("../outside")).is_err());
        assert!(catalog
            .remember(WorkbenchSession {
                session_id: "valid".to_string(),
                workspace_root: Some("/work\nproject".to_string()),
            })
            .is_err());
        assert!(catalog.list().expect("list unchanged").is_empty());
    }

    #[test]
    fn caps_catalog_and_ignores_corrupt_files() {
        let root = tempfile::tempdir().expect("temp root");
        let path = root.path().join(CATALOG_FILE);
        let catalog = WorkbenchCatalog::at(path.clone());
        for index in 0..(MAX_SESSIONS + 3) {
            catalog
                .remember(session(&format!("session-{index}")))
                .expect("remember bounded session");
        }
        assert_eq!(catalog.list().expect("list").len(), MAX_SESSIONS);

        fs::write(&path, b"not json").expect("write corrupt catalog");
        assert!(WorkbenchCatalog::at(path)
            .list()
            .expect("load corrupt catalog")
            .is_empty());
    }
}
