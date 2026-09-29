const NOTIFICATIONS_KEY = "tauri-desktop-notifications";
const NOTIFICATION_EVENTS_KEY = "tauri-desktop-notification-events";
const PROGRESS_MODE_KEY = "tauri-progress-mode";
const SIDEBAR_VISIBLE_KEY = "tauri-sidebar-visible";
export const TAURI_PROGRESS_MODE_CHANGED = "tauri-progress-mode-changed";
export type TauriProgressMode = "standard" | "deep";
let sessionValue: boolean | null = null;
export type TauriNotificationKind = "turn_done" | "approval_request" | "ask_request";
export type TauriNotificationEvents = Record<TauriNotificationKind, boolean>;
const DEFAULT_NOTIFICATION_EVENTS: TauriNotificationEvents = {
  turn_done: true,
  approval_request: true,
  ask_request: true,
};
let notificationEventsValue: TauriNotificationEvents | null = null;
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

export function getTauriNotificationEvents(): TauriNotificationEvents {
  if (notificationEventsValue !== null) return { ...notificationEventsValue };
  try {
    const raw = localStorage.getItem(NOTIFICATION_EVENTS_KEY);
    if (!raw) return { ...DEFAULT_NOTIFICATION_EVENTS };
    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return { ...DEFAULT_NOTIFICATION_EVENTS };
    const value = parsed as Partial<TauriNotificationEvents>;
    return {
      turn_done: typeof value.turn_done === "boolean" ? value.turn_done : true,
      approval_request: typeof value.approval_request === "boolean" ? value.approval_request : true,
      ask_request: typeof value.ask_request === "boolean" ? value.ask_request : true,
    };
  } catch {
    return { ...DEFAULT_NOTIFICATION_EVENTS };
  }
}

export function setTauriNotificationEvent(kind: TauriNotificationKind, enabled: boolean): TauriNotificationEvents {
  const next = { ...getTauriNotificationEvents(), [kind]: enabled };
  notificationEventsValue = next;
  try {
    localStorage.setItem(NOTIFICATION_EVENTS_KEY, JSON.stringify(next));
  } catch {
    // The current window still uses the updated preference if storage is unavailable.
  }
  return { ...next };
}

export function isTauriNotificationEnabled(kind: TauriNotificationKind): boolean {
  return getTauriNotificationsEnabled() && getTauriNotificationEvents()[kind];
}
