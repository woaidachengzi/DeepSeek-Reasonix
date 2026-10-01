import type { ThemePackView } from "../lib/themePack";
import { registerTrustedThemeBackgroundURLs } from "../lib/themePack";
import { THEME_STYLES, type ThemeStyle } from "../lib/theme";

// Vite expands glob calls at compile time; there is no runtime glob function.
// Its compile-time MODE distinguishes bundled/dev modules from direct Node
// component imports, which intentionally have no bundled asset catalog.
const manifestFiles = (import.meta.env?.MODE ? import.meta.glob("../../../themes/official/*/theme.json", {
  eager: true,
  import: "default",
}) : {}) as Record<string, Record<string, unknown>>;
const assetFiles = (import.meta.env?.MODE ? import.meta.glob("../../../themes/official/*/*.webp", {
  eager: true,
  query: "?url",
  import: "default",
}) : {}) as Record<string, string>;

const styleNames: Record<ThemeStyle, string> = {
  graphite: "settings.style.graphite.zh",
  aurora: "settings.style.aurora.zh",
  slate: "settings.style.slate.zh",
  carbon: "settings.style.carbon.zh",
  nocturne: "settings.style.nocturne.zh",
  amber: "settings.style.amber.zh",
};

function buildOfficialThemes(): ThemePackView[] {
  return Object.entries(manifestFiles).flatMap(([manifestPath, manifest]) => {
    const directory = manifestPath.slice(0, manifestPath.lastIndexOf("/"));
    const id = typeof manifest.id === "string" ? manifest.id : "";
    if (!id.startsWith("official-")) return [];
    const backgroundUrl = assetFiles[`${directory}/background.webp`];
    const previewUrl = assetFiles[`${directory}/preview.webp`];
    if (!backgroundUrl || !previewUrl) return [];
    return [{
      ...manifest,
      id,
      name: typeof manifest.name === "string" ? manifest.name : id,
      nameKey: `settings.themes.official.${id}.name`,
      descriptionKey: `settings.themes.official.${id}.description`,
      baseStyle: typeof manifest.baseStyle === "string" ? manifest.baseStyle : "graphite",
      builtin: true,
      kind: "official" as const,
      active: false,
      hasBackground: true,
      backgroundUrl,
      previewUrl,
      tokens: manifest.tokens as ThemePackView["tokens"],
      recipes: manifest.recipes as ThemePackView["recipes"],
      background: manifest.background as ThemePackView["background"],
    } satisfies ThemePackView];
  });
}

export const TAURI_OFFICIAL_THEME_PACKS = buildOfficialThemes();
registerTrustedThemeBackgroundURLs(TAURI_OFFICIAL_THEME_PACKS.map((pack) => pack.backgroundUrl || ""));

export function tauriThemeCatalog(baseStyle: ThemeStyle, activeThemeId: string): ThemePackView[] {
  const basePacks: ThemePackView[] = THEME_STYLES.map((style) => ({
    id: style,
    name: style,
    nameKey: styleNames[style],
    author: "Reasonix",
    baseStyle: style,
    builtin: true,
    kind: "base",
    active: !activeThemeId && style === baseStyle,
    hasBackground: false,
    tokens: {},
    recipes: { density: "comfortable", corners: "soft" },
  }));
  const official = TAURI_OFFICIAL_THEME_PACKS.map((pack) => ({
    ...pack,
    active: pack.id === activeThemeId,
  }));
  return [...official, ...basePacks];
}

export function tauriThemePackById(id: string): ThemePackView | null {
  return TAURI_OFFICIAL_THEME_PACKS.find((pack) => pack.id === id) ?? null;
}
