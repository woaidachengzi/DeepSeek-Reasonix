import { useSyncExternalStore } from "react";

export type TauriDesktopLayout = "workbench" | "creation";

const STORAGE_KEY = "reasonix.tauri.desktop-layout.v1";
const subscribers = new Set<() => void>();
let current: TauriDesktopLayout | null = null;

export function getTauriDesktopLayout(): TauriDesktopLayout {
  if (current) return current;
  try { current = localStorage.getItem(STORAGE_KEY) === "creation" ? "creation" : "workbench"; }
  catch { current = "workbench"; }
  return current;
}

export function setTauriDesktopLayout(next: TauriDesktopLayout): void {
  const normalized = next === "creation" ? "creation" : "workbench";
  if (current === normalized) return;
  current = normalized;
  try { localStorage.setItem(STORAGE_KEY, normalized); }
  catch { /* The preference still applies for this window. */ }
  subscribers.forEach(subscriber => subscriber());
}

export function useTauriDesktopLayout(): TauriDesktopLayout {
  return useSyncExternalStore(
    subscriber => { subscribers.add(subscriber); return () => { subscribers.delete(subscriber); }; },
    getTauriDesktopLayout,
    () => "workbench",
  );
}
