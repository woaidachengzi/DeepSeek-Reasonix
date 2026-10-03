import { useSyncExternalStore } from "react";
import { comboFromKeyboardEvent, type ShortcutCombo, type ShortcutPlatform } from "../lib/keyboardShortcuts";
import macosMenuShortcuts from "./macosMenuShortcuts.json";

export const TAURI_SHORTCUT_ACTIONS = [
  "new_session",
  "close_panel",
  "settings",
  "command_palette",
  "show_shortcuts",
  "diagnostics",
  "toggle_sidebar",
  "workspace_files",
  "refresh_session",
  "send_message",
  "composer_newline",
  "add_selection",
  "open_appearance",
  "open_model_preferences",
  "open_model_services",
  "open_usage_stats",
  "open_general",
  "open_bots",
  "open_mcp",
  "open_remote",
  "open_skills",
  "open_plugins",
  "open_subagents",
  "open_hooks",
  "open_memory",
  "open_permissions",
  "open_sandbox",
  "open_network",
  "open_storage",
  "open_shortcuts",
  "open_updates",
  "open_about",
  "goto_session_1",
  "goto_session_2",
  "goto_session_3",
  "goto_session_4",
  "goto_session_5",
  "goto_session_6",
  "goto_session_7",
  "goto_session_8",
  "goto_session_9",
  "text_size_increase",
  "text_size_decrease",
  "text_size_reset",
] as const;
export type TauriShortcutAction = typeof TAURI_SHORTCUT_ACTIONS[number];
export const TAURI_SHORTCUT_TABS = {
  open_general: "general",
  open_bots: "bots",
  open_mcp: "mcp",
  open_remote: "remote",
  open_skills: "skills",
  open_plugins: "plugins",
  open_subagents: "subagents",
  open_hooks: "hooks",
  open_memory: "memory",
  open_permissions: "permissions",
  open_sandbox: "sandbox",
  open_network: "network",
  open_storage: "data",
  open_shortcuts: "shortcuts",
  open_updates: "updates",
  open_about: "about",
} as const satisfies Partial<Record<TauriShortcutAction, string>>;
type Overrides = Partial<Record<TauriShortcutAction, ShortcutCombo>>;

const STORAGE_KEY = "reasonix.tauri.shortcuts.v1";
const EMPTY_OVERRIDES: Overrides = {};
const actionSet = new Set<string>(TAURI_SHORTCUT_ACTIONS);
const nativeCombos = [...macosMenuShortcuts.required, ...macosMenuShortcuts.optional];
const subscribers = new Set<() => void>();
let current: Overrides | null = null;

function normalizeCombo(value: unknown): ShortcutCombo | null {
  if (!value || typeof value !== "object") return null;
  const record = value as Record<string, unknown>;
  if (typeof record.key !== "string" || !record.key || ["Meta", "Control", "Alt", "Shift"].includes(record.key)) return null;
  return {
    key: record.key === " " ? "Space" : record.key.length === 1 ? record.key.toLowerCase() : record.key,
    ctrl: record.ctrl === true,
    meta: record.meta === true,
    alt: record.alt === true,
    shift: record.shift === true,
  };
}

function validForAction(action: TauriShortcutAction, combo: ShortcutCombo): boolean {
  if (action === "composer_newline") return combo.key === "Enter" && combo.shift === true;
  // All Preview shortcuts require a primary modifier, so typing and native
  // textarea editing remain outside the application shortcut layer.
  if (!combo.meta && !combo.ctrl) return false;
  return action !== "send_message" || combo.key === "Enter";
}

function readOverrides(): Overrides {
  if (current) return current;
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    const parsed: unknown = raw ? JSON.parse(raw) : {};
    const next: Overrides = {};
    if (parsed && typeof parsed === "object") {
      for (const action of TAURI_SHORTCUT_ACTIONS) {
        const combo = normalizeCombo((parsed as Record<string, unknown>)[action]);
        if (combo && validForAction(action, combo)) next[action] = combo;
      }
    }
    current = next;
  } catch { current = {}; }
  return current;
}

function notify(): void { subscribers.forEach(subscriber => subscriber()); }

if (typeof window !== "undefined") {
  window.addEventListener("storage", event => {
    if (event.key !== STORAGE_KEY) return;
    current = null;
    notify();
  });
}

export function defaultTauriShortcut(action: TauriShortcutAction, platform: ShortcutPlatform): ShortcutCombo {
  const defaults: Record<TauriShortcutAction, string> = {
    new_session: "n",
    close_panel: "w",
    settings: ",",
    command_palette: "k",
    show_shortcuts: "?",
    diagnostics: ".",
    toggle_sidebar: "b",
    workspace_files: "f",
    refresh_session: "r",
    send_message: "Enter",
    composer_newline: "Enter",
    add_selection: "l",
    open_appearance: "a",
    open_model_preferences: "m",
    open_model_services: "p",
    open_usage_stats: "u",
    open_general: "o",
    open_bots: "b",
    open_mcp: "c",
    open_remote: "r",
    open_skills: "s",
    open_plugins: "g",
    open_subagents: "d",
    open_hooks: "h",
    open_memory: "y",
    open_permissions: "x",
    open_sandbox: "z",
    open_network: "w",
    open_storage: "t",
    open_shortcuts: "k",
    open_updates: "v",
    open_about: "i",
    goto_session_1: "1",
    goto_session_2: "2",
    goto_session_3: "3",
    goto_session_4: "4",
    goto_session_5: "5",
    goto_session_6: "6",
    goto_session_7: "7",
    goto_session_8: "8",
    goto_session_9: "9",
    text_size_increase: "=",
    text_size_decrease: "-",
    text_size_reset: "0",
  };
  const key = defaults[action];
  const modifier = platform === "darwin" ? { meta: true } : { ctrl: true };
  if (action === "composer_newline") return { key, shift: true };
  return action.startsWith("open_") || action === "workspace_files"
    ? { key, ...modifier, shift: true }
    : action === "show_shortcuts"
      ? { key, ...modifier, shift: true }
    : { key, ...modifier };
}

export function getTauriShortcut(action: TauriShortcutAction, platform: ShortcutPlatform): ShortcutCombo {
  const override = readOverrides()[action];
  // Older Preview preferences could capture a native menu chord. Keep the
  // stored preference available for reset, but never dispatch that binding.
  return override && !nativeTauriShortcutConflict(override, platform)
    ? override : defaultTauriShortcut(action, platform);
}

function sameCombo(left: ShortcutCombo, right: ShortcutCombo): boolean {
  return left.key === right.key && Boolean(left.ctrl) === Boolean(right.ctrl) && Boolean(left.meta) === Boolean(right.meta)
    && Boolean(left.alt) === Boolean(right.alt) && Boolean(left.shift) === Boolean(right.shift);
}

export function nativeTauriShortcutConflict(combo: ShortcutCombo, platform: ShortcutPlatform): boolean {
  const normalized = normalizeCombo(combo);
  return platform === "darwin" && Boolean(normalized && nativeCombos.some(native => sameCombo(native, normalized)));
}

export function tauriShortcutConflict(action: TauriShortcutAction, combo: ShortcutCombo, platform: ShortcutPlatform): TauriShortcutAction | null {
  return TAURI_SHORTCUT_ACTIONS.find(other => other !== action && sameCombo(getTauriShortcut(other, platform), combo)) ?? null;
}

export function isValidTauriShortcut(action: TauriShortcutAction, combo: ShortcutCombo): boolean {
  const normalized = normalizeCombo(combo);
  return Boolean(normalized && validForAction(action, normalized));
}

export function setTauriShortcut(action: TauriShortcutAction, combo: ShortcutCombo | null, platform: ShortcutPlatform): boolean {
  if (!actionSet.has(action)) return false;
  const normalized = combo ? normalizeCombo(combo) : null;
  if (combo && (!normalized || !validForAction(action, normalized))) return false;
  const target = normalized ?? defaultTauriShortcut(action, platform);
  if (nativeTauriShortcutConflict(target, platform) || tauriShortcutConflict(action, target, platform)) return false;
  const next = { ...readOverrides() };
  if (normalized && !sameCombo(normalized, defaultTauriShortcut(action, platform))) next[action] = normalized;
  else delete next[action];
  current = next;
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(next)); } catch { /* The shortcut still works in this window. */ }
  notify();
  return true;
}

export function resetTauriShortcuts(): void {
  current = {};
  try { localStorage.removeItem(STORAGE_KEY); } catch { /* The live shortcuts are still reset. */ }
  notify();
}

export function matchesTauriShortcut(event: KeyboardEvent, action: TauriShortcutAction, platform: ShortcutPlatform): boolean {
  if (isTauriCompositionKey(event)) return false;
  const combo = comboFromKeyboardEvent(event);
  return Boolean(combo && !nativeTauriShortcutConflict(combo, platform) && sameCombo(combo, getTauriShortcut(action, platform)));
}

export function isTauriCompositionKey(event: Pick<KeyboardEvent, "isComposing" | "keyCode">): boolean {
  return event.isComposing || event.keyCode === 229;
}

export function useTauriShortcuts(): Overrides {
  return useSyncExternalStore(
    subscriber => { subscribers.add(subscriber); return () => { subscribers.delete(subscriber); }; },
    readOverrides,
    () => EMPTY_OVERRIDES,
  );
}
