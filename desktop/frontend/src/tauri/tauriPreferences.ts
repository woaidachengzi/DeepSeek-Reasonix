const NOTIFICATIONS_KEY = "tauri-desktop-notifications";
let sessionValue: boolean | null = null;

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
