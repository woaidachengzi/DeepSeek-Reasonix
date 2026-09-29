import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { changeTauriSandboxSettings, tauriMessageFrom, tauriSandboxSettings, type TauriSandboxChange, type TauriSandboxSettings } from "../lib/tauriBridge";
import { useT } from "../lib/i18n";

type SandboxDraft = Required<TauriSandboxChange>;
const SHELL_OPTIONS = ["auto", "bash", "powershell", "pwsh"] as const;

export function TauriSandboxSettings({ workspaceRoot, currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  workspaceRoot?: string;
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const t = useT();
  const [view, setView] = useState<TauriSandboxSettings | null>(null);
  const [draft, setDraft] = useState<SandboxDraft | null>(null);
  const [path, setPath] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [pendingApply, setPendingApply] = useState(false);
  const busyRef = useRef(false);

  const reload = useCallback(() => {
    setLoading(true);
    setError("");
    void tauriSandboxSettings(workspaceRoot).then(next => {
      setView(next);
      setDraft({ bash: next.bash, network: next.network, workspaceRoot: next.workspaceRoot, allowWrite: next.allowWrite, shell: next.shell });
    }).catch(err => { setView(null); setDraft(null); setError(tauriMessageFrom(err)); }).finally(() => setLoading(false));
  }, [workspaceRoot]);
  useEffect(() => { reload(); }, [reload]);

  const addPath = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const value = path.trim();
    if (!value || !draft || draft.allowWrite.includes(value)) return;
    setDraft({ ...draft, allowWrite: [...draft.allowWrite, value] });
    setPath("");
  };

  const save = async () => {
    if (!draft || busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const saved = await changeTauriSandboxSettings({ ...draft, shell: draft.shell === view?.shell ? undefined : draft.shell });
      let next = saved;
      let refreshWarning = "";
      if (workspaceRoot) {
        try { next = await tauriSandboxSettings(workspaceRoot); }
        catch (cause) { refreshWarning = ` ${t("settings.sandbox.refreshFailed", { error: tauriMessageFrom(cause) })}`; }
      }
      setView(next);
      setDraft({ bash: next.bash, network: next.network, workspaceRoot: next.workspaceRoot, allowWrite: next.allowWrite, shell: next.shell });
      setPendingApply(true);
      setNotice(`${t("settings.sandbox.saved")}${refreshWarning}`);
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const applyCurrent = async () => {
    if (!onApplyToCurrentSession || busyRef.current || currentSessionState !== "idle" || currentSessionHasAttachments) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      if (await onApplyToCurrentSession()) {
        setPendingApply(false);
        setNotice(t("settings.permission.applied"));
      } else setError(t("settings.permission.applyFailed"));
    } catch { setError(t("settings.permission.applyFailed")); }
    finally { busyRef.current = false; setBusy(false); }
  };

  const changed = Boolean(view && draft && (view.bash !== draft.bash || view.network !== draft.network || view.workspaceRoot !== draft.workspaceRoot || view.shell !== draft.shell || JSON.stringify(view.allowWrite) !== JSON.stringify(draft.allowWrite)));
  return <div className="tauri-settings-section tauri-sandbox-settings">
    <h3>{t("settings.sandboxTitle")}</h3>
    <p>{t("settings.sandboxBoundaryHint")}</p>
    {loading ? <div className="tauri-settings-loading">{t("common.loading")}</div> : view && draft ? <>
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.shellInterpreter")}<small>{t("settings.sandbox.shellHint")}</small></span><select className="tauri-settings-input" aria-label={t("settings.shellInterpreter")} value={draft.shell} disabled={busy} onChange={event => setDraft({ ...draft, shell: event.target.value })}>{!SHELL_OPTIONS.includes(draft.shell as typeof SHELL_OPTIONS[number]) && <option value={draft.shell}>{t("settings.sandbox.unknownShell", { shell: draft.shell })}</option>}<option value="auto">{t("settings.shellAuto")}</option><option value="bash">{t("settings.shellBash")}</option><option value="powershell">{t("settings.shellPowershell")}</option><option value="pwsh">{t("settings.shellPwsh")}</option></select></div>
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.resolvedShell")}<small>{t("settings.sandbox.detectionHint")}</small></span><output className="tauri-sandbox-settings__resolved-shell">{view.resolvedShell || t("settings.shellNotDetected")}</output></div>
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.bashSandbox")}<small>{view.platform === "windows" ? t("settings.bashUnavailableWindows") : t("settings.sandbox.isolationHint")}</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.bashSandbox")}>{(["enforce", "off"] as const).map(mode => <button key={mode} type="button" role="radio" aria-checked={draft.bash === mode} className={`tauri-settings-radio${draft.bash === mode ? " is-active" : ""}`} disabled={busy || view.platform === "windows"} onClick={() => setDraft({ ...draft, bash: mode })}>{mode === "enforce" ? t("settings.bashEnforceShort") : t("settings.bashOffShort")}</button>)}</div></div>
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.allowNetwork")}<small>{t("settings.sandbox.networkHint")}</small></span><label><input type="checkbox" aria-label={t("settings.allowNetwork")} checked={draft.network} disabled={busy} onChange={event => setDraft({ ...draft, network: event.target.checked })} /></label></div>
      <h3>{t("settings.effectiveWriteRoots")}</h3>
      <p>{t("settings.sandbox.fileScopeHint")}</p>
      <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="tauri-sandbox-root">{t("settings.workspaceRoot")}</label><input id="tauri-sandbox-root" className="tauri-settings-input" aria-label={t("settings.workspaceRoot")} value={draft.workspaceRoot} maxLength={4096} disabled={busy} placeholder={t("settings.workspaceDefault")} onChange={event => setDraft({ ...draft, workspaceRoot: event.target.value })} /></div>
      <div className="tauri-permissions-rule-card"><h4>{t("settings.sandbox.extraWritableDirectories")}</h4><p>{t("settings.sandbox.extraWritableHint")}</p><div className="tauri-permissions-rule-list">{draft.allowWrite.length ? draft.allowWrite.map(item => <div className="tauri-permissions-rule" key={item}><code>{item}</code><button type="button" aria-label={t("settings.sandbox.removeWritableDirectory", { path: item })} disabled={busy} onClick={() => setDraft({ ...draft, allowWrite: draft.allowWrite.filter(value => value !== item) })}>{t("settings.permission.remove")}</button></div>) : <span>{t("settings.sandbox.noExtraDirectories")}</span>}</div><form onSubmit={addPath}><input aria-label={t("settings.sandbox.addWritableDirectory")} value={path} maxLength={4096} disabled={busy} placeholder={t("settings.sandbox.directoryPath")} onChange={event => setPath(event.target.value)} /><button type="submit" className="tauri-settings-button" disabled={busy || !path.trim() || draft.allowWrite.length >= 64}>{t("common.add")}</button></form></div>
      <div className="tauri-permissions-rule-card"><h4>{t("settings.effectiveWriteRoots")}</h4><p>{t("settings.sandbox.currentProjectRootsHint")}</p>{view.effectiveRootsError ? <p className="tauri-diagnostic-error" role="status">{t("settings.sandbox.resolveFailed", { error: view.effectiveRootsError })}</p> : !workspaceRoot ? <p>{t("settings.sandbox.selectProject")}</p> : view.effectiveWriteRoots.length ? <div className="tauri-permissions-rule-list">{view.effectiveWriteRoots.map((root, index) => <code key={`${root}-${index}`}>{root}</code>)}</div> : <p>{t("settings.noEffectiveWriteRoots")}</p>}</div>
      <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy || !changed} onClick={() => void save()}>{t("settings.sandbox.save")}</button>{pendingApply && currentSessionState && onApplyToCurrentSession && <button type="button" className="tauri-settings-button" disabled={busy || changed || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>{t("settings.permission.applyCurrent")}</button>}</div>
    </> : <button type="button" className="tauri-settings-button" onClick={reload}>{t("common.retry")}</button>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
