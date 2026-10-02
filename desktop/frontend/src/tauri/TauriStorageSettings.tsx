import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Check, Copy, RefreshCw } from "lucide-react";
import { tauriMessageFrom, tauriStorageSettings, tauriWorkspaceRootsAvailability, type TauriStorageSettings as StorageView } from "../lib/tauriBridge";
import { useT } from "../lib/i18n";
import { writeClipboardText } from "../lib/clipboard";

export function TauriStorageSettings({ workspaceRoot, defaultWorkspace = "", onChooseDefaultWorkspace, onClearDefaultWorkspace }: {
  workspaceRoot?: string;
  defaultWorkspace?: string;
  onChooseDefaultWorkspace?: () => Promise<string | null>;
  onClearDefaultWorkspace?: () => void;
}) {
  const t = useT();
  const [view, setView] = useState<StorageView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState("");
  const [copyError, setCopyError] = useState("");
  const [copyBusy, setCopyBusy] = useState(false);
  const copyRequest = useRef(0);
  const copyPending = useRef(false);
  const invalidateCopy = useCallback(() => {
    copyRequest.current += 1;
    copyPending.current = false;
    setCopied("");
    setCopyError("");
    setCopyBusy(false);
  }, []);
  const [actionBusy, setActionBusy] = useState(false);
  const [defaultWorkspaceAvailable, setDefaultWorkspaceAvailable] = useState<boolean | null>(null);
  const requestRef = useRef(0);
  const load = useCallback(async () => {
    invalidateCopy();
    const request = ++requestRef.current;
    setLoading(true);
    setError("");
    try {
      const next = await tauriStorageSettings();
      if (request === requestRef.current) setView(next);
    } catch (cause) {
      if (request === requestRef.current) { setView(null); setError(tauriMessageFrom(cause)); }
    } finally {
      if (request === requestRef.current) setLoading(false);
    }
  }, [invalidateCopy]);
  useLayoutEffect(() => {
    invalidateCopy();
    return () => { copyRequest.current += 1; copyPending.current = false; };
  }, [invalidateCopy, workspaceRoot, defaultWorkspace, view, t]);
  useEffect(() => { void load(); return () => { requestRef.current += 1; }; }, [load]);
  useEffect(() => {
    let current = true;
    setDefaultWorkspaceAvailable(null);
    if (defaultWorkspace) void tauriWorkspaceRootsAvailability([defaultWorkspace]).then(
      result => { if (current) setDefaultWorkspaceAvailable(result[0] ?? null); },
      () => { if (current) setDefaultWorkspaceAvailable(null); },
    );
    return () => { current = false; };
  }, [defaultWorkspace]);

  const copy = async (label: string, path: string) => {
    if (copyPending.current) return;
    const request = ++copyRequest.current;
    copyPending.current = true;
    setCopyBusy(true);
    setCopied("");
    setCopyError("");
    try {
      if (!await writeClipboardText(path)) throw new Error("clipboard unavailable");
      if (request === copyRequest.current) setCopied(label);
    } catch {
      if (request === copyRequest.current) setCopyError(t("settings.storage.copyFailed", { label }));
    } finally {
      if (request === copyRequest.current) { copyPending.current = false; setCopyBusy(false); }
    }
  };
  const chooseDefault = async () => {
    if (!onChooseDefaultWorkspace || actionBusy) return;
    setActionBusy(true); setError("");
    try { await onChooseDefaultWorkspace(); }
    catch (cause) { setError(tauriMessageFrom(cause)); }
    finally { setActionBusy(false); }
  };
  const clearDefault = () => {
    if (!onClearDefaultWorkspace || actionBusy) return;
    setError("");
    try { onClearDefaultWorkspace(); }
    catch (cause) { setError(tauriMessageFrom(cause)); }
  };
  const paths = view ? [
    { label: t("settings.storage.currentWorkspace"), value: workspaceRoot ?? "" },
    { label: t("settings.storage.profile"), value: view.profilePath },
    { label: t("settings.storage.state"), value: view.statePath },
    { label: t("settings.storageCache"), value: view.cachePath },
    { label: t("settings.storageExtensions"), value: view.extensionsPath },
  ] : [];

  const defaultWorkspaceLabel = t("settings.storage.defaultWorkspace");

  return <section className="tauri-settings-section tauri-storage-settings" aria-label={t("settings.storageTitle")}>
    <div className="tauri-settings-data__heading"><div><h3>{t("settings.storageTitle")}</h3><p>{t("settings.storage.previewHint")}</p></div><button type="button" className="tauri-settings-button" disabled={loading} onClick={() => void load()}><RefreshCw size={13} />{t("settings.storage.refresh")}</button></div>
    {loading && <div className="tauri-settings-loading">{t("settings.loading")}</div>}
    <div className="tauri-storage-paths">
      <div className="tauri-storage-path"><span>{defaultWorkspaceLabel}</span><div><input aria-label={defaultWorkspaceLabel} value={defaultWorkspace} readOnly placeholder={t("settings.storage.unset")} /><button type="button" aria-label={t("settings.storage.copyLabel", { label: defaultWorkspaceLabel })} title={copied === defaultWorkspaceLabel ? t("settings.storage.copied") : t("settings.storage.copyPath")} disabled={!defaultWorkspace || copyBusy} aria-busy={copyBusy} onClick={() => void copy(defaultWorkspaceLabel, defaultWorkspace)}>{copied === defaultWorkspaceLabel ? <Check size={15} /> : <Copy size={15} />}</button>{onChooseDefaultWorkspace && <button type="button" className="tauri-storage-path__select" disabled={actionBusy} onClick={() => void chooseDefault()}>{t("settings.storage.choose")}</button>}{onClearDefaultWorkspace && <button type="button" className="tauri-storage-path__select" disabled={actionBusy || !defaultWorkspace} onClick={clearDefault}>{t("settings.storage.clear")}</button>}</div></div>
      {defaultWorkspace && defaultWorkspaceAvailable === false && <p className="tauri-storage-path__warning" role="status">{t("settings.storage.unavailableWorkspace")}</p>}
      {paths.map(({ label, value }) => <div className="tauri-storage-path" key={label}><span>{label}</span><div><input aria-label={label} value={value} readOnly placeholder={t("settings.storage.unset")} /><button type="button" aria-label={t("settings.storage.copyLabel", { label })} title={copied === label ? t("settings.storage.copied") : t("settings.storage.copyPath")} disabled={!value || copyBusy} aria-busy={copyBusy} onClick={() => void copy(label, value)}>{copied === label ? <Check size={15} /> : <Copy size={15} />}</button></div></div>)}
    </div>
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {copyError && <p className="tauri-diagnostic-error" role="alert">{copyError}</p>}
  </section>;
}
