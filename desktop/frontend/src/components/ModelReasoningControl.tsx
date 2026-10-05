import { useState } from "react";
import { RotateCcw } from "lucide-react";
import { useProviderT } from "../lib/providerSettingsLocale";

export function ModelReasoningControl({ options, supportedEfforts, defaultEffort, disabled, onChange }: {
  options: string[]; supportedEfforts: string[]; defaultEffort: string; disabled: boolean;
  onChange: (levels: string[], defaultLevel: string) => void;
}) {
  const t = useProviderT();
  const [custom, setCustom] = useState("");
  const [error, setError] = useState(false);
  const available = [...new Set([...options, ...supportedEfforts])].filter(level => level && level !== "auto");
  const selected = supportedEfforts.length ? supportedEfforts : available;
  const add = () => {
    const levels = custom.split(/[,，\s]+/).filter(Boolean).map(level => level.toLowerCase());
    if (!levels.length || levels.some(level => level === "auto" || !/^[a-z0-9][a-z0-9_-]{0,63}$/.test(level))) { setError(true); return; }
    onChange([...new Set([...selected, ...levels])], defaultEffort);
    setCustom(""); setError(false);
  };
  return <div className="provider-model-dialog__effort-card">
    <label>{t("settings.modelDialog.reasoningEffortOptions")}<button type="button" className="btn provider-icon-action" aria-label={t("settings.modelDialog.resetReasoningEffort")} disabled={disabled} onClick={() => { onChange([], ""); setCustom(""); setError(false); }}><RotateCcw size={16}/></button></label>
    <div className="provider-model-dialog__chips">
      {available.map(level => <label key={level}><input type="checkbox" aria-label={level} checked={selected.includes(level)} disabled={disabled || (selected.length === 1 && selected.includes(level))} onChange={event => {
        const next = event.target.checked ? [...selected, level] : selected.filter(item => item !== level);
        // An empty declaration means inheritance; reset explicitly restores it.
        if (next.length) onChange(next, next.includes(defaultEffort) ? defaultEffort : "");
      }}/>{level}</label>)}
    </div>
    <label>{t("settings.modelDialog.reasoningEffortDefault")}<select className="mem-select" value={defaultEffort} disabled={disabled || !selected.length} onChange={event => onChange(selected, event.target.value)}><option value="">{t("settings.modelDialog.automatic")}</option>{selected.map(level => <option key={level} value={level}>{level}</option>)}</select></label>
    <label>{t("settings.modelDialog.customEfforts")}<input className="mem-input" value={custom} disabled={disabled} placeholder="low, medium, high" onChange={event => { setCustom(event.target.value); setError(false); }} onKeyDown={event => { if (event.key === "Enter") { event.preventDefault(); add(); } }}/></label>
    <button type="button" className="btn" disabled={disabled || !custom.trim()} onClick={add}>{t("common.add")}</button>
    {error && <p role="alert">{t("settings.modelDialog.invalidEfforts")}</p>}
    <p>{t("settings.modelDialog.reasoningEffortHint")}</p>
    <p>{t("settings.modelDialog.customEffortsHint")}</p>
  </div>;
}
