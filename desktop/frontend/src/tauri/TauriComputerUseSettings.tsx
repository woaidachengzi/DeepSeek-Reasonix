import { useCallback, useEffect, useRef, useState } from "react";
import { CircleAlert, CircleCheck, ExternalLink, Monitor, RefreshCw } from "lucide-react";
import { SettingsSection } from "../components/SettingsForm";
import { useT } from "../lib/i18n";
import { tauriSandboxSettings, tauriMessageFrom, type TauriSandboxSettings } from "../lib/tauriBridge";
import { openComputerUsePermission, tauriComputerUseStatus, type ComputerUsePermission, type ComputerUseStatus } from "../lib/tauriComputerUse";
import "./TauriComputerUseSettings.css";

const settingsAPI = { status: tauriComputerUseStatus, sandbox: tauriSandboxSettings, openPermission: openComputerUsePermission };

export function TauriComputerUseSettings({ workspaceRoot, sessionId, onOpenSandbox, onOpenPermissions, onOpenModels, api = settingsAPI }: {
  workspaceRoot?: string;
  sessionId?: string;
  onOpenSandbox: () => void;
  onOpenPermissions: () => void;
  onOpenModels: () => void;
  api?: typeof settingsAPI;
}) {
  const t = useT();
  const [view, setView] = useState<{ native: ComputerUseStatus; sandbox: TauriSandboxSettings } | null>(null);
  const [loading, setLoading] = useState(true);
  const [opening, setOpening] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const epoch = useRef(0);
  const openingRef = useRef<number | null>(null);
  const load = useCallback(async () => {
    const request = ++epoch.current;
    openingRef.current = null;
    setOpening(false);
    setLoading(true);
    setView(null);
    setError("");
    setNotice("");
    try {
      const [native, sandbox] = await Promise.all([api.status(), api.sandbox(workspaceRoot, sessionId)]);
      if (epoch.current === request) setView({ native, sandbox });
    } catch (cause) {
      if (epoch.current === request) setError(tauriMessageFrom(cause));
    } finally {
      if (epoch.current === request) setLoading(false);
    }
  }, [api, workspaceRoot, sessionId]);
  useEffect(() => { void load(); return () => { epoch.current++; }; }, [load]);

  const openPermission = async (permission: ComputerUsePermission) => {
    if (openingRef.current !== null) return;
    const request = epoch.current;
    openingRef.current = request;
    setOpening(true);
    setError("");
    setNotice("");
    try {
      await api.openPermission(permission);
      if (epoch.current === request) setNotice(t("settings.computerUse.opened"));
    } catch (cause) {
      if (epoch.current === request) setError(tauriMessageFrom(cause));
    } finally {
      if (openingRef.current === request) openingRef.current = null;
      if (epoch.current === request) setOpening(false);
    }
  };

  return <div className="tauri-settings-section tauri-computer-use-settings" aria-busy={loading}>
    {loading ? <p role="status">{t("common.loading")}</p> : view ? <>
      <SettingsSection title={t("settings.computerUse.control")}>
        <div className="tauri-computer-use-native">
          <span className="tauri-computer-use-native__icon"><Monitor size={25} aria-hidden="true" /></span>
          <div><strong>{t("settings.computerUse.native")}</strong><p>{view.native.available ? t("settings.computerUse.included") : t("settings.computerUse.unsupported")}</p></div>
        </div>
        <div className="tauri-computer-use-row">
          <div><strong>{t("settings.computerUse.workspace")}</strong><p>{view.sandbox.bash === "enforce" ? t("settings.computerUse.blocked") : t("settings.computerUse.allowed")}</p></div>
          <button type="button" className="tauri-settings-button" onClick={onOpenSandbox}>{t("settings.computerUse.sandbox")}</button>
        </div>
        <p>{t("settings.computerUse.sandboxHint")}</p>
      </SettingsSection>
      <SettingsSection title={t("settings.computerUse.systemPermissions")} description={t("settings.computerUse.permissionHint")}>
        {view.native.available && (!view.native.accessibility || !view.native.screenRecording) && <p role="note">{t("settings.computerUse.permissionRecovery")}</p>}
        {(["accessibility", "screen_recording"] as const).map(permission => {
          const granted = permission === "accessibility" ? view.native.accessibility : view.native.screenRecording;
          const label = permission === "accessibility" ? t("settings.computerUse.accessibility") : t("settings.computerUse.screenRecording");
          const Icon = view.native.available && granted ? CircleCheck : CircleAlert;
          return <div className="tauri-computer-use-row" key={permission}>
            <div><strong>{label}</strong><p className="tauri-computer-use-permission"><Icon size={14} aria-hidden="true" />{!view.native.available ? t("settings.computerUse.notApplicable") : granted ? t("settings.computerUse.granted") : t("settings.computerUse.notGranted")}</p></div>
            <button type="button" className="tauri-settings-button" disabled={!view.native.available || opening} aria-label={t("settings.computerUse.openPermission", { permission: label })} onClick={() => void openPermission(permission)}><ExternalLink size={14} aria-hidden="true" />{t("settings.computerUse.openSystemSettings")}</button>
          </div>;
        })}
      </SettingsSection>
      <SettingsSection title={t("settings.computerUse.rules")}>
        <div className="tauri-computer-use-row"><div><strong>{t("settings.computerUse.approval")}</strong><p>{t("settings.computerUse.approvalHint")}</p></div><button type="button" className="tauri-settings-button" onClick={onOpenPermissions}>{t("settings.computerUse.manageApproval")}</button></div>
        <div className="tauri-computer-use-row"><div><strong>{t("settings.computerUse.scope")}</strong><p>{t("settings.computerUse.scopeHint")}</p></div></div>
        <div className="tauri-computer-use-row"><div><strong>{t("settings.computerUse.images")}</strong><p>{t("settings.computerUse.imagesHint")}</p></div></div>
      </SettingsSection>
      <SettingsSection title={t("settings.computerUse.start")} description={t("settings.computerUse.startHint")}>
        <blockquote>{t("settings.computerUse.example")}</blockquote>
        <button type="button" className="tauri-settings-button" onClick={onOpenModels}>{t("settings.computerUse.models")}</button>
      </SettingsSection>
    </> : null}
    {error && <p className="tauri-diagnostic-error" role="alert">{t("settings.computerUse.error", { error })}</p>}
    {notice && <p role="status">{notice}</p>}
    <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={loading || opening} onClick={() => void load()}><RefreshCw size={14} aria-hidden="true" />{t("settings.computerUse.refresh")}</button></div>
  </div>;
}
