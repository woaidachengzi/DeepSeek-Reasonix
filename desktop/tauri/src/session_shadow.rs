use std::collections::{HashMap, HashSet};

use serde::Serialize;

use crate::{
    bridge::{SessionDirectoryEntry, SessionPhysicalInventory},
    workbench_catalog::WorkbenchSession,
};

// The legacy catalog is only a bounded recent list. Additional SQLite rows
// are expected (and are the reason for this migration), not a comparison
// failure. No IDs, titles, paths, or transcript content enter this report.
#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionShadowReport {
    pub legacy_count: usize,
    pub directory_count: usize,
    pub matched_count: usize,
    pub directory_only_count: usize,
    pub missing_from_directory: usize,
    pub title_mismatches: usize,
    pub workspace_mismatches: usize,
    pub order_mismatches: usize,
    pub missing_transcripts: usize,
    pub physical_state_mismatches: usize,
    pub unclaimed_transcripts: usize,
    pub inventory_errors: usize,
    pub legacy_matches_directory: bool,
}

pub fn compare(
    legacy: &[WorkbenchSession],
    directory: &[SessionDirectoryEntry],
    physical: &SessionPhysicalInventory,
) -> Result<SessionShadowReport, String> {
    let mut by_id = HashMap::with_capacity(directory.len());
    for (index, entry) in directory.iter().enumerate() {
        if by_id.insert(entry.id.as_str(), index).is_some() {
            return Err("session directory contains duplicate IDs".to_string());
        }
    }
    let mut seen_legacy = HashSet::with_capacity(legacy.len());
    let mut missing_from_directory = 0;
    let mut title_mismatches = 0;
    let mut workspace_mismatches = 0;
    let mut missing_transcripts = 0;
    let mut physical_by_id = HashMap::with_capacity(physical.states.len());
    for state in &physical.states {
        if physical_by_id.insert(state.id.as_str(), state).is_some() {
            return Err("session inventory contains duplicate identity IDs".to_string());
        }
    }
    let physical_state_mismatches = directory
        .iter()
        .filter(|entry| {
            let Some(state) = physical_by_id.get(entry.id.as_str()) else {
                return true;
            };
            !state.readable || state.exists != (entry.state == "ready")
        })
        .count();
    let mut legacy_common = Vec::new();
    for session in legacy {
        if !seen_legacy.insert(session.session_id.as_str()) {
            return Err("legacy session catalog contains duplicate IDs".to_string());
        }
        let Some(&index) = by_id.get(session.session_id.as_str()) else {
            missing_from_directory += 1;
            continue;
        };
        let entry = &directory[index];
        legacy_common.push(session.session_id.as_str());
        if session.title.as_deref().unwrap_or("") != entry.title {
            title_mismatches += 1;
        }
        if session.workspace_root.as_deref().unwrap_or("").trim()
            != entry.workspace_root.as_deref().unwrap_or("")
        {
            workspace_mismatches += 1;
        }
        if entry.state == "missing"
            || (entry.state == "ready"
                && physical_by_id
                    .get(entry.id.as_str())
                    .is_some_and(|state| !state.exists))
        {
            missing_transcripts += 1;
        }
    }
    let directory_common: Vec<_> = directory
        .iter()
        .filter(|entry| seen_legacy.contains(entry.id.as_str()))
        .map(|entry| entry.id.as_str())
        .collect();
    let order_mismatches = legacy_common
        .iter()
        .zip(&directory_common)
        .filter(|(left, right)| *left != *right)
        .count();
    let matched_count = legacy_common.len();
    Ok(SessionShadowReport {
        legacy_count: legacy.len(),
        directory_count: directory.len(),
        matched_count,
        directory_only_count: directory.len() - matched_count,
        missing_from_directory,
        title_mismatches,
        workspace_mismatches,
        order_mismatches,
        missing_transcripts,
        physical_state_mismatches,
        unclaimed_transcripts: physical.unclaimed_count,
        inventory_errors: physical.error_count,
        legacy_matches_directory: missing_from_directory == 0
            && title_mismatches == 0
            && workspace_mismatches == 0
            && order_mismatches == 0
            && missing_transcripts == 0
            && physical_state_mismatches == 0
            && physical.unclaimed_count == 0
            && physical.error_count == 0,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::bridge::SessionPhysicalState;

    fn old(id: &str) -> WorkbenchSession {
        WorkbenchSession {
            session_id: id.into(),
            title: None,
            workspace_root: None,
        }
    }

    fn new(id: &str) -> SessionDirectoryEntry {
        SessionDirectoryEntry {
            id: id.into(),
            title: String::new(),
            title_source: "fallback".into(),
            workspace_root: None,
            state: "ready".into(),
            missing: false,
            position: 0,
            updated_at_ms: 0,
        }
    }

    fn physical(states: &[(&str, bool)]) -> SessionPhysicalInventory {
        SessionPhysicalInventory {
            states: states
                .iter()
                .map(|(id, exists)| SessionPhysicalState {
                    id: (*id).into(),
                    exists: *exists,
                    readable: true,
                })
                .collect(),
            unclaimed_count: 0,
            error_count: 0,
        }
    }

    #[test]
    fn additional_directory_rows_do_not_invalidate_the_legacy_subset() {
        let report = compare(
            &[old("recent"), old("older")],
            &[new("recent"), new("extra"), new("older")],
            &physical(&[("recent", true), ("extra", true), ("older", true)]),
        )
        .expect("compare");
        assert_eq!(report.matched_count, 2);
        assert_eq!(report.directory_only_count, 1);
        assert!(report.legacy_matches_directory);
    }

    #[test]
    fn reports_metadata_order_and_file_state_without_private_values() {
        let mut newer = new("newer");
        newer.title = "different".into();
        newer.state = "missing".into();
        newer.missing = true;
        let mut older = new("older");
        older.workspace_root = Some("/different".into());
        let legacy = [
            WorkbenchSession {
                session_id: "newer".into(),
                title: Some("Old title".into()),
                workspace_root: None,
            },
            WorkbenchSession {
                session_id: "older".into(),
                title: None,
                workspace_root: Some("/project".into()),
            },
            old("absent"),
        ];
        let report = compare(
            &legacy,
            &[older, newer],
            &physical(&[("older", true), ("newer", false)]),
        )
        .expect("compare");
        assert_eq!(report.missing_from_directory, 1);
        assert_eq!(report.title_mismatches, 1);
        assert_eq!(report.workspace_mismatches, 1);
        assert_eq!(report.order_mismatches, 2);
        assert_eq!(report.missing_transcripts, 1);
        assert!(!report.legacy_matches_directory);
        let json = serde_json::to_string(&report).expect("serialize report");
        assert!(!json.contains("Old title"));
        assert!(!json.contains("/project"));
        assert!(!json.contains("newer"));
    }

    #[test]
    fn duplicate_ids_fail_closed() {
        let state = physical(&[("same", true)]);
        assert!(compare(&[old("same"), old("same")], &[new("same")], &state).is_err());
        assert!(compare(&[old("same")], &[new("same"), new("same")], &state).is_err());
    }

    #[test]
    fn physical_state_drift_blocks_a_clean_shadow_result() {
        let report = compare(
            &[old("ready")],
            &[new("ready")],
            &physical(&[("ready", false)]),
        )
        .expect("compare");
        assert_eq!(report.physical_state_mismatches, 1);
        assert!(!report.legacy_matches_directory);
    }

    #[test]
    fn incomplete_or_unclaimed_inventory_cannot_look_clean() {
        let mut inventory = physical(&[]);
        inventory.unclaimed_count = 1;
        inventory.error_count = 1;
        let report = compare(&[old("ready")], &[new("ready")], &inventory).expect("compare");
        assert_eq!(report.physical_state_mismatches, 1);
        assert_eq!(report.unclaimed_transcripts, 1);
        assert_eq!(report.inventory_errors, 1);
        assert!(!report.legacy_matches_directory);
    }
}
