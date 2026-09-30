//! Installed application metadata; executable paths never leave the host.
use serde::Serialize;
use std::path::PathBuf;

#[cfg(all(unix, any(target_os = "linux", test)))]
#[cfg_attr(not(target_os = "linux"), allow(dead_code))]
pub(crate) mod linux;

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OpenerView {
    pub(crate) id: &'static str,
    pub(crate) name: String,
    pub(crate) kind: &'static str,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub(crate) icon_data_url: String,
}
#[derive(Serialize)]
pub struct OpenersView {
    pub(crate) openers: Vec<OpenerView>,
    pub(crate) preferred: String,
    #[serde(skip_serializing_if = "is_false")]
    pub(crate) workspace_openable: bool,
}
#[derive(Clone)]
pub(crate) struct OpenerSpec {
    pub(crate) view: OpenerView,
    pub(crate) target: PathBuf,
    #[cfg(target_os = "linux")]
    pub(crate) linux_launch: linux::Launch,
    #[cfg(target_os = "linux")]
    pub(crate) icon_source: Option<PathBuf>,
}

pub(crate) fn selected_opener(specs: Vec<OpenerSpec>, id: &str) -> Result<OpenerSpec, String> {
    let id = id.trim().to_ascii_lowercase();
    specs
        .into_iter()
        .find(|spec| spec.view.id == id)
        .ok_or_else(|| "selected application is no longer installed; choose another opener".into())
}

// Fixed native catalog: the renderer can only select an ID that is currently
// installed. Keep filesystem launch paths and launch arguments out of the UI.
pub(crate) fn installed_openers() -> Vec<OpenerSpec> {
    #[cfg(target_os = "macos")]
    return objc2::rc::autoreleasepool(|_| discover_openers());
    #[cfg(not(target_os = "macos"))]
    discover_openers()
}

fn discover_openers() -> Vec<OpenerSpec> {
    #[cfg(target_os = "macos")]
    {
        let mut roots = vec![
            PathBuf::from("/Applications"),
            PathBuf::from("/System/Applications"),
            PathBuf::from("/System/Applications/Utilities"),
            PathBuf::from("/System/Library/CoreServices"),
        ];
        if let Some(home) = std::env::var_os("HOME") {
            roots.push(PathBuf::from(home).join("Applications"));
        }
        let candidates: &[(&str, &str, &str, &[&str])] = &[
            ("vscode", "VS Code", "editor", &["Visual Studio Code"]),
            (
                "vscode-insiders",
                "VS Code Insiders",
                "editor",
                &["Visual Studio Code - Insiders"],
            ),
            ("cursor", "Cursor", "editor", &["Cursor"]),
            ("finder", "Finder", "file-manager", &["Finder"]),
            ("terminal", "Terminal", "terminal", &["Terminal"]),
            ("iterm", "iTerm2", "terminal", &["iTerm", "iTerm2"]),
            ("ghostty", "Ghostty", "terminal", &["Ghostty"]),
            ("xcode", "Xcode", "editor", &["Xcode"]),
            (
                "android-studio",
                "Android Studio",
                "editor",
                &["Android Studio"],
            ),
            ("goland", "GoLand", "editor", &["GoLand"]),
            ("pycharm", "PyCharm", "editor", &["PyCharm", "PyCharm CE"]),
            (
                "intellij-idea",
                "IntelliJ IDEA",
                "editor",
                &[
                    "IntelliJ IDEA",
                    "IntelliJ IDEA Ultimate",
                    "IntelliJ IDEA CE",
                ],
            ),
            ("webstorm", "WebStorm", "editor", &["WebStorm"]),
            ("datagrip", "DataGrip", "editor", &["DataGrip"]),
            ("codebuddy", "CodeBuddy", "editor", &["CodeBuddy"]),
            ("windsurf", "Windsurf", "editor", &["Windsurf"]),
            ("zed", "Zed", "editor", &["Zed"]),
            ("sublime-text", "Sublime Text", "editor", &["Sublime Text"]),
            ("kiro", "Kiro", "editor", &["Kiro"]),
        ];
        let workspace = objc2_app_kit::NSWorkspace::sharedWorkspace();
        let spotlight = mac_indexed_applications();
        candidates
            .iter()
            .filter_map(|(id, name, kind, aliases)| {
                let registered = mac_bundle_ids(id).iter().find_map(|bundle_id| {
                    workspace
                        .URLForApplicationWithBundleIdentifier(
                            &objc2_foundation::NSString::from_str(bundle_id),
                        )
                        .and_then(|url| url.path())
                        .map(|path| PathBuf::from(path.to_string()))
                        .filter(|path| path.is_dir())
                });
                let target = registered
                    .or_else(|| {
                        aliases
                            .iter()
                            .flat_map(|alias| {
                                roots
                                    .iter()
                                    .map(move |root| root.join(format!("{alias}.app")))
                            })
                            .find(|path| path.is_dir())
                    })
                    .or_else(|| {
                        aliases
                            .iter()
                            .find_map(|alias| spotlight.get(&alias.to_ascii_lowercase()).cloned())
                    })?;
                let target = target.canonicalize().ok()?;
                Some(OpenerSpec {
                    view: OpenerView {
                        id,
                        name: (*name).into(),
                        kind,
                        icon_data_url: String::new(),
                    },
                    target,
                })
            })
            .collect()
    }
    #[cfg(target_os = "linux")]
    {
        linux::Discovery::from_environment()
            .discover()
            .into_iter()
            .map(|app| OpenerSpec {
                view: OpenerView {
                    id: app.id,
                    name: app.name,
                    kind: app.kind,
                    icon_data_url: String::new(),
                },
                target: app.target,
                linux_launch: app.launch,
                icon_source: app.icon,
            })
            .collect()
    }
    #[cfg(not(any(target_os = "macos", target_os = "linux")))]
    {
        let paths = std::env::var_os("PATH")
            .map(|path| std::env::split_paths(&path).collect::<Vec<_>>())
            .unwrap_or_default();
        let candidates: &[(&str, &str, &str, &[&str])] = &[
            ("vscode", "VS Code", "editor", &["code", "Code.exe"]),
            (
                "vscode-insiders",
                "VS Code Insiders",
                "editor",
                &["code-insiders", "Code - Insiders.exe"],
            ),
            ("cursor", "Cursor", "editor", &["cursor", "Cursor.exe"]),
            (
                "android-studio",
                "Android Studio",
                "editor",
                &["android-studio", "studio", "studio64.exe"],
            ),
            (
                "goland",
                "GoLand",
                "editor",
                &["goland", "goland.sh", "goland64.exe"],
            ),
            (
                "pycharm",
                "PyCharm",
                "editor",
                &["pycharm", "pycharm.sh", "pycharm64.exe"],
            ),
            (
                "intellij-idea",
                "IntelliJ IDEA",
                "editor",
                &["idea", "idea.sh", "idea64.exe"],
            ),
            (
                "webstorm",
                "WebStorm",
                "editor",
                &["webstorm", "webstorm.sh", "webstorm64.exe"],
            ),
            (
                "datagrip",
                "DataGrip",
                "editor",
                &["datagrip", "datagrip.sh", "datagrip64.exe"],
            ),
            (
                "codebuddy",
                "CodeBuddy",
                "editor",
                &["codebuddy", "CodeBuddy.exe"],
            ),
            (
                "windsurf",
                "Windsurf",
                "editor",
                &["windsurf", "Windsurf.exe"],
            ),
            ("zed", "Zed", "editor", &["zed", "zed.exe"]),
            (
                "sublime-text",
                "Sublime Text",
                "editor",
                &["subl", "sublime_text", "sublime_text.exe"],
            ),
            ("kiro", "Kiro", "editor", &["kiro", "Kiro.exe"]),
        ];
        candidates
            .iter()
            .filter_map(|(id, name, kind, aliases)| {
                let target = aliases
                    .iter()
                    .flat_map(|alias| paths.iter().map(move |root| root.join(alias)))
                    .find(|path| path.is_file())?;
                Some(OpenerSpec {
                    view: OpenerView {
                        id,
                        name: (*name).into(),
                        kind,
                        icon_data_url: String::new(),
                    },
                    target,
                })
            })
            .collect()
    }
}

/// Match Wails fallback without overwriting a preference for an uninstalled app.
pub(crate) fn resolved_preference<'a>(
    specs: &'a [OpenerSpec],
    preferred: &str,
) -> Option<&'a OpenerSpec> {
    let preferred = preferred.trim().to_ascii_lowercase();
    specs
        .iter()
        .find(|spec| spec.view.id == preferred)
        .or_else(|| specs.iter().find(|spec| spec.view.kind == "file-manager"))
        .or_else(|| specs.first())
}

#[cfg(target_os = "macos")]
fn mac_bundle_ids(id: &str) -> &'static [&'static str] {
    match id {
        "vscode" => &["com.microsoft.VSCode"],
        "vscode-insiders" => &["com.microsoft.VSCodeInsiders"],
        "cursor" => &["com.todesktop.230313mzl4w4u92"],
        "finder" => &["com.apple.finder"],
        "terminal" => &["com.apple.Terminal"],
        "iterm" => &["com.googlecode.iterm2"],
        "ghostty" => &["com.mitchellh.ghostty"],
        "xcode" => &["com.apple.dt.Xcode"],
        "android-studio" => &["com.google.android.studio"],
        "goland" => &["com.jetbrains.goland"],
        "pycharm" => &["com.jetbrains.pycharm", "com.jetbrains.pycharm.ce"],
        "intellij-idea" => &["com.jetbrains.intellij", "com.jetbrains.intellij.ce"],
        "webstorm" => &["com.jetbrains.WebStorm"],
        "datagrip" => &["com.jetbrains.datagrip"],
        "codebuddy" => &["com.tencent.codebuddy"],
        "windsurf" => &["com.exafunction.windsurf"],
        "zed" => &["dev.zed.Zed"],
        "sublime-text" => &["com.sublimetext.4", "com.sublimetext.3"],
        "kiro" => &["dev.kiro.desktop"],
        _ => &[],
    }
}

pub(crate) fn views_with_icons(specs: Vec<OpenerSpec>) -> Vec<OpenerView> {
    specs
        .into_iter()
        .map(|mut spec| {
            #[cfg(target_os = "macos")]
            {
                spec.view.icon_data_url = mac_application_icon(&spec.target).unwrap_or_default();
            }
            #[cfg(target_os = "linux")]
            {
                spec.view.icon_data_url = spec
                    .icon_source
                    .as_deref()
                    .and_then(linux::icon_data_url)
                    .unwrap_or_default();
            }
            spec.view
        })
        .collect()
}

#[cfg(target_os = "macos")]
fn mac_application_icon(path: &std::path::Path) -> Option<String> {
    use base64::Engine;
    use objc2::AnyThread;
    use objc2_app_kit::{NSBitmapImageFileType, NSBitmapImageRep, NSWorkspace};
    use objc2_core_graphics::{
        CGBitmapContextCreate, CGBitmapContextCreateImage, CGColorSpace, CGContext,
        CGImageAlphaInfo,
    };
    use objc2_foundation::{NSDictionary, NSPoint, NSRect, NSSize, NSString};
    objc2::rc::autoreleasepool(|_| {
        let image = NSWorkspace::sharedWorkspace().iconForFile(&NSString::from_str(path.to_str()?));
        let mut rect = NSRect::new(NSPoint::new(0.0, 0.0), NSSize::new(64.0, 64.0));
        // Safety: rect is a live stack value and hints is absent. AppKit picks
        // a native representation; CoreGraphics then renders into owned pixels.
        let native = unsafe { image.CGImageForProposedRect_context_hints(&mut rect, None, None) }?;
        let color_space = CGColorSpace::new_device_rgb()?;
        // Safety: null data delegates the 64x64 RGBA allocation to CoreGraphics.
        // The retained context owns it throughout rendering and image creation.
        let context = unsafe {
            CGBitmapContextCreate(
                std::ptr::null_mut(),
                64,
                64,
                8,
                64 * 4,
                Some(&color_space),
                CGImageAlphaInfo::PremultipliedLast.0,
            )
        }?;
        CGContext::draw_image(
            Some(&context),
            NSRect::new(NSPoint::new(0.0, 0.0), NSSize::new(64.0, 64.0)),
            Some(&native),
        );
        let scaled = CGBitmapContextCreateImage(Some(&context))?;
        let bitmap = NSBitmapImageRep::initWithCGImage(NSBitmapImageRep::alloc(), &scaled);
        // Safety: PNG accepts an empty properties dictionary; there are no
        // untyped values to pass into AppKit's encoder.
        let png = unsafe {
            bitmap.representationUsingType_properties(
                NSBitmapImageFileType::PNG,
                &NSDictionary::new(),
            )
        }?;
        let bytes = png.to_vec();
        if bytes.len() > 64 << 10 || !bytes.starts_with(b"\x89PNG\r\n\x1a\n") {
            return None;
        }
        Some(format!(
            "data:image/png;base64,{}",
            base64::engine::general_purpose::STANDARD.encode(bytes)
        ))
    })
}

// Display metadata is expensive to render. Launch and preference writes always
// rediscover installed apps so removal cannot turn this cache into permission.
pub(crate) fn cached_views() -> Vec<OpenerView> {
    use std::sync::{Mutex, OnceLock};
    use std::time::{Duration, Instant};
    type CachedViews = Option<(Instant, Vec<OpenerView>)>;
    static CACHE: OnceLock<Mutex<CachedViews>> = OnceLock::new();
    let mut cache = CACHE
        .get_or_init(|| Mutex::new(None))
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    if let Some((loaded, views)) = &*cache {
        if loaded.elapsed() < Duration::from_secs(15) {
            return views.clone();
        }
    }
    let views = views_with_icons(installed_openers());
    *cache = Some((Instant::now(), views.clone()));
    views
}

#[cfg(target_os = "macos")]
fn mac_indexed_applications() -> std::collections::BTreeMap<String, PathBuf> {
    use std::io::Read;
    use std::process::{Command, Stdio};
    use std::time::{Duration, Instant};
    let Ok(mut child) = Command::new("/usr/bin/mdfind")
        .args(["-0", "kMDItemContentType == 'com.apple.application-bundle'"])
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()
    else {
        return Default::default();
    };
    let output = child.stdout.take().expect("metadata stdout pipe");
    let reader = std::thread::spawn(move || {
        let mut bytes = Vec::new();
        output.take(8 << 20).read_to_end(&mut bytes).map(|_| bytes)
    });
    let deadline = Instant::now() + Duration::from_secs(2);
    let completed = loop {
        match child.try_wait() {
            Ok(Some(status)) => break status.success(),
            Ok(None) if Instant::now() < deadline => std::thread::sleep(Duration::from_millis(10)),
            _ => {
                let _ = child.kill();
                let _ = child.wait();
                break false;
            }
        }
    };
    let Ok(Ok(bytes)) = reader.join() else {
        return Default::default();
    };
    if !completed || bytes.len() >= 8 << 20 {
        return Default::default();
    }
    indexed_applications_from_bytes(&bytes)
}

#[cfg(target_os = "macos")]
fn indexed_applications_from_bytes(bytes: &[u8]) -> std::collections::BTreeMap<String, PathBuf> {
    let mut index = std::collections::BTreeMap::new();
    for value in bytes.split(|byte| *byte == 0) {
        let Ok(value) = std::str::from_utf8(value) else {
            continue;
        };
        let path = PathBuf::from(value);
        if !path.is_absolute()
            || !path.is_dir()
            || !path
                .extension()
                .is_some_and(|ext| ext.eq_ignore_ascii_case("app"))
        {
            continue;
        }
        let Some(name) = path.file_stem().and_then(|name| name.to_str()) else {
            continue;
        };
        index.entry(name.to_ascii_lowercase()).or_insert(path);
    }
    index
}

fn is_false(value: &bool) -> bool {
    !*value
}

#[cfg(test)]
mod tests {
    use super::*;
    fn spec(id: &'static str, kind: &'static str) -> OpenerSpec {
        OpenerSpec {
            view: OpenerView {
                id,
                name: id.into(),
                kind,
                icon_data_url: String::new(),
            },
            target: PathBuf::from("/native/app"),
            #[cfg(target_os = "linux")]
            linux_launch: linux::Launch::Path,
            #[cfg(target_os = "linux")]
            icon_source: None,
        }
    }
    #[test]
    fn preference_falls_back_to_file_manager_then_first_without_mutating_saved_id() {
        let specs = vec![spec("vscode", "editor"), spec("finder", "file-manager")];
        assert_eq!(
            resolved_preference(&specs, " VSCODE ").unwrap().view.id,
            "vscode"
        );
        assert_eq!(
            resolved_preference(&specs, "uninstalled").unwrap().view.id,
            "finder"
        );
        assert_eq!(
            resolved_preference(&specs[..1], "uninstalled")
                .unwrap()
                .view
                .id,
            "vscode"
        );
        assert!(resolved_preference(&[], "finder").is_none());
        assert_eq!(
            selected_opener(specs, " VSCODE ").unwrap().view.id,
            "vscode"
        );
    }
    #[cfg(target_os = "macos")]
    #[test]
    fn custom_indexed_bundles_support_nul_separators_and_ignore_invalid_entries() {
        let root = tempfile::tempdir().unwrap();
        let custom = root
            .path()
            .join("Custom location\n工具")
            .join("Visual Studio Code.app");
        std::fs::create_dir_all(&custom).unwrap();
        let normal_file = root.path().join("File.app");
        std::fs::write(&normal_file, "not a bundle").unwrap();
        let listing = format!(
            "{}\0{}\0relative.app\0{}\0",
            custom.display(),
            normal_file.display(),
            root.path().join("missing.app").display()
        );
        let index = indexed_applications_from_bytes(listing.as_bytes());
        assert_eq!(index.len(), 1);
        assert_eq!(index["visual studio code"], custom);
    }

    #[cfg(target_os = "macos")]
    #[test]
    fn native_catalog_resolves_finder_terminal_and_png_icons_without_renderer_paths() {
        use base64::Engine;
        let specs = installed_openers();
        for id in ["finder", "terminal"] {
            let spec = specs
                .iter()
                .find(|spec| spec.view.id == id)
                .expect("system application installed");
            assert!(spec.target.is_absolute() && spec.target.is_dir());
            let icon = mac_application_icon(&spec.target)
                .unwrap_or_else(|| panic!("native PNG icon for {id}"));
            let png = base64::engine::general_purpose::STANDARD
                .decode(icon.strip_prefix("data:image/png;base64,").unwrap())
                .unwrap();
            assert!(png.starts_with(b"\x89PNG\r\n\x1a\n"));
            assert_eq!(u32::from_be_bytes(png[16..20].try_into().unwrap()), 64);
            assert_eq!(u32::from_be_bytes(png[20..24].try_into().unwrap()), 64);
            let wire = serde_json::to_string(&spec.view).unwrap();
            assert!(!wire.contains(spec.target.to_str().unwrap()));
        }
    }
}
