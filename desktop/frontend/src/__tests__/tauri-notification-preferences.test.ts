import {
  getTauriNotificationEvents,
  getTauriNotificationsEnabled,
  isTauriNotificationEnabled,
  setTauriNotificationEvent,
  setTauriNotificationsEnabled,
} from "../tauri/tauriPreferences";

const values = new Map<string, string>();
Object.defineProperty(globalThis, "localStorage", {
  configurable: true,
  value: {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
  },
});

let passed = 0;
function ok(condition: unknown, label: string) {
  if (!condition) throw new Error(`FAIL ${label}`);
  passed += 1;
  process.stdout.write(`  PASS  ${label}\n`);
}

ok(getTauriNotificationsEnabled(), "legacy installs keep the existing enabled-by-default notification switch");
values.set("tauri-desktop-notification-events", "not-json");
const defaults = getTauriNotificationEvents();
ok(defaults.turn_done && defaults.approval_request && defaults.ask_request, "all three event types default on to match stable notification behavior");
values.delete("tauri-desktop-notification-events");
setTauriNotificationEvent("approval_request", false);
ok(!isTauriNotificationEnabled("approval_request") && isTauriNotificationEnabled("turn_done"), "event preferences persist independently");
ok(JSON.parse(values.get("tauri-desktop-notification-events") ?? "{}").approval_request === false, "event preference is saved to Preview local storage");
setTauriNotificationsEnabled(false);
ok(!isTauriNotificationEnabled("turn_done"), "the master switch suppresses every event type");
setTauriNotificationsEnabled(true);
ok(isTauriNotificationEnabled("turn_done") && !isTauriNotificationEnabled("approval_request"), "reenabling the master switch restores the saved event choices");
process.stdout.write(`\n${passed} passed, 0 failed\n`);
