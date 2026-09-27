import { useSyncExternalStore } from "react";

export const TAURI_STATUS_BAR_ITEM_IDS = ["workspace", "model", "session", "observed_tokens", "turn_tokens", "cache_hit", "bridge"] as const;
export type TauriStatusBarItemId = typeof TAURI_STATUS_BAR_ITEM_IDS[number];
export type TauriStatusBarStyle = "icon" | "text";
export interface TauriStatusBarPreferences {
  style: TauriStatusBarStyle;
  items: TauriStatusBarItemId[];
}

const STORAGE_KEY = "reasonix.tauri.status-bar.v1";
const DEFAULT_PREFERENCES: TauriStatusBarPreferences = { style: "text", items: [...TAURI_STATUS_BAR_ITEM_IDS] };
const validItems = new Set<string>(TAURI_STATUS_BAR_ITEM_IDS);
const subscribers = new Set<() => void>();
let current: TauriStatusBarPreferences | null = null;

export function normalizeTauriStatusBarPreferences(value: unknown): TauriStatusBarPreferences {
  if (!value || typeof value !== "object") return { ...DEFAULT_PREFERENCES, items: [...DEFAULT_PREFERENCES.items] };
  const record = value as Record<string, unknown>;
  const items = Array.isArray(record.items)
    ? [...new Set(record.items.filter((item): item is TauriStatusBarItemId => typeof item === "string" && validItems.has(item)))]
    : [...DEFAULT_PREFERENCES.items];
  return { style: record.style === "icon" ? "icon" : "text", items };
}

export function getTauriStatusBarPreferences(): TauriStatusBarPreferences {
  if (current) return current;
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    current = normalizeTauriStatusBarPreferences(stored ? JSON.parse(stored) : null);
  } catch {
    current = normalizeTauriStatusBarPreferences(null);
  }
  return current;
}

export function setTauriStatusBarPreferences(next: TauriStatusBarPreferences): void {
  current = normalizeTauriStatusBarPreferences(next);
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(current)); } catch { /* This WebView still uses the live preference. */ }
  subscribers.forEach(subscriber => subscriber());
}

export function useTauriStatusBarPreferences(): TauriStatusBarPreferences {
  return useSyncExternalStore(
    subscriber => { subscribers.add(subscriber); return () => { subscribers.delete(subscriber); }; },
    getTauriStatusBarPreferences,
    () => DEFAULT_PREFERENCES,
  );
}
