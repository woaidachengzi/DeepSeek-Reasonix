use std::{collections::BTreeMap, fs, io::Write, path::PathBuf, sync::Mutex};

use serde::{Deserialize, Serialize};
use tauri::Manager;

const FILE_NAME: &str = "host-preferences.json";
const USER_THEME_LIMIT: usize = 48;
const THEME_TOKENS: &[&str] = &[
    "bg",
    "bgSoft",
    "bgElev",
    "panel",
    "sidebar",
    "chat",
    "workspace",
    "workspaceFiles",
    "border",
    "borderSoft",
    "fg",
    "fgDim",
    "fgFaint",
    "accent",
    "accentFg",
    "ok",
    "warn",
    "err",
];
const BASE_STYLES: &[&str] = &["graphite", "aurora", "slate", "carbon", "nocturne", "amber"];

#[derive(Clone, Debug, Default, Deserialize, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ThemeTokens {
    #[serde(default)]
    pub light: BTreeMap<String, String>,
    #[serde(default)]
    pub dark: BTreeMap<String, String>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct UserTheme {
    pub id: String,
    pub name: String,
    #[serde(default)]
    pub author: Option<String>,
    #[serde(default)]
    pub description: Option<String>,
    #[serde(default)]
    pub license: Option<String>,
    pub base_style: String,
    #[serde(default)]
    pub tokens: ThemeTokens,
    #[serde(default = "default_density")]
    pub density: String,
    #[serde(default = "default_corners")]
    pub corners: String,
    #[serde(default)]
    pub background: Option<ThemeBackground>,
    #[serde(default)]
    pub task_background: Option<ThemeSceneBackground>,
    #[serde(skip)]
    pub(crate) background_asset_bytes: Option<Vec<u8>>,
    #[serde(skip)]
    pub(crate) task_background_asset_bytes: Option<Vec<u8>>,
    #[serde(default, skip_serializing)]
    pub(crate) background_asset_data_url: Option<String>,
    #[serde(default, skip_serializing)]
    pub(crate) task_background_asset_data_url: Option<String>,
    #[serde(default, skip_serializing)]
    pub(crate) clear_background: bool,
    #[serde(default, skip_serializing)]
    pub(crate) clear_task_background: bool,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ThemeBackground {
    #[serde(default)]
    pub image: String,
    #[serde(default = "default_focus")]
    pub focus_x: f64,
    #[serde(default = "default_focus")]
    pub focus_y: f64,
    #[serde(default = "default_safe_area")]
    pub safe_area: String,
    #[serde(default = "default_full_opacity")]
    pub home_opacity: f64,
    #[serde(default = "default_task_opacity")]
    pub task_opacity: f64,
    #[serde(default = "default_overlay_strength")]
    pub overlay_strength: f64,
    #[serde(default)]
    pub pane_opacity: Option<f64>,
}

impl Default for ThemeBackground {
    fn default() -> Self {
        Self {
            image: String::new(),
            focus_x: 0.5,
            focus_y: 0.5,
            safe_area: "center".into(),
            home_opacity: 1.0,
            task_opacity: 0.28,
            overlay_strength: 0.62,
            pane_opacity: Some(0.5),
        }
    }
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ThemeSceneBackground {
    #[serde(default)]
    pub image: String,
    #[serde(default = "default_focus")]
    pub focus_x: f64,
    #[serde(default = "default_focus")]
    pub focus_y: f64,
    #[serde(default = "default_safe_area")]
    pub safe_area: String,
    #[serde(default = "default_task_opacity")]
    pub opacity: f64,
    #[serde(default = "default_overlay_strength")]
    pub overlay_strength: f64,
    #[serde(default)]
    pub pane_opacity: Option<f64>,
}

impl Default for ThemeSceneBackground {
    fn default() -> Self {
        Self {
            image: String::new(),
            focus_x: 0.5,
            focus_y: 0.5,
            safe_area: "center".into(),
            opacity: 0.28,
            overlay_strength: 0.62,
            pane_opacity: Some(0.68),
        }
    }
}

fn default_focus() -> f64 {
    0.5
}
fn default_safe_area() -> String {
    "center".into()
}
fn default_full_opacity() -> f64 {
    1.0
}
fn default_task_opacity() -> f64 {
    0.28
}
fn default_overlay_strength() -> f64 {
    0.62
}

fn default_density() -> String {
    "comfortable".into()
}
fn default_corners() -> String {
    "soft".into()
}

#[derive(Clone, Copy, Debug, Deserialize, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum CloseBehavior {
    KeepRunning,
    Quit,
}

impl CloseBehavior {
    pub(crate) fn effective_for_tray(self, platform: &str, available: bool) -> Self {
        if platform == "linux" && !available { Self::Quit } else { self }
    }

    fn platform_default(platform: &str) -> Self {
        // Linux desktops do not consistently expose an AppIndicator tray.
        // Preserve explicit saved choices; only a new/missing setting uses this.
        if platform == "linux" { Self::Quit } else { Self::KeepRunning }
    }
}

impl Default for CloseBehavior {
    fn default() -> Self {
        Self::platform_default(std::env::consts::OS)
    }
}

pub struct HostPreferences {
    path: PathBuf,
    asset_root: PathBuf,
    preferences: Mutex<SavedPreferences>,
}

#[derive(Clone, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct SavedPreferences {
    #[serde(default)]
    close_behavior: CloseBehavior,
    #[serde(default = "default_zoom_factor")]
    zoom_factor: f64,
    #[serde(default)]
    active_theme_id: String,
    #[serde(default)]
    user_themes: Vec<UserTheme>,
}

impl Default for SavedPreferences {
    fn default() -> Self {
        Self {
            close_behavior: CloseBehavior::default(),
            zoom_factor: default_zoom_factor(),
            active_theme_id: String::new(),
            user_themes: Vec::new(),
        }
    }
}

impl Default for UserTheme {
    fn default() -> Self {
        Self {
            id: String::new(),
            name: String::new(),
            author: None,
            description: None,
            license: None,
            base_style: "graphite".into(),
            tokens: ThemeTokens::default(),
            density: default_density(),
            corners: default_corners(),
            background: None,
            task_background: None,
            background_asset_bytes: None,
            task_background_asset_bytes: None,
            background_asset_data_url: None,
            task_background_asset_data_url: None,
            clear_background: false,
            clear_task_background: false,
        }
    }
}

fn default_zoom_factor() -> f64 {
    1.0
}

impl HostPreferences {
    pub fn for_app(app: &tauri::App) -> Result<Self, String> {
        let directory = app
            .path()
            .app_data_dir()
            .map_err(|error| format!("resolve Tauri preferences directory: {error}"))?;
        fs::create_dir_all(&directory)
            .map_err(|error| format!("create Tauri preferences directory: {error}"))?;
        let path = directory.join(FILE_NAME);
        let asset_root = directory.join("theme-assets");
        fs::create_dir_all(&asset_root)
            .map_err(|error| format!("create theme asset directory: {error}"))?;
        let saved = read_preferences(&path).unwrap_or_default();
        Ok(Self {
            path,
            asset_root,
            preferences: Mutex::new(saved),
        })
    }

    pub fn close_behavior(&self) -> CloseBehavior {
        self.preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .close_behavior
    }

    pub fn set_close_behavior(&self, behavior: CloseBehavior) -> Result<(), String> {
        let mut current = self
            .preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        if current.close_behavior == behavior {
            return Ok(());
        }
        let next = SavedPreferences {
            close_behavior: behavior,
            ..current.clone()
        };
        self.write_preferences(next.clone())?;
        *current = next;
        Ok(())
    }

    pub fn zoom_factor(&self) -> f64 {
        self.preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .zoom_factor
    }

    pub fn set_zoom_factor(&self, zoom_factor: f64) -> Result<(), String> {
        if !zoom_factor.is_finite() || !(0.5..=2.0).contains(&zoom_factor) {
            return Err("zoom factor must be between 0.5 and 2.0".to_string());
        }
        let snapped = ((zoom_factor * 20.0).round() / 20.0 * 100.0).round() / 100.0;
        let mut current = self
            .preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        if (current.zoom_factor - snapped).abs() < f64::EPSILON {
            return Ok(());
        }
        let next = SavedPreferences {
            zoom_factor: snapped,
            ..current.clone()
        };
        self.write_preferences(next.clone())?;
        *current = next;
        Ok(())
    }

    pub fn active_theme_id(&self) -> String {
        self.preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .active_theme_id
            .clone()
    }

    pub fn set_active_theme_id(&self, id: String) -> Result<(), String> {
        const OFFICIAL_THEMES: &[&str] = &[
            "official-rose-dawn",
            "official-fortune-forge",
            "official-crimson-horizon",
            "official-sage-breeze",
            "official-spark-notebook",
            "official-violet-starlight",
            "official-cyan-stage",
            "official-noir-gold",
        ];
        let id = id.trim();
        let mut current = self
            .preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        if !id.is_empty()
            && !OFFICIAL_THEMES.contains(&id)
            && !current.user_themes.iter().any(|theme| theme.id == id)
            && !valid_plugin_theme_id(id)
        {
            return Err("unknown theme id".to_string());
        }
        if current.active_theme_id == id {
            return Ok(());
        }
        let next = SavedPreferences {
            active_theme_id: id.to_string(),
            ..current.clone()
        };
        self.write_preferences(next.clone())?;
        *current = next;
        Ok(())
    }

    pub fn user_themes(&self) -> Vec<UserTheme> {
        self.preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .user_themes
            .clone()
    }

    pub fn save_user_theme(&self, mut theme: UserTheme) -> Result<UserTheme, String> {
        let mut current = self
            .preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let mut themes = current.user_themes.clone();
        if let Some(index) = themes.iter().position(|existing| existing.id == theme.id) {
            if theme.author.is_none() {
                theme.author = themes[index].author.clone();
            }
            if theme.description.is_none() {
                theme.description = themes[index].description.clone();
            }
            if theme.license.is_none() {
                theme.license = themes[index].license.clone();
            }
            if theme.clear_background {
                theme.background = None;
            } else if theme.background.is_none() {
                theme.background = themes[index].background.clone();
            }
            if theme.clear_task_background {
                theme.task_background = None;
            } else if theme.task_background.is_none() {
                theme.task_background = themes[index].task_background.clone();
            }
            themes[index] = theme.clone();
        } else {
            if themes.len() >= USER_THEME_LIMIT {
                return Err(format!(
                    "at most {USER_THEME_LIMIT} custom themes are supported"
                ));
            }
            themes.push(theme.clone());
        }
        theme = validate_user_theme(theme)?;
        if let Some(saved) = themes.iter_mut().find(|saved| saved.id == theme.id) {
            *saved = theme.clone();
        }

        let asset_dir = self.asset_root.join(&theme.id);
        let staging_dir = self.asset_root.join(format!(".{}-save", theme.id));
        let backup_dir = self.asset_root.join(format!(".{}-backup", theme.id));
        for temporary in [&staging_dir, &backup_dir] {
            if temporary.exists() {
                fs::remove_dir_all(temporary)
                    .map_err(|error| format!("clear theme asset transaction: {error}"))?;
            }
        }
        fs::create_dir_all(&staging_dir)
            .map_err(|error| format!("prepare theme asset update: {error}"))?;
        let old_theme = current
            .user_themes
            .iter()
            .find(|existing| existing.id == theme.id);
        let write_scene = |image: Option<&str>, bytes: Option<&Vec<u8>>| -> Result<(), String> {
            let Some(image) = image.filter(|image| !image.is_empty()) else {
                return Ok(());
            };
            let destination = staging_dir.join(image);
            if let Some(bytes) = bytes {
                fs::write(destination, bytes).map_err(|error| format!("stage theme image: {error}"))
            } else if let Some(previous) =
                old_theme.and_then(|old| self.theme_asset_path(old, image))
            {
                fs::copy(previous, destination)
                    .map(|_| ())
                    .map_err(|error| format!("preserve theme image: {error}"))
            } else {
                Err("theme image data is missing; choose the image again".into())
            }
        };
        let asset_result = (|| {
            write_scene(
                theme.background.as_ref().map(|bg| bg.image.as_str()),
                theme.background_asset_bytes.as_ref(),
            )?;
            write_scene(
                theme.task_background.as_ref().map(|bg| bg.image.as_str()),
                theme.task_background_asset_bytes.as_ref(),
            )?;
            Ok::<_, String>(())
        })();
        if let Err(error) = asset_result {
            let _ = fs::remove_dir_all(&staging_dir);
            return Err(error);
        }
        theme.background_asset_bytes = None;
        theme.task_background_asset_bytes = None;
        theme.background_asset_data_url = None;
        theme.task_background_asset_data_url = None;
        theme.clear_background = false;
        theme.clear_task_background = false;
        if let Some(saved) = themes.iter_mut().find(|saved| saved.id == theme.id) {
            *saved = theme.clone();
        }
        let had_assets = asset_dir.exists();
        if had_assets {
            if let Err(error) = fs::rename(&asset_dir, &backup_dir) {
                let _ = fs::remove_dir_all(&staging_dir);
                return Err(format!("stage current theme assets: {error}"));
            }
        }
        if let Err(error) = fs::rename(&staging_dir, &asset_dir) {
            if had_assets {
                let _ = fs::rename(&backup_dir, &asset_dir);
            }
            let _ = fs::remove_dir_all(&staging_dir);
            return Err(format!("publish theme asset update: {error}"));
        }
        let next = SavedPreferences {
            user_themes: themes,
            ..current.clone()
        };
        if let Err(error) = self.write_preferences(next.clone()) {
            let _ = fs::remove_dir_all(&asset_dir);
            if had_assets {
                let _ = fs::rename(&backup_dir, &asset_dir);
            }
            return Err(error);
        }
        *current = next;
        if had_assets {
            let _ = fs::remove_dir_all(&backup_dir);
        }
        Ok(theme)
    }

    pub fn import_user_theme(&self, mut theme: UserTheme) -> Result<UserTheme, String> {
        let source_id = theme.id.to_ascii_lowercase();
        let mut slug = String::new();
        let mut separator = false;
        for character in source_id.chars() {
            if character.is_ascii_alphanumeric() {
                if separator && !slug.is_empty() {
                    slug.push('-');
                }
                separator = false;
                slug.push(character);
            } else {
                separator = true;
            }
            if slug.len() >= 48 {
                break;
            }
        }
        let slug = slug.trim_matches('-');
        let slug = if slug.is_empty() { "imported" } else { slug };
        let base_id = format!("user-{slug}");

        let mut current = self
            .preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        if current.user_themes.len() >= USER_THEME_LIMIT {
            return Err(format!(
                "at most {USER_THEME_LIMIT} custom themes are supported"
            ));
        }
        let mut id = base_id.clone();
        let mut suffix = 2_u32;
        while current.user_themes.iter().any(|existing| existing.id == id) {
            id = format!("{base_id}-{suffix}");
            suffix += 1;
        }
        theme.id = id;
        let mut theme = validate_user_theme(theme)?;
        let asset_dir = self.asset_root.join(&theme.id);
        let staging_dir = self.asset_root.join(format!(".{}-import", theme.id));
        if staging_dir.exists() {
            fs::remove_dir_all(&staging_dir)
                .map_err(|error| format!("clear staged theme assets: {error}"))?;
        }
        fs::create_dir_all(&staging_dir)
            .map_err(|error| format!("prepare theme assets: {error}"))?;
        let write_asset = |image: &str, bytes: &Option<Vec<u8>>| -> Result<(), String> {
            if image.is_empty() {
                return Ok(());
            }
            let bytes = bytes
                .as_ref()
                .ok_or_else(|| "theme image asset is missing".to_string())?;
            let file = PathBuf::from(image);
            if file.file_name().and_then(|name| name.to_str()) != Some(image) {
                return Err("theme image name must be a plain file name".into());
            }
            fs::write(staging_dir.join(image), bytes)
                .map_err(|error| format!("save theme image: {error}"))
        };
        let asset_result = (|| {
            if let Some(background) = &theme.background {
                write_asset(&background.image, &theme.background_asset_bytes)?;
            }
            if let Some(background) = &theme.task_background {
                write_asset(&background.image, &theme.task_background_asset_bytes)?;
            }
            Ok::<_, String>(())
        })();
        if let Err(error) = asset_result {
            let _ = fs::remove_dir_all(&staging_dir);
            return Err(error);
        }
        theme.background_asset_bytes = None;
        theme.task_background_asset_bytes = None;
        if asset_dir.exists() {
            let _ = fs::remove_dir_all(&staging_dir);
            return Err("theme asset directory already exists".into());
        }
        if let Err(error) = fs::rename(&staging_dir, &asset_dir) {
            let _ = fs::remove_dir_all(&staging_dir);
            return Err(format!("publish theme assets: {error}"));
        }
        let mut themes = current.user_themes.clone();
        themes.push(theme.clone());
        let next = SavedPreferences {
            user_themes: themes,
            ..current.clone()
        };
        if let Err(error) = self.write_preferences(next.clone()) {
            let _ = fs::remove_dir_all(&asset_dir);
            return Err(error);
        }
        *current = next;
        Ok(theme)
    }

    pub fn theme_asset_path(&self, theme: &UserTheme, image: &str) -> Option<PathBuf> {
        let path = PathBuf::from(image);
        if !safe_theme_id(&theme.id)
            || image.is_empty()
            || path.file_name().and_then(|name| name.to_str()) != Some(image)
        {
            return None;
        }
        let candidate = self.asset_root.join(&theme.id).join(path);
        let root = self.asset_root.canonicalize().ok()?;
        let resolved = candidate.canonicalize().ok()?;
        if !resolved.starts_with(root) || !candidate.symlink_metadata().ok()?.file_type().is_file()
        {
            return None;
        }
        Some(resolved)
    }

    pub fn delete_user_theme(&self, id: &str) -> Result<(), String> {
        let mut current = self
            .preferences
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let themes: Vec<_> = current
            .user_themes
            .iter()
            .filter(|theme| theme.id != id)
            .cloned()
            .collect();
        if themes.len() == current.user_themes.len() {
            return Err("custom theme not found".to_string());
        }
        let next = SavedPreferences {
            user_themes: themes,
            active_theme_id: if current.active_theme_id == id {
                String::new()
            } else {
                current.active_theme_id.clone()
            },
            ..current.clone()
        };
        self.write_preferences(next.clone())?;
        *current = next;
        if safe_theme_id(id) {
            let _ = fs::remove_dir_all(self.asset_root.join(id));
        }
        Ok(())
    }

    fn write_preferences(&self, saved: SavedPreferences) -> Result<(), String> {
        let data = serde_json::to_vec(&saved)
            .map_err(|error| format!("encode Tauri preferences: {error}"))?;
        let temporary = self.path.with_extension("json.tmp");
        let result = (|| -> std::io::Result<()> {
            let mut file = fs::File::create(&temporary)?;
            file.write_all(&data)?;
            file.sync_all()?;
            fs::rename(&temporary, &self.path)
        })();
        if let Err(error) = result {
            let _ = fs::remove_file(&temporary);
            return Err(format!("save Tauri preferences: {error}"));
        }
        Ok(())
    }
}

fn valid_plugin_theme_id(id: &str) -> bool {
    let Some(rest) = id.strip_prefix("plugin:") else {
        return false;
    };
    let Some((plugin, theme)) = rest.split_once(':') else {
        return false;
    };
    let valid_plugin_name = !plugin.is_empty()
        && plugin.len() <= 64
        && plugin.as_bytes()[0].is_ascii_alphanumeric()
        && plugin
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || b"._-".contains(&byte));
    let valid_theme_id = !theme.is_empty()
        && theme.len() <= 64
        && theme.as_bytes()[0].is_ascii_lowercase()
        && theme
            .bytes()
            .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-')
        && !theme.ends_with('-')
        && !theme.contains("--");
    valid_plugin_name && valid_theme_id
}

fn safe_theme_id(id: &str) -> bool {
    id.starts_with("user-")
        && id.len() <= 64
        && !id.ends_with('-')
        && !id.contains("--")
        && id
            .bytes()
            .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-')
}

pub(crate) fn validate_user_theme(mut theme: UserTheme) -> Result<UserTheme, String> {
    let id = theme.id.trim();
    if !id.starts_with("user-")
        || id.len() > 64
        || !id
            .bytes()
            .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-')
    {
        return Err("custom theme id must start with user- and contain lowercase letters, digits, and hyphens".to_string());
    }
    if id.ends_with('-') || id.contains("--") {
        return Err("invalid custom theme id".to_string());
    }
    theme.id = id.to_string();
    theme.name = theme.name.trim().to_string();
    if theme.name.is_empty()
        || theme.name.chars().count() > 64
        || theme.name.chars().any(char::is_control)
    {
        return Err("custom theme name must contain 1–64 printable characters".to_string());
    }
    theme.base_style = theme.base_style.trim().to_ascii_lowercase();
    if !BASE_STYLES.contains(&theme.base_style.as_str()) {
        return Err("invalid custom theme base style".to_string());
    }
    if !matches!(theme.density.as_str(), "compact" | "comfortable") {
        return Err("invalid theme density".to_string());
    }
    if !matches!(theme.corners.as_str(), "square" | "soft" | "round") {
        return Err("invalid theme corner style".to_string());
    }
    validate_theme_colors(&mut theme.tokens.light)?;
    validate_theme_colors(&mut theme.tokens.dark)?;
    if let Some(background) = &theme.background {
        validate_theme_image_ref(&background.image)?;
        validate_scene_values(
            background.focus_x,
            background.focus_y,
            background.home_opacity,
            background.overlay_strength,
            background.pane_opacity,
        )?;
        if !(0.0..=1.0).contains(&background.task_opacity)
            || !matches!(background.safe_area.as_str(), "left" | "right" | "center")
        {
            return Err("invalid theme background settings".into());
        }
    }
    if let Some(background) = &theme.task_background {
        validate_theme_image_ref(&background.image)?;
        validate_scene_values(
            background.focus_x,
            background.focus_y,
            background.opacity,
            background.overlay_strength,
            background.pane_opacity,
        )?;
        if !matches!(background.safe_area.as_str(), "left" | "right" | "center") {
            return Err("invalid task background settings".into());
        }
    }
    if let (Some(home), Some(task)) = (&theme.background, &theme.task_background) {
        if !home.image.is_empty() && home.image == task.image {
            return Err("theme scenes must use separate image files".into());
        }
    }
    Ok(theme)
}

fn validate_theme_image_ref(image: &str) -> Result<(), String> {
    if image.is_empty() {
        return Ok(());
    }
    let path = PathBuf::from(image);
    if path.file_name().and_then(|name| name.to_str()) != Some(image)
        || image.contains("..")
        || image.len() > 128
        || !matches!(
            image
                .rsplit_once('.')
                .map(|(_, ext)| ext.to_ascii_lowercase())
                .as_deref(),
            Some("png" | "jpg" | "jpeg" | "webp")
        )
        || !image
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || b"._-".contains(&byte))
    {
        return Err("invalid theme image reference".into());
    }
    Ok(())
}

fn validate_scene_values(
    focus_x: f64,
    focus_y: f64,
    opacity: f64,
    overlay: f64,
    pane: Option<f64>,
) -> Result<(), String> {
    if [focus_x, focus_y, opacity, overlay]
        .into_iter()
        .chain(pane)
        .any(|value| !value.is_finite() || !(0.0..=1.0).contains(&value))
    {
        return Err("theme image settings must be between 0 and 1".into());
    }
    Ok(())
}

fn validate_theme_colors(tokens: &mut BTreeMap<String, String>) -> Result<(), String> {
    if tokens.len() > THEME_TOKENS.len() {
        return Err("too many theme color tokens".to_string());
    }
    for (key, color) in tokens.iter_mut() {
        if !THEME_TOKENS.contains(&key.as_str()) {
            return Err(format!("unknown theme color token: {key}"));
        }
        let value = color.trim();
        let digits = value.strip_prefix('#').unwrap_or("");
        if !(digits.len() == 6 || digits.len() == 8)
            || !digits.bytes().all(|byte| byte.is_ascii_hexdigit())
        {
            return Err(format!("theme color {key} must be #RRGGBB or #RRGGBBAA"));
        }
        *color = value.to_ascii_lowercase();
    }
    Ok(())
}

fn read_preferences(path: &std::path::Path) -> Option<SavedPreferences> {
    let data = fs::read(path).ok()?;
    serde_json::from_slice::<SavedPreferences>(&data).ok()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn close_default_is_safe_without_a_linux_tray_and_preserves_saved_choices() {
        assert_eq!(CloseBehavior::platform_default("linux"), CloseBehavior::Quit);
        for platform in ["macos", "windows"] {
            assert_eq!(CloseBehavior::platform_default(platform), CloseBehavior::KeepRunning);
        }
        let saved: SavedPreferences = serde_json::from_str(r#"{"closeBehavior":"keep_running"}"#).unwrap();
        assert_eq!(saved.close_behavior, CloseBehavior::KeepRunning);
        let missing: SavedPreferences = serde_json::from_str("{}").unwrap();
        assert_eq!(missing.close_behavior, CloseBehavior::default());
    }

    #[test]
    fn absent_tray_only_changes_linux_background_close() {
        assert_eq!(CloseBehavior::KeepRunning.effective_for_tray("linux", false), CloseBehavior::Quit);
        assert_eq!(CloseBehavior::KeepRunning.effective_for_tray("linux", true), CloseBehavior::KeepRunning);
        for platform in ["linux", "macos", "windows"] {
            assert_eq!(CloseBehavior::Quit.effective_for_tray(platform, false), CloseBehavior::Quit);
        }
        for platform in ["macos", "windows"] {
            assert_eq!(CloseBehavior::KeepRunning.effective_for_tray(platform, false), CloseBehavior::KeepRunning);
        }
    }

    #[test]
    fn close_preference_survives_restart_and_ignores_corrupt_data() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join(FILE_NAME);
        let store = HostPreferences {
            path: path.clone(),
            asset_root: directory.path().join("theme-assets"),
            preferences: Mutex::new(SavedPreferences { close_behavior: CloseBehavior::KeepRunning, ..SavedPreferences::default() }),
        };
        store
            .set_close_behavior(CloseBehavior::Quit)
            .expect("save close behavior");
        assert_eq!(store.close_behavior(), CloseBehavior::Quit);
        assert_eq!(
            read_preferences(&path).unwrap().close_behavior,
            CloseBehavior::Quit
        );
        assert_eq!(read_preferences(&path).unwrap().zoom_factor, 1.0);
        store.set_zoom_factor(1.27).expect("save zoom factor");
        assert_eq!(store.zoom_factor(), 1.25);
        assert_eq!(read_preferences(&path).unwrap().zoom_factor, 1.25);
        fs::write(path.as_path(), b"invalid json").expect("corrupt preference file");
        assert!(read_preferences(&path).is_none());
    }

    #[test]
    fn legacy_close_only_preferences_keep_the_default_zoom() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join(FILE_NAME);
        fs::write(&path, br#"{"closeBehavior":"quit"}"#).expect("write legacy preferences");
        let saved = read_preferences(&path).expect("read legacy preferences");
        assert_eq!(saved.close_behavior, CloseBehavior::Quit);
        assert_eq!(saved.zoom_factor, 1.0);
    }

    #[test]
    fn failed_save_keeps_the_previous_runtime_choice() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let store = HostPreferences {
            path: directory.path().join("missing").join(FILE_NAME),
            asset_root: directory.path().join("theme-assets"),
            preferences: Mutex::new(SavedPreferences { close_behavior: CloseBehavior::KeepRunning, ..SavedPreferences::default() }),
        };
        assert!(store.set_close_behavior(CloseBehavior::Quit).is_err());
        assert_eq!(store.close_behavior(), CloseBehavior::KeepRunning);
    }

    #[test]
    fn invalid_zoom_does_not_change_the_saved_value() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let store = HostPreferences {
            path: directory.path().join(FILE_NAME),
            asset_root: directory.path().join("theme-assets"),
            preferences: Mutex::new(SavedPreferences::default()),
        };
        assert!(store.set_zoom_factor(f64::NAN).is_err());
        assert!(store.set_zoom_factor(2.1).is_err());
        assert_eq!(store.zoom_factor(), 1.0);
    }

    #[test]
    fn active_official_theme_survives_restart_and_rejects_untrusted_ids() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join(FILE_NAME);
        let store = HostPreferences {
            path: path.clone(),
            asset_root: directory.path().join("theme-assets"),
            preferences: Mutex::new(SavedPreferences::default()),
        };
        assert!(store
            .set_active_theme_id("../../settings.json".into())
            .is_err());
        assert_eq!(store.active_theme_id(), "");
        store
            .set_active_theme_id("official-rose-dawn".into())
            .expect("save official theme");
        let saved = read_preferences(&path).expect("read saved theme");
        assert_eq!(saved.active_theme_id, "official-rose-dawn");
        assert_eq!(saved.zoom_factor, 1.0);
        store
            .set_active_theme_id("plugin:sample:neon-night".into())
            .expect("save contributed theme id");
        assert_eq!(
            read_preferences(&path)
                .expect("read contributed theme")
                .active_theme_id,
            "plugin:sample:neon-night"
        );
        assert!(store
            .set_active_theme_id("plugin:../sample:neon-night".into())
            .is_err());
    }

    #[test]
    fn custom_theme_is_validated_persisted_activated_and_deleted() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join(FILE_NAME);
        let store = HostPreferences {
            path: path.clone(),
            asset_root: directory.path().join("theme-assets"),
            preferences: Mutex::new(SavedPreferences::default()),
        };
        let theme = UserTheme {
            id: "user-warm-night".into(),
            name: "Warm Night".into(),
            base_style: "carbon".into(),
            tokens: ThemeTokens {
                light: BTreeMap::new(),
                dark: BTreeMap::from([
                    ("accent".into(), "#d9b45b".into()),
                    ("bg".into(), "#0d0b09".into()),
                ]),
            },
            density: "comfortable".into(),
            corners: "soft".into(),
            ..UserTheme::default()
        };
        assert_eq!(
            store
                .save_user_theme(theme.clone())
                .expect("save user theme"),
            theme
        );
        store
            .set_active_theme_id("user-warm-night".into())
            .expect("activate user theme");
        let restored = read_preferences(&path).expect("read custom theme");
        assert_eq!(restored.user_themes, vec![theme]);
        assert_eq!(restored.active_theme_id, "user-warm-night");
        store
            .delete_user_theme("user-warm-night")
            .expect("delete theme");
        assert!(store.user_themes().is_empty());
        assert_eq!(store.active_theme_id(), "");
    }

    #[test]
    fn imported_theme_gets_a_safe_unique_preview_id() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join(FILE_NAME);
        let store = HostPreferences {
            path: path.clone(),
            asset_root: directory.path().join("theme-assets"),
            preferences: Mutex::new(SavedPreferences::default()),
        };
        let imported = UserTheme {
            id: "Warm Night / v2".into(),
            name: "Warm Night".into(),
            base_style: "carbon".into(),
            tokens: ThemeTokens::default(),
            density: "compact".into(),
            corners: "round".into(),
            ..UserTheme::default()
        };
        let first = store
            .import_user_theme(imported.clone())
            .expect("import theme");
        let second = store
            .import_user_theme(imported)
            .expect("import duplicate as copy");
        assert_eq!(first.id, "user-warm-night-v2");
        assert_eq!(second.id, "user-warm-night-v2-2");
        assert_eq!(read_preferences(&path).unwrap().user_themes.len(), 2);
    }

    #[test]
    fn imported_theme_assets_are_published_separately_from_preferences() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let asset_root = directory.path().join("theme-assets");
        let store = HostPreferences {
            path: directory.path().join(FILE_NAME),
            asset_root: asset_root.clone(),
            preferences: Mutex::new(SavedPreferences::default()),
        };
        let bytes = b"validated image bytes".to_vec();
        let theme = UserTheme {
            id: "background-pack".into(),
            name: "Background Pack".into(),
            background: Some(ThemeBackground {
                image: "background.webp".into(),
                focus_x: 0.5,
                focus_y: 0.5,
                safe_area: "center".into(),
                home_opacity: 1.0,
                task_opacity: 0.28,
                overlay_strength: 0.62,
                pane_opacity: None,
            }),
            background_asset_bytes: Some(bytes.clone()),
            ..UserTheme::default()
        };
        let saved = store.import_user_theme(theme).expect("import image theme");
        let image = store
            .theme_asset_path(&saved, "background.webp")
            .expect("safe asset path");
        assert_eq!(fs::read(image).expect("read saved image"), bytes);
        let persisted = read_preferences(&store.path).expect("read persisted theme");
        assert_eq!(
            persisted.user_themes[0].background.as_ref().unwrap().image,
            "background.webp"
        );
        assert!(persisted.user_themes[0].background_asset_bytes.is_none());
    }

    #[test]
    fn saving_a_theme_replaces_and_clears_scene_assets_transactionally() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let store = HostPreferences {
            path: directory.path().join(FILE_NAME),
            asset_root: directory.path().join("theme-assets"),
            preferences: Mutex::new(SavedPreferences::default()),
        };
        let mut theme = UserTheme {
            id: "user-wallpaper".into(),
            name: "Wallpaper".into(),
            ..UserTheme::default()
        };
        theme.background = Some(ThemeBackground {
            image: "background.png".into(),
            ..ThemeBackground::default()
        });
        theme.background_asset_bytes = Some(b"first image".to_vec());
        let saved = store.save_user_theme(theme).expect("save first image");
        let image_path = store
            .theme_asset_path(&saved, "background.png")
            .expect("first image path");
        assert_eq!(fs::read(&image_path).unwrap(), b"first image");

        let mut edited = saved.clone();
        edited.background_asset_bytes = Some(b"replacement image".to_vec());
        let saved = store.save_user_theme(edited).expect("replace image");
        let image_path = store
            .theme_asset_path(&saved, "background.png")
            .expect("replacement image path");
        assert_eq!(fs::read(&image_path).unwrap(), b"replacement image");

        let mut cleared = saved;
        cleared.clear_background = true;
        let saved = store.save_user_theme(cleared).expect("clear image");
        assert!(saved.background.is_none());
        assert!(store.theme_asset_path(&saved, "background.png").is_none());
    }

    #[test]
    fn custom_theme_rejects_paths_unknown_tokens_and_css_values() {
        let directory = tempfile::tempdir().expect("temporary directory");
        let store = HostPreferences {
            path: directory.path().join(FILE_NAME),
            asset_root: directory.path().join("theme-assets"),
            preferences: Mutex::new(SavedPreferences::default()),
        };
        let base = UserTheme {
            id: "user-safe".into(),
            name: "Safe".into(),
            base_style: "graphite".into(),
            tokens: ThemeTokens::default(),
            density: "comfortable".into(),
            corners: "soft".into(),
            ..UserTheme::default()
        };
        let mut path_theme = base.clone();
        path_theme.id = "user-../../settings".into();
        assert!(store.save_user_theme(path_theme).is_err());
        let mut unknown_token = base.clone();
        unknown_token
            .tokens
            .dark
            .insert("custom".into(), "#000000".into());
        assert!(store.save_user_theme(unknown_token).is_err());
        let mut css_value = base;
        css_value
            .tokens
            .dark
            .insert("bg".into(), "url(https://example.test)".into());
        assert!(store.save_user_theme(css_value).is_err());
        assert!(store.user_themes().is_empty());
    }
}
