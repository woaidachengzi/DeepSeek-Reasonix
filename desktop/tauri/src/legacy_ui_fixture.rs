//! Controlled source for opt-in packaged UI migration acceptance. The actual
//! legacy reader seeds only a verified nonpersistent WKWebsiteDataStore.
use std::{
    collections::BTreeMap,
    fs::{self, File},
    io::{Read, Write},
    os::unix::fs::{MetadataExt, PermissionsExt},
    path::{Path, PathBuf},
    sync::atomic::{AtomicUsize, Ordering},
};

use serde::Deserialize;

const FLAG: &str = "REASONIX_TAURI_LEGACY_UI_SMOKE";
const FILE: &str = "reasonix-native-legacy-ui-source.json";
static READS: AtomicUsize = AtomicUsize::new(0);

extern "C" {
    fn geteuid() -> u32;
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Control {
    nonce: String,
    workspace: String,
}

pub struct Fixture {
    directory: PathBuf,
    pub values: BTreeMap<String, String>,
}

pub fn load() -> Result<Option<Fixture>, String> {
    match std::env::var_os(FLAG) {
        None => return Ok(None),
        Some(value) if value == "private-source" => (),
        _ => return Err("Invalid private UI migration mode".into()),
    }
    let directory = std::env::var_os("TMPDIR").ok_or("Private UI migration TMPDIR missing")?;
    read(Path::new(&directory)).map(Some)
}

fn private(path: &Path, directory: bool) -> Result<fs::Metadata, String> {
    let meta = fs::symlink_metadata(path).map_err(|_| "Private UI fixture missing")?;
    // SAFETY: geteuid has no arguments or side effects and is available on macOS.
    let uid = unsafe { geteuid() };
    if meta.file_type().is_symlink()
        || meta.uid() != uid
        || (directory && !meta.is_dir())
        || (!directory && !meta.is_file())
        || meta.permissions().mode() & 0o777 != if directory { 0o700 } else { 0o600 }
    {
        return Err("UI migration fixture must be an owned private ordinary file/directory".into());
    }
    Ok(meta)
}

fn read(directory: &Path) -> Result<Fixture, String> {
    if !directory.is_absolute()
        || directory.file_name().and_then(|name| name.to_str()) != Some("tmp")
    {
        return Err("UI migration fixture must use its private tmp directory".into());
    }
    let root = directory.parent().ok_or("UI migration root missing")?;
    if !root
        .file_name()
        .and_then(|name| name.to_str())
        .is_some_and(|name| name.starts_with("reasonix-ui-migration-"))
        || fs::canonicalize(directory).map_err(|_| "UI migration tmp unavailable")? != directory
    {
        return Err("UI migration fixture root is not an isolated canonical test directory".into());
    }
    private(root, true)?;
    private(directory, true)?;
    let source = directory.join(FILE);
    let original = private(&source, false)?;
    let file = File::open(&source).map_err(|_| "Private UI source unavailable")?;
    let opened = file
        .metadata()
        .map_err(|_| "Private UI source metadata unavailable")?;
    if opened.dev() != original.dev() || opened.ino() != original.ino() || opened.len() > 8192 {
        return Err("Private UI source changed or exceeds bounds".into());
    }
    let mut bytes = Vec::new();
    file.take(8193)
        .read_to_end(&mut bytes)
        .map_err(|_| "Private UI source unreadable")?;
    if bytes.len() > 8192 {
        return Err("Private UI source exceeds bounds".into());
    }
    let control: Control =
        serde_json::from_slice(&bytes).map_err(|_| "Private UI source invalid")?;
    let workspace = root.join("旧界面 工作区");
    if control.nonce.len() != 32
        || !control.nonce.bytes().all(|byte| byte.is_ascii_hexdigit())
        || Path::new(&control.workspace) != workspace
    {
        return Err("Private UI source identity/workspace invalid".into());
    }
    private(&workspace, true)?;
    Ok(Fixture {
        directory: directory.to_path_buf(),
        values: BTreeMap::from([
            (
                "reasonix.tauri.default-workspace.v1".into(),
                control.workspace,
            ),
            ("tauri-progress-mode".into(), "deep".into()),
            ("reasonix-text-size".into(), "large".into()),
        ]),
    })
}

impl Fixture {
    pub fn seed_script(&self) -> Result<String, String> {
        let values = serde_json::to_string(&self.values).map_err(|_| "Encode private UI source")?;
        Ok(format!("if (localStorage.length !== 0) throw new Error('private-source-not-empty'); for (const [key, value] of Object.entries({values})) localStorage.setItem(key, value);"))
    }

    pub fn receipt(&self, values: &BTreeMap<String, String>) -> Result<(), String> {
        if values != &self.values {
            return Err("Private WKWebView source did not return the exact fixture".into());
        }
        // No paths, preference values or token are written to this receipt.
        let mut file = tempfile::NamedTempFile::new_in(&self.directory)
            .map_err(|_| "Create private UI receipt")?;
        file.as_file()
            .set_permissions(fs::Permissions::from_mode(0o600))
            .map_err(|_| "Protect private UI receipt")?;
        let reads = READS.fetch_add(1, Ordering::SeqCst) + 1;
        let bytes = serde_json::to_vec(
            &serde_json::json!({"nonPersistent":true,"readCount":reads,"valueCount":values.len()}),
        )
        .map_err(|_| "Encode private UI receipt")?;
        file.write_all(&bytes)
            .map_err(|_| "Write private UI receipt")?;
        file.persist(
            self.directory
                .join("reasonix-native-legacy-ui-receipt.json"),
        )
        .map_err(|_| "Publish private UI receipt")?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn private_source_rejects_public_paths_symlinks_and_foreign_workspace() {
        let root = tempfile::Builder::new()
            .prefix("reasonix-ui-migration-")
            .tempdir_in("/private/tmp")
            .unwrap();
        fs::set_permissions(root.path(), fs::Permissions::from_mode(0o700)).unwrap();
        let directory = root.path().join("tmp");
        let workspace = root.path().join("旧界面 工作区");
        for path in [&directory, &workspace] {
            fs::create_dir(path).unwrap();
            fs::set_permissions(path, fs::Permissions::from_mode(0o700)).unwrap();
        }
        let source = directory.join(FILE);
        let data = serde_json::json!({"nonce":"a".repeat(32),"workspace":workspace});
        fs::write(&source, data.to_string()).unwrap();
        fs::set_permissions(&source, fs::Permissions::from_mode(0o600)).unwrap();
        let fixture = read(&directory).unwrap();
        assert_eq!(fixture.values.len(), 3);
        let original = fs::read(&source).unwrap();
        fixture.receipt(&fixture.values).unwrap();
        assert_eq!(fs::read(&source).unwrap(), original);
        assert!(fixture.receipt(&BTreeMap::new()).is_err());
        fs::set_permissions(&source, fs::Permissions::from_mode(0o644)).unwrap();
        assert!(read(&directory).is_err());
        fs::remove_file(&source).unwrap();
        std::os::unix::fs::symlink(
            directory.join("reasonix-native-legacy-ui-receipt.json"),
            &source,
        )
        .unwrap();
        assert!(read(&directory).is_err());
        fs::remove_file(&source).unwrap();
        fs::write(
            &source,
            serde_json::json!({"nonce":"a".repeat(32),"workspace":"/private/tmp"}).to_string(),
        )
        .unwrap();
        fs::set_permissions(&source, fs::Permissions::from_mode(0o600)).unwrap();
        assert!(read(&directory).is_err());
        fs::write(&source, data.to_string()).unwrap();
        fs::set_permissions(&directory, fs::Permissions::from_mode(0o755)).unwrap();
        assert!(read(&directory).is_err());
    }
}
