import type { ITheme } from "@xterm/xterm";

import { getResolvedTheme, type ResolvedTheme } from "./theme";

export type TerminalThemePreference = "auto" | "dark" | "light";

export type TerminalPaletteKey =
  | "background" | "foreground" | "cursor" | "selectionBackground"
  | "black" | "red" | "green" | "yellow" | "blue" | "magenta" | "cyan" | "white"
  | "brightBlack" | "brightRed" | "brightGreen" | "brightYellow" | "brightBlue" | "brightMagenta" | "brightCyan" | "brightWhite";
export type TerminalPalette = Record<TerminalPaletteKey, string>;

const TERMINAL_PALETTE_KEY = "reasonix-terminal-palette-v1";

const AUTO_THEME_MEDIA_QUERY = "(prefers-color-scheme: light)";
let currentTerminalTheme: TerminalThemePreference = "auto";
const listeners = new Set<() => void>();

const darkANSI: ITheme = {
  black: "#24272b",
  red: "#ff6b6b",
  green: "#89d185",
  yellow: "#f2cc60",
  blue: "#6ca4f8",
  magenta: "#c58af9",
  cyan: "#56d4dd",
  white: "#d8d8d8",
  brightBlack: "#70777f",
  brightRed: "#ff8787",
  brightGreen: "#a6e3a1",
  brightYellow: "#ffe08a",
  brightBlue: "#8ab4f8",
  brightMagenta: "#d7a7ff",
  brightCyan: "#79e2e8",
  brightWhite: "#ffffff",
};

const lightANSI: ITheme = {
  black: "#25272a",
  red: "#b4232d",
  green: "#3f6f16",
  yellow: "#8a5700",
  blue: "#1d5fbf",
  magenta: "#7c3aaa",
  cyan: "#08746f",
  white: "#555b61",
  brightBlack: "#6b7077",
  brightRed: "#c3343f",
  brightGreen: "#4b7a1f",
  brightYellow: "#986000",
  brightBlue: "#2c6cc5",
  brightMagenta: "#9751c6",
  brightCyan: "#0e7d77",
  brightWhite: "#34383d",
};

export const DEFAULT_TERMINAL_PALETTE: TerminalPalette = {
  background: "#111315", foreground: "#e8e5df", cursor: "#e6a15c", selectionBackground: "#4a6d8c",
  black: darkANSI.black!, red: darkANSI.red!, green: darkANSI.green!, yellow: darkANSI.yellow!, blue: darkANSI.blue!, magenta: darkANSI.magenta!, cyan: darkANSI.cyan!, white: darkANSI.white!,
  brightBlack: darkANSI.brightBlack!, brightRed: darkANSI.brightRed!, brightGreen: darkANSI.brightGreen!, brightYellow: darkANSI.brightYellow!, brightBlue: darkANSI.brightBlue!, brightMagenta: darkANSI.brightMagenta!, brightCyan: darkANSI.brightCyan!, brightWhite: darkANSI.brightWhite!,
};

let customPalette: TerminalPalette = { ...DEFAULT_TERMINAL_PALETTE };
let customPaletteEnabled = false;

function validHex(value: unknown): value is string {
  return typeof value === "string" && /^#[0-9a-fA-F]{6}$/.test(value);
}

function readCustomPalette(): void {
  try {
    const stored = localStorage.getItem(TERMINAL_PALETTE_KEY);
    if (!stored) return;
    const parsed = JSON.parse(stored) as { enabled?: unknown; colors?: unknown };
    if (!parsed.colors || typeof parsed.colors !== "object") return;
    const next = { ...DEFAULT_TERMINAL_PALETTE };
    for (const key of Object.keys(next) as TerminalPaletteKey[]) {
      const value = (parsed.colors as Record<string, unknown>)[key];
      if (validHex(value)) next[key] = value;
    }
    customPalette = next;
    customPaletteEnabled = parsed.enabled === true;
  } catch { /* Use defaults when WebView storage is unavailable or malformed. */ }
}

if (typeof localStorage !== "undefined") readCustomPalette();

export function getCustomTerminalPalette(): { enabled: boolean; colors: TerminalPalette } {
  return { enabled: customPaletteEnabled, colors: { ...customPalette } };
}

export function setCustomTerminalPalette(colors: TerminalPalette, enabled = customPaletteEnabled): void {
  const next = { ...DEFAULT_TERMINAL_PALETTE };
  for (const key of Object.keys(next) as TerminalPaletteKey[]) {
    if (validHex(colors[key])) next[key] = colors[key];
  }
  customPalette = next;
  customPaletteEnabled = enabled;
  try { localStorage.setItem(TERMINAL_PALETTE_KEY, JSON.stringify({ enabled, colors: next })); } catch { /* Keep the live palette even when persistence is unavailable. */ }
  for (const listener of listeners) listener();
}

// Wails dispatches bound Go calls on separate goroutines. Keep terminal-theme
// writes in click order so an older save can never finish after newer intent.
export function createTerminalThemeSaveQueue(
  persist: (theme: TerminalThemePreference) => Promise<void>,
): (theme: TerminalThemePreference) => Promise<void> {
  let tail = Promise.resolve();
  return (theme) => {
    const save = tail.then(() => persist(theme));
    tail = save.catch(() => undefined);
    return save;
  };
}

export function normalizeTerminalThemePreference(value: unknown): TerminalThemePreference {
  return value === "dark" || value === "light" ? value : "auto";
}

export function getTerminalThemePreference(): TerminalThemePreference {
  return currentTerminalTheme;
}

export function getResolvedTerminalTheme(
  preference: TerminalThemePreference = getTerminalThemePreference(),
): ResolvedTheme {
  return preference === "auto" ? getResolvedTheme() : preference;
}

export function applyTerminalThemePreference(value: unknown): TerminalThemePreference {
  const next = normalizeTerminalThemePreference(value);
  currentTerminalTheme = next;
  if (typeof document !== "undefined") {
    if (next === "auto") document.documentElement.removeAttribute("data-terminal-theme");
    else document.documentElement.setAttribute("data-terminal-theme", next);
  }
  for (const listener of listeners) listener();
  return next;
}

export function onTerminalThemePreferenceChange(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function cssToken(style: CSSStyleDeclaration, name: string, fallback: string): string {
  return style.getPropertyValue(name).trim() || fallback;
}

function contrastingText(hex: string): string {
  const channels = [1, 3, 5].map((start) => Number.parseInt(hex.slice(start, start + 2), 16) / 255);
  const linear = channels.map((channel) => channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4);
  const luminance = 0.2126 * linear[0] + 0.7152 * linear[1] + 0.0722 * linear[2];
  return luminance > 0.179 ? "#000000" : "#ffffff";
}

export function terminalThemeForElement(element: Element): ITheme {
  const resolved = getResolvedTerminalTheme();
  const palette = customPaletteEnabled ? customPalette : resolved === "light" ? lightANSI : darkANSI;
  const style = getComputedStyle(element);
  return {
    ...palette,
    background: customPaletteEnabled ? customPalette.background : cssToken(style, "--terminal-bg", resolved === "light" ? "#f7f8fa" : "#111315"),
    foreground: customPaletteEnabled ? customPalette.foreground : cssToken(style, "--terminal-fg", resolved === "light" ? "#25272a" : "#e8e5df"),
    cursor: customPaletteEnabled ? customPalette.cursor : cssToken(style, "--terminal-cursor", resolved === "light" ? "#9a4f00" : "#e6a15c"),
    cursorAccent: customPaletteEnabled ? customPalette.background : cssToken(style, "--terminal-bg", resolved === "light" ? "#f7f8fa" : "#111315"),
    selectionBackground: customPaletteEnabled ? customPalette.selectionBackground : cssToken(style, "--terminal-selection", resolved === "light" ? "#6ea8fe" : "#4a6d8c"),
    // Explicit themes keep at least 4.5:1 contrast over their selection
    // backgrounds. Auto mode inherits from the CSS layer.
    selectionForeground: customPaletteEnabled ? contrastingText(customPalette.selectionBackground) : cssToken(style, "--terminal-selection-fg", resolved === "light" ? "#0b0f14" : "#ffffff"),
  };
}

// Watch both explicit app-theme mutations and OS theme changes. Theme packs can
// replace root CSS variables without changing the terminal preference itself,
// so the root style attribute is observed as well.
export function observeTerminalTheme(element: Element, listener: () => void): () => void {
  const unsubscribe = onTerminalThemePreferenceChange(listener);
  const root = typeof document !== "undefined" ? document.documentElement : null;
  const observer = root && typeof MutationObserver !== "undefined"
    ? new MutationObserver(listener)
    : null;
  observer?.observe(root!, {
    attributes: true,
    attributeFilter: ["data-theme", "data-theme-style", "data-terminal-theme", "style"],
  });

  const media = typeof window !== "undefined" && window.matchMedia
    ? window.matchMedia(AUTO_THEME_MEDIA_QUERY)
    : null;
  media?.addEventListener?.("change", listener);
  void element;

  return () => {
    unsubscribe();
    observer?.disconnect();
    media?.removeEventListener?.("change", listener);
  };
}
