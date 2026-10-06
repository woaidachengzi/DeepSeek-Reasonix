import { useEffect, useRef, useState } from "react";
import { RefreshCw, ShieldCheck } from "lucide-react";
import { changeTauriBotPairing, tauriBotPairing, tauriMessageFrom, type TauriBotPairingView } from "../lib/tauriBridge";
import { useManagementT } from "./tauriManagementI18n";

export function TauriBotPairingManager({ disabled, onApproved, onBusyChange }: {disabled: boolean; onApproved: () => void; onBusyChange: (busy: boolean) => void}) {
  const t = useManagementT();
  const [view, setView] = useState<TauriBotPairingView | null>(null);
  const [confirmCode, setConfirmCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const mounted = useRef(true);
  const operation = useRef(false);
  const reload = async () => {
    if (operation.current) return;
    operation.current = true; setBusy(true); setError("");
    try {const next = await tauriBotPairing(); if (mounted.current) setView(next);}
    catch (error) {if (mounted.current) setError(tauriMessageFrom(error));}
    finally {operation.current = false;if (mounted.current) setBusy(false);}
  };
  useEffect(() => {mounted.current=true;void reload();return () => {mounted.current=false;};}, []);
  const decide = async (action: "approve" | "reject", code: string) => {
    if (disabled || operation.current) return;
    operation.current=true;setBusy(true);onBusyChange(true);setError("");
    try {const next=await changeTauriBotPairing(action,code);if (mounted.current) {setView(next);setConfirmCode("");if(action==="approve") onApproved();}}
    catch (error) {if(mounted.current) setError(tauriMessageFrom(error));}
    finally {operation.current=false;if(mounted.current){setBusy(false);onBusyChange(false);}}
  };
  return <section className="tauri-bot-channel-access">
    <div className="tauri-settings-actions"><h3>{t("settings.bots.pairingRequests")}</h3><button type="button" className="tauri-settings-button" disabled={busy || disabled} onClick={() => void reload()}><RefreshCw size={14}/>{t("settings.bots.refresh")}</button></div>
    {error && <p role="alert" className="tauri-diagnostic-error">{error}</p>}
    {view?.requests.length===0 && <p>{t("settings.bots.noPairingRequests")}</p>}
    {view?.requests.map(request => <div className="tauri-bot-channel-runtime__body" key={request.code}>
      <strong>{request.user_name || request.user_id}</strong><small>{request.platform} · {request.connection_id || request.domain} · {request.user_id}</small><small>{t("settings.bots.pairingExpires", {time:new Date(request.expires_at).toLocaleString()})}</small>
      {confirmCode===request.code ? <><p>{t("settings.bots.pairingApprovalConfirm")}</p><div className="tauri-settings-actions"><button className="tauri-settings-button" type="button" disabled={disabled || busy} onClick={() => void decide("approve",request.code)}><ShieldCheck size={14}/>{t("settings.bots.confirmPairing")}</button><button className="tauri-settings-button" type="button" disabled={busy} onClick={() => setConfirmCode("")}>{t("common.cancel")}</button></div></> : <div className="tauri-settings-actions"><button className="tauri-settings-button" type="button" disabled={disabled || busy} onClick={() => setConfirmCode(request.code)}>{t("settings.bots.approvePairing")}</button><button className="tauri-settings-button" type="button" disabled={disabled || busy} onClick={() => void decide("reject",request.code)}>{t("settings.bots.rejectPairing")}</button></div>}
    </div>)}
  </section>;
}
