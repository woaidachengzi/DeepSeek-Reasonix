//! Native document actions. Renderer input is a path or an installed-app ID,
//! never an executable name or command line. Save destinations come from the
//! OS dialog and are replaced atomically without truncating the source.
use crate::bridge::BridgeSupervisor;
use crate::opener_catalog::{
    cached_views, installed_openers, resolved_preference, selected_opener, OpenersView,
};
use crate::protocol_generated::BridgeWorkspaceTargetResponse;
use std::{
    fs::{self, File},
    io,
    path::{Path, PathBuf},
};
use tauri::Manager;
use tauri_plugin_dialog::DialogExt;
use tauri_plugin_opener::OpenerExt;

fn main_window(window: &tauri::WebviewWindow) -> Result<(), String> {
    if window.label() != "main" {
        return Err("local document actions require the main window".into());
    }
    Ok(())
}

fn unsafe_windows_syntax(value: &str) -> bool {
    let path = value.replace('\\', "/");
    if path.contains('\0')
        || ["//.", "//?"]
            .iter()
            .any(|prefix| path == *prefix || path.starts_with(&format!("{prefix}/")))
    {
        return true;
    }
    let drive = path.as_bytes().get(1) == Some(&b':')
        && path.as_bytes().first().is_some_and(u8::is_ascii_alphabetic);
    if !drive && !path.starts_with("//") {
        return false;
    }
    path[2..].contains(':')
        || path[2..].split('/').any(|part| {
            let base = part
                .trim_end_matches([' ', '.'])
                .split('.')
                .next()
                .unwrap_or("")
                .to_uppercase();
            matches!(
                base.as_str(),
                "CON" | "PRN" | "AUX" | "NUL" | "CLOCK$" | "CONIN$" | "CONOUT$"
            ) || ["COM", "LPT"].iter().any(|prefix| {
                base.strip_prefix(prefix).is_some_and(|suffix| {
                    matches!(
                        suffix,
                        "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9" | "¹" | "²" | "³"
                    )
                })
            })
        })
}

fn absolute_path(value: &str) -> Result<PathBuf, String> {
    if value.trim().is_empty() || value.len() > 32 << 10 || unsafe_windows_syntax(value) {
        return Err("invalid local path; choose an absolute document path".into());
    }
    let path = if value.starts_with("file://") {
        // URL parsing normalizes dot segments. Inspect the decoded authority
        // first so file:////./PhysicalDrive0 cannot become an ordinary UNC path.
        let raw = percent_encoding::percent_decode_str(&value["file:".len()..])
            .decode_utf8()
            .map_err(|_| "invalid local file URL encoding")?
            .replace('\\', "/");
        if raw.starts_with("//")
            && matches!(
                raw.trim_start_matches('/').split('/').next(),
                Some("." | "?")
            )
        {
            return Err("invalid local file URL device authority".into());
        }
        let url = url::Url::parse(value).map_err(|_| "invalid local file URL")?;
        if url.query().is_some()
            || url.fragment().is_some()
            || !url.username().is_empty()
            || url.password().is_some()
        {
            return Err("invalid local file URL".into());
        }
        url.to_file_path()
            .map_err(|_| "file URL is not a local path")?
    } else {
        PathBuf::from(value)
    };
    if !path.is_absolute() || unsafe_windows_syntax(&path.to_string_lossy()) {
        return Err("invalid local path; choose an absolute document path".into());
    }
    Ok(path)
}

fn executable_suffix(path: &Path) -> bool {
    let base = path.file_name().unwrap_or_default().to_string_lossy();
    let base = base.trim_end_matches([' ', '.']);
    matches!(
        Path::new(base)
            .extension()
            .unwrap_or_default()
            .to_string_lossy()
            .to_ascii_lowercase()
            .as_str(),
        "app"
            | "bat"
            | "cmd"
            | "com"
            | "exe"
            | "desktop"
            | "ps1"
            | "vbs"
            | "jse"
            | "js"
            | "lnk"
            | "url"
            | "scr"
            | "msi"
            | "reg"
            | "pif"
            | "hta"
            | "wsf"
    )
}

fn document_path(value: &str, for_open: bool) -> Result<PathBuf, String> {
    let path = absolute_path(value)?;
    let resolved = path.canonicalize().map_err(|error| {
        format!("cannot access document: {error}; check the path and permissions")
    })?;
    let metadata = fs::metadata(&resolved).map_err(|error| error.to_string())?;
    if !metadata.is_file() && !metadata.is_dir() {
        return Err("only regular files and directories can be selected".into());
    }
    #[cfg(unix)]
    let executable_mode = {
        use std::os::unix::fs::PermissionsExt;
        metadata.is_file() && metadata.permissions().mode() & 0o111 != 0
    };
    #[cfg(not(unix))]
    let executable_mode = false;
    if for_open && (executable_suffix(&path) || executable_suffix(&resolved) || executable_mode) {
        return Err("cannot open an executable target from a document link; use its containing folder instead".into());
    }
    Ok(resolved)
}

fn path_text(path: &Path) -> Result<&str, String> {
    path.to_str()
        .ok_or_else(|| "document path is not valid UTF-8; choose a readable path".into())
}

#[tauri::command]
pub async fn open_local_path(window: tauri::WebviewWindow, path: String) -> Result<(), String> {
    main_window(&window)?;
    tauri::async_runtime::spawn_blocking(move || {
        let path = document_path(&path, true)?;
        window
            .opener()
            .open_path(path_text(&path)?, None::<&str>)
            .map_err(|error| error.to_string())
    })
    .await
    .map_err(|error| error.to_string())?
}

#[tauri::command]
pub async fn reveal_local_path(window: tauri::WebviewWindow, path: String) -> Result<(), String> {
    main_window(&window)?;
    tauri::async_runtime::spawn_blocking(move || {
        let path = document_path(&path, false)?;
        window
            .opener()
            .reveal_item_in_dir(path)
            .map_err(|error| error.to_string())
    })
    .await
    .map_err(|error| error.to_string())?
}

fn save_source(path: &str) -> Result<(PathBuf, File), String> {
    let path = document_path(path, false)?;
    path_text(&path)?;
    let file = File::open(&path)
        .map_err(|error| format!("cannot read source: {error}; check file permissions"))?;
    if !file
        .metadata()
        .map_err(|error| error.to_string())?
        .is_file()
    {
        return Err("cannot save a directory as a file".into());
    }
    Ok((path, file))
}

fn destination_is_source(file: &File, target: &Path) -> Result<bool, String> {
    match crate::workbench_projects::same_file_as_path(target, file) {
        Ok(same) => Ok(same),
        Err(error) if error.kind() == io::ErrorKind::NotFound => Ok(false),
        Err(error) => Err(error.to_string()),
    }
}

fn verify_save_source(source: &Path, file: &File) -> Result<(), String> {
    if !crate::workbench_projects::same_file_as_path(source, file).unwrap_or(false) {
        return Err("source file changed; reopen the document and retry saving".into());
    }
    Ok(())
}

fn copy_to_selected_destination(
    source: &Path,
    mut file: File,
    target: Option<PathBuf>,
) -> Result<String, String> {
    let Some(target) = target else {
        return Ok(String::new());
    };
    let target = absolute_path(
        target
            .to_str()
            .ok_or("destination path is not valid UTF-8")?,
    )?;
    verify_save_source(source, &file)?;
    if destination_is_source(&file, &target)? {
        return Err("destination is the source file; choose another path".into());
    }
    if fs::metadata(&target).is_ok_and(|metadata| metadata.is_dir()) {
        return Err("destination is a directory; choose a file path".into());
    }
    let mut temporary = tempfile::NamedTempFile::new_in(
        target
            .parent()
            .ok_or("destination has no parent directory")?,
    )
    .map_err(|error| format!("cannot create destination: {error}; choose a writable folder"))?;
    io::copy(&mut file, &mut temporary)
        .map_err(|error| format!("cannot copy document: {error}"))?;
    temporary
        .as_file()
        .set_permissions(
            file.metadata()
                .map_err(|error| error.to_string())?
                .permissions(),
        )
        .map_err(|error| error.to_string())?;
    temporary
        .as_file()
        .sync_all()
        .map_err(|error| error.to_string())?;
    verify_save_source(source, &file)?;
    if destination_is_source(&file, &target)? {
        return Err("destination is the source file; choose another path".into());
    }
    temporary.persist(&target).map_err(|error| {
        format!(
            "cannot save document: {}; choose a writable destination",
            error.error
        )
    })?;
    Ok(target.to_string_lossy().into_owned())
}

#[tauri::command]
pub async fn save_local_path_as(
    window: tauri::WebviewWindow,
    path: String,
) -> Result<String, String> {
    main_window(&window)?;
    let (source, file) = tauri::async_runtime::spawn_blocking(move || save_source(&path))
        .await
        .map_err(|error| error.to_string())??;
    let (sender, receiver) = std::sync::mpsc::channel();
    window
        .dialog()
        .file()
        .set_title("Save file as")
        .set_directory(source.parent().ok_or("source has no parent")?)
        .set_file_name(
            source
                .file_name()
                .ok_or("source has no filename")?
                .to_string_lossy(),
        )
        .save_file(move |selection| {
            let _ = sender.send(selection);
        });
    tauri::async_runtime::spawn_blocking(move || {
        let selected = receiver
            .recv()
            .map_err(|_| "save dialog closed unexpectedly; retry saving")?
            .map(|selected| selected.into_path().map_err(|error| error.to_string()))
            .transpose()?;
        copy_to_selected_destination(&source, file, selected)
    })
    .await
    .map_err(|error| error.to_string())?
}

#[tauri::command]
pub async fn local_path_openers(window: tauri::WebviewWindow) -> Result<OpenersView, String> {
    main_window(&window)?;
    tauri::async_runtime::spawn_blocking(move || {
        let preferred = window
            .state::<BridgeSupervisor>()
            .desktop_preferences()?
            .external_opener;
        let openers = cached_views();
        let selected = openers
            .iter()
            .find(|view| view.id == preferred)
            .or_else(|| openers.iter().find(|view| view.kind == "file-manager"))
            .or_else(|| openers.first())
            .map(|view| view.id)
            .unwrap_or("");
        Ok(OpenersView {
            preferred: selected.into(),
            openers,
            workspace_openable: false,
        })
    })
    .await
    .map_err(|error| error.to_string())?
}

#[tauri::command]
pub async fn set_preferred_external_opener(
    window: tauri::WebviewWindow,
    id: String,
) -> Result<(), String> {
    main_window(&window)?;
    tauri::async_runtime::spawn_blocking(move || {
        let spec = selected_opener(installed_openers(), &id)?;
        window
            .state::<BridgeSupervisor>()
            .set_desktop_external_opener(spec.view.id)?;
        Ok(())
    })
    .await
    .map_err(|error| error.to_string())?
}

#[tauri::command]
pub async fn open_local_path_with(
    window: tauri::WebviewWindow,
    path: String,
    id: String,
) -> Result<(), String> {
    main_window(&window)?;
    tauri::async_runtime::spawn_blocking(move || {
        let path = document_path(&path, true)?;
        launch_with_opener(&window, path, &id)
    })
    .await
    .map_err(|error| error.to_string())?
}

fn launch_with_opener(
    window: &tauri::WebviewWindow,
    path: PathBuf,
    id: &str,
) -> Result<(), String> {
    let specs = installed_openers();
    let spec = if id.trim().is_empty() {
        let preferred = window
            .state::<BridgeSupervisor>()
            .desktop_preferences()?
            .external_opener;
        resolved_preference(&specs, &preferred)
            .cloned()
            .ok_or("no installed opener is available")?
    } else {
        selected_opener(specs, id)?
    };
    #[cfg(target_os = "linux")]
    {
        let command = crate::opener_catalog::linux::command(
            &spec.target,
            &spec.linux_launch,
            &path,
            spec.view.kind == "terminal",
        )?;
        crate::opener_catalog::linux::launch(
            command,
            matches!(
                spec.linux_launch,
                crate::opener_catalog::linux::Launch::Gio(_)
                    | crate::opener_catalog::linux::Launch::GioOpen
            ) || spec.view.kind == "file-manager",
        )
    }
    #[cfg(target_os = "windows")]
    {
        let plan =
            crate::opener_catalog::windows::launch_plan(&spec.target, spec.windows_mode, &path)?;
        crate::opener_catalog::windows::native::launch(plan)
    }
    #[cfg(not(any(target_os = "linux", target_os = "windows")))]
    {
        let launch = if spec.view.kind == "terminal" && path.is_file() {
            path.parent().ok_or("file has no parent folder")?
        } else {
            &path
        };
        #[cfg(target_os = "macos")]
        if spec.view.id == "ghostty" {
            // Ghostty needs its explicit working-directory argument; handing
            // it a directory document would not open a terminal there.
            let mut command = std::process::Command::new("/usr/bin/open");
            command
                .arg("-na")
                .arg(&spec.target)
                .arg("--args")
                .arg(format!("--working-directory={}", path_text(launch)?));
            // open hands off to LaunchServices and then exits. Wait for that
            // handoff, not for the GUI application, so a broken bundle cannot
            // report success merely because the launcher process was spawned.
            return macos_launch::run(command, std::time::Duration::from_secs(10));
        }
        window
            .opener()
            .open_path(path_text(launch)?, Some(path_text(&spec.target)?))
            .map_err(|error| error.to_string())
    }
}

#[cfg(target_os = "macos")]
mod macos_launch {
    use std::{
        process::{Command, Stdio},
        time::{Duration, Instant},
    };

    pub(super) fn run(mut command: Command, timeout: Duration) -> Result<(), String> {
        const FAILED: &str =
            "cannot open selected application; choose an installed application and retry";
        let mut child = command
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()
            .map_err(|_| FAILED)?;
        let deadline = Instant::now() + timeout;
        loop {
            match child.try_wait() {
                Ok(Some(status)) => {
                    return if status.success() {
                        Ok(())
                    } else {
                        Err(FAILED.into())
                    }
                }
                Ok(None) if Instant::now() < deadline => {
                    std::thread::sleep(Duration::from_millis(20))
                }
                result => {
                    // Only our launcher is stopped, never the target app or
                    // unrelated terminal sessions. Always reap the child.
                    let _ = child.kill();
                    let _ = child.wait();
                    return Err(if result.is_err() {
                        FAILED
                    } else {
                        "opening selected application timed out; check the application and retry"
                    }
                    .into());
                }
            }
        }
    }

    #[cfg(test)]
    mod tests {
        use super::*;
        #[test]
        fn actual_launchservices_rejection_is_not_success() {
            let root = tempfile::tempdir().unwrap();
            let app = root.path().join("Ghostty 中文\n\"$.app");
            std::fs::create_dir_all(app.join("Contents/MacOS")).unwrap();
            std::fs::write(app.join("Contents/Info.plist"), b"<?xml version=\"1.0\"?><plist version=\"1.0\"><dict><key>CFBundleExecutable</key><string>missing-executable</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>").unwrap();
            let mut command = Command::new("/usr/bin/open");
            command
                .arg("-na")
                .arg(app)
                .arg("--args")
                .arg("--working-directory=/private/tmp");
            assert!(run(command, Duration::from_secs(5))
                .unwrap_err()
                .contains("choose an installed application"));
        }
        #[test]
        fn successful_handoff_is_accepted() {
            assert!(run(Command::new("/usr/bin/true"), Duration::from_secs(2)).is_ok());
        }
        #[test]
        fn stuck_owned_launcher_is_bounded_and_reaped() {
            let mut command = Command::new("/bin/sleep");
            command.arg("10");
            assert!(run(command, Duration::from_millis(20))
                .unwrap_err()
                .contains("timed out"));
        }
    }
}

fn session_workspace(
    target: BridgeWorkspaceTargetResponse,
    requested: &str,
) -> Result<PathBuf, String> {
    if target.session_id != requested {
        return Err("session workspace identity changed; reload the session".into());
    }
    let root = document_path(&target.workspace_root, true)?;
    if !root.is_dir() {
        return Err("session workspace is no longer a directory; choose another workspace".into());
    }
    Ok(root)
}

#[tauri::command]
pub async fn workspace_external_openers(
    window: tauri::WebviewWindow,
    session_id: String,
) -> Result<OpenersView, String> {
    main_window(&window)?;
    tauri::async_runtime::spawn_blocking(move || {
        let supervisor = window.state::<BridgeSupervisor>();
        session_workspace(supervisor.workspace_target(&session_id)?, &session_id)?;
        let preferred = supervisor.desktop_preferences()?.external_opener;
        let openers = cached_views();
        let selected = openers
            .iter()
            .find(|view| view.id == preferred)
            .or_else(|| openers.iter().find(|view| view.kind == "file-manager"))
            .or_else(|| openers.first())
            .map(|view| view.id)
            .unwrap_or("");
        Ok(OpenersView {
            preferred: selected.into(),
            openers,
            workspace_openable: true,
        })
    })
    .await
    .map_err(|error| error.to_string())?
}

#[tauri::command]
pub async fn open_workspace_external(
    window: tauri::WebviewWindow,
    session_id: String,
    id: String,
) -> Result<(), String> {
    main_window(&window)?;
    tauri::async_runtime::spawn_blocking(move || {
        let target = window
            .state::<BridgeSupervisor>()
            .workspace_target(&session_id)?;
        let root = session_workspace(target, &session_id)?;
        launch_with_opener(&window, root, &id)
    })
    .await
    .map_err(|error| error.to_string())?
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::opener_catalog::{OpenerSpec, OpenerView};

    #[test]
    fn workspace_launch_uses_authoritative_session_identity_and_existing_directory() {
        let root = tempfile::tempdir().unwrap();
        let session = |id: &str, workspace: Option<String>| BridgeWorkspaceTargetResponse {
            protocol_version: 1,
            session_id: id.into(),
            workspace_root: workspace.unwrap_or_default(),
        };
        let path = root.path().to_str().unwrap().to_string();
        assert_eq!(
            session_workspace(session("source", Some(path.clone())), "source").unwrap(),
            root.path().canonicalize().unwrap()
        );
        assert!(session_workspace(session("switched", Some(path.clone())), "source").is_err());
        assert!(session_workspace(session("source", None), "source").is_err());
        let file = root.path().join("document.md");
        fs::write(&file, "document").unwrap();
        assert!(session_workspace(
            session("source", Some(file.to_string_lossy().into())),
            "source"
        )
        .is_err());
        assert!(session_workspace(
            session(
                "source",
                Some(root.path().join("missing").to_string_lossy().into())
            ),
            "source"
        )
        .is_err());
    }

    #[test]
    fn documents_with_spaces_unicode_and_shell_characters_are_paths() {
        let root = tempfile::tempdir().unwrap();
        let path = root.path().join("报告 ' $test (1).md");
        fs::write(&path, "document").unwrap();
        assert_eq!(
            document_path(path.to_str().unwrap(), true).unwrap(),
            path.canonicalize().unwrap()
        );
        let url = url::Url::from_file_path(&path).unwrap();
        assert_eq!(
            document_path(url.as_str(), true).unwrap(),
            path.canonicalize().unwrap()
        );
        assert!(document_path(root.path().to_str().unwrap(), true).is_ok());
    }

    #[test]
    fn rejects_devices_streams_relative_paths_and_nonfile_urls() {
        for path in [
            "",
            " ",
            "docs/readme.md",
            "javascript:alert(1)",
            "https://example.test/file",
            "file:///tmp/a?query=1",
            "file:///tmp/a#fragment",
            "file:////./PhysicalDrive0",
            "file:////%2e/PhysicalDrive0",
            "file://%3f/C:/file",
            "\\\\.\\PhysicalDrive0",
            "C:/NUL.txt",
            "C:/docs/COM¹",
            "C:/file.txt:stream",
            "//nas/share/LPT9",
        ] {
            assert!(absolute_path(path).is_err(), "accepted {path}");
        }
        assert!(unsafe_windows_syntax("C:/file:stream"));
        assert!(!unsafe_windows_syntax("C:/资料/my-file.md"));
    }

    #[test]
    fn refuses_executable_documents_but_can_reveal_or_copy_them() {
        let root = tempfile::tempdir().unwrap();
        for name in [
            "unsafe.exe",
            "unsafe.EXE. ",
            "unsafe.desktop",
            "unsafe.bat",
            "unsafe.js",
            "unsafe.url",
        ] {
            let path = root.path().join(name);
            fs::write(&path, "executable content").unwrap();
            assert!(
                document_path(path.to_str().unwrap(), true).is_err(),
                "opened {name}"
            );
            assert!(
                document_path(path.to_str().unwrap(), false).is_ok(),
                "cannot reveal {name}"
            );
        }
        let app = root.path().join("unsafe.app");
        fs::create_dir(&app).unwrap();
        assert!(document_path(app.to_str().unwrap(), true).is_err());
        assert!(save_source(app.to_str().unwrap()).is_err());
    }

    #[cfg(unix)]
    #[test]
    fn refuses_executable_modes_and_symlink_aliases() {
        use std::os::unix::fs::{symlink, PermissionsExt};
        let root = tempfile::tempdir().unwrap();
        let executable = root.path().join("binary");
        fs::write(&executable, "binary").unwrap();
        fs::set_permissions(&executable, fs::Permissions::from_mode(0o755)).unwrap();
        let alias = root.path().join("document.md");
        symlink(&executable, &alias).unwrap();
        assert!(document_path(executable.to_str().unwrap(), true).is_err());
        assert!(document_path(alias.to_str().unwrap(), true).is_err());
        let application = root.path().join("unsafe.app");
        fs::create_dir(&application).unwrap();
        let folder = root.path().join("folder");
        symlink(&application, &folder).unwrap();
        assert!(document_path(folder.to_str().unwrap(), true).is_err());
    }

    #[test]
    fn save_cancel_and_failures_keep_the_source_and_existing_destination() {
        let root = tempfile::tempdir().unwrap();
        let source = root.path().join("source.md");
        fs::write(&source, "original").unwrap();
        let source_file = || File::open(&source).unwrap();
        assert_eq!(
            copy_to_selected_destination(&source, source_file(), None).unwrap(),
            ""
        );
        assert!(
            copy_to_selected_destination(&source, source_file(), Some(source.clone())).is_err()
        );
        assert!(copy_to_selected_destination(
            &source,
            source_file(),
            Some(root.path().to_path_buf())
        )
        .is_err());
        assert!(copy_to_selected_destination(
            &source,
            source_file(),
            Some(root.path().join("missing/file.md"))
        )
        .is_err());
        assert_eq!(fs::read_to_string(&source).unwrap(), "original");
        assert_eq!(
            fs::read_dir(root.path()).unwrap().count(),
            1,
            "failed copies leave no temporary files"
        );
        let target = root.path().join("copy ' 中文.md");
        fs::write(&target, "previous destination").unwrap();
        assert_eq!(
            copy_to_selected_destination(&source, source_file(), Some(target.clone())).unwrap(),
            target.to_str().unwrap()
        );
        assert_eq!(fs::read_to_string(&target).unwrap(), "original");
        assert_eq!(fs::read_to_string(&source).unwrap(), "original");
    }

    #[test]
    fn save_refuses_replaced_source_while_dialog_was_open() {
        let root = tempfile::tempdir().unwrap();
        let source = root.path().join("source.md");
        let original = root.path().join("original-inode.md");
        fs::write(&source, "original opened content").unwrap();
        let file = File::open(&source).unwrap();
        fs::rename(&source, &original).unwrap();
        fs::write(&source, "new source must remain").unwrap();
        assert_eq!(
            copy_to_selected_destination(&source, file.try_clone().unwrap(), None).unwrap(),
            ""
        );
        let other = root.path().join("other.md");
        fs::write(&other, "existing destination must remain").unwrap();
        for target in [&source, &other] {
            let error = copy_to_selected_destination(
                &source,
                file.try_clone().unwrap(),
                Some(target.clone()),
            )
            .unwrap_err();
            assert!(error.starts_with("source file changed;"));
        }
        assert_eq!(
            fs::read_to_string(&other).unwrap(),
            "existing destination must remain"
        );
        assert_eq!(
            fs::read_to_string(&source).unwrap(),
            "new source must remain"
        );
        assert_eq!(
            fs::read_to_string(&original).unwrap(),
            "original opened content"
        );
        assert_eq!(fs::read_dir(root.path()).unwrap().count(), 3);
        let (_, reopened) = save_source(source.to_str().unwrap()).unwrap();
        copy_to_selected_destination(&source, reopened, Some(other.clone())).unwrap();
        assert_eq!(
            fs::read_to_string(&other).unwrap(),
            "new source must remain"
        );
    }

    #[test]
    fn save_rejects_hardlink_alias_to_source_before_replacing_it() {
        let root = tempfile::tempdir().unwrap();
        let source = root.path().join("original.md");
        let alias = root.path().join("alias.md");
        fs::write(&source, "must remain").unwrap();
        fs::hard_link(&source, &alias).unwrap();
        assert!(copy_to_selected_destination(
            &source,
            File::open(&source).unwrap(),
            Some(alias.clone())
        )
        .is_err());
        assert_eq!(fs::read_to_string(&alias).unwrap(), "must remain");
        assert!(destination_is_source(&File::open(source).unwrap(), &alias).unwrap());
    }

    #[cfg(unix)]
    #[test]
    fn save_rejects_source_symlink_alias_and_preserves_private_permissions() {
        use std::os::unix::fs::{symlink, PermissionsExt};
        let root = tempfile::tempdir().unwrap();
        let source = root.path().join("private.md");
        let alias = root.path().join("alias.md");
        fs::write(&source, "private document").unwrap();
        fs::set_permissions(&source, fs::Permissions::from_mode(0o600)).unwrap();
        symlink(&source, &alias).unwrap();
        assert!(
            copy_to_selected_destination(&source, File::open(&source).unwrap(), Some(alias))
                .is_err()
        );
        let target = root.path().join("copy.md");
        copy_to_selected_destination(&source, File::open(&source).unwrap(), Some(target.clone()))
            .unwrap();
        assert_eq!(
            fs::metadata(target).unwrap().permissions().mode() & 0o777,
            0o600
        );
    }

    #[test]
    fn missing_source_does_not_open_a_save_dialog() {
        let root = tempfile::tempdir().unwrap();
        assert!(save_source(root.path().join("missing.md").to_str().unwrap()).is_err());
        assert!(save_source(root.path().to_str().unwrap()).is_err());
    }

    #[cfg(unix)]
    #[test]
    fn unreadable_source_is_refused_before_showing_a_save_dialog() {
        use std::os::unix::fs::PermissionsExt;
        let root = tempfile::tempdir().unwrap();
        let source = root.path().join("unreadable.md");
        fs::write(&source, "original").unwrap();
        fs::set_permissions(&source, fs::Permissions::from_mode(0o000)).unwrap();
        let result = save_source(source.to_str().unwrap());
        fs::set_permissions(&source, fs::Permissions::from_mode(0o600)).unwrap();
        // Elevated runners can read mode-000 files; they cannot exercise this
        // OS denial. Ordinary desktop/CI users must receive the actionable error.
        if let Err(error) = result {
            assert!(error.contains("check file permissions"));
        } else {
            eprintln!(
                "runner bypasses file permissions; real permission-denial coverage unavailable"
            );
        }
        assert_eq!(fs::read_to_string(source).unwrap(), "original");
    }

    #[test]
    fn renderer_cannot_supply_a_program_path_or_arguments_as_an_opener_id() {
        let specs = || {
            vec![OpenerSpec {
                view: OpenerView {
                    id: "vscode",
                    name: "VS Code".into(),
                    kind: "editor",
                    icon_data_url: String::new(),
                },
                target: PathBuf::from("/Applications/Visual Studio Code.app"),
                #[cfg(target_os = "linux")]
                linux_launch: crate::opener_catalog::linux::Launch::Path,
                #[cfg(target_os = "linux")]
                icon_source: None,
                #[cfg(target_os = "windows")]
                windows_mode: crate::opener_catalog::windows::Mode::Path,
                #[cfg(target_os = "windows")]
                windows_icon: PathBuf::from("/native/app"),
            }]
        };
        for id in [
            "/bin/sh",
            "/Applications/Visual Studio Code.app",
            "vscode --execute",
            "vscode; touch /tmp/injected",
            "uninstalled-editor",
        ] {
            assert!(selected_opener(specs(), id).is_err());
        }
        assert_eq!(
            selected_opener(specs(), "vscode").unwrap().target,
            PathBuf::from("/Applications/Visual Studio Code.app")
        );
    }
}
