import { useEffect, useRef, useState, type FormEvent } from "react";
import { changeTauriPermissionSettings, tauriMessageFrom, tauriPermissionSettings, type TauriPermissionList, type TauriPermissionMode, type TauriPermissionSettings } from "../lib/tauriBridge";
import { useT } from "../lib/i18n";

const RULE_LISTS: TauriPermissionList[] = ["deny", "ask", "allow"];

export function TauriPermissionsSettings({ currentSessionState, currentSessionHasAttachments, onApplyToCurrentSession }: {
  currentSessionState?: "idle" | "running" | "paused";
  currentSessionHasAttachments?: boolean;
  onApplyToCurrentSession?: () => Promise<boolean>;
}) {
  const t = useT();
  const [view, setView] = useState<TauriPermissionSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [pendingApply, setPendingApply] = useState(false);
  const [drafts, setDrafts] = useState<Record<TauriPermissionList, string>>({ allow: "", ask: "", deny: "" });
  const busyRef = useRef(false);

  const reload = () => {
    setLoading(true);
    setError("");
    void tauriPermissionSettings().then(setView).catch(err => setError(tauriMessageFrom(err))).finally(() => setLoading(false));
  };
  useEffect(reload, []);

  const change = async (action: "mode" | "add" | "remove", list?: TauriPermissionList, rule?: string, mode?: TauriPermissionMode) => {
    if (busyRef.current) return;
    const normalizedRule = rule?.trim() ?? "";
    if (action !== "mode" && !normalizedRule) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const next = action === "mode"
        ? await changeTauriPermissionSettings({ action, mode: mode! })
        : await changeTauriPermissionSettings({ action, list: list!, rule: normalizedRule });
      setView(next);
      setPendingApply(true);
      setNotice(t("settings.permission.saved"));
      if (action === "add" && list) setDrafts(previous => ({ ...previous, [list]: "" }));
    } catch (err) {
      setError(tauriMessageFrom(err));
    } finally {
      busyRef.current = false;
      setBusy(false);
    }
  };

  const submitRule = (event: FormEvent<HTMLFormElement>, list: TauriPermissionList) => {
    event.preventDefault();
    void change("add", list, drafts[list]);
  };

  const applyCurrent = async () => {
    if (!onApplyToCurrentSession || busyRef.current || currentSessionState !== "idle" || currentSessionHasAttachments) return;
    busyRef.current = true;
    setBusy(true);
    setError("");
    try {
      const applied = await onApplyToCurrentSession();
      if (applied) {
        setPendingApply(false);
        setNotice(t("settings.permission.applied"));
      } else setError(t("settings.permission.applyFailed"));
    } catch { setError(t("settings.permission.applyFailed")); }
    finally { busyRef.current = false; setBusy(false); }
  };

  return <div className="tauri-settings-section tauri-permissions-settings">
    <h3>{t("settings.permission.defaultDecision")}</h3>
    <p>{t("settings.permission.defaultDecisionHint")}</p>
    {loading ? <div className="tauri-settings-loading">{t("common.loading")}</div> : view ? <>
      <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.writerMode")}<small>{t("settings.permissionsModeHint")}</small></span><div className="tauri-settings-radio-group" role="radiogroup" aria-label={t("settings.permission.defaultDecision")}>{(["ask", "allow", "deny"] as const).map(mode => <button key={mode} type="button" role="radio" aria-checked={view.mode === mode} className={`tauri-settings-radio${view.mode === mode ? " is-active" : ""}`} disabled={busy} onClick={() => void change("mode", undefined, undefined, mode)}>{mode === "ask" ? t("settings.modeAsk") : mode === "allow" ? t("settings.modeAllow") : t("settings.modeDeny")}</button>)}</div></div>
      <h3>{t("settings.permissionRules")}</h3>
      <p>{t("settings.ruleForm")}</p>
      <div className="tauri-permissions-rules">{RULE_LISTS.map(list => {
        const title = t(list === "deny" ? "settings.ruleDeny" : list === "ask" ? "settings.ruleAsk" : "settings.ruleAllow");
        const hint = t(list === "deny" ? "settings.ruleDenyHint" : list === "ask" ? "settings.ruleAskHint" : "settings.ruleAllowHint");
        return <section key={list} className="tauri-permissions-rule-card" aria-label={t("settings.permission.ruleGroup", { title })}>
        <h4>{title}</h4><p>{hint}</p>
        <div className="tauri-permissions-rule-list">{view[list].length ? view[list].map(rule => <div className="tauri-permissions-rule" key={rule}><code>{rule}</code><button type="button" aria-label={t("settings.permission.removeRule", { title, rule })} disabled={busy} onClick={() => void change("remove", list, rule)}>{t("settings.permission.remove")}</button></div>) : <span>{t("common.none")}</span>}</div>
        <form onSubmit={event => submitRule(event, list)}><input aria-label={t("settings.permission.addRule", { title })} value={drafts[list]} maxLength={2048} disabled={busy} placeholder={t("settings.permission.rulePlaceholder")} onChange={event => setDrafts(previous => ({ ...previous, [list]: event.target.value }))} /><button type="submit" className="tauri-settings-button" disabled={busy || !drafts[list].trim()}>{t("common.add")}</button></form>
      </section>;
      })}</div>
      {pendingApply && currentSessionState && onApplyToCurrentSession && <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy || currentSessionState !== "idle" || currentSessionHasAttachments} onClick={() => void applyCurrent()}>{t("settings.permission.applyCurrent")}</button></div>}
    </> : <button type="button" className="tauri-settings-button" onClick={reload}>{t("common.retry")}</button>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
