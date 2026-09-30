//! Opaque, durable credential identity for one Preview core profile. This file
//! contains no secrets and survives profile moves. Existing invalid identities
//! are never replaced with a fresh identity that would orphan saved keys.
use rand::TryRngCore;
use serde::{Deserialize, Serialize};
use std::{
    fs,
    io::{Read, Write},
    path::Path,
};

const FILE: &str = "tauri-credential-profile.json";
const BACKUP: &str = "tauri-credential-profile.backup.json";
const LIMIT: u64 = 512;
const INVALID: &str = "credential profile identity is unavailable; restore its metadata backup or check profile permissions";

#[derive(Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
struct Identity {
    version: u8,
    id: String,
}

fn read_identity(path: &Path) -> Result<Option<Identity>, String> {
    let meta = match fs::symlink_metadata(path) {
        Ok(meta) => meta,
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(_) => return Err(INVALID.into()),
    };
    if !meta.is_file() || meta.file_type().is_symlink() || meta.len() > LIMIT {
        return Err(INVALID.into());
    }
    let file = fs::File::open(path).map_err(|_| INVALID)?;
    if !crate::workbench_projects::same_file_as_path(path, &file).map_err(|_| INVALID)? {
        return Err(INVALID.into());
    }
    let current = fs::symlink_metadata(path).map_err(|_| INVALID)?;
    if current.file_type().is_symlink() || !current.is_file() {
        return Err(INVALID.into());
    }
    let mut bytes = Vec::new();
    file.take(LIMIT + 1)
        .read_to_end(&mut bytes)
        .map_err(|_| INVALID)?;
    if bytes.len() as u64 > LIMIT {
        return Err(INVALID.into());
    }
    let identity: Identity = serde_json::from_slice(&bytes).map_err(|_| INVALID)?;
    if identity.version != 1
        || identity.id.len() != 32
        || !identity
            .id
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
    {
        return Err(INVALID.into());
    }
    Ok(Some(identity))
}

fn persist_identity(path: &Path, identity: &Identity) -> Result<(), String> {
    let mut temp =
        tempfile::NamedTempFile::new_in(path.parent().ok_or(INVALID)?).map_err(|_| INVALID)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        temp.as_file()
            .set_permissions(fs::Permissions::from_mode(0o600))
            .map_err(|_| INVALID)?;
    }
    let bytes = serde_json::to_vec(identity).map_err(|_| INVALID)?;
    temp.write_all(&bytes).map_err(|_| INVALID)?;
    temp.as_file().sync_all().map_err(|_| INVALID)?;
    temp.persist_noclobber(path)
        .map(|_| ())
        .map_err(|_| INVALID.into())
}

pub(crate) fn service_for_profile(home: &Path) -> Result<String, String> {
    fs::create_dir_all(home).map_err(|_| INVALID)?;
    let home = home.canonicalize().map_err(|_| INVALID)?;
    if !home.is_dir() {
        return Err(INVALID.into());
    }
    let path = home.join(FILE);
    let backup = home.join(BACKUP);
    let identity = match read_identity(&path) {
        Ok(Some(identity)) => identity,
        Err(_) => read_identity(&backup)?.ok_or(INVALID)?,
        Ok(None) => {
            let candidate = match read_identity(&backup)? {
                Some(identity) => identity,
                None => {
                    let mut bytes = [0u8; 16];
                    rand::rngs::OsRng
                        .try_fill_bytes(&mut bytes)
                        .map_err(|_| INVALID)?;
                    Identity {
                        version: 1,
                        id: bytes.iter().map(|byte| format!("{byte:02x}")).collect(),
                    }
                }
            };
            // A concurrent first launch may have won; adopt the winner. A
            // failed write must never silently choose a process-only identity.
            let _ = persist_identity(&path, &candidate);
            read_identity(&path)?.ok_or(INVALID)?
        }
    };
    match read_identity(&backup) {
        Ok(Some(saved)) if saved.id != identity.id => return Err(INVALID.into()),
        Ok(None) => {
            let _ = persist_identity(&backup, &identity);
        }
        _ => {}
    }
    Ok(format!("com.reasonix.desktop.profile.{}", identity.id))
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn profiles_are_distinct_restart_stable_and_survive_moves_with_backup_recovery() {
        let root = tempfile::tempdir().unwrap();
        let first = root.path().join("first");
        let other = root.path().join("other");
        let id = service_for_profile(&first).unwrap();
        assert_ne!(id, service_for_profile(&other).unwrap());
        assert_eq!(id, service_for_profile(&first).unwrap());
        let other_backup = fs::read(other.join(BACKUP)).unwrap();
        let first_backup = fs::read(first.join(BACKUP)).unwrap();
        fs::write(first.join(BACKUP), &other_backup).unwrap();
        assert!(
            service_for_profile(&first).is_err(),
            "conflicting valid identities must not choose an arbitrary store"
        );
        assert_eq!(fs::read(first.join(BACKUP)).unwrap(), other_backup);
        fs::write(first.join(BACKUP), first_backup).unwrap();
        let moved = root.path().join("moved");
        fs::rename(&first, &moved).unwrap();
        assert_eq!(id, service_for_profile(&moved).unwrap());
        fs::write(moved.join(FILE), "bad metadata").unwrap();
        assert_eq!(id, service_for_profile(&moved).unwrap());
        assert_eq!(
            fs::read_to_string(moved.join(FILE)).unwrap(),
            "bad metadata",
            "recovery does not overwrite damaged primary metadata"
        );
        fs::remove_file(moved.join(FILE)).unwrap();
        assert_eq!(id, service_for_profile(&moved).unwrap());
        fs::write(moved.join(FILE), "bad metadata").unwrap();
        fs::write(moved.join(BACKUP), "bad backup").unwrap();
        assert!(service_for_profile(&moved).is_err());
        assert_eq!(
            fs::read_to_string(moved.join(BACKUP)).unwrap(),
            "bad backup"
        );
    }
    #[test]
    fn concurrent_first_launch_adopts_one_identity_without_lost_backup() {
        let root = tempfile::tempdir().unwrap();
        let home = root.path().join("profile");
        let results = std::thread::scope(|scope| {
            let workers: Vec<_> = (0..8)
                .map(|_| scope.spawn(|| service_for_profile(&home)))
                .collect();
            workers
                .into_iter()
                .map(|worker| worker.join().unwrap())
                .collect::<Vec<_>>()
        });
        let expected = service_for_profile(&home).unwrap();
        for result in results {
            assert_eq!(result.unwrap(), expected);
        }
        assert_eq!(
            read_identity(&home.join(FILE)).unwrap().unwrap().id,
            read_identity(&home.join(BACKUP)).unwrap().unwrap().id
        );
    }
    #[cfg(unix)]
    #[test]
    fn aliases_share_an_identity_and_credential_metadata_is_private() {
        use std::os::unix::{fs::symlink, fs::PermissionsExt};
        let root = tempfile::tempdir().unwrap();
        let home = root.path().join("profile");
        let expected = service_for_profile(&home).unwrap();
        let alias = root.path().join("alias");
        symlink(&home, &alias).unwrap();
        assert_eq!(service_for_profile(&alias).unwrap(), expected);
        assert_eq!(
            fs::metadata(home.join(FILE)).unwrap().permissions().mode() & 0o777,
            0o600
        );
        fs::remove_file(home.join(FILE)).unwrap();
        fs::remove_file(home.join(BACKUP)).unwrap();
        let target = root.path().join("foreign.json");
        fs::write(
            &target,
            r#"{"version":1,"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}"#,
        )
        .unwrap();
        symlink(&target, home.join(FILE)).unwrap();
        assert!(service_for_profile(&home).is_err());
        assert!(fs::symlink_metadata(home.join(FILE))
            .unwrap()
            .file_type()
            .is_symlink());
    }
}
