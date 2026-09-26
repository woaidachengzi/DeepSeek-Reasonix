use std::{
    fs,
    io::{Read, Write},
    path::{Path, PathBuf},
    sync::Mutex,
};

use serde::{Deserialize, Serialize};
use tauri::Manager;

const CATALOG_FILE: &str = "workbench-sessions.json";
const MAX_SESSIONS: usize = 50;
const MAX_SESSION_ID_LEN: usize = 128;
const MAX_WORKSPACE_ROOT_LEN: usize = 4096;
const MAX_SESSION_TITLE_CHARS: usize = 120;
const MAX_CATALOG_BYTES: u64 = 1024 * 1024;

/// A reopenable session known to the Tauri Preview workbench. This catalog is
/// host UI state, kept outside both the stable profile and Go core session data.
#[derive(Clone, Debug, Deserialize, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkbenchSession {
    pub session_id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub workspace_root: Option<String>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkbenchTitle {
    pub session_id: String,
    pub title: String,
}

pub struct WorkbenchCatalog {
    path: PathBuf,
    state: Mutex<CatalogState>,
}

struct CatalogState {
    sessions: Vec<WorkbenchSession>,
    load_error: Option<String>,
}

impl CatalogState {
    fn ensure_loaded(&self) -> Result<(), String> {
        self.load_error.clone().map_or(Ok(()), Err)
    }
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
        let (sessions, load_error) = match read_sessions(&path) {
            Ok(sessions) => (sessions, None),
            Err(error) => (Vec::new(), Some(error)),
        };
        Self {
            path,
            state: Mutex::new(CatalogState {
                sessions,
                load_error,
            }),
        }
    }

    pub fn path(&self) -> &Path {
        &self.path
    }

    pub fn list(&self) -> Result<Vec<WorkbenchSession>, String> {
        let state = self
            .state
            .lock()
            .map_err(|_| "workbench catalog lock is unavailable".to_string())?;
        state.ensure_loaded()?;
        Ok(state.sessions.clone())
    }

    pub fn remember(&self, mut session: WorkbenchSession) -> Result<Vec<WorkbenchSession>, String> {
        validate_session(&session)?;
        let mut state = self
            .state
            .lock()
            .map_err(|_| "workbench catalog lock is unavailable".to_string())?;
        state.ensure_loaded()?;
        // Opening an existing row only refreshes its metadata. Reordering on
        // every activate makes project folders jump under the cursor.
        if let Some(index) = state
            .sessions
            .iter()
            .position(|existing| existing.session_id == session.session_id)
        {
            if session.title.is_none() {
                session.title = state.sessions[index].title.clone();
            }
            let mut updated = state.sessions.clone();
            updated[index] = session;
            write_sessions(&self.path, &updated)?;
            state.sessions = updated;
            return Ok(state.sessions.clone());
        }
        let mut updated = Vec::with_capacity(MAX_SESSIONS);
        updated.push(session);
        updated.extend(state.sessions.iter().take(MAX_SESSIONS - 1).cloned());
        write_sessions(&self.path, &updated)?;
        state.sessions = updated.clone();
        Ok(updated)
    }

    /// Removes a session from the host-owned recent-session catalog. The core
    /// artifacts are deleted separately by the bridge; this catalog update is
    /// intentionally idempotent so a retry can finish a partially completed
    /// delete without resurrecting the row.
    pub fn forget(&self, session_id: &str) -> Result<Vec<WorkbenchSession>, String> {
        let session_id = session_id.trim();
        validate_session(&WorkbenchSession {
            session_id: session_id.to_string(),
            title: None,
            workspace_root: None,
        })?;
        let mut state = self
            .state
            .lock()
            .map_err(|_| "workbench catalog lock is unavailable".to_string())?;
        state.ensure_loaded()?;
        let updated: Vec<_> = state
            .sessions
            .iter()
            .filter(|session| session.session_id != session_id)
            .cloned()
            .collect();
        if updated.as_slice() != state.sessions.as_slice() {
            write_sessions(&self.path, &updated)?;
            state.sessions = updated.clone();
        }
        Ok(updated)
    }

    /// Fill missing legacy titles without changing the recent-use order or
    /// overwriting explicit/manual titles (including a concurrent rename).
    pub fn fill_titles(
        &self,
        titles: Vec<WorkbenchTitle>,
    ) -> Result<Vec<WorkbenchSession>, String> {
        let mut state = self
            .state
            .lock()
            .map_err(|_| "workbench catalog lock is unavailable".to_string())?;
        state.ensure_loaded()?;
        let mut updated = state.sessions.clone();
        for item in titles {
            validate_session(&WorkbenchSession {
                session_id: item.session_id.clone(),
                title: Some(item.title.clone()),
                workspace_root: None,
            })?;
            if let Some(existing) = updated
                .iter_mut()
                .find(|entry| entry.session_id == item.session_id && entry.title.is_none())
            {
                existing.title = Some(item.title);
            }
        }
        if updated != state.sessions {
            write_sessions(&self.path, &updated)?;
            state.sessions = updated;
        }
        Ok(state.sessions.clone())
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
    if let Some(title) = session.title.as_deref() {
        if title.chars().count() > MAX_SESSION_TITLE_CHARS || title.chars().any(char::is_control) {
            return Err("workbench session title is invalid".to_string());
        }
    }
    Ok(())
}

fn read_sessions(path: &Path) -> Result<Vec<WorkbenchSession>, String> {
    let metadata = match fs::symlink_metadata(path) {
        Ok(metadata) => metadata,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(Vec::new()),
        Err(error) => return Err(format!("inspect workbench session catalog failed: {error}")),
    };
    if metadata.file_type().is_symlink()
        || !metadata.is_file()
        || metadata.len() > MAX_CATALOG_BYTES
    {
        return Err("workbench session catalog is not a regular file within the size limit".into());
    }
    let file = fs::File::open(path)
        .map_err(|error| format!("read workbench session catalog failed: {error}"))?;
    let opened_metadata = file
        .metadata()
        .map_err(|error| format!("inspect opened workbench session catalog failed: {error}"))?;
    if !opened_metadata.is_file() || opened_metadata.len() > MAX_CATALOG_BYTES {
        return Err("workbench session catalog is not a regular file within the size limit".into());
    }
    let mut encoded = Vec::new();
    file.take(MAX_CATALOG_BYTES.saturating_add(1))
        .read_to_end(&mut encoded)
        .map_err(|error| format!("read workbench session catalog failed: {error}"))?;
    if encoded.len() as u64 > MAX_CATALOG_BYTES {
        return Err("workbench session catalog exceeds the size limit".to_string());
    }
    let decoded = serde_json::from_slice::<Vec<WorkbenchSession>>(&encoded)
        .map_err(|_| "workbench session catalog is not valid JSON".to_string())?;
    if decoded.len() > MAX_SESSIONS {
        return Err("workbench session catalog contains too many sessions".to_string());
    }

    let mut sessions = Vec::with_capacity(MAX_SESSIONS);
    for session in decoded {
        validate_session(&session)
            .map_err(|_| "workbench session catalog contains an invalid session".to_string())?;
        if sessions
            .iter()
            .any(|existing: &WorkbenchSession| existing.session_id == session.session_id)
        {
            return Err("workbench session catalog contains duplicate session IDs".to_string());
        }
        sessions.push(session);
    }
    Ok(sessions)
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
            title: None,
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
                title: None,
                workspace_root: Some("/work/project".to_string()),
            })
            .expect("remember second");
        catalog
            .remember(session("one"))
            .expect("reopen first in place");

        // New rows prepend; reopening an existing row does not move it.
        let expected = vec![
            WorkbenchSession {
                session_id: "two".to_string(),
                title: None,
                workspace_root: Some("/work/project".to_string()),
            },
            session("one"),
        ];
        assert_eq!(catalog.list().expect("list sessions"), expected);
        assert_eq!(WorkbenchCatalog::at(path).list().expect("reload"), expected);
    }

    #[test]
    fn reopening_existing_session_keeps_sidebar_order() {
        let root = tempfile::tempdir().expect("temp root");
        let path = root.path().join(CATALOG_FILE);
        let catalog = WorkbenchCatalog::at(path.clone());
        catalog
            .remember(WorkbenchSession {
                session_id: "older".into(),
                title: Some("Older".into()),
                workspace_root: Some("/work/a".into()),
            })
            .expect("older");
        catalog
            .remember(WorkbenchSession {
                session_id: "newer".into(),
                title: Some("Newer".into()),
                workspace_root: Some("/work/b".into()),
            })
            .expect("newer");
        let after_insert = catalog.list().expect("list after insert");
        assert_eq!(
            after_insert
                .iter()
                .map(|entry| entry.session_id.as_str())
                .collect::<Vec<_>>(),
            vec!["newer", "older"]
        );
        let updated = catalog
            .remember(WorkbenchSession {
                session_id: "older".into(),
                title: Some("Still older title".into()),
                workspace_root: Some("/work/a".into()),
            })
            .expect("reopen older");
        assert_eq!(
            updated
                .iter()
                .map(|entry| entry.session_id.as_str())
                .collect::<Vec<_>>(),
            vec!["newer", "older"]
        );
        assert_eq!(updated[1].title.as_deref(), Some("Still older title"));
    }

    #[test]
    fn failed_existing_session_update_does_not_change_in_memory_catalog() {
        let root = tempfile::tempdir().expect("temp root");
        let path = root.path().join(CATALOG_FILE);
        let catalog = WorkbenchCatalog::at(path.clone());
        catalog
            .remember(WorkbenchSession {
                session_id: "stable".into(),
                title: Some("Original title".into()),
                workspace_root: None,
            })
            .expect("remember initial session");

        // Make the final atomic rename fail after the catalog has loaded.
        fs::remove_file(&path).expect("remove catalog destination");
        fs::create_dir(&path).expect("replace destination with directory");

        let result = catalog.remember(WorkbenchSession {
            session_id: "stable".into(),
            title: Some("Uncommitted title".into()),
            workspace_root: None,
        });
        assert!(result.is_err(), "destination directory must reject persist");
        assert_eq!(
            catalog.list().expect("list after failed write"),
            vec![WorkbenchSession {
                session_id: "stable".into(),
                title: Some("Original title".into()),
                workspace_root: None,
            }]
        );
    }

    #[test]
    fn forgets_deleted_session_and_persists_after_reload() {
        let root = tempfile::tempdir().expect("temp root");
        let path = root.path().join(CATALOG_FILE);
        let catalog = WorkbenchCatalog::at(path.clone());
        catalog.remember(session("one")).expect("remember first");
        catalog.remember(session("two")).expect("remember second");

        let expected = vec![session("two")];
        assert_eq!(catalog.forget("one").expect("forget first"), expected);
        assert_eq!(WorkbenchCatalog::at(path).list().expect("reload"), expected);
        assert_eq!(catalog.forget("one").expect("idempotent forget"), expected);
    }

    #[test]
    fn rejects_unsafe_session_ids_and_workspace_paths() {
        let root = tempfile::tempdir().expect("temp root");
        let catalog = WorkbenchCatalog::at(root.path().join(CATALOG_FILE));
        assert!(catalog.remember(session("../outside")).is_err());
        assert!(catalog
            .remember(WorkbenchSession {
                session_id: "valid".to_string(),
                title: None,
                workspace_root: Some("/work\nproject".to_string()),
            })
            .is_err());
        assert!(catalog.list().expect("list unchanged").is_empty());
    }

    #[test]
    fn accepts_legacy_entries_without_a_title_and_rejects_invalid_titles() {
        let root = tempfile::tempdir().expect("temp root");
        let path = root.path().join(CATALOG_FILE);
        fs::write(&path, br#"[{"sessionId":"legacy"}]"#).expect("write legacy catalog");
        let catalog = WorkbenchCatalog::at(path);
        assert_eq!(
            catalog.list().expect("legacy entry"),
            vec![session("legacy")]
        );

        assert!(catalog
            .remember(WorkbenchSession {
                session_id: "new-session".to_string(),
                title: Some("bad\ntitle".to_string()),
                workspace_root: None,
            })
            .is_err());
    }

    #[test]
    fn fills_only_missing_titles_without_reordering_or_losing_them_on_open() {
        let root = tempfile::tempdir().expect("temp root");
        let path = root.path().join(CATALOG_FILE);
        let catalog = WorkbenchCatalog::at(path.clone());
        catalog.remember(session("older")).expect("older");
        catalog
            .remember(WorkbenchSession {
                session_id: "manual".into(),
                title: Some("My title".into()),
                workspace_root: None,
            })
            .expect("manual");
        let filled = catalog
            .fill_titles(vec![
                WorkbenchTitle {
                    session_id: "older".into(),
                    title: "First question".into(),
                },
                WorkbenchTitle {
                    session_id: "manual".into(),
                    title: "Wrong title".into(),
                },
                WorkbenchTitle {
                    session_id: "unknown".into(),
                    title: "Ignored".into(),
                },
            ])
            .expect("fill");
        assert_eq!(
            filled
                .iter()
                .map(|entry| entry.session_id.as_str())
                .collect::<Vec<_>>(),
            vec!["manual", "older"]
        );
        assert_eq!(filled[0].title.as_deref(), Some("My title"));
        assert_eq!(filled[1].title.as_deref(), Some("First question"));
        let reopened = catalog.remember(session("older")).expect("reopen");
        assert_eq!(
            reopened
                .iter()
                .map(|entry| entry.session_id.as_str())
                .collect::<Vec<_>>(),
            vec!["manual", "older"]
        );
        assert_eq!(reopened[1].title.as_deref(), Some("First question"));
        assert_eq!(
            WorkbenchCatalog::at(path).list().expect("reload")[1]
                .title
                .as_deref(),
            Some("First question")
        );
    }

    #[test]
    fn caps_catalog_and_fails_closed_on_corrupt_files() {
        let root = tempfile::tempdir().expect("temp root");
        let path = root.path().join(CATALOG_FILE);
        let catalog = WorkbenchCatalog::at(path.clone());
        for index in 0..(MAX_SESSIONS + 3) {
            catalog
                .remember(session(&format!("session-{index}")))
                .expect("remember bounded session");
        }
        assert_eq!(catalog.list().expect("list").len(), MAX_SESSIONS);

        let corrupt = b"not json";
        fs::write(&path, corrupt).expect("write corrupt catalog");
        let catalog = WorkbenchCatalog::at(path.clone());
        assert_eq!(
            catalog.list(),
            Err("workbench session catalog is not valid JSON".to_string())
        );
        assert!(catalog.remember(session("new-session")).is_err());
        assert_eq!(fs::read(path).expect("preserve corrupt catalog"), corrupt);
    }

    #[test]
    fn oversized_catalog_fails_closed_without_truncating_source() {
        let root = tempfile::tempdir().expect("temp root");
        let path = root.path().join(CATALOG_FILE);
        let sessions: Vec<_> = (0..=MAX_SESSIONS)
            .map(|index| session(&format!("session-{index}")))
            .collect();
        let encoded = serde_json::to_vec(&sessions).expect("encode oversized catalog");
        fs::write(&path, &encoded).expect("write oversized catalog");

        let catalog = WorkbenchCatalog::at(path.clone());
        assert_eq!(
            catalog.list(),
            Err("workbench session catalog contains too many sessions".to_string())
        );
        assert!(catalog.remember(session("new-session")).is_err());
        assert_eq!(fs::read(path).expect("preserve oversized catalog"), encoded);
    }

    #[test]
    fn invalid_or_duplicate_catalog_rows_fail_closed_without_dropping_source() {
        let root = tempfile::tempdir().expect("temp root");
        let path = root.path().join(CATALOG_FILE);
        let cases = [
            vec![WorkbenchSession {
                session_id: "invalid-title".into(),
                title: Some("bad\ntitle".into()),
                workspace_root: None,
            }],
            vec![WorkbenchSession {
                session_id: "invalid-workspace".into(),
                title: None,
                workspace_root: Some("/work\nproject".into()),
            }],
            vec![session("duplicate"), session("duplicate")],
        ];

        for rows in cases {
            let encoded = serde_json::to_vec(&rows).expect("encode invalid catalog");
            fs::write(&path, &encoded).expect("write invalid catalog");
            let catalog = WorkbenchCatalog::at(path.clone());
            assert!(catalog.list().is_err(), "invalid catalog loaded: {rows:?}");
            assert!(catalog.remember(session("new-session")).is_err());
            assert_eq!(fs::read(&path).expect("preserve invalid catalog"), encoded);
        }
    }
}
