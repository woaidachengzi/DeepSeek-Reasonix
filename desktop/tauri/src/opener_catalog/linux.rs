//! Linux catalog and launch plans. Desktop files stay native; GIO expands
//! their Exec fields. Workspace text never becomes shell source.
use std::{
    collections::{BTreeMap, HashSet},
    ffi::{OsStr, OsString},
    fs,
    io::Read,
    path::{Path, PathBuf},
    process::{Command, Stdio},
};

const ENTRY_LIMIT: u64 = 64 * 1024;
const SCAN_LIMIT: usize = 4096;

#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) enum Launch {
    Path,
    Gio(PathBuf),
    GioOpen,
    Terminal(&'static str),
}
#[derive(Clone, Debug)]
pub(crate) struct Application {
    pub id: &'static str,
    pub name: String,
    pub kind: &'static str,
    pub target: PathBuf,
    pub launch: Launch,
    pub icon: Option<PathBuf>,
}
#[derive(Clone, Debug)]
struct Entry {
    path: PathBuf,
    id: String,
    name: String,
    icon: String,
    exec: String,
    try_exec: String,
    available: bool,
}
#[derive(Clone, Debug)]
pub(crate) struct Discovery {
    pub data_roots: Vec<PathBuf>,
    pub icon_roots: Vec<PathBuf>,
    pub paths: Vec<PathBuf>,
    pub directory_app: Option<String>,
}

impl Discovery {
    pub fn from_environment() -> Self {
        #[cfg(target_os = "linux")]
        let directory_app = {
            use gio::prelude::AppInfoExt;
            gio::AppInfo::default_for_type("inode/directory", false)
                .and_then(|app| app.id())
                .map(|id| id.to_string())
        };
        #[cfg(not(target_os = "linux"))]
        let directory_app = None;
        Self::from_values(
            std::env::var_os("HOME"),
            std::env::var_os("XDG_DATA_HOME"),
            std::env::var_os("XDG_DATA_DIRS"),
            std::env::var_os("PATH"),
            directory_app,
        )
    }

    fn from_values(
        home: Option<OsString>,
        data_home: Option<OsString>,
        data_dirs: Option<OsString>,
        search_path: Option<OsString>,
        directory_app: Option<String>,
    ) -> Self {
        let home = home.map(PathBuf::from).filter(|p| p.is_absolute());
        let data_home = data_home
            .filter(|value| !value.is_empty())
            .map(PathBuf::from)
            .filter(|p| p.is_absolute())
            .or_else(|| home.as_ref().map(|p| p.join(".local/share")));
        let mut data_roots: Vec<_> = data_home.into_iter().collect();
        let dirs = data_dirs
            .filter(|value| !value.is_empty())
            .unwrap_or_else(|| "/usr/local/share:/usr/share".into());
        data_roots.extend(std::env::split_paths(&dirs).filter(|p| p.is_absolute()));
        let mut seen = HashSet::new();
        data_roots.retain(|p| seen.insert(p.clone()));
        let mut icon_roots: Vec<_> = home.iter().map(|p| p.join(".icons")).collect();
        icon_roots.extend(data_roots.iter().map(|p| p.join("icons")));
        let paths = search_path
            .map(|v| {
                std::env::split_paths(&v)
                    .filter(|p| p.is_absolute())
                    .collect()
            })
            .unwrap_or_default();
        Self {
            data_roots,
            icon_roots,
            paths,
            directory_app,
        }
    }

    fn executable(&self, name: &str) -> Option<PathBuf> {
        let name = Path::new(name);
        if name.is_absolute() {
            return executable_file(name);
        }
        if name.components().count() != 1 || name.as_os_str().is_empty() {
            return None;
        }
        self.paths
            .iter()
            .filter(|p| p.is_absolute())
            .find_map(|p| executable_file(&p.join(name)))
    }

    fn entries(&self) -> BTreeMap<String, Entry> {
        let mut entries = BTreeMap::new();
        let mut scanned = 0;
        for root in &self.data_roots {
            scan_entries(
                &root.join("applications"),
                &root.join("applications"),
                0,
                &mut scanned,
                &mut entries,
            );
            if scanned >= SCAN_LIMIT {
                break;
            }
        }
        entries
    }

    fn icon(&self, value: &str) -> Option<PathBuf> {
        let path = Path::new(value);
        if path.is_absolute() {
            return icon_file(path);
        }
        if value.is_empty()
            || value.len() > 256
            || !matches!(
                (path.components().next(), path.components().nth(1)),
                (Some(std::path::Component::Normal(_)), None)
            )
            || value.chars().any(|c| c.is_control() || c == '\\')
        {
            return None;
        }
        let extensions: &[&str] = if path.extension().and_then(OsStr::to_str).is_some_and(|ext| {
            ["png", "svg", "webp", "jpg", "jpeg", "xpm"]
                .contains(&ext.to_ascii_lowercase().as_str())
        }) {
            &[""]
        } else {
            &[".png", ".svg", ".webp", ".jpg", ".jpeg", ".xpm"]
        };
        for root in &self.icon_roots {
            // hicolor is the required fallback; search installed themes too,
            // using bounded directory enumeration, never globbing an icon key.
            let mut themes = vec![root.join("hicolor")];
            if let Ok(children) = fs::read_dir(root) {
                themes.extend(
                    children
                        .take(64)
                        .filter_map(Result::ok)
                        .filter(|e| e.file_type().is_ok_and(|t| t.is_dir()))
                        .map(|e| e.path()),
                );
            }
            for theme in themes {
                for size in ["64x64", "48x48", "128x128", "32x32", "256x256", "scalable"] {
                    for suffix in extensions {
                        if let Some(path) = icon_file(
                            &theme
                                .join(size)
                                .join("apps")
                                .join(format!("{value}{suffix}")),
                        ) {
                            return Some(path);
                        }
                    }
                }
            }
            for suffix in extensions {
                if let Some(path) = icon_file(&root.join(format!("{value}{suffix}"))) {
                    return Some(path);
                }
            }
        }
        self.data_roots.iter().find_map(|root| {
            extensions.iter().find_map(|suffix| {
                icon_file(&root.join("pixmaps").join(format!("{value}{suffix}")))
            })
        })
    }

    pub fn discover(&self) -> Vec<Application> {
        let entries = self.entries();
        let gio = self.executable("gio");
        let mut applications = vec![];
        // Match fixed Wails catalog identities; no renderer supplied program.
        let candidates: &[(&str, &str, &[&str], &[&str])] = &[
            (
                "vscode",
                "VS Code",
                &["code"],
                &["code", "visual-studio-code", "com.visualstudio.code"],
            ),
            (
                "vscode-insiders",
                "VS Code Insiders",
                &["code-insiders"],
                &["code-insiders", "visual-studio-code-insiders"],
            ),
            (
                "cursor",
                "Cursor",
                &["cursor"],
                &["cursor", "com.todesktop.230313mzl4w4u92"],
            ),
            (
                "android-studio",
                "Android Studio",
                &["android-studio", "studio", "studio.sh"],
                &["android-studio", "com.google.AndroidStudio"],
            ),
            (
                "goland",
                "GoLand",
                &["goland", "goland.sh"],
                &["jetbrains-goland", "goland"],
            ),
            (
                "pycharm",
                "PyCharm",
                &["pycharm", "pycharm.sh"],
                &["jetbrains-pycharm", "pycharm"],
            ),
            (
                "intellij-idea",
                "IntelliJ IDEA",
                &["idea", "idea.sh", "intellij-idea"],
                &["jetbrains-idea", "intellij-idea"],
            ),
            (
                "webstorm",
                "WebStorm",
                &["webstorm", "webstorm.sh"],
                &["jetbrains-webstorm", "webstorm"],
            ),
            (
                "datagrip",
                "DataGrip",
                &["datagrip", "datagrip.sh"],
                &["jetbrains-datagrip", "datagrip"],
            ),
            (
                "codebuddy",
                "CodeBuddy",
                &["codebuddy"],
                &["codebuddy", "com.tencent.codebuddy"],
            ),
            (
                "windsurf",
                "Windsurf",
                &["windsurf"],
                &["windsurf", "com.exafunction.windsurf"],
            ),
            ("zed", "Zed", &["zed"], &["dev.zed.Zed", "zed"]),
            (
                "sublime-text",
                "Sublime Text",
                &["subl", "sublime_text"],
                &["sublime_text", "sublime-text"],
            ),
            ("kiro", "Kiro", &["kiro"], &["kiro", "dev.kiro.desktop"]),
        ];
        for (id, name, binaries, aliases) in candidates {
            let entry = matching_entry(&entries, aliases);
            if entry.is_some_and(|e| !self.entry_available(e)) {
                continue;
            }
            let direct = binaries.iter().find_map(|name| self.executable(name));
            let selected = direct.map(|target| (target, Launch::Path)).or_else(|| {
                entry
                    .zip(gio.as_ref())
                    .map(|(entry, gio)| (gio.clone(), Launch::Gio(entry.path.clone())))
            });
            if let Some((target, launch)) = selected {
                applications.push(Application {
                    id,
                    name: name.to_string(),
                    kind: "editor",
                    target,
                    launch,
                    icon: entry.and_then(|e| self.icon(&e.icon)),
                });
            }
        }
        let directory_entry = self
            .directory_app
            .as_ref()
            .and_then(|id| entries.get(id))
            .filter(|e| self.entry_available(e));
        let file_manager = self
            .executable("xdg-open")
            .map(|p| (p, Launch::Path))
            .or_else(|| gio.clone().map(|p| (p, Launch::GioOpen)));
        if let Some((target, launch)) = file_manager {
            applications.push(Application {
                id: "file-manager",
                name: directory_entry
                    .map(|e| e.name.clone())
                    .unwrap_or_else(|| "File Manager".into()),
                kind: "file-manager",
                target,
                launch,
                icon: directory_entry.and_then(|e| self.icon(&e.icon)),
            });
        }
        for (id, name, binaries, aliases) in [
            (
                "ghostty",
                "Ghostty",
                &["ghostty"][..],
                &["com.mitchellh.ghostty", "ghostty"][..],
            ),
            (
                "gnome-terminal",
                "GNOME Terminal",
                &["gnome-terminal"],
                &["org.gnome.Terminal", "gnome-terminal"],
            ),
            (
                "konsole",
                "Konsole",
                &["konsole"],
                &["org.kde.konsole", "konsole"],
            ),
            ("kitty", "Kitty", &["kitty"], &["kitty"]),
            (
                "alacritty",
                "Alacritty",
                &["alacritty"],
                &["Alacritty", "alacritty"],
            ),
            (
                "terminal",
                "Terminal",
                &["x-terminal-emulator"],
                &["terminal"],
            ),
        ] {
            let entry = matching_entry(&entries, aliases);
            if entry.is_some_and(|e| !self.entry_available(e)) {
                continue;
            }
            let target = binaries
                .iter()
                .find_map(|name| self.executable(name))
                .or_else(|| {
                    // An absolute executable in a desktop entry supports custom
                    // installs. Known basename ensures the directory flags match.
                    let program = first_exec_word(&entry?.exec)?;
                    if !binaries.contains(&Path::new(&program).file_name()?.to_str()?) {
                        return None;
                    }
                    self.executable(&program)
                });
            if let Some(target) = target {
                applications.push(Application {
                    id,
                    name: name.into(),
                    kind: "terminal",
                    target,
                    launch: Launch::Terminal(id),
                    icon: entry.and_then(|e| self.icon(&e.icon)),
                });
            }
        }
        applications
    }

    fn entry_available(&self, entry: &Entry) -> bool {
        entry.available && (entry.try_exec.is_empty() || self.executable(&entry.try_exec).is_some())
    }
}

fn executable_file(path: &Path) -> Option<PathBuf> {
    use std::os::unix::fs::PermissionsExt;
    let info = fs::metadata(path).ok()?;
    if !info.is_file() || info.permissions().mode() & 0o111 == 0 {
        return None;
    }
    path.canonicalize().ok()
}
fn icon_file(path: &Path) -> Option<PathBuf> {
    let info = fs::metadata(path).ok()?;
    if !info.is_file() || info.len() == 0 || info.len() > 1024 * 1024 {
        return None;
    }
    path.canonicalize().ok()
}
fn scan_entries(
    root: &Path,
    dir: &Path,
    depth: usize,
    scanned: &mut usize,
    entries: &mut BTreeMap<String, Entry>,
) {
    if depth > 4 || *scanned >= SCAN_LIMIT {
        return;
    }
    let Ok(children) = fs::read_dir(dir) else {
        return;
    };
    let mut children: Vec<_> = children
        .take(SCAN_LIMIT - *scanned)
        .filter_map(Result::ok)
        .collect();
    children.sort_by_key(|e| e.file_name());
    for child in children {
        if *scanned >= SCAN_LIMIT {
            return;
        }
        *scanned += 1;
        if child.file_type().is_ok_and(|t| t.is_dir()) {
            scan_entries(root, &child.path(), depth + 1, scanned, entries);
            continue;
        }
        if child.path().extension() != Some(OsStr::new("desktop")) {
            continue;
        }
        let path = child.path();
        let Ok(relative) = path.strip_prefix(root) else {
            continue;
        };
        let Some(id) = relative.to_str().map(|p| p.replace('/', "-")) else {
            continue;
        };
        // User entries (including Hidden=true) mask system entries by ID.
        entries.entry(id.clone()).or_insert_with(|| {
            parse_entry(&path, &id).unwrap_or_else(|| Entry {
                path,
                id,
                name: String::new(),
                icon: String::new(),
                exec: String::new(),
                try_exec: String::new(),
                available: false,
            })
        });
    }
}
fn parse_entry(path: &Path, id: &str) -> Option<Entry> {
    let info = fs::metadata(path).ok()?;
    if !info.is_file() || info.len() > ENTRY_LIMIT {
        return None;
    }
    let mut text = String::new();
    fs::File::open(path)
        .ok()?
        .take(ENTRY_LIMIT + 1)
        .read_to_string(&mut text)
        .ok()?;
    if text.len() as u64 > ENTRY_LIMIT || text.contains('\0') {
        return None;
    }
    let mut fields = BTreeMap::new();
    let mut inside = false;
    let mut seen = false;
    for line in text.lines().map(str::trim) {
        if line.starts_with('[') && line.ends_with(']') {
            inside = line == "[Desktop Entry]";
            if inside {
                if seen {
                    return None;
                }
                seen = true;
            }
            continue;
        }
        if !inside || line.is_empty() || line.starts_with('#') {
            continue;
        }
        let Some((key, value)) = line.split_once('=') else {
            continue;
        };
        let key = key.trim();
        if [
            "Type",
            "Name",
            "Icon",
            "Exec",
            "TryExec",
            "Hidden",
            "DBusActivatable",
        ]
        .contains(&key)
            && fields.insert(key, value.trim()).is_some()
        {
            return None;
        }
    }
    let get = |key| fields.get(key).copied().unwrap_or("");
    let name = get("Name");
    let available = seen
        && get("Type") == "Application"
        && get("Hidden") != "true"
        && !name.is_empty()
        && name.len() <= 512
        && !name.chars().any(char::is_control)
        && (!get("Exec").is_empty() || get("DBusActivatable") == "true");
    Some(Entry {
        path: path.into(),
        id: id.into(),
        name: name.into(),
        icon: get("Icon").into(),
        exec: get("Exec").into(),
        try_exec: get("TryExec").into(),
        available,
    })
}
fn normalized(value: &str) -> String {
    value
        .chars()
        .filter(|c| !matches!(c, ' ' | '-' | '_' | '.'))
        .flat_map(char::to_lowercase)
        .collect()
}
fn matching_entry<'a>(entries: &'a BTreeMap<String, Entry>, aliases: &[&str]) -> Option<&'a Entry> {
    for alias in aliases {
        let want = normalized(alias);
        if let Some(entry) = entries.values().find(|entry| {
            normalized(entry.id.trim_end_matches(".desktop")) == want
                || normalized(&entry.name) == want
        }) {
            return Some(entry);
        }
    }
    aliases
        .iter()
        .filter_map(|alias| {
            let want = normalized(alias);
            (want.len() >= 5)
                .then(|| {
                    entries.values().find(|entry| {
                        normalized(&entry.id).contains(&want)
                            || normalized(&entry.name).contains(&want)
                    })
                })
                .flatten()
        })
        .next()
}
fn first_exec_word(value: &str) -> Option<String> {
    let value = value.trim();
    let word = if let Some(quoted) = value.strip_prefix('"') {
        let end = quoted.find('"')?;
        if quoted[..end].contains('\\')
            || !quoted[end + 1..].starts_with(char::is_whitespace) && end + 1 != quoted.len()
        {
            return None;
        }
        &quoted[..end]
    } else {
        value.split_whitespace().next()?
    };
    if word.is_empty()
        || word
            .chars()
            .any(|c| c.is_control() || "=$`%;&|<>".contains(c))
    {
        return None;
    }
    Some(word.into())
}

pub(crate) fn command(
    target: &Path,
    mode: &Launch,
    path: &Path,
    terminal: bool,
) -> Result<Command, String> {
    let dir = if path.is_dir() {
        path
    } else {
        path.parent().ok_or("document has no parent folder")?
    };
    let launch = if terminal { dir } else { path };
    let mut args: Vec<OsString> = vec![];
    match mode {
        Launch::Path => args.push(launch.into()),
        Launch::Gio(desktop) => {
            args.extend(["launch".into(), desktop.as_os_str().into(), path.into()])
        }
        Launch::GioOpen => args.extend(["open".into(), path.into()]),
        Launch::Terminal("ghostty" | "gnome-terminal") => {
            let mut arg = OsString::from("--working-directory=");
            arg.push(dir);
            args.push(arg);
        }
        Launch::Terminal("konsole") => args.extend(["--workdir".into(), dir.into()]),
        Launch::Terminal("kitty") => args.extend(["--directory".into(), dir.into()]),
        Launch::Terminal("alacritty") => args.extend(["--working-directory".into(), dir.into()]),
        Launch::Terminal("terminal") => {}
        Launch::Terminal(_) => {
            return Err("unsupported terminal; select another installed application".into())
        }
    }
    let mut command = Command::new(target);
    command
        .args(args)
        .current_dir(dir)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null());
    Ok(command)
}

pub(crate) fn launch(mut command: Command, wait_for_launcher: bool) -> Result<(), String> {
    let mut child = command.spawn().map_err(|_| {
        "selected application could not start; check installation and retry".to_string()
    })?;
    // GIO/xdg-open normally exit after submitting a desktop launch. Surface
    // their refusal without forwarding diagnostics, paths or command output.
    // Some handlers stay alive; bound the wait and reap those off-thread.
    if wait_for_launcher {
        let deadline = std::time::Instant::now() + std::time::Duration::from_secs(3);
        loop {
            match child.try_wait() {
                Ok(Some(status)) if status.success() => return Ok(()),
                Ok(Some(_)) => return Err("selected application rejected the launch; check its installation and desktop association, then retry".into()),
                Err(_) => {
                    std::thread::spawn(move || { let _ = child.wait(); });
                    return Err("application launch status is unavailable; check the application and retry".into());
                }
                Ok(None) if std::time::Instant::now() >= deadline => break,
                Ok(None) => std::thread::sleep(std::time::Duration::from_millis(10)),
            }
        }
    }
    std::thread::spawn(move || {
        let _ = child.wait();
    });
    Ok(())
}

#[cfg(target_os = "linux")]
pub(crate) fn icon_data_url(path: &Path) -> Option<String> {
    use base64::Engine;
    let path = icon_file(path)?;
    // Native loaders handle PNG/SVG/theme artwork off the UI thread. No raw
    // SVG, executable path or file URL crosses the WebView boundary.
    use gdk_pixbuf::prelude::PixbufLoaderExt;
    let mut source = Vec::new();
    fs::File::open(path)
        .ok()?
        .take(1024 * 1024 + 1)
        .read_to_end(&mut source)
        .ok()?;
    if source.is_empty() || source.len() > 1024 * 1024 {
        return None;
    }
    let loader = gdk_pixbuf::PixbufLoader::new();
    loader.connect_size_prepared(|loader, width, height| {
        if width > 0 && height > 0 {
            let longest = width.max(height) as i64;
            loader.set_size(
                (width as i64 * 64 / longest).max(1) as i32,
                (height as i64 * 64 / longest).max(1) as i32,
            );
        }
    });
    let decoded = loader.write(&source);
    let closed = loader.close();
    decoded.ok()?;
    closed.ok()?;
    let pixbuf = loader.pixbuf()?;
    if pixbuf.width() > 64 || pixbuf.height() > 64 {
        return None;
    }
    let bytes = pixbuf.save_to_bufferv("png", &[]).ok()?;
    if bytes.len() > 64 * 1024 || !bytes.starts_with(b"\x89PNG\r\n\x1a\n") {
        return None;
    }
    Some(format!(
        "data:image/png;base64,{}",
        base64::engine::general_purpose::STANDARD.encode(bytes)
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::os::unix::fs::PermissionsExt;

    struct Fixture {
        root: tempfile::TempDir,
        discovery: Discovery,
    }
    impl Fixture {
        fn new() -> Self {
            let root = tempfile::tempdir().unwrap();
            let user = root.path().join("user");
            let system = root.path().join("system");
            let bin = root.path().join("bin");
            for dir in [&user, &system] {
                fs::create_dir_all(dir.join("applications")).unwrap();
            }
            fs::create_dir(&bin).unwrap();
            let discovery = Discovery {
                icon_roots: vec![user.join("icons"), system.join("icons")],
                data_roots: vec![user, system],
                paths: vec![bin],
                directory_app: None,
            };
            Self { root, discovery }
        }
        fn executable(&self, name: &str) -> PathBuf {
            let path = self.discovery.paths[0].join(name);
            fs::write(&path, "#!/bin/sh\nexit 0\n").unwrap();
            fs::set_permissions(&path, fs::Permissions::from_mode(0o755)).unwrap();
            path
        }
        fn entry(&self, root: usize, name: &str, fields: &str) -> PathBuf {
            let path = self.discovery.data_roots[root]
                .join("applications")
                .join(name);
            fs::create_dir_all(path.parent().unwrap()).unwrap();
            fs::write(
                &path,
                format!("[Desktop Entry]\nType=Application\n{fields}\n"),
            )
            .unwrap();
            path
        }
    }

    #[test]
    fn xdg_layout_uses_absolute_user_priority_defaults_and_filters_relative_paths() {
        let d = Discovery::from_values(
            Some("/home/test".into()),
            Some("/custom/share".into()),
            Some("/custom/share:relative:/srv/share:/usr/share".into()),
            Some(":relative:/usr/bin:/opt/bin".into()),
            None,
        );
        assert_eq!(
            d.data_roots,
            vec![
                PathBuf::from("/custom/share"),
                "/srv/share".into(),
                "/usr/share".into()
            ]
        );
        assert_eq!(d.icon_roots[0], PathBuf::from("/home/test/.icons"));
        assert_eq!(d.paths, vec![PathBuf::from("/usr/bin"), "/opt/bin".into()]);
        let d = Discovery::from_values(
            Some("/home/test".into()),
            Some("relative".into()),
            Some("".into()),
            None,
            None,
        );
        assert_eq!(
            d.data_roots,
            vec![
                PathBuf::from("/home/test/.local/share"),
                "/usr/local/share".into(),
                "/usr/share".into()
            ]
        );
        let d = Discovery::from_values(Some("relative".into()), None, None, None, None);
        assert_eq!(
            d.data_roots,
            vec![PathBuf::from("/usr/local/share"), "/usr/share".into()]
        );
    }

    #[test]
    fn desktop_actions_do_not_override_main_entry_and_malformed_entries_are_unavailable() {
        let f = Fixture::new();
        let path = f.entry(0, "code.desktop", "Name=VS Code\nExec=code %U\nNoDisplay=true\n[Desktop Action Other]\nName=Other\nExec=/bin/sh");
        let entry = parse_entry(&path, "code.desktop").unwrap();
        assert_eq!(entry.name, "VS Code");
        assert_eq!(entry.exec, "code %U");
        assert!(entry.available);
        for fields in [
            "Name=Code\nExec=code\nHidden=true",
            "Name=Code\nExec=code\nType=Link",
            "Exec=code",
            "Name=Code\nExec=",
            "Name=Code\nExec=code\nName=Other",
            "Name=Code\nExec=code\n[Desktop Entry]\nName=Other",
            "Name=Code\0\nExec=code",
        ] {
            fs::write(
                &path,
                format!("[Desktop Entry]\nType=Application\n{fields}"),
            )
            .unwrap();
            assert!(
                !parse_entry(&path, "code.desktop").is_some_and(|e| e.available),
                "{fields:?}"
            );
        }
        fs::write(&path, "x".repeat(ENTRY_LIMIT as usize + 1)).unwrap();
        assert!(parse_entry(&path, "code.desktop").is_none());
        assert!(parse_entry(f.root.path(), "directory.desktop").is_none());
    }

    #[test]
    fn user_tombstones_override_system_and_tryexec_gates_even_existing_cli() {
        let f = Fixture::new();
        f.executable("code");
        f.entry(1, "code.desktop", "Name=VS Code\nExec=code %U");
        let user = f.entry(0, "code.desktop", "Name=VS Code\nExec=code %U\nHidden=true");
        assert!(!f.discovery.discover().iter().any(|a| a.id == "vscode"));
        fs::remove_file(&user).unwrap();
        assert!(f.discovery.discover().iter().any(|a| a.id == "vscode"));
        f.entry(
            0,
            "code.desktop",
            "Name=VS Code\nExec=code %U\nTryExec=missing-code",
        );
        assert!(!f.discovery.discover().iter().any(|a| a.id == "vscode"));
        f.executable("missing-code");
        assert!(f.discovery.discover().iter().any(|a| a.id == "vscode"));
    }

    #[test]
    fn nested_desktop_editor_uses_gio_and_default_directory_app_supplies_name_and_icon() {
        let mut f = Fixture::new();
        let gio = f.executable("gio");
        let desktop = f.entry(
            0,
            "vendor/jetbrains-goland.desktop",
            "Name=GoLand\nExec=\"/opt/Custom IDE/bin/goland\" %F",
        );
        f.entry(
            1,
            "org.kde.dolphin.desktop",
            "Name=Dolphin\nExec=dolphin %U\nIcon=org.kde.dolphin",
        );
        f.discovery.directory_app = Some("org.kde.dolphin.desktop".into());
        let icon = f.discovery.icon_roots[1].join("hicolor/64x64/apps/org.kde.dolphin.png");
        fs::create_dir_all(icon.parent().unwrap()).unwrap();
        fs::write(&icon, b"image fixture").unwrap();
        let apps = f.discovery.discover();
        let editor = apps.iter().find(|a| a.id == "goland").unwrap();
        assert_eq!(editor.target, gio.canonicalize().unwrap());
        assert_eq!(editor.launch, Launch::Gio(desktop.clone()));
        let manager = apps.iter().find(|a| a.id == "file-manager").unwrap();
        assert_eq!(manager.name, "Dolphin");
        assert_eq!(manager.launch, Launch::GioOpen);
        assert_eq!(manager.icon, Some(icon.canonicalize().unwrap()));
        let cmd = command(&editor.target, &editor.launch, f.root.path(), false).unwrap();
        assert_eq!(
            cmd.get_args().collect::<Vec<_>>(),
            vec![
                OsStr::new("launch"),
                desktop.as_os_str(),
                f.root.path().as_os_str()
            ]
        );
    }

    #[test]
    fn executables_require_absolute_roots_and_executable_files_and_disappearance_is_rechecked() {
        let mut f = Fixture::new();
        let code = f.executable("code");
        assert!(f.discovery.discover().iter().any(|a| a.id == "vscode"));
        fs::set_permissions(&code, fs::Permissions::from_mode(0o644)).unwrap();
        assert!(!f.discovery.discover().iter().any(|a| a.id == "vscode"));
        f.executable("code");
        fs::remove_file(&code).unwrap();
        assert!(!f.discovery.discover().iter().any(|a| a.id == "vscode"));
        f.discovery.paths = vec![PathBuf::new(), PathBuf::from("."), PathBuf::from("bin")];
        assert!(f.discovery.executable("code").is_none());
        assert!(f.discovery.executable("../code").is_none());
        fs::create_dir_all(&code).unwrap();
        fs::set_permissions(&code, fs::Permissions::from_mode(0o755)).unwrap();
        assert!(executable_file(&code).is_none());
    }

    #[test]
    fn custom_terminal_install_accepts_only_known_direct_executable() {
        let f = Fixture::new();
        let bin = f.root.path().join("Custom Terminal/bin/kitty");
        fs::create_dir_all(bin.parent().unwrap()).unwrap();
        fs::write(&bin, "executable").unwrap();
        fs::set_permissions(&bin, fs::Permissions::from_mode(0o755)).unwrap();
        f.entry(
            0,
            "kitty.desktop",
            &format!("Name=Kitty\nExec=\"{}\" %U", bin.display()),
        );
        let app = f
            .discovery
            .discover()
            .into_iter()
            .find(|a| a.id == "kitty")
            .unwrap();
        assert_eq!(app.target, bin.canonicalize().unwrap());
        assert_eq!(app.launch, Launch::Terminal("kitty"));
        for exec in [
            "env kitty",
            "/bin/sh -c kitty",
            "\"/opt/kitty\"evil",
            "\"/opt/kit\\ty\"",
            "kitty;touch",
            "kitty%U",
            "",
            "\"unclosed",
        ] {
            assert!(
                !first_exec_word(exec).is_some_and(|word| word == "kitty" || word == "/opt/kitty"),
                "{exec}"
            );
        }
    }

    #[test]
    fn icon_keys_cannot_traverse_roots_and_oversized_files_are_ignored() {
        let f = Fixture::new();
        let icon = f.discovery.data_roots[0].join("pixmaps/code.svg");
        fs::create_dir_all(icon.parent().unwrap()).unwrap();
        fs::write(&icon, b"svg fixture").unwrap();
        assert_eq!(f.discovery.icon("code"), Some(icon.canonicalize().unwrap()));
        assert_eq!(
            f.discovery.icon(icon.to_str().unwrap()),
            Some(icon.canonicalize().unwrap())
        );
        for key in [
            "..",
            ".",
            "../pixmaps/code.svg",
            "code/../../etc/passwd",
            "code\\bad",
            "code\n",
        ] {
            assert!(f.discovery.icon(key).is_none(), "{key:?}");
        }
        fs::write(&icon, vec![0u8; 1024 * 1024 + 1]).unwrap();
        assert!(f.discovery.icon("code").is_none());
    }

    #[test]
    fn terminal_launch_preserves_special_directory_as_one_argument_and_uses_file_parent() {
        let f = Fixture::new();
        let dir = f.root.path().join("work 'quoted' $; % & space");
        fs::create_dir(&dir).unwrap();
        let file = dir.join("readme.txt");
        fs::write(&file, "doc").unwrap();
        for (id, flag) in [
            ("konsole", "--workdir"),
            ("kitty", "--directory"),
            ("alacritty", "--working-directory"),
        ] {
            let cmd = command(
                Path::new("/native/terminal"),
                &Launch::Terminal(id),
                &file,
                true,
            )
            .unwrap();
            assert_eq!(
                cmd.get_args().collect::<Vec<_>>(),
                vec![OsStr::new(flag), dir.as_os_str()]
            );
            assert_eq!(cmd.get_current_dir(), Some(dir.as_path()));
        }
        for id in ["ghostty", "gnome-terminal"] {
            let cmd = command(
                Path::new("/native/terminal"),
                &Launch::Terminal(id),
                &dir,
                true,
            )
            .unwrap();
            let mut arg = OsString::from("--working-directory=");
            arg.push(&dir);
            assert_eq!(cmd.get_args().collect::<Vec<_>>(), vec![arg.as_os_str()]);
        }
        let mut cmd = command(
            Path::new("/bin/pwd"),
            &Launch::Terminal("terminal"),
            &file,
            true,
        )
        .unwrap();
        assert_eq!(cmd.get_args().count(), 0);
        cmd.stdout(Stdio::piped());
        let output = cmd.output().unwrap();
        assert!(output.status.success());
        assert_eq!(
            Path::new(String::from_utf8(output.stdout).unwrap().trim())
                .canonicalize()
                .unwrap(),
            dir.canonicalize().unwrap()
        );
        assert!(command(
            Path::new("/native/terminal"),
            &Launch::Terminal("unknown"),
            &dir,
            true
        )
        .is_err());
    }

    #[test]
    fn native_launcher_reports_spawn_and_exit_failures_without_diagnostics() {
        let f = Fixture::new();
        let program = f.executable("gio");
        fs::write(
            &program,
            "#!/bin/sh\nprintf 'private diagnostic' >&2\nexit 17\n",
        )
        .unwrap();
        let cmd = command(&program, &Launch::GioOpen, f.root.path(), false).unwrap();
        let error = launch(cmd, true).unwrap_err();
        assert!(error.contains("check its installation"));
        assert!(!error.contains("private diagnostic"));
        fs::write(&program, "#!/bin/sh\nexit 0\n").unwrap();
        let cmd = command(&program, &Launch::GioOpen, f.root.path(), false).unwrap();
        assert!(launch(cmd, true).is_ok());
        fs::remove_file(program).unwrap();
        assert!(launch(
            command(
                Path::new("/missing/launcher"),
                &Launch::GioOpen,
                f.root.path(),
                false
            )
            .unwrap(),
            true
        )
        .unwrap_err()
        .contains("check installation"));
    }

    #[cfg(target_os = "linux")]
    #[test]
    fn native_icon_loader_rasterizes_svg_and_rejects_invalid_or_large_input() {
        use base64::Engine;
        let f = Fixture::new();
        let icon = f.root.path().join("icon.svg");
        fs::write(&icon, r##"<svg xmlns="http://www.w3.org/2000/svg" width="128" height="256"><rect width="128" height="256" fill="#369"/></svg>"##).unwrap();
        let data = icon_data_url(&icon).unwrap();
        let bytes = base64::engine::general_purpose::STANDARD
            .decode(data.strip_prefix("data:image/png;base64,").unwrap())
            .unwrap();
        let png = gdk_pixbuf::Pixbuf::from_read(std::io::Cursor::new(bytes)).unwrap();
        assert_eq!((png.width(), png.height()), (32, 64));
        fs::write(&icon, b"not an image").unwrap();
        assert!(icon_data_url(&icon).is_none());
        fs::write(&icon, vec![0u8; 1024 * 1024 + 1]).unwrap();
        assert!(icon_data_url(&icon).is_none());
    }
}
