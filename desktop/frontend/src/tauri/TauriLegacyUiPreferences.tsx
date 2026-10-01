import { useState } from "react";
import { useI18n } from "../lib/i18n";
import { tauriLegacyUiPreferences, tauriMessageFrom } from "../lib/tauriBridge";
import { hasUiPreferenceMigration, importUiPreferences, parseLegacyUiSnapshot, rollbackUiPreferences, type LegacyUiSnapshot } from "./legacyUiMigration";

import { translateUiMigration, type UiMigrationKey } from "./uiMigrationCopy";

const names: Record<string, UiMigrationKey> = {
  "reasonix.tauri.default-workspace.v1": "workspace",
  "reasonix.tauri.shortcuts.v1": "shortcuts",
  "reasonix.tauri.desktop-layout.v1": "layout",
  "reasonix.tauri.status-bar.v1": "statusBar",
  "reasonix.tauri.workbench.collapsed-projects.v1": "projects",
  "tauri-desktop-notifications": "notifications",
  "tauri-desktop-notification-events": "notificationEvents",
  "tauri-progress-mode": "progress",
  "tauri-sidebar-visible": "sidebar",
  "reasonix-region-typography-v1": "typography",
  "notificationSoundSuccess": "successSound",
  "notificationSoundAttention": "attentionSound",
  "notificationSoundVolume": "volume",
  "generativeMusicPreset": "music",
  "reasonix-conv-width": "width",
  "reasonix-text-size": "textSize",
  "reasonix-font-family": "font",
  "reasonix-font-family-custom": "customFont",
  "reasonix-mono-font-family": "monoFont",
  "reasonix-mono-font-family-custom": "customMonoFont",
  "tauri-theme": "theme",
  "tauri-theme-style": "themeStyle",
};

export function TauriLegacyUiPreferences({ disabled = false }: { disabled?: boolean }) {
  const { locale } = useI18n();
  const t = (key: UiMigrationKey, vars?: Record<string, string | number>) => translateUiMigration(locale, key, vars);
  const [snapshot, setSnapshot] = useState<LegacyUiSnapshot | null>(null);
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [journal, setJournal] = useState(() => {
    try { return hasUiPreferenceMigration(); } catch { return true; }
  });
  const run = async (action: () => Promise<void> | void) => {
    if (disabled || busy) return;
    setBusy(true); setNotice(""); setError("");
    try { await action(); } catch (cause) { setError(tauriMessageFrom(cause)); }
    finally {
      try { setJournal(hasUiPreferenceMigration()); } catch (cause) { setError(tauriMessageFrom(cause)); }
      setBusy(false);
    }
  };
  return <div className="tauri-settings-data__actions">
    <h4>{t("title")}</h4>
    <p>{t("hint")}</p>
    <button className="tauri-settings-button" type="button" disabled={disabled || busy} onClick={() => void run(async () => {
      setSnapshot(null); setConfirmed(false);
      const next = parseLegacyUiSnapshot(await tauriLegacyUiPreferences());
      setSnapshot(next);
      if (Object.keys(next.values).length === 0) setNotice(t("empty"));
    })}>{t("preview")}</button>
    {snapshot && Object.keys(snapshot.values).length > 0 && <>
      <dl className="tauri-settings-ui-migration-values">{Object.entries(snapshot.values).map(([key, value]) => <div className="tauri-settings-data__path" key={key}>
        <dt>{t(names[key])}</dt><dd><code>{value}</code></dd>
      </div>)}</dl>
      <label><input type="checkbox" checked={confirmed} disabled={disabled || busy || journal} onChange={event => setConfirmed(event.target.checked)} />{t("confirm")}</label>
      <button className="tauri-settings-button" type="button" disabled={disabled || busy || !confirmed || journal} onClick={() => void run(() => {
        const count = importUiPreferences(snapshot, confirmed);
        setConfirmed(false); setNotice(t("imported", { count }));
      })}>{t("import")}</button>
    </>}
    {journal && <><p>{t("rollbackHint")}</p>
      <button className="tauri-settings-button" type="button" disabled={disabled || busy} onClick={() => void run(() => {
        const result = rollbackUiPreferences();
        setConfirmed(false);
        setNotice(t(result.conflicts ? "conflicts" : "restored", result));
      })}>{t("rollback")}</button>
    </>}
    {notice && <p className="tauri-settings-data__notice" role="status">{notice}</p>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
  </div>;
}
