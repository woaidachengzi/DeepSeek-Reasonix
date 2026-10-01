import { useState } from "react";
import { useT, type DictKey } from "../lib/i18n";
import { tauriLegacyUiPreferences, tauriMessageFrom } from "../lib/tauriBridge";
import { hasUiPreferenceMigration, importUiPreferences, parseLegacyUiSnapshot, rollbackUiPreferences, type LegacyUiSnapshot } from "./legacyUiMigration";

const names: Record<string, DictKey> = {
  "reasonix.tauri.default-workspace.v1": "settings.uiMigration.workspace",
  "reasonix.tauri.shortcuts.v1": "settings.uiMigration.shortcuts",
  "reasonix.tauri.desktop-layout.v1": "settings.uiMigration.layout",
  "reasonix.tauri.status-bar.v1": "settings.uiMigration.statusBar",
  "reasonix.tauri.workbench.collapsed-projects.v1": "settings.uiMigration.projects",
  "tauri-desktop-notifications": "settings.uiMigration.notifications",
  "tauri-desktop-notification-events": "settings.uiMigration.notificationEvents",
  "tauri-progress-mode": "settings.uiMigration.progress",
  "tauri-sidebar-visible": "settings.uiMigration.sidebar",
  "reasonix-region-typography-v1": "settings.uiMigration.typography",
  "notificationSoundSuccess": "settings.uiMigration.successSound",
  "notificationSoundAttention": "settings.uiMigration.attentionSound",
  "notificationSoundVolume": "settings.uiMigration.volume",
  "generativeMusicPreset": "settings.uiMigration.music",
  "reasonix-conv-width": "settings.uiMigration.width",
  "reasonix-text-size": "settings.uiMigration.textSize",
  "reasonix-font-family": "settings.uiMigration.font",
  "reasonix-font-family-custom": "settings.uiMigration.customFont",
  "reasonix-mono-font-family": "settings.uiMigration.monoFont",
  "reasonix-mono-font-family-custom": "settings.uiMigration.customMonoFont",
  "tauri-theme": "settings.uiMigration.theme",
  "tauri-theme-style": "settings.uiMigration.themeStyle",
};

export function TauriLegacyUiPreferences({ disabled = false }: { disabled?: boolean }) {
  const t = useT();
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
    <h4>{t("settings.uiMigration.title")}</h4>
    <p>{t("settings.uiMigration.hint")}</p>
    <button className="tauri-settings-button" type="button" disabled={disabled || busy} onClick={() => void run(async () => {
      setSnapshot(null); setConfirmed(false);
      const next = parseLegacyUiSnapshot(await tauriLegacyUiPreferences());
      setSnapshot(next);
      if (Object.keys(next.values).length === 0) setNotice(t("settings.uiMigration.empty"));
    })}>{t("settings.uiMigration.preview")}</button>
    {snapshot && Object.keys(snapshot.values).length > 0 && <>
      <dl className="tauri-settings-ui-migration-values">{Object.entries(snapshot.values).map(([key, value]) => <div className="tauri-settings-data__path" key={key}>
        <dt>{t(names[key])}</dt><dd><code>{value}</code></dd>
      </div>)}</dl>
      <label><input type="checkbox" checked={confirmed} disabled={disabled || busy || journal} onChange={event => setConfirmed(event.target.checked)} />{t("settings.uiMigration.confirm")}</label>
      <button className="tauri-settings-button" type="button" disabled={disabled || busy || !confirmed || journal} onClick={() => void run(() => {
        const count = importUiPreferences(snapshot, confirmed);
        setConfirmed(false); setNotice(t("settings.uiMigration.imported", { count }));
      })}>{t("settings.uiMigration.import")}</button>
    </>}
    {journal && <><p>{t("settings.uiMigration.rollbackHint")}</p>
      <button className="tauri-settings-button" type="button" disabled={disabled || busy} onClick={() => void run(() => {
        const result = rollbackUiPreferences();
        setConfirmed(false);
        setNotice(t(result.conflicts ? "settings.uiMigration.conflicts" : "settings.uiMigration.restored", result));
      })}>{t("settings.uiMigration.rollback")}</button>
    </>}
    {notice && <p className="tauri-settings-data__notice" role="status">{notice}</p>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
  </div>;
}
