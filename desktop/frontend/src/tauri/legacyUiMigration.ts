import keys from "./legacyUiPreferenceKeys.json";

const allowed = new Set(keys);
const JOURNAL_KEY = "reasonix.tauri.ui-preference-migration.v1";
const SOURCE = "tauri://localhost";
const LIMIT = 262_144;
export interface LegacyUiSnapshot { source: string; values: Record<string, string> }
interface Journal { version: 1; source: string; before: Record<string, string | null>; after: Record<string, string> }
const invalid = () => new Error("旧界面偏好记录无效，请重新预览或手动选择偏好。");

function object(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}
function bounded(value: unknown, nullable = false): value is Record<string, string | null> {
  if (!object(value) || new TextEncoder().encode(JSON.stringify(value)).length > LIMIT) return false;
  return Object.entries(value).every(([key, entry]) => allowed.has(key) &&
    ((nullable && entry === null) || (typeof entry === "string" && entry.length <= 65_536 && !entry.includes("\0"))));
}

export function parseLegacyUiSnapshot(value: unknown): LegacyUiSnapshot {
  if (!object(value) || value.source !== SOURCE || Object.keys(value).length !== 2 || !bounded(value.values)) throw invalid();
  const values = value.values as Record<string, string>;
  const workspace = values["reasonix.tauri.default-workspace.v1"];
  if (workspace !== undefined && (workspace.length > 4096 || !workspace.startsWith("/") || /[\x00-\x1f\x7f]/.test(workspace))) throw invalid();
  // Existing preference readers validate/normalize their domain schemas. Here
  // JSON preferences must at least be parseable objects, never executable text.
  for (const key of ["reasonix.tauri.shortcuts.v1", "reasonix.tauri.status-bar.v1", "reasonix.tauri.workbench.collapsed-projects.v1", "reasonix-region-typography-v1", "tauri-desktop-notification-events"]) {
    if (values[key] !== undefined) {
      try { if (!object(JSON.parse(values[key]))) throw invalid(); } catch { throw invalid(); }
    }
  }
  return { source: SOURCE, values: { ...values } };
}

function readJournal(store: Storage): Journal | null {
  const raw = store.getItem(JOURNAL_KEY);
  if (raw === null) return null;
  if (raw.length > LIMIT * 2 + 4096) throw invalid();
  let journal: unknown;
  try { journal = JSON.parse(raw); } catch { throw invalid(); }
  if (!object(journal) || journal.version !== 1 || journal.source !== SOURCE || Object.keys(journal).length !== 4 ||
    !bounded(journal.before, true) || !bounded(journal.after) ||
    Object.keys(journal.before).sort().join("\n") !== Object.keys(journal.after).sort().join("\n")) throw invalid();
  return journal as unknown as Journal;
}

export function hasUiPreferenceMigration(store: Storage = localStorage): boolean { return readJournal(store) !== null; }

export function rollbackUiPreferences(store: Storage = localStorage): { restored: number; conflicts: number } {
  const journal = readJournal(store);
  if (!journal) return { restored: 0, conflicts: 0 };
  let restored = 0;
  let conflicts = 0;
  for (const [key, after] of Object.entries(journal.after)) {
    const current = store.getItem(key);
    if (current === journal.before[key]) continue; // Already restored, including partial rollback.
    if (current !== after) { conflicts++; continue; } // Preserve later user edits.
    const before = journal.before[key];
    if (before === null) store.removeItem(key); else store.setItem(key, before);
    restored++;
  }
  if (conflicts === 0) store.removeItem(JOURNAL_KEY);
  return { restored, conflicts };
}

export function importUiPreferences(input: unknown, confirmed: boolean, store: Storage = localStorage): number {
  if (!confirmed) throw new Error("请先确认旧工作区和偏好属于要迁入的档案。");
  if (readJournal(store)) throw new Error("已有界面偏好迁移记录，请先撤回该次迁移。");
  const snapshot = parseLegacyUiSnapshot(input);
  const entries = Object.entries(snapshot.values).filter(([key, value]) => store.getItem(key) !== value);
  if (!entries.length) return 0;
  const before = Object.fromEntries(entries.map(([key]) => [key, store.getItem(key)]));
  if (!bounded(before, true)) throw new Error("当前界面偏好过大，无法保留回退记录，请手动设置。");
  const journal: Journal = { version: 1, source: SOURCE, before, after: Object.fromEntries(entries) };
  // Persist the complete rollback record before the first preference write.
  // A terminated or quota-failed import remains recoverable on next startup.
  store.setItem(JOURNAL_KEY, JSON.stringify(journal));
  try { for (const [key, value] of entries) store.setItem(key, value); }
  catch {
    try { rollbackUiPreferences(store); } catch { /* Durable record is retained for retry. */ }
    throw new Error("界面偏好写入失败，请撤回迁移后释放存储空间并重试。");
  }
  return entries.length;
}
