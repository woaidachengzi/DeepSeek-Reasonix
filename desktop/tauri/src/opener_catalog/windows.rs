//! Fixed Windows catalog. Registry/program discovery and launch details remain
//! on the host; the renderer only selects an installed application ID.
use std::{
    ffi::OsString,
    fs,
    path::{Path, PathBuf},
};

#[cfg(target_os = "windows")]
pub(crate) mod native;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum Mode {
    Path,
    FileExplorer,
    WindowsTerminal,
    Console,
}
#[derive(Clone, Debug)]
pub(crate) struct Application {
    pub id: &'static str,
    pub name: &'static str,
    pub kind: &'static str,
    pub target: PathBuf,
    pub mode: Mode,
    pub icon: PathBuf,
}
#[derive(Clone, Debug)]
pub(crate) struct Discovery {
    pub paths: Vec<PathBuf>,
    pub local: Option<PathBuf>,
    pub program: Option<PathBuf>,
    pub program_x86: Option<PathBuf>,
    pub windows: Option<PathBuf>,
}
impl Discovery {
    pub fn from_environment() -> Self {
        let root = |key| {
            std::env::var_os(key)
                .map(PathBuf::from)
                .filter(|p| p.is_absolute())
        };
        Self {
            paths: std::env::var_os("PATH")
                .map(|value| {
                    std::env::split_paths(&value)
                        .filter(|p| p.is_absolute())
                        .collect()
                })
                .unwrap_or_default(),
            local: root("LOCALAPPDATA"),
            program: root("ProgramFiles"),
            program_x86: root("ProgramFiles(x86)"),
            windows: root("WINDIR"),
        }
    }

    fn executable(
        &self,
        names: &[&str],
        patterns: &[(Option<&Path>, &[&str])],
        app_path: &impl Fn(&str) -> Option<PathBuf>,
    ) -> Option<PathBuf> {
        for name in names {
            if let Some(path) = self
                .paths
                .iter()
                .filter(|p| p.is_absolute())
                .find_map(|root| executable_file(&root.join(name)))
            {
                return Some(path);
            }
            if let Some(path) = app_path(name).and_then(|path| executable_file(&path)) {
                return Some(path);
            }
        }
        patterns.iter().find_map(|(root, parts)| {
            expand_pattern(root.as_ref().copied()?, parts)
                .into_iter()
                .find_map(|path| executable_file(&path))
        })
    }

    pub fn discover(&self, app_path: impl Fn(&str) -> Option<PathBuf>) -> Vec<Application> {
        let mut apps = vec![];
        let local = self.local.as_deref();
        let program = self.program.as_deref();
        let x86 = self.program_x86.as_deref();
        let windows = self.windows.as_deref();
        let mut add =
            |id, name, kind, mode, names: &[&str], candidates: &[(Option<&Path>, &[&str])]| {
                if let Some(target) = self.executable(names, candidates, &app_path) {
                    apps.push(Application {
                        id,
                        name,
                        kind,
                        icon: target.clone(),
                        target,
                        mode,
                    });
                }
            };
        add(
            "vscode",
            "VS Code",
            "editor",
            Mode::Path,
            &["Code.exe"],
            &[
                (local, &["Programs", "Microsoft VS Code", "Code.exe"]),
                (program, &["Microsoft VS Code", "Code.exe"]),
                (x86, &["Microsoft VS Code", "Code.exe"]),
            ],
        );
        add(
            "vscode-insiders",
            "VS Code Insiders",
            "editor",
            Mode::Path,
            &["Code - Insiders.exe"],
            &[
                (
                    local,
                    &[
                        "Programs",
                        "Microsoft VS Code Insiders",
                        "Code - Insiders.exe",
                    ],
                ),
                (
                    program,
                    &["Microsoft VS Code Insiders", "Code - Insiders.exe"],
                ),
            ],
        );
        add(
            "cursor",
            "Cursor",
            "editor",
            Mode::Path,
            &["Cursor.exe"],
            &[
                (local, &["Programs", "cursor", "Cursor.exe"]),
                (program, &["Cursor", "Cursor.exe"]),
            ],
        );
        add(
            "file-explorer",
            "File Explorer",
            "file-manager",
            Mode::FileExplorer,
            &["explorer.exe"],
            &[(windows, &["explorer.exe"])],
        );
        add(
            "windows-terminal",
            "Windows Terminal",
            "terminal",
            Mode::WindowsTerminal,
            &["wt.exe"],
            &[(local, &["Microsoft", "WindowsApps", "wt.exe"])],
        );
        add(
            "powershell",
            "PowerShell",
            "terminal",
            Mode::Console,
            &["pwsh.exe", "powershell.exe"],
            &[
                (program, &["PowerShell", "*", "pwsh.exe"]),
                (
                    windows,
                    &["System32", "WindowsPowerShell", "v1.0", "powershell.exe"],
                ),
            ],
        );
        add(
            "command-prompt",
            "Command Prompt",
            "terminal",
            Mode::Console,
            &["cmd.exe"],
            &[(windows, &["System32", "cmd.exe"])],
        );
        add(
            "android-studio",
            "Android Studio",
            "editor",
            Mode::Path,
            &["studio64.exe", "studio.exe"],
            &[(
                program,
                &["Android", "Android Studio", "bin", "studio64.exe"],
            )],
        );
        for (id, name, binaries, product, toolbox) in [
            (
                "goland",
                "GoLand",
                &["goland64.exe", "goland.exe"][..],
                "GoLand *",
                "GoLand",
            ),
            (
                "pycharm",
                "PyCharm",
                &["pycharm64.exe", "pycharm.exe"],
                "PyCharm *",
                "PyCharm*",
            ),
            (
                "intellij-idea",
                "IntelliJ IDEA",
                &["idea64.exe", "idea.exe"],
                "IntelliJ IDEA *",
                "IDEA*",
            ),
            (
                "webstorm",
                "WebStorm",
                &["webstorm64.exe", "webstorm.exe"],
                "WebStorm *",
                "WebStorm",
            ),
            (
                "datagrip",
                "DataGrip",
                &["datagrip64.exe", "datagrip.exe"],
                "DataGrip *",
                "DataGrip",
            ),
        ] {
            add(
                id,
                name,
                "editor",
                Mode::Path,
                binaries,
                &[
                    (program, &["JetBrains", product, "bin", binaries[0]]),
                    (
                        local,
                        &[
                            "JetBrains",
                            "Toolbox",
                            "apps",
                            toolbox,
                            "*",
                            "*",
                            "bin",
                            binaries[0],
                        ],
                    ),
                ],
            );
        }
        for (id, name, executable, folder) in [
            ("codebuddy", "CodeBuddy", "CodeBuddy.exe", "CodeBuddy"),
            ("windsurf", "Windsurf", "Windsurf.exe", "Windsurf"),
            ("zed", "Zed", "zed.exe", "Zed"),
            ("kiro", "Kiro", "Kiro.exe", "Kiro"),
        ] {
            add(
                id,
                name,
                "editor",
                Mode::Path,
                &[executable],
                &[
                    (local, &["Programs", folder, executable]),
                    (program, &[folder, executable]),
                ],
            );
        }
        add(
            "sublime-text",
            "Sublime Text",
            "editor",
            Mode::Path,
            &["sublime_text.exe"],
            &[(program, &["Sublime Text", "sublime_text.exe"])],
        );
        let fallback = apps
            .iter()
            .find(|a| a.id == "powershell")
            .map(|a| a.target.clone());
        if let Some(wt) = apps.iter_mut().find(|a| a.id == "windows-terminal") {
            let mut candidates = vec![];
            for (root, parts) in [
                (program, &["WindowsApps"][..]),
                (local, &["Microsoft", "WindowsApps"][..]),
            ] {
                for package in [
                    "Microsoft.WindowsTerminal_*",
                    "Microsoft.WindowsTerminalPreview_*",
                ] {
                    let mut segments = parts.to_vec();
                    segments.extend([package, "WindowsTerminal.exe"]);
                    if let Some(root) = root {
                        candidates.extend(expand_pattern(root, &segments));
                    }
                }
            }
            candidates.push(wt.target.clone());
            wt.icon = terminal_icon_source(&candidates, fallback.as_deref())
                .unwrap_or_else(|| wt.target.clone());
        }
        apps
    }
}

fn executable_file(path: &Path) -> Option<PathBuf> {
    if !path.is_absolute()
        || !path
            .extension()
            .and_then(|p| p.to_str())
            .is_some_and(|ext| ext.eq_ignore_ascii_case("exe"))
        || !fs::metadata(path).is_ok_and(|m| m.is_file())
    {
        return None;
    }
    // Preserve App Execution Aliases and avoid turning ordinary paths into
    // verbatim/device prefixes which ShellExecute may not accept.
    Some(path.into())
}
pub(crate) fn registered_executable(value: &str) -> Option<PathBuf> {
    let value = value.trim();
    let value = if value.starts_with('"') {
        value.strip_prefix('"')?.strip_suffix('"')?
    } else {
        value
    };
    if value.is_empty() || value.len() > 32768 || value.chars().any(|c| c.is_control() || c == '"')
    {
        return None;
    }
    executable_file(Path::new(value))
}

// The only wildcard is one '*' per host-owned path segment. Environment roots
// are literal and renderer text never participates. Bound visits and results.
fn expand_pattern(root: &Path, parts: &[&str]) -> Vec<PathBuf> {
    if !root.is_absolute() {
        return vec![];
    }
    let mut paths = vec![root.to_path_buf()];
    let mut visits = 0;
    for part in parts {
        let mut next = vec![];
        for path in paths {
            if part.contains('*') {
                let Ok(entries) = fs::read_dir(path) else {
                    continue;
                };
                let mut entries: Vec<_> = entries
                    .take(4096usize.saturating_sub(visits).min(512))
                    .filter_map(Result::ok)
                    .collect();
                visits += entries.len();
                entries.sort_by_key(|e| e.file_name().to_ascii_lowercase());
                for entry in entries {
                    if entry
                        .file_name()
                        .to_str()
                        .is_some_and(|name| wildcard_match(part, name))
                    {
                        next.push(entry.path());
                    }
                }
            } else {
                next.push(path.join(part));
            }
            if next.len() >= 512 || visits >= 4096 {
                break;
            }
        }
        next.truncate(512);
        paths = next;
    }
    paths
}
fn wildcard_match(pattern: &str, value: &str) -> bool {
    let pattern = pattern.to_ascii_lowercase();
    let value = value.to_ascii_lowercase();
    match pattern.split_once('*') {
        Some((prefix, suffix)) => {
            !suffix.contains('*')
                && value.len() >= prefix.len() + suffix.len()
                && value.starts_with(prefix)
                && value.ends_with(suffix)
        }
        None => value == pattern,
    }
}
fn terminal_icon_source(candidates: &[PathBuf], fallback: Option<&Path>) -> Option<PathBuf> {
    candidates
        .iter()
        .find(|p| fs::metadata(p).is_ok_and(|m| m.is_file() && m.len() > 0))
        .cloned()
        .or_else(|| {
            fallback
                .filter(|p| fs::metadata(p).is_ok_and(|m| m.is_file() && m.len() > 0))
                .map(PathBuf::from)
        })
        .or_else(|| {
            candidates
                .iter()
                .find(|p| fs::metadata(p).is_ok_and(|m| m.is_file()))
                .cloned()
        })
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum Verb {
    Open,
    Explore,
}
#[derive(Clone, Debug, Eq, PartialEq)]
pub(crate) enum Plan {
    Process {
        target: PathBuf,
        args: Vec<OsString>,
        dir: PathBuf,
    },
    Shell {
        file: PathBuf,
        dir: PathBuf,
        verb: Verb,
    },
}
pub(crate) fn launch_plan(target: &Path, mode: Mode, path: &Path) -> Result<Plan, String> {
    if !target.is_absolute() || !path.is_absolute() {
        return Err(
            "application and workspace paths must be absolute; choose an installed application"
                .into(),
        );
    }
    let dir = if path.is_dir() {
        path
    } else {
        path.parent().ok_or("file has no parent folder")?
    };
    Ok(match mode {
        Mode::Path => Plan::Process {
            target: target.into(),
            args: vec![path.into()],
            dir: dir.into(),
        },
        Mode::WindowsTerminal => Plan::Process {
            target: target.into(),
            args: vec!["-d".into(), dir.into()],
            dir: dir.into(),
        },
        Mode::Console => Plan::Shell {
            file: target.into(),
            dir: dir.into(),
            verb: Verb::Open,
        },
        Mode::FileExplorer => Plan::Shell {
            file: if path.is_dir() {
                path.join("")
            } else {
                path.into()
            },
            dir: dir.into(),
            verb: if path.is_dir() {
                Verb::Explore
            } else {
                Verb::Open
            },
        },
    })
}

pub(crate) fn composites_rgba(
    black: &[u8],
    white: &[u8],
    width: u32,
    height: u32,
) -> Option<Vec<u8>> {
    let count = width.checked_mul(height)?.checked_mul(4)? as usize;
    if width == 0
        || height == 0
        || width > 64
        || height > 64
        || black.len() != count
        || white.len() != count
    {
        return None;
    }
    let mut result = vec![0; count];
    for ((dark, light), rgba) in black
        .chunks_exact(4)
        .zip(white.chunks_exact(4))
        .zip(result.chunks_exact_mut(4))
    {
        let difference: u32 = light[..3]
            .iter()
            .zip(&dark[..3])
            .map(|(a, b)| a.saturating_sub(*b) as u32)
            .sum();
        let alpha = 255 - difference / 3;
        for (dst, src) in rgba[..3].iter_mut().zip(dark[..3].iter().rev()) {
            *dst = ((*src as u32 * 255).checked_div(alpha).unwrap_or(0).min(255)) as u8;
        }
        rgba[3] = alpha as u8;
    }
    Some(result)
}
pub(crate) fn png_data_url(rgba: &[u8], width: u32, height: u32) -> Option<String> {
    use base64::Engine;
    if width == 0
        || height == 0
        || width > 64
        || height > 64
        || rgba.len() != width as usize * height as usize * 4
    {
        return None;
    }
    let mut output = Vec::new();
    {
        let mut encoder = png::Encoder::new(&mut output, width, height);
        encoder.set_color(png::ColorType::Rgba);
        encoder.set_depth(png::BitDepth::Eight);
        let mut writer = encoder.write_header().ok()?;
        writer.write_image_data(rgba).ok()?;
        writer.finish().ok()?;
    }
    if output.len() > 64 * 1024 {
        return None;
    }
    Some(format!(
        "data:image/png;base64,{}",
        base64::engine::general_purpose::STANDARD.encode(output)
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::cell::Cell;
    struct Fixture {
        root: tempfile::TempDir,
        discovery: Discovery,
    }
    impl Fixture {
        fn new() -> Self {
            let root = tempfile::tempdir().unwrap();
            let d = Discovery {
                paths: vec![root.path().join("PATH")],
                local: Some(root.path().join("local")),
                program: Some(root.path().join("program")),
                program_x86: Some(root.path().join("x86")),
                windows: Some(root.path().join("windows")),
            };
            Self { root, discovery: d }
        }
        fn file(&self, parts: &[&str], data: &[u8]) -> PathBuf {
            let mut path = self.root.path().to_path_buf();
            for part in parts {
                path.push(part);
            }
            fs::create_dir_all(path.parent().unwrap()).unwrap();
            fs::write(&path, data).unwrap();
            path
        }
    }
    fn app<'a>(apps: &'a [Application], id: &str) -> &'a Application {
        apps.iter().find(|a| a.id == id).unwrap()
    }

    #[test]
    fn discovery_prefers_path_then_registered_custom_install_then_defaults_and_rechecks_removal() {
        let f = Fixture::new();
        let path = f.file(&["PATH", "Code.exe"], b"program");
        let custom = f.file(&["Custom Editor", "Code.exe"], b"program");
        let default = f.file(
            &["local", "Programs", "Microsoft VS Code", "Code.exe"],
            b"program",
        );
        let reads = Cell::new(0);
        let registry = |name: &str| {
            reads.set(reads.get() + 1);
            (name == "Code.exe").then(|| custom.clone())
        };
        assert_eq!(app(&f.discovery.discover(registry), "vscode").target, path);
        fs::remove_file(&path).unwrap();
        assert_eq!(
            app(&f.discovery.discover(registry), "vscode").target,
            custom
        );
        fs::remove_file(&custom).unwrap();
        assert_eq!(
            app(&f.discovery.discover(registry), "vscode").target,
            default
        );
        fs::remove_file(default).unwrap();
        assert!(!f
            .discovery
            .discover(registry)
            .iter()
            .any(|a| a.id == "vscode"));
        assert!(reads.get() > 0);
    }

    #[test]
    fn fixed_catalog_discovers_all_baseline_editor_and_console_identities() {
        let f = Fixture::new();
        for executable in [
            "Code.exe",
            "Code - Insiders.exe",
            "Cursor.exe",
            "studio64.exe",
            "goland64.exe",
            "pycharm64.exe",
            "idea64.exe",
            "webstorm64.exe",
            "datagrip64.exe",
            "CodeBuddy.exe",
            "Windsurf.exe",
            "zed.exe",
            "sublime_text.exe",
            "Kiro.exe",
            "wt.exe",
            "pwsh.exe",
            "cmd.exe",
            "explorer.exe",
        ] {
            f.file(&["PATH", executable], b"program");
        }
        let apps = f.discovery.discover(|_| None);
        assert_eq!(apps.len(), 18);
        assert_eq!(app(&apps, "file-explorer").mode, Mode::FileExplorer);
        assert_eq!(app(&apps, "windows-terminal").mode, Mode::WindowsTerminal);
        assert_eq!(app(&apps, "powershell").mode, Mode::Console);
        assert_eq!(app(&apps, "command-prompt").mode, Mode::Console);
        for id in [
            "vscode",
            "vscode-insiders",
            "cursor",
            "android-studio",
            "goland",
            "pycharm",
            "intellij-idea",
            "webstorm",
            "datagrip",
            "codebuddy",
            "windsurf",
            "zed",
            "sublime-text",
            "kiro",
        ] {
            assert_eq!(app(&apps, id).mode, Mode::Path);
        }
    }

    #[test]
    fn installation_roots_cover_x86_editors_toolbox_version_trees_and_system_consoles() {
        let f = Fixture::new();
        let code = f.file(&["x86", "Microsoft VS Code", "Code.exe"], b"program");
        let goland = f.file(
            &[
                "program",
                "JetBrains",
                "GoLand 2026.1",
                "bin",
                "goland64.exe",
            ],
            b"program",
        );
        let pycharm = f.file(
            &[
                "local",
                "JetBrains",
                "Toolbox",
                "apps",
                "PyCharm-P",
                "ch-0",
                "261.1",
                "bin",
                "pycharm64.exe",
            ],
            b"program",
        );
        let explorer = f.file(&["windows", "explorer.exe"], b"program");
        let ps = f.file(
            &[
                "windows",
                "System32",
                "WindowsPowerShell",
                "v1.0",
                "powershell.exe",
            ],
            b"program",
        );
        let cmd = f.file(&["windows", "System32", "cmd.exe"], b"program");
        let apps = f.discovery.discover(|_| None);
        for (id, target) in [
            ("vscode", code),
            ("goland", goland),
            ("pycharm", pycharm),
            ("file-explorer", explorer),
            ("powershell", ps),
            ("command-prompt", cmd),
        ] {
            assert_eq!(app(&apps, id).target, target);
        }
    }

    #[test]
    fn registered_values_reject_command_lines_relative_paths_and_non_executables() {
        let f = Fixture::new();
        let target = f.file(&["Custom App", "Code.exe"], b"program");
        assert_eq!(
            registered_executable(&format!("  \"{}\"  ", target.display())),
            Some(target.clone())
        );
        assert_eq!(
            registered_executable(target.to_str().unwrap()),
            Some(target.clone())
        );
        for value in [
            format!("\"{}\" --execute", target.display()),
            format!("{} --execute", target.display()),
            format!("\"{}", target.display()),
            format!("{}\0", target.display()),
            format!("{}\ninvalid", target.display()),
            "Code.exe".into(),
            "https://example.com/Code.exe".into(),
            "\\\\.\\pipe\\reasonix.exe".into(),
            "".into(),
        ] {
            assert!(registered_executable(&value).is_none(), "{value:?}");
        }
        let script = f.file(&["Code.cmd"], b"script");
        assert!(registered_executable(script.to_str().unwrap()).is_none());
        let folder = f.root.path().join("Folder.exe");
        fs::create_dir(&folder).unwrap();
        assert!(registered_executable(folder.to_str().unwrap()).is_none());
        fs::remove_file(target.clone()).unwrap();
        assert!(registered_executable(target.to_str().unwrap()).is_none());
    }

    #[test]
    fn pattern_roots_are_literal_and_enumeration_is_bounded() {
        let f = Fixture::new();
        let target = f.file(
            &["root[1]", "PyCharm-P", "bin", "pycharm64.exe"],
            b"program",
        );
        let matches = expand_pattern(
            &f.root.path().join("root[1]"),
            &["PyCharm*", "bin", "pycharm64.exe"],
        );
        assert_eq!(matches, vec![target]);
        assert!(expand_pattern(Path::new("relative"), &["*"]).is_empty());
        assert!(wildcard_match("PyCharm*", "pycharm-community"));
        assert!(!wildcard_match("PyCharm*", "not-pycharm"));
        for index in 0..520 {
            fs::create_dir_all(
                f.root
                    .path()
                    .join("versions")
                    .join(format!("version-{index}")),
            )
            .unwrap();
        }
        assert!(expand_pattern(&f.root.path().join("versions"), &["*"]).len() <= 512);
    }

    #[test]
    fn terminal_icon_prefers_package_then_renderable_console_before_zero_byte_alias() {
        let f = Fixture::new();
        let alias = f.file(&["local", "Microsoft", "WindowsApps", "wt.exe"], b"");
        let ps = f.file(&["PATH", "powershell.exe"], b"console");
        let apps = f.discovery.discover(|_| None);
        assert_eq!(app(&apps, "windows-terminal").target, alias);
        assert_eq!(app(&apps, "windows-terminal").icon, ps);
        let package = f.file(
            &[
                "program",
                "WindowsApps",
                "Microsoft.WindowsTerminal_1.0",
                "WindowsTerminal.exe",
            ],
            b"package",
        );
        assert_eq!(
            app(&f.discovery.discover(|_| None), "windows-terminal").icon,
            package
        );
        assert_eq!(
            terminal_icon_source(std::slice::from_ref(&alias), None),
            Some(alias)
        );
    }

    #[test]
    fn terminal_plans_preserve_metacharacters_and_file_parent_without_shell_parameters() {
        let f = Fixture::new();
        let target = f.file(&["terminal.exe"], b"program");
        let dir = f.root.path().join("repo & %TEMP% ^ (工作区)");
        fs::create_dir(&dir).unwrap();
        let document = dir.join("read me.txt");
        fs::write(&document, "doc").unwrap();
        for path in [&dir, &document] {
            assert_eq!(
                launch_plan(&target, Mode::WindowsTerminal, path).unwrap(),
                Plan::Process {
                    target: target.clone(),
                    args: vec!["-d".into(), dir.as_os_str().into()],
                    dir: dir.clone()
                }
            );
            assert_eq!(
                launch_plan(&target, Mode::Console, path).unwrap(),
                Plan::Shell {
                    file: target.clone(),
                    dir: dir.clone(),
                    verb: Verb::Open
                }
            );
        }
        assert_eq!(
            launch_plan(&target, Mode::Path, &document).unwrap(),
            Plan::Process {
                target: target.clone(),
                args: vec![document.as_os_str().into()],
                dir: dir.clone()
            }
        );
        assert!(launch_plan(Path::new("cmd.exe"), Mode::Console, &dir).is_err());
        assert!(launch_plan(&target, Mode::Console, Path::new("relative")).is_err());
    }

    #[test]
    fn folder_shell_plan_uses_explore_and_trailing_separator_despite_sibling_shortcut() {
        let f = Fixture::new();
        let target = f.file(&["explorer.exe"], b"program");
        let dir = f.root.path().join("workspace");
        fs::create_dir(&dir).unwrap();
        f.file(&["workspace.lnk"], b"shortcut");
        let Plan::Shell { file, verb, .. } =
            launch_plan(&target, Mode::FileExplorer, &dir).unwrap()
        else {
            panic!("expected native folder plan");
        };
        assert_eq!(verb, Verb::Explore);
        assert!(file
            .as_os_str()
            .to_str()
            .unwrap()
            .ends_with(std::path::MAIN_SEPARATOR));
        assert_eq!(file, dir);
        let document = f.file(&["workspace", "readme.txt"], b"doc");
        let Plan::Shell { file, verb, .. } =
            launch_plan(&target, Mode::FileExplorer, &document).unwrap()
        else {
            panic!("expected native document plan");
        };
        assert_eq!(verb, Verb::Open);
        assert_eq!(file, document);
    }

    #[test]
    fn rgba_reconstruction_preserves_opaque_transparent_and_partial_alpha_in_png() {
        use base64::Engine;
        let black = [0, 0, 0, 255, 0, 0, 0, 255, 0, 0, 128, 255];
        let white = [0, 0, 0, 255, 255, 255, 255, 255, 127, 127, 255, 255];
        let rgba = composites_rgba(&black, &white, 3, 1).unwrap();
        assert_eq!(rgba, [0, 0, 0, 255, 0, 0, 0, 0, 255, 0, 0, 128]);
        let data = png_data_url(&rgba, 3, 1).unwrap();
        let bytes = base64::engine::general_purpose::STANDARD
            .decode(data.strip_prefix("data:image/png;base64,").unwrap())
            .unwrap();
        let mut reader = png::Decoder::new(std::io::Cursor::new(bytes))
            .read_info()
            .unwrap();
        let mut pixels = vec![0; reader.output_buffer_size().unwrap()];
        let info = reader.next_frame(&mut pixels).unwrap();
        assert_eq!((info.width, info.height), (3, 1));
        assert_eq!(&pixels[..info.buffer_size()], &rgba);
        assert!(composites_rgba(&black, &white[..4], 3, 1).is_none());
        assert!(composites_rgba(&[], &[], u32::MAX, 2).is_none());
        assert!(png_data_url(&[], 65, 1).is_none());
    }
}
