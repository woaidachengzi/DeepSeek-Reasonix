import { useSyncExternalStore } from "react";

export const TAURI_STATUS_BAR_ITEM_IDS = ["workspace", "model", "balance", "session", "observed_tokens", "turn_tokens", "turn_tps", "turn_output_tokens", "turn_cache_tokens", "turn_cost", "session_turns", "session_cost", "context", "compact", "cache", "cache_avg", "bridge"] as const;
export type TauriStatusBarItemId = typeof TAURI_STATUS_BAR_ITEM_IDS[number];
export type TauriStatusBarStyle = "icon" | "text";
export interface TauriStatusBarPreferences {
  style: TauriStatusBarStyle;
  items: TauriStatusBarItemId[];
}

const STORAGE_KEY = "reasonix.tauri.status-bar.v1";
const DEFAULT_PREFERENCES: TauriStatusBarPreferences = { style: "text", items: [...TAURI_STATUS_BAR_ITEM_IDS] };
const validItems = new Set<string>(TAURI_STATUS_BAR_ITEM_IDS);
const PREVIOUS_DEFAULT_ITEMS = ["workspace", "model", "balance", "session", "observed_tokens", "turn_tokens", "turn_tps", "turn_output_tokens", "turn_cache_tokens", "turn_cost", "session_turns", "session_cost", "context", "compact", "cache_hit", "bridge"];
const OLDER_DEFAULT_ITEMS = PREVIOUS_DEFAULT_ITEMS.filter(item => item !== "turn_tps");
const subscribers = new Set<() => void>();
let current: TauriStatusBarPreferences | null = null;

export function normalizeTauriStatusBarPreferences(value: unknown): TauriStatusBarPreferences {
  if (!value || typeof value !== "object") return { ...DEFAULT_PREFERENCES, items: [...DEFAULT_PREFERENCES.items] };
  const record = value as Record<string, unknown>;
  const rawItems = Array.isArray(record.items) ? record.items : null;
  const matchesLegacyDefaults = rawItems !== null && [PREVIOUS_DEFAULT_ITEMS, OLDER_DEFAULT_ITEMS].some(defaults =>
    rawItems.length === defaults.length && rawItems.every((item, index) => item === defaults[index]),
  );
  const items = matchesLegacyDefaults
    ? [...DEFAULT_PREFERENCES.items]
    : rawItems
    ? [...new Set(rawItems.flatMap(item => {
      if (item === "cache_hit") return ["cache_avg" as const];
      return typeof item === "string" && validItems.has(item) ? [item as TauriStatusBarItemId] : [];
    }))]
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
