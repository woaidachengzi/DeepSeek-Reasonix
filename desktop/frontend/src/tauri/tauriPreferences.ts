const NOTIFICATIONS_KEY = "tauri-desktop-notifications";
const PROGRESS_MODE_KEY = "tauri-progress-mode";
const SIDEBAR_VISIBLE_KEY = "tauri-sidebar-visible";
export const TAURI_PROGRESS_MODE_CHANGED = "tauri-progress-mode-changed";
export type TauriProgressMode = "standard" | "deep";
let sessionValue: boolean | null = null;
let progressSessionValue: TauriProgressMode | null = null;

export function getTauriProgressMode(): TauriProgressMode {
  if (progressSessionValue !== null) return progressSessionValue;
  try {
    return localStorage.getItem(PROGRESS_MODE_KEY) === "deep" ? "deep" : "standard";
  } catch {
    return "standard";
  }
}

export function setTauriProgressMode(mode: TauriProgressMode): void {
  progressSessionValue = mode;
  try {
    localStorage.setItem(PROGRESS_MODE_KEY, mode);
  } catch {
    // The live preference still applies when WebView storage is unavailable.
  }
  if (typeof window !== "undefined") window.dispatchEvent(new Event(TAURI_PROGRESS_MODE_CHANGED));
}

export function getTauriSidebarVisible(): boolean {
  try {
    return localStorage.getItem(SIDEBAR_VISIBLE_KEY) !== "off";
  } catch {
    return true;
  }
}

export function setTauriSidebarVisible(visible: boolean): void {
  try {
    localStorage.setItem(SIDEBAR_VISIBLE_KEY, visible ? "on" : "off");
  } catch {
    // The current-window toggle still applies if WebView storage is unavailable.
  }
}

export function getTauriNotificationsEnabled(): boolean {
  if (sessionValue !== null) return sessionValue;
  try {
    return localStorage.getItem(NOTIFICATIONS_KEY) !== "off";
  } catch {
    return true;
  }
}

export function setTauriNotificationsEnabled(enabled: boolean): void {
  sessionValue = enabled;
  try {
    localStorage.setItem(NOTIFICATIONS_KEY, enabled ? "on" : "off");
  } catch {
    // Keep the UI usable when WebView storage is unavailable.
  }
}
