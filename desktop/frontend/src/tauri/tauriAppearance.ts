import { applyTheme, isThemeStyle, type Theme, type ThemeStyle } from "../lib/theme";

const MODE_KEY = "tauri-theme";
const STYLE_KEY = "tauri-theme-style";

export type TauriAppearance = { mode: Theme; style: ThemeStyle };

export function readTauriAppearance(): TauriAppearance {
  try {
    const storedMode = localStorage.getItem(MODE_KEY);
    const storedStyle = localStorage.getItem(STYLE_KEY);
    return {
      mode: storedMode === "light" || storedMode === "dark" ? storedMode : "auto",
      style: isThemeStyle(storedStyle) ? storedStyle : "graphite",
    };
  } catch {
    return { mode: "auto", style: "graphite" };
  }
}

export function applyTauriAppearance(appearance: TauriAppearance): void {
  applyTheme(appearance.mode, appearance.style);
  try {
    localStorage.setItem(MODE_KEY, appearance.mode);
    localStorage.setItem(STYLE_KEY, appearance.style);
  } catch {
    // Appearance still applies to this window when storage is unavailable.
  }
}

export function initTauriAppearance(): void {
  const appearance = readTauriAppearance();
  applyTheme(appearance.mode, appearance.style);
}
