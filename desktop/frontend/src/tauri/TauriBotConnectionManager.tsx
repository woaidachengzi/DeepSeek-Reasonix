import { useEffect, useRef, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { changeTauriBotSettings, tauriMessageFrom, type TauriBotConnectionInput, type TauriBotSettings } from "../lib/tauriBridge";
import { useManagementT } from "./tauriManagementI18n";

const emptyConnection = (): TauriBotConnectionInput => ({ id: "", platform: "feishu", domain: "feishu", label: "", identity: "", secret: "" });

export function TauriBotConnectionManager({ settings, disabled, onSaved, onError, onBusyChange }: { settings: TauriBotSettings; disabled: boolean; onSaved: (settings: TauriBotSettings) => void; onError: (message: string) => void; onBusyChange: (busy: boolean) => void }) {
  const t = useManagementT();
  const [draft, setDraft] = useState(emptyConnection);
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [removeId, setRemoveId] = useState("");
  const operation = useRef(false);
  const mounted = useRef(true);
  useEffect(() => {mounted.current=true;return () => {mounted.current=false;};}, []);
  const save = async () => {
    if (disabled || operation.current) return;
    operation.current = true; setBusy(true); onBusyChange(true); onError("");
    try {
      const next=await changeTauriBotSettings({ action: "create_connection", connection: draft });
      if (!mounted.current) return;
      onSaved(next);
      setDraft(emptyConnection()); setEditing(false);
    } catch (error) { if(mounted.current) onError(tauriMessageFrom(error)); }
    finally { operation.current = false; if(mounted.current){setBusy(false);onBusyChange(false);} }
  };
  const remove = async () => {
    if (!removeId || disabled || operation.current) return;
    operation.current = true; setBusy(true); onBusyChange(true); onError("");
    try { const next=await changeTauriBotSettings({ action: "remove_connection", channelId: removeId }); if(mounted.current){onSaved(next);setRemoveId("");} }
    catch (error) { if(mounted.current) onError(tauriMessageFrom(error)); }
    finally { operation.current = false; if(mounted.current){setBusy(false);onBusyChange(false);} }
  };
  return <section className="tauri-bot-channel-access">
    <div className="tauri-settings-actions"><button className="tauri-settings-button" type="button" disabled={disabled || busy} onClick={() => setEditing(true)}><Plus size={14}/>{t("settings.bots.addConnection")}</button></div>
    {editing && <div className="tauri-bot-channel-runtime__body">
      <p>{t("settings.bots.newConnectionHint")}</p>
      <label className="tauri-bot-runtime-field"><span>{t("settings.bots.connectionId")}</span><input className="tauri-settings-input" maxLength={64} value={draft.id} disabled={busy} onChange={event => setDraft(current => ({ ...current, id: event.target.value }))}/></label>
      <label className="tauri-bot-runtime-field"><span>{t("settings.bots.connectionName")}</span><input className="tauri-settings-input" maxLength={128} value={draft.label} disabled={busy} onChange={event => setDraft(current => ({ ...current, label: event.target.value }))}/></label>
      <label className="tauri-bot-runtime-field"><span>{t("settings.bots.connectionPlatform")}</span><select className="tauri-settings-input" value={`${draft.platform}:${draft.domain}`} disabled={busy} onChange={event => { const [platform, domain] = event.target.value.split(":"); setDraft(current => ({ ...current, platform: platform as TauriBotConnectionInput["platform"], domain })); }}><option value="feishu:feishu">{t("settings.bots.channel.feishu")}</option><option value="feishu:lark">Lark</option><option value="qq:qq">QQ</option><option value="weixin:weixin">Weixin</option></select></label>
      <label className="tauri-bot-runtime-field"><span>{t("settings.bots.identity")}</span><input className="tauri-settings-input" autoComplete="off" maxLength={512} value={draft.identity} disabled={busy} onChange={event => setDraft(current => ({ ...current, identity: event.target.value }))}/></label>
      <label className="tauri-bot-runtime-field"><span>{t("settings.bots.credential")}</span><input className="tauri-settings-input" type="password" autoComplete="new-password" maxLength={8192} value={draft.secret} disabled={busy} onChange={event => setDraft(current => ({ ...current, secret: event.target.value }))}/></label>
      <div className="tauri-settings-actions"><button className="tauri-settings-button" type="button" disabled={disabled || busy || !/^[a-zA-Z0-9][a-zA-Z0-9._-]*$/.test(draft.id) || !draft.identity.trim() || !draft.secret.trim()} onClick={() => void save()}>{t("common.save")}</button><button className="tauri-settings-button" type="button" disabled={busy} onClick={() => {setEditing(false);setDraft(emptyConnection());}}>{t("common.cancel")}</button></div>
    </div>}
    {settings.channels.filter(channel => !channel.id.startsWith("legacy:")).map(channel => <div className="tauri-settings-actions" key={channel.id}><span>{channel.label} · {channel.id}</span><button className="tauri-settings-button" type="button" disabled={disabled || busy} onClick={() => setRemoveId(channel.id)}><Trash2 size={14}/>{t("settings.bots.removeConnection")}</button></div>)}
    {removeId && <div role="group" aria-label={t("settings.bots.removeConnection")}><p>{t("settings.bots.removeConnectionConfirm", { id: removeId })}</p><div className="tauri-settings-actions"><button className="tauri-settings-button" type="button" disabled={disabled || busy} onClick={() => void remove()}>{t("settings.bots.confirmRemoveConnection")}</button><button className="tauri-settings-button" type="button" disabled={busy} onClick={() => setRemoveId("")}>{t("common.cancel")}</button></div></div>}
  </section>;
}
