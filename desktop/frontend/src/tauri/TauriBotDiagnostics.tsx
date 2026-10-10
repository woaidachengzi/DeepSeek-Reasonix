import { useCallback, useEffect, useRef, useState } from "react";
import { RefreshCw } from "lucide-react";
import type { BotDiagnostics } from "../lib/botDiagnostics";
import { tauriBotConnectionDiagnostics, type TauriBotSettings } from "../lib/tauriBridge";
import { useManagementT } from "./tauriManagementI18n";

export function TauriBotDiagnostics({ settings, disabled }: { settings: TauriBotSettings | null; disabled: boolean }) {
  const t = useManagementT();
  const [view, setView] = useState<BotDiagnostics | null>(null);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const epoch = useRef(0);
  const active = useRef(false);
  const pending = useRef(false);
  const load = useCallback(async () => {
    if (!active.current || !settings || disabled || pending.current) return;
    const request = ++epoch.current;
    pending.current = true;
    setLoading(true); setView(null); setFailed(false);
    try {
      const next = await tauriBotConnectionDiagnostics();
      if (active.current && request === epoch.current) setView(next);
    } catch {
      // A platform/SDK exception can contain private details. Render fixed recovery copy.
      if (active.current && request === epoch.current) setFailed(true);
    } finally {
      if (active.current && request === epoch.current) { pending.current = false; setLoading(false); }
    }
  }, [settings, disabled]);
  useEffect(() => {
    active.current = true;
    setView(null); setFailed(false); setLoading(false);
    void load();
    return () => { active.current = false; ++epoch.current; pending.current = false; };
  }, [load]);
  return <section className="tauri-bot-diagnostics" aria-label={t("settings.bots.diagnosticsTitle")}>
    <h3>{t("settings.bots.diagnosticsTitle")}</h3>
    <p className="tauri-settings-hint">{t("settings.bots.diagnosticsHint")}</p>
    <button className="tauri-settings-button" type="button" disabled={!settings || disabled || loading} onClick={() => void load()}>
      <RefreshCw size={14} aria-hidden="true" />{t("settings.bots.diagnosticsRefresh")}
    </button>
    {loading ? <p role="status">{t("common.loading")}</p> : null}
    {failed ? <p className="tauri-diagnostic-error" role="alert">{t("settings.bots.diagnosticsFailed")}</p> : null}
    {view?.connections.length === 0 ? <p className="tauri-settings-hint">{t("settings.bots.diagnosticsEmpty")}</p> : null}
    {view && view.connections.length > 0 ? <dl className="tauri-bot-diagnostics-list">
      {view.connections.map(row => <div key={row.id}>
        <dt>{row.id}</dt><dd>
          <span>{t("settings.bots.diagnosticsConfig")}: {t(`settings.bots.diagnostic.${row.configStatus}`)}</span>
          <span>{t("settings.bots.diagnosticsRuntime")}: {t(`settings.bots.diagnostic.${row.runtimeStatus}`)}</span>
        </dd>
      </div>)}
    </dl> : null}
  </section>;
}
