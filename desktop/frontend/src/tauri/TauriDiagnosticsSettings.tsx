import { lazy, Suspense, useCallback, useState } from "react";
import { RefreshCw } from "lucide-react";
import { useT } from "../lib/i18n";
import { tauriCapabilityDiagnostics, tauriRuntimeDoctor, type TauriBridgeStatus, type TauriSessionShadowReport } from "../lib/tauriBridge";
import type { TauriSettingsTab } from "./tauriSettingsNavigation";
import { TauriAITraceSettings } from "./TauriAITraceSettings";

const TauriCapabilityDiagnosticsPage = lazy(() => import("../components/DiagnosticsSettingsPage").then(module => ({ default: module.DiagnosticsSettingsPage })));

export function DiagnosticsSettings({ sessionId, bridgeStatus, catalogAudit, catalogAuditError, sessionPageSource, hostError, workspaceRoot, busy, onRestartBridge, onRefreshCatalogAudit, onOpenData, onOpenProviders, onNavigateSettings }: {
  sessionId?: string;
  bridgeStatus?: TauriBridgeStatus | null;
  catalogAudit?: TauriSessionShadowReport | null;
  catalogAuditError?: string;
  sessionPageSource?: string;
  hostError?: string;
  workspaceRoot?: string;
  busy: boolean;
  onRestartBridge?: () => Promise<boolean>;
  onRefreshCatalogAudit?: () => Promise<void>;
  onOpenData: () => void;
  onOpenProviders: () => void;
  onNavigateSettings: (tab: TauriSettingsTab) => void;
}) {
  const t = useT();
  const [restarting, setRestarting] = useState(false);
  const [checking, setChecking] = useState(false);
  const [auditRefreshError, setAuditRefreshError] = useState(false);
  const [restartResult, setRestartResult] = useState<"ok" | "failed" | "">("");
  const loadCapabilities = useCallback((includeRuntime: boolean) => tauriCapabilityDiagnostics(workspaceRoot ?? "", includeRuntime), [workspaceRoot]);
  const sourceLabel = t(sessionPageSource === "identity" ? "settings.diagnostics.source.identity" : sessionPageSource === "partial_identity" ? "settings.diagnostics.source.partialIdentity" : sessionPageSource === "identity_unverified" ? "settings.diagnostics.source.unverified" : sessionPageSource === "legacy" ? "settings.diagnostics.source.legacy" : sessionPageSource === "cached" ? "settings.diagnostics.source.cached" : "settings.diagnostics.source.unavailable");
  const restart = async () => {
    if (!onRestartBridge || restarting) return;
    setRestarting(true);
    setRestartResult("");
    try {
      setRestartResult(await onRestartBridge() ? "ok" : "failed");
    } catch {
      setRestartResult("failed");
    } finally {
      setRestarting(false);
    }
  };
  const refreshAudit = async () => {
    if (!onRefreshCatalogAudit || checking) return;
    setChecking(true);
    setAuditRefreshError(false);
    try {
      await onRefreshCatalogAudit();
    } catch {
      setAuditRefreshError(true);
    } finally {
      setChecking(false);
    }
  };
  return <div className="tauri-settings-section tauri-settings-diagnostics">
    <TauriAITraceSettings key={sessionId ?? ""} sessionId={sessionId} />
    <h3>{t("settings.diagnostics.localService")}</h3>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.diagnostics.bridge")}<small>{t("settings.diagnostics.bridgeHint")}</small></span><span className={`tauri-settings-badge${bridgeStatus?.running ? " is-ready" : ""}`}>{bridgeStatus?.running ? t("settings.diagnostics.bridgeRunning", { protocol: bridgeStatus.protocolVersion ?? "?" }) : t("settings.diagnostics.notConnected")}</span></div>
    {onRestartBridge && <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" onClick={() => void restart()} disabled={busy || restarting}>{t("settings.diagnostics.restartBridge")}</button></div>}
    {restartResult === "ok" && <p className="tauri-settings-data__notice" role="status">{t("settings.diagnostics.bridgeRestarted")}</p>}
    {restartResult === "failed" && <p className="tauri-diagnostic-error" role="alert">{hostError || t("settings.diagnostics.bridgeRestartFailed")}</p>}
    <h3>{t("settings.diagnostics.sessionCatalog")}</h3>
    <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.diagnostics.activeSource")}<small>{t("settings.diagnostics.sourceHint")}</small></span><span className="tauri-settings-value">{sourceLabel}</span></div>
    {catalogAudit ? <div className="tauri-settings-audit">
      <div><span>{t("settings.diagnostics.legacyCatalog")}</span><strong>{catalogAudit.legacyCount}</strong></div><div><span>{t("settings.diagnostics.identityCatalog")}</span><strong>{catalogAudit.directoryCount}</strong></div><div><span>{t("settings.diagnostics.matched")}</span><strong>{catalogAudit.matchedCount}</strong></div><div><span>{t("settings.diagnostics.unclaimed")}</span><strong>{catalogAudit.unclaimedTranscripts}</strong></div>
      <p>{catalogAudit.legacyMatchesDirectory ? t("settings.diagnostics.catalogConsistent") : t("settings.diagnostics.catalogDifferences", { missing: catalogAudit.missingFromDirectory, titles: catalogAudit.titleMismatches, workspaces: catalogAudit.workspaceMismatches, order: catalogAudit.orderMismatches, disk: catalogAudit.physicalStateMismatches, errors: catalogAudit.inventoryErrors })}</p>
    </div> : <p>{catalogAuditError || t("settings.diagnostics.catalogLoading")}</p>}
    {catalogAudit && catalogAuditError && <p className="tauri-diagnostic-error" role="alert">{catalogAuditError}</p>}
    {auditRefreshError && <p className="tauri-diagnostic-error" role="alert">{t("settings.diagnostics.catalogCheckFailed")}</p>}
    <div className="tauri-settings-actions">
      {onRefreshCatalogAudit && <button type="button" className="tauri-settings-button" onClick={() => void refreshAudit()} disabled={busy || checking}><RefreshCw size={13} />{t("settings.diagnostics.recheck")}</button>}
      <button type="button" className="tauri-settings-button" onClick={onOpenData}>{t("settings.diagnostics.openData")}</button>
      <button type="button" className="tauri-settings-button" onClick={onOpenProviders}>{t("settings.diagnostics.openProviders")}</button>
    </div>
    <h3>{t("settings.diagnostics.capabilityTitle")}</h3>
    <Suspense fallback={<div className="tauri-settings-loading">{t("common.loading")}</div>}>
      <TauriCapabilityDiagnosticsPage capabilityDiagnostics={loadCapabilities} runtimeDoctorProvider={tauriRuntimeDoctor} onNavigate={tab => onNavigateSettings(tab as TauriSettingsTab)} />
    </Suspense>
  </div>;
}
