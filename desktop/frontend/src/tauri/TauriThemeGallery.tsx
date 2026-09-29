import { useMemo, useState, type FormEvent } from "react";
import { ArrowLeft, Check, Download, Trash2, Upload } from "lucide-react";
import { ThemePreviewSurface } from "../components/ThemePreviewSurface";
import { useT } from "../lib/i18n";
import { isSafeHex, themePackKind, themeTokenKeys, type ThemePackTokens, type ThemePackView } from "../lib/themePack";
import type { Theme, ThemeStyle } from "../lib/theme";
import { tauriThemeCatalog } from "./tauriThemeCatalog";

const TOKEN_GROUPS: { label: string; keys: string[] }[] = [
  { label: "settings.themeTokens.surfaces", keys: ["bg", "bgSoft", "bgElev", "panel", "sidebar", "chat", "workspace", "workspaceFiles"] },
  { label: "settings.themeTokens.borderText", keys: ["border", "borderSoft", "fg", "fgDim", "fgFaint"] },
  { label: "settings.themeTokens.accentStatus", keys: ["accent", "accentFg", "ok", "warn", "err"] },
];

const DEFAULT_THEME_TOKENS = {
  light: { bg: "#fcf8ee", fg: "#2a241b", accent: "#7a5a16" },
  dark: { bg: "#151515", fg: "#f4f4f4", accent: "#ff6a45" },
};

function initialThemeTokens(pack: ThemePackView, mode: "light" | "dark"): Record<string, string> {
  return { ...DEFAULT_THEME_TOKENS[mode], ...pack.tokens[mode] };
}

export function TauriThemeGallery({
  mode,
  baseStyle,
  activeThemeId,
  userThemes,
  error,
  onBack,
  onApply,
  onSaveTheme,
  onDeleteTheme,
  onImportTheme,
  onExportTheme,
}: {
  mode: Theme;
  baseStyle: ThemeStyle;
  activeThemeId: string;
  userThemes: ThemePackView[];
  error: string;
  onBack: () => void;
  onApply: (id: string) => Promise<void>;
  onSaveTheme: (theme: Pick<ThemePackView, "id" | "name" | "baseStyle" | "tokens" | "recipes">) => Promise<ThemePackView | null>;
  onDeleteTheme: (id: string) => Promise<boolean>;
  onImportTheme: () => Promise<ThemePackView | null>;
  onExportTheme: (id: string) => Promise<boolean>;
}) {
  const t = useT();
  const packs = useMemo(() => [...tauriThemeCatalog(baseStyle, activeThemeId), ...userThemes.map(pack => ({ ...pack, active: pack.id === activeThemeId }))], [baseStyle, activeThemeId, userThemes]);
  const [selectedId, setSelectedId] = useState(activeThemeId || baseStyle);
  const [previewMode, setPreviewMode] = useState<"light" | "dark">(mode === "light" ? "light" : "dark");
  const [scene, setScene] = useState<"home" | "task">("home");
  const [saving, setSaving] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editingThemeId, setEditingThemeId] = useState("");
  const [themeName, setThemeName] = useState("");
  const [lightTokens, setLightTokens] = useState<Record<string, string>>({ ...DEFAULT_THEME_TOKENS.light });
  const [darkTokens, setDarkTokens] = useState<Record<string, string>>({ ...DEFAULT_THEME_TOKENS.dark });
  const [tokenMode, setTokenMode] = useState<"light" | "dark">("dark");
  const [density, setDensity] = useState<"compact" | "comfortable">("comfortable");
  const [corners, setCorners] = useState<"square" | "soft" | "round">("soft");
  const [deleteArmed, setDeleteArmed] = useState(false);
  const selected = packs.find((pack) => pack.id === selectedId) ?? packs[0];
  const official = packs.filter((pack) => themePackKind(pack) === "official");
  const base = packs.filter((pack) => themePackKind(pack) === "base");
  const mine = packs.filter((pack) => themePackKind(pack) === "user");
  const isActive = selected.id === (activeThemeId || baseStyle);
  const title = (pack: ThemePackView) => pack.nameKey ? t(pack.nameKey as never) : pack.name;
  const description = (pack: ThemePackView) => pack.descriptionKey ? t(pack.descriptionKey as never) : pack.description || "";
  const apply = async () => {
    setSaving(true);
    try { await onApply(selected.id); } finally { setSaving(false); }
  };

  const startCreate = () => {
    const source = selected;
    setThemeName(`${source.name} Custom`);
    setEditingThemeId("");
    setDensity(source.recipes.density === "compact" ? "compact" : "comfortable");
    setCorners(source.recipes.corners === "square" || source.recipes.corners === "round" ? source.recipes.corners : "soft");
    setLightTokens(initialThemeTokens(source, "light"));
    setDarkTokens(initialThemeTokens(source, "dark"));
    setTokenMode("dark");
    setEditing(true);
    setDeleteArmed(false);
  };

  const startEdit = () => {
    if (themePackKind(selected) !== "user") return;
    setThemeName(selected.name);
    setEditingThemeId(selected.id);
    setDensity(selected.recipes.density === "compact" ? "compact" : "comfortable");
    setCorners(selected.recipes.corners === "square" || selected.recipes.corners === "round" ? selected.recipes.corners : "soft");
    setLightTokens(initialThemeTokens(selected, "light"));
    setDarkTokens(initialThemeTokens(selected, "dark"));
    setTokenMode("dark");
    setEditing(true);
    setDeleteArmed(false);
  };

  const saveTheme = async (event: FormEvent) => {
    event.preventDefault();
    const slug = themeName.trim().toLocaleLowerCase().normalize("NFKD").replace(/\p{Diacritic}/gu, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "custom";
    let id = editingThemeId || `user-${slug}`;
    let suffix = 2;
    while (!editingThemeId && userThemes.some(theme => theme.id === id)) id = `user-${slug}-${suffix++}`;
    const tokens: ThemePackTokens = {
      light: lightTokens,
      dark: darkTokens,
    };
    setSaving(true);
    try {
      const saved = await onSaveTheme({ id, name: themeName.trim(), baseStyle: selected.baseStyle, tokens, recipes: { ...selected.recipes, density, corners } });
      if (saved) {
        setSelectedId(saved.id);
        setEditing(false);
      }
    } finally {
      setSaving(false);
    }
  };

  const updateToken = (key: string, value: string) => {
    const update = (current: Record<string, string>) => {
      const next = { ...current };
      if (value.trim()) next[key] = value.trim();
      else delete next[key];
      return next;
    };
    if (tokenMode === "light") setLightTokens(update);
    else setDarkTokens(update);
  };

  const deleteSelected = async () => {
    if (!deleteArmed) { setDeleteArmed(true); return; }
    setSaving(true);
    try {
      if (await onDeleteTheme(selected.id)) {
        setSelectedId(baseStyle);
        setDeleteArmed(false);
      }
    } finally {
      setSaving(false);
    }
  };

  const importTheme = async () => {
    setSaving(true);
    try {
      const imported = await onImportTheme();
      if (imported) {
        setSelectedId(imported.id);
        setDeleteArmed(false);
        setEditing(false);
      }
    } finally {
      setSaving(false);
    }
  };

  const exportSelected = async () => {
    if (themePackKind(selected) !== "user") return;
    setSaving(true);
    try { await onExportTheme(selected.id); } finally { setSaving(false); }
  };

  const renderCard = (pack: ThemePackView) => {
    const active = pack.id === (activeThemeId || baseStyle);
    const kind = themePackKind(pack);
    return <button
      key={pack.id}
      type="button"
      role="option"
      aria-selected={selectedId === pack.id}
      className={`theme-gallery-card${selectedId === pack.id ? " theme-gallery-card--selected" : ""}${active ? " theme-gallery-card--active" : ""}`}
      onClick={() => setSelectedId(pack.id)}
    >
      <div className="theme-gallery-card__thumb">
        {pack.previewUrl ? <img src={pack.previewUrl} alt="" loading="lazy" /> : <ThemePreviewSurface pack={pack} mode="dark" scene="home" variant="thumbnail" />}
        {active && <span className="theme-gallery-card__check" aria-hidden="true"><Check size={13} /></span>}
      </div>
      <div className="theme-gallery-card__name">{title(pack)}</div>
      {active && <div className="theme-gallery-card__status">{t("settings.themeGallery.current")}</div>}
      {kind === "official" && <div className="theme-gallery-card__license">{t("settings.themeGallery.kindOfficial")}</div>}
      {kind === "user" && <div className="theme-gallery-card__license">{t("settings.themeGallery.kindUser")}</div>}
    </button>;
  };

  return <div className="theme-gallery tauri-theme-gallery">
    <header className="theme-gallery__top">
      <div className="theme-gallery__crumbs">
        <button type="button" className="theme-gallery__back" onClick={onBack}><ArrowLeft size={14} />{t("settings.themeGallery.back")}</button>
        <h2 className="theme-gallery__title">{t("settings.themeGallery.title")}</h2>
        <p className="theme-gallery__sub">{t("settings.themeGallery.subtitle")}</p>
      </div>
      <div className="theme-gallery__actions">
        <button type="button" className="btn btn--secondary" disabled={saving} onClick={() => void importTheme()}><Upload size={14} />{t("settings.themeLibrary.import")}</button>
        <button type="button" className="btn btn--secondary" disabled={saving} onClick={startCreate}>{t("settings.themeGallery.createTheme")}</button>
      </div>
    </header>
    <div className="theme-gallery__body">
      <div className="theme-gallery__grid" role="listbox" aria-label={t("settings.themeGallery.title")}>
        <section className="theme-gallery__grid-section" role="group" aria-label={t("settings.themeGallery.sectionFlagship")}>
          <div className="theme-gallery__section-head"><h3>{t("settings.themeGallery.sectionFlagship")}</h3><span>{official.length}</span></div>
          {official.map(renderCard)}
        </section>
        <section className="theme-gallery__grid-section" role="group" aria-label={t("settings.themeGallery.tabBase")}>
          <div className="theme-gallery__section-head"><h3>{t("settings.themeGallery.tabBase")}</h3><span>{base.length}</span></div>
          {base.map(renderCard)}
        </section>
        <section className="theme-gallery__grid-section" role="group" aria-label={t("settings.themeGallery.sectionMine")}>
          <div className="theme-gallery__section-head"><h3>{t("settings.themeGallery.sectionMine")}</h3><span>{mine.length}</span></div>
          {mine.map(renderCard)}
        </section>
      </div>
      <aside className="theme-gallery__detail" aria-live="polite">
        <div className="theme-gallery__detail-preview"><ThemePreviewSurface pack={selected} mode={previewMode} scene={scene} /></div>
          <div className="theme-gallery__detail-meta">
          <div className="theme-gallery__detail-head">
            <div className="theme-gallery__detail-title-row"><h3 className="theme-gallery__detail-name">{title(selected)}</h3>
              <span className="theme-gallery__badge">{themePackKind(selected) === "official" ? t("settings.themeGallery.kindOfficial") : themePackKind(selected) === "user" ? t("settings.themeGallery.kindUser") : t("settings.themeGallery.kindBase")}</span>
              {isActive && <span className="theme-gallery__detail-status"><Check size={12} />{t("settings.themeLibrary.active")}</span>}
            </div>
          </div>
          <p className="theme-gallery__detail-desc">{description(selected) || selected.author}</p>
          {editing ? <form className="tauri-theme-editor" onSubmit={event => void saveTheme(event)}>
            <label>{t("settings.themeGallery.themeName")}<input value={themeName} maxLength={64} onChange={event => setThemeName(event.target.value)} required /></label>
            <fieldset className="tauri-theme-editor__color-section"><legend>{t("settings.themeLibrary.fieldTokens")}</legend>
              <div className="tauri-theme-editor__mode" role="group" aria-label={t("settings.themeLibrary.fieldTokens")}>
                {(["light", "dark"] as const).map(value => <button key={value} type="button" className={tokenMode === value ? "is-active" : ""} onClick={() => setTokenMode(value)}>{value === "light" ? t("settings.themeGallery.lightMode") : t("settings.themeGallery.darkMode")}</button>)}
              </div>
              {TOKEN_GROUPS.map(group => <section className="tauri-theme-editor__token-group" key={group.label}>
                <h4>{t(group.label as never)}</h4>
                <div className="tauri-theme-editor__token-grid">
                  {group.keys.filter(key => themeTokenKeys().includes(key)).map(key => {
                    const value = (tokenMode === "light" ? lightTokens : darkTokens)[key] ?? "";
                    const tokenLabel = t(`settings.themeTokens.key.${key}` as never);
                    return <label key={key} className="tauri-theme-editor__token">
                      <span>{tokenLabel}<code>{key}</code></span>
                      <div><input type="color" aria-label={`${tokenLabel} color`} value={isSafeHex(value) ? value.slice(0, 7) : "#888888"} onChange={event => updateToken(key, event.target.value)} />
                        <input type="text" aria-label={`${tokenLabel} hex`} value={value} placeholder="#RRGGBB" maxLength={9} onChange={event => updateToken(key, event.target.value)} /></div>
                    </label>;
                  })}
                </div>
              </section>)}
            </fieldset>
            <label>{t("settings.themeLibrary.fieldRecipes")}<div className="tauri-theme-editor__recipes">
              <select aria-label={t("settings.themeEditor.density.comfortable")} value={density} onChange={event => setDensity(event.target.value as "compact" | "comfortable")}>
                <option value="comfortable">{t("settings.themeEditor.density.comfortable")}</option><option value="compact">{t("settings.themeEditor.density.compact")}</option>
              </select>
              <select aria-label={t("settings.themeEditor.corners.soft")} value={corners} onChange={event => setCorners(event.target.value as "square" | "soft" | "round")}>
                <option value="square">{t("settings.themeEditor.corners.square")}</option><option value="soft">{t("settings.themeEditor.corners.soft")}</option><option value="round">{t("settings.themeEditor.corners.round")}</option>
              </select>
            </div></label>
            <div><button type="submit" className="btn btn--primary" disabled={saving}>{t("settings.themeGallery.saveTheme")}</button><button type="button" className="btn btn--secondary" disabled={saving} onClick={() => setEditing(false)}>{t("settings.themeGallery.back")}</button></div>
          </form> : <>
          <div className="theme-gallery__preview-controls">
            <div className="theme-gallery__preview-control">
              <span className="theme-gallery__preview-label">{t("settings.themeGallery.appearancePreview")}</span>
              <div className="theme-gallery__preview-segmented" role="group">
                {(["light", "dark"] as const).map(value => <button key={value} type="button" className={previewMode === value ? "is-active" : ""} onClick={() => setPreviewMode(value)}>{value === "light" ? t("settings.themeLight") : t("settings.themeDark")}</button>)}
              </div>
            </div>
            <div className="theme-gallery__preview-control">
              <span className="theme-gallery__preview-label">{t("settings.themeGallery.scenePreview")}</span>
              <div className="theme-gallery__preview-segmented" role="group">
                {(["home", "task"] as const).map(value => <button key={value} type="button" className={scene === value ? "is-active" : ""} onClick={() => setScene(value)}>{value === "home" ? t("settings.themeGallery.sceneHome") : t("settings.themeGallery.sceneTask")}</button>)}
              </div>
            </div>
          </div>
          <button type="button" className="btn btn--primary theme-gallery__apply" disabled={saving || isActive} onClick={() => void apply()}>
            {isActive ? t("settings.themeLibrary.active") : t("settings.themeGallery.apply")}
          </button>
          {themePackKind(selected) === "user" && <>
            <button type="button" className="btn btn--secondary" disabled={saving} onClick={startEdit}>{t("settings.themeGallery.editTheme")}</button>
            <button type="button" className="btn btn--secondary" disabled={saving} onClick={() => void exportSelected()}><Download size={14} />{t("settings.themeLibrary.export")}</button>
            <button type="button" className="btn btn--secondary" disabled={saving} onClick={() => void deleteSelected()}><Trash2 size={14} />{deleteArmed ? t("settings.themeGallery.deleteThemeConfirm") : t("settings.themeGallery.deleteTheme")}</button>
          </>}
          </>}
          {error && <p className="tauri-settings-zoom__error" role="alert">{error}</p>}
        </div>
      </aside>
    </div>
  </div>;
}
