import { useSyncExternalStore } from "react";
import { comboFromKeyboardEvent, type ShortcutCombo, type ShortcutPlatform } from "../lib/keyboardShortcuts";

export const TAURI_SHORTCUT_ACTIONS = ["new_session", "settings", "diagnostics", "workspace_files", "refresh_session", "send_message"] as const;
export type TauriShortcutAction = typeof TAURI_SHORTCUT_ACTIONS[number];
type Overrides = Partial<Record<TauriShortcutAction, ShortcutCombo>>;

const STORAGE_KEY = "reasonix.tauri.shortcuts.v1";
const EMPTY_OVERRIDES: Overrides = {};
const actionSet = new Set<string>(TAURI_SHORTCUT_ACTIONS);
const subscribers = new Set<() => void>();
let current: Overrides | null = null;

function normalizeCombo(value: unknown): ShortcutCombo | null {
  if (!value || typeof value !== "object") return null;
  const record = value as Record<string, unknown>;
  if (typeof record.key !== "string" || !record.key || ["Meta", "Control", "Alt", "Shift"].includes(record.key)) return null;
  return {
    key: record.key.length === 1 ? record.key.toLowerCase() : record.key,
    ctrl: record.ctrl === true,
    meta: record.meta === true,
    alt: record.alt === true,
    shift: record.shift === true,
  };
}

function validForAction(action: TauriShortcutAction, combo: ShortcutCombo): boolean {
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
  const key = { new_session: "n", settings: ",", diagnostics: ".", workspace_files: "b", refresh_session: "r", send_message: "Enter" }[action];
  return platform === "darwin" ? { key, meta: true } : { key, ctrl: true };
}

export function getTauriShortcut(action: TauriShortcutAction, platform: ShortcutPlatform): ShortcutCombo {
  return readOverrides()[action] ?? defaultTauriShortcut(action, platform);
}

function sameCombo(left: ShortcutCombo, right: ShortcutCombo): boolean {
  return left.key === right.key && Boolean(left.ctrl) === Boolean(right.ctrl) && Boolean(left.meta) === Boolean(right.meta)
    && Boolean(left.alt) === Boolean(right.alt) && Boolean(left.shift) === Boolean(right.shift);
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
  if (combo && (!normalized || !validForAction(action, normalized) || tauriShortcutConflict(action, normalized, platform))) return false;
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
  const combo = comboFromKeyboardEvent(event);
  return Boolean(combo && sameCombo(combo, getTauriShortcut(action, platform)));
}

export function useTauriShortcuts(): Overrides {
  return useSyncExternalStore(
    subscriber => { subscribers.add(subscriber); return () => { subscribers.delete(subscriber); }; },
    readOverrides,
    () => EMPTY_OVERRIDES,
  );
}
