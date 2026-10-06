import { TauriRemoteForwards } from "./TauriRemoteForwards";
import { useEffect, useRef, useState } from "react";
import { browseTauriRemoteHost, changeTauriRemoteSettings, connectTauriRemoteHost, disconnectTauriRemoteHost, previewTauriRemoteFile, saveTauriRemoteFile, scanTauriRemoteSSHConfig, tauriMessageFrom, tauriRemoteSettings, type TauriRemoteHostInput, type TauriRemoteSettings, type TauriRemoteConnectResponse } from "../lib/tauriBridge";
import type { BridgeRemoteBrowseResponse, BridgeRemoteFilePreviewResponse, BridgeRemoteFileSaveRequest } from "../lib/bridgeProtocol.generated";
import { useT } from "../lib/i18n";

const EMPTY_HOST: TauriRemoteHostInput = {
  name: "", host: "", port: 0, user: "", identityFile: "", proxyJump: "", workspace: "",
  serveInstall: "auto", credentialMode: "remote", useSSHConfig: false,
  passwordAction: "keep", password: "", passphraseAction: "keep", passphrase: "",
};

const REMOTE_ERROR_KEYS = {
  connection_failed: "settings.remote.error.connection_failed",
  authentication_failed: "settings.remote.error.authentication_failed",
  host_key_mismatch: "settings.remote.error.host_key_mismatch",
} as const;

function editableHost(host: TauriRemoteSettings["hosts"][number]): TauriRemoteHostInput {
  const { passwordSet: _passwordSet, passphraseSet: _passphraseSet, connection: _connection, ...editable } = host;
  return { ...editable, passwordAction: "keep", password: "", passphraseAction: "keep", passphrase: "" };
}

export function TauriRemoteSettings() {
  const t = useT();
  const [view, setView] = useState<TauriRemoteSettings | null>(null);
  const [draft, setDraft] = useState<TauriRemoteHostInput>(EMPTY_HOST);
  const [editing, setEditing] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [aliases, setAliases] = useState<string[]>([]);
  const [pendingRemove, setPendingRemove] = useState<TauriRemoteSettings["hosts"][number] | null>(null);
  const [fixedName, setFixedName] = useState(false);
  const [connecting, setConnecting] = useState("");
  const [connectionStates, setConnectionStates] = useState<Record<string, TauriRemoteConnectResponse>>({});
  const [credentialAttempts, setCredentialAttempts] = useState<Record<string, { password: string; passphrase: string }>>({});
  const [browsingHost, setBrowsingHost] = useState("");
  const [browseListing, setBrowseListing] = useState<BridgeRemoteBrowseResponse | null>(null);
  const [browseBusy, setBrowseBusy] = useState(false);
  const [browseError, setBrowseError] = useState(false);
  const [remotePreview, setRemotePreview] = useState<BridgeRemoteFilePreviewResponse | null>(null);
  const [remoteDraft, setRemoteDraft] = useState("");
  const [remotePreviewBusy, setRemotePreviewBusy] = useState(false);
  const [remotePreviewError, setRemotePreviewError] = useState(false);
  const [remoteSaveBusy, setRemoteSaveBusy] = useState(false);
  const [remoteSaveError, setRemoteSaveError] = useState(false);
  const hasUnsavedRemoteDraft = Boolean(remotePreview && remoteDraft !== remotePreview.content);
  const confirmDiscardRemoteDraft = () => !hasUnsavedRemoteDraft || window.confirm(t("settings.remote.discardEditsConfirm"));

  const mounted = useRef(true);
  const viewEpoch = useRef(0);
  const connectEpoch = useRef(0);
  const browseEpoch = useRef(0);
  const previewEpoch = useRef(0);
  const reload = () => {
    const epoch = ++viewEpoch.current;
    setLoading(true); setError("");
    void tauriRemoteSettings().then(next => { if (mounted.current && epoch === viewEpoch.current) {setView(next);setConnectionStates({});} })
      .catch(error => {if (mounted.current && epoch === viewEpoch.current) setError(tauriMessageFrom(error));})
      .finally(() => {if (mounted.current && epoch === viewEpoch.current) setLoading(false);});
  };
  useEffect(() => {
    mounted.current = true; reload();
    return () => { mounted.current = false; ++viewEpoch.current; ++connectEpoch.current; ++browseEpoch.current; ++previewEpoch.current; };
  }, []);
  const connectionFor = (name: string) => connectionStates[name] ?? view?.hosts.find(host => host.name === name)?.connection;

  const save = async () => {
    if (busy) return;
    ++viewEpoch.current;
    setBusy(true); setError(""); setNotice("");
    try {
      const next = await changeTauriRemoteSettings({ action: "upsert", host: draft });
      setView(next); setConnectionStates(current => Object.fromEntries(Object.entries(current).filter(([name]) => name !== draft.name))); setDraft(EMPTY_HOST); setEditing(false);
      setNotice(t("settings.remote.saved"));
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setBusy(false); }
  };

  const remove = async (name: string) => {
    if (busy) return false;
    setBusy(true); setError(""); setNotice("");
    try { setView(await changeTauriRemoteSettings({ action: "remove", name })); setNotice(t("settings.remote.removed")); return true; }
    catch (err) { setError(tauriMessageFrom(err)); return false; }
    finally { setBusy(false); }
  };

  const scan = async () => {
    setBusy(true); setError("");
    try {
      const result = await scanTauriRemoteSSHConfig();
      setAliases(result.aliases.map(entry => entry.alias));
      setNotice(t("settings.remote.scanCount", { count: result.aliases.length }));
    } catch (err) { setError(tauriMessageFrom(err)); }
    finally { setBusy(false); }
  };

  const connect = async (name: string, trustFingerprint?: string, attempt?: { password: string; passphrase: string }) => {
    if (connecting || busy) return;
    const epoch = ++connectEpoch.current;
    setConnecting(name); setError(""); setNotice("");
    try {
      const result = await connectTauriRemoteHost({ name, ...(trustFingerprint ? { trustFingerprint } : {}), ...(attempt?.password ? { password: attempt.password } : {}), ...(attempt?.passphrase ? { passphrase: attempt.passphrase } : {}) });
      if (!mounted.current || epoch !== connectEpoch.current) return;
      setConnectionStates(current => ({ ...current, [name]: result }));
      if (result.status === "connected") {
        setCredentialAttempts(current => Object.fromEntries(Object.entries(current).filter(([host]) => host !== name)));
        setNotice(t("settings.remote.connected", { host: result.host || name }));
      }
    } catch (err) { if (mounted.current && epoch === connectEpoch.current) setError(tauriMessageFrom(err)); }
    finally { if (mounted.current && epoch === connectEpoch.current) setConnecting(""); }
  };

  const disconnect = async (name: string) => {
    if (busy || !confirmDiscardRemoteDraft()) return;
    const epoch = ++connectEpoch.current;
    ++browseEpoch.current; ++previewEpoch.current; setBrowsingHost(""); setBrowseListing(null); setRemotePreview(null);
    setConnecting(name); setError(""); setNotice("");
    try {
      await disconnectTauriRemoteHost(name);
      if (!mounted.current || epoch !== connectEpoch.current) return;
      setConnectionStates(current => ({...current,[name]:{protocolVersion:1,status:"disconnected"}}));
      setCredentialAttempts(current => Object.fromEntries(Object.entries(current).filter(([host]) => host !== name)));
      setNotice(t("settings.remote.disconnected", { host: name }));
    } catch (err) { if (mounted.current && epoch === connectEpoch.current) setError(tauriMessageFrom(err)); }
    finally { if (mounted.current && epoch === connectEpoch.current) setConnecting(""); }
  };

  const loadRemoteDirectory = async (name: string, path?: string, discardConfirmed = false) => {
    if (!discardConfirmed && !confirmDiscardRemoteDraft()) return;
    const epoch = ++browseEpoch.current; ++previewEpoch.current; setRemotePreviewBusy(false);
    setBrowseBusy(true); setBrowseError(false); setRemotePreview(null); setRemotePreviewError(false);
    try { const next = await browseTauriRemoteHost(name,path); if (mounted.current && epoch === browseEpoch.current) setBrowseListing(next); }
    catch { if (mounted.current && epoch === browseEpoch.current) setBrowseError(true); }
    finally { if (mounted.current && epoch === browseEpoch.current) setBrowseBusy(false); }
  };

  const loadRemotePreview = async (name: string, path: string) => {
    if (!confirmDiscardRemoteDraft()) return;
    const epoch = ++previewEpoch.current;
    setRemotePreviewBusy(true); setRemotePreviewError(false);
    setRemoteSaveError(false);
    try {
      const result = await previewTauriRemoteFile(name, path);
      if (!mounted.current || epoch !== previewEpoch.current) return;
      setRemotePreview(result);
      setRemoteDraft(result.content);
    }
    catch { if (mounted.current && epoch === previewEpoch.current) {setRemotePreview(null);setRemotePreviewError(true);} }
    finally { if (mounted.current && epoch === previewEpoch.current) setRemotePreviewBusy(false); }
  };

  const saveRemoteDraft = async () => {
    if (!remotePreview || remotePreview.kind !== "text" || remotePreview.truncated || remoteDraft === remotePreview.content || remoteSaveBusy) return;
    const epoch = previewEpoch.current;
    setRemoteSaveBusy(true); setRemoteSaveError(false);
    const request: BridgeRemoteFileSaveRequest = { name: browsingHost, path: remotePreview.path, revision: remotePreview.revision, content: remoteDraft };
    try {
      const saved = await saveTauriRemoteFile(request);
      if (!mounted.current || epoch !== previewEpoch.current) return;
      setRemotePreview(current => current ? { ...current, path: saved.path, content: remoteDraft, revision: saved.revision } : current);
      setNotice(t("settings.remote.fileSaved"));
    } catch { if (mounted.current && epoch === previewEpoch.current) setRemoteSaveError(true); }
    finally { if (mounted.current) setRemoteSaveBusy(false); }
  };

  const beginBrowse = async (name: string, path?: string) => {
    if (!confirmDiscardRemoteDraft()) return;
    setBrowsingHost(name); setBrowseListing(null); setBrowseError(false); setRemotePreview(null); setRemotePreviewError(false);
    await loadRemoteDirectory(name, path, true);
  };

  const chooseRemoteDirectory = () => {
    if (!browseListing || !confirmDiscardRemoteDraft()) return;
    setDraft(current => ({ ...current, workspace: browseListing.path }));
    setBrowsingHost(""); setBrowseListing(null);
  };

  const beginEdit = (host?: TauriRemoteHostInput, keepName = false) => {
    setDraft(host ?? EMPTY_HOST);
    setFixedName(keepName);
    setEditing(true);
    setError("");
  };

  return <div className="tauri-settings-section">
    <h3>{t("settings.remote.title")}</h3>
    <p>{t("settings.remote.description")}</p>
    <p className="tauri-settings-muted">{t("settings.remote.connectionNote")}</p>
    {loading ? <div className="tauri-settings-loading">{t("common.loading")}</div> : <>
        {view && <>
        <p className="tauri-settings-muted">{t("settings.remote.configPath", { path: view.configPath })}</p>
        <div className="tauri-settings-actions">
          <button type="button" className="tauri-settings-button" disabled={busy || Boolean(connecting) || editing || Boolean(browsingHost)} onClick={reload}>{t("settings.bots.refresh")}</button>
          <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void scan()}>{t("settings.remote.scan")}</button>
          <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => beginEdit()}>{t("settings.remote.add")}</button>
        </div>
        {aliases.length > 0 && <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.remote.sshAliases")}</span><div className="tauri-settings-actions">{aliases.map(alias => <button key={alias} type="button" className="tauri-settings-button" disabled={busy} onClick={() => beginEdit({ ...EMPTY_HOST, name: alias, host: alias, useSSHConfig: true })}>{alias}</button>)}</div></div>}
        {view.hosts.length === 0 ? <p>{t("settings.remote.empty")}</p> : <div className="tauri-remote-host-list">{view.hosts.map(host => {
          const connection = connectionFor(host.name);
          const errorKey = connection?.status === "failed"
            ? REMOTE_ERROR_KEYS[connection.message as keyof typeof REMOTE_ERROR_KEYS] ?? REMOTE_ERROR_KEYS.connection_failed
            : REMOTE_ERROR_KEYS.connection_failed;
          return <section className="tauri-remote-host-card" key={host.name}>
          <div><strong>{host.name}</strong><small>{host.useSSHConfig ? t("settings.remote.fromSSHConfig") : `${host.user ? `${host.user}@` : ""}${host.host}${host.port ? `:${host.port}` : ""}`}</small>{host.workspace && <small>{host.workspace}</small>}
            {connection?.status === "connected" && <small role="status">{t("settings.remote.connectionVerified", { fingerprint: connection.fingerprint || "" })}</small>}
            {connection?.status === "host_key_confirmation" && <div className="tauri-remote-host-confirm" role="group" aria-label={t("settings.remote.hostKeyTitle")}><small>{t("settings.remote.hostKeyPrompt", { host: connection.host || host.name, keyType: connection.keyType || "SSH", address: connection.address || "" })}</small><code>{connection.fingerprint}</code><button type="button" className="tauri-settings-button" disabled={Boolean(connecting)} onClick={() => void connect(host.name, connection.fingerprint)}>{t("settings.remote.trustAndConnect")}</button></div>}
            {connection?.status === "failed" && <small role="alert">{t(errorKey)}</small>}
            {connection?.status === "failed" && connection.message === "authentication_failed" && <div className="tauri-remote-credential-prompt" role="group" aria-label={t("settings.remote.credentialsPromptTitle")}>
              <p>{t("settings.remote.credentialsPromptHint")}</p>
              <label htmlFor={`remote-password-${host.name}`}>{t("settings.remote.password")}<input id={`remote-password-${host.name}`} className="tauri-settings-input" type="password" autoComplete="new-password" value={credentialAttempts[host.name]?.password ?? ""} onChange={event => { const password = event.currentTarget.value; setCredentialAttempts(current => ({ ...current, [host.name]: { password, passphrase: current[host.name]?.passphrase ?? "" } })); }} /></label>
              <label htmlFor={`remote-passphrase-${host.name}`}>{t("settings.remote.passphrase")}<input id={`remote-passphrase-${host.name}`} className="tauri-settings-input" type="password" autoComplete="new-password" value={credentialAttempts[host.name]?.passphrase ?? ""} onChange={event => { const passphrase = event.currentTarget.value; setCredentialAttempts(current => ({ ...current, [host.name]: { password: current[host.name]?.password ?? "", passphrase } })); }} /></label>
              <button type="button" className="tauri-settings-button" disabled={Boolean(connecting)} onClick={() => void connect(host.name, undefined, credentialAttempts[host.name] ?? { password: "", passphrase: "" })}>{connecting === host.name ? t("settings.remote.connecting") : t("settings.remote.retryCredentials")}</button>
            </div>}
          </div>
          {connection?.status === "connected" && <TauriRemoteForwards key={host.name} name={host.name}/>}
          <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={Boolean(connecting) || busy} onClick={() => void connect(host.name)}>{connecting === host.name ? t("settings.remote.connecting") : t(connection?.status === "connected" ? "settings.remote.connectedAction" : "settings.remote.connect")}</button>{(connection?.status === "connected" || connecting === host.name || connection?.status === "reconnecting") && <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => void disconnect(host.name)}>{connecting === host.name ? t("common.cancel") : t("settings.remote.disconnect")}</button>}<button type="button" className="tauri-settings-button" disabled={busy || Boolean(connecting)} onClick={() => beginEdit(editableHost(host), true)}>{t("common.edit")}</button><button type="button" className="tauri-settings-button tauri-settings-button--danger" disabled={busy || Boolean(connecting)} onClick={() => setPendingRemove(host)}>{t("common.delete")}</button></div>
          </section>;
        })}</div>}
      </>}
      {editing && <section className="tauri-remote-host-editor">
        <h3>{t("settings.remote.editorTitle")}</h3>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="remote-host-name">{t("settings.remote.name")}</label><input id="remote-host-name" className="tauri-settings-input" maxLength={64} value={draft.name} disabled={busy || fixedName} onChange={event => setDraft({ ...draft, name: event.target.value })} /></div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="remote-host-address">{t("settings.remote.host")}</label><input id="remote-host-address" className="tauri-settings-input" maxLength={2048} value={draft.host} disabled={busy} onChange={event => setDraft({ ...draft, host: event.target.value })} /></div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="remote-host-user">{t("settings.remote.user")}</label><input id="remote-host-user" className="tauri-settings-input" maxLength={256} value={draft.user} disabled={busy} onChange={event => setDraft({ ...draft, user: event.target.value })} /></div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="remote-host-port">{t("settings.remote.port")}</label><input id="remote-host-port" className="tauri-settings-input" inputMode="numeric" value={draft.port || ""} disabled={busy} onChange={event => setDraft({ ...draft, port: Number(event.target.value) || 0 })} /></div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="remote-host-key">{t("settings.remote.identityFile")}</label><input id="remote-host-key" className="tauri-settings-input" maxLength={4096} value={draft.identityFile} disabled={busy} onChange={event => setDraft({ ...draft, identityFile: event.target.value })} /></div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="remote-host-jump">{t("settings.remote.proxyJump")}</label><input id="remote-host-jump" className="tauri-settings-input" maxLength={2048} value={draft.proxyJump} disabled={busy} onChange={event => setDraft({ ...draft, proxyJump: event.target.value })} /></div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="remote-host-workspace">{t("settings.remote.workspace")}</label><div className="tauri-remote-workspace-field"><input id="remote-host-workspace" className="tauri-settings-input" maxLength={4096} value={draft.workspace} disabled={busy} onChange={event => setDraft({ ...draft, workspace: event.target.value })} /><button type="button" className="tauri-settings-button" disabled={busy || connectionFor(draft.name)?.status !== "connected"} onClick={() => void beginBrowse(draft.name, draft.workspace || undefined)}>{t("settings.remote.browse")}</button></div>{connectionFor(draft.name)?.status !== "connected" && <small className="tauri-settings-muted">{t("settings.remote.connectFirst")}</small>}</div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="remote-host-install">{t("settings.remote.serveInstall")}</label><select id="remote-host-install" className="tauri-settings-input" value={draft.serveInstall} disabled={busy} onChange={event => setDraft({ ...draft, serveInstall: event.target.value })}><option value="auto">auto</option><option value="npm">npm</option><option value="upload">upload</option><option value="never">never</option></select></div>
        <div className="tauri-settings-field"><label className="tauri-settings-field-label" htmlFor="remote-host-credentials">{t("settings.remote.credentialMode")}</label><select id="remote-host-credentials" className="tauri-settings-input" value={draft.credentialMode} disabled={busy} onChange={event => setDraft({ ...draft, credentialMode: event.target.value })}><option value="remote">remote</option><option value="local-proxy">local-proxy</option></select></div>
        <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.remote.password")}<small>{view?.hosts.find(host => host.name === draft.name)?.passwordSet && draft.passwordAction === "keep" ? t("settings.remote.credentialSaved") : t("settings.remote.passwordHint")}</small></span><div className="tauri-network-secret"><input aria-label={t("settings.remote.password")} className="tauri-settings-input" type="password" autoComplete="new-password" maxLength={4096} value={draft.password ?? ""} disabled={busy} placeholder={t("settings.remote.secretPlaceholder")} onChange={event => setDraft({ ...draft, password: event.target.value, passwordAction: event.target.value ? "replace" : "keep" })} />{(view?.hosts.find(host => host.name === draft.name)?.passwordSet || draft.passwordAction === "replace") && <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setDraft({ ...draft, password: "", passwordAction: "clear" })}>{t("settings.remote.clearCredential")}</button>}</div></div>
        <div className="tauri-settings-field"><span className="tauri-settings-field-label">{t("settings.remote.passphrase")}<small>{view?.hosts.find(host => host.name === draft.name)?.passphraseSet && draft.passphraseAction === "keep" ? t("settings.remote.credentialSaved") : t("settings.remote.passphraseHint")}</small></span><div className="tauri-network-secret"><input aria-label={t("settings.remote.passphrase")} className="tauri-settings-input" type="password" autoComplete="new-password" maxLength={4096} value={draft.passphrase ?? ""} disabled={busy} placeholder={t("settings.remote.secretPlaceholder")} onChange={event => setDraft({ ...draft, passphrase: event.target.value, passphraseAction: event.target.value ? "replace" : "keep" })} />{(view?.hosts.find(host => host.name === draft.name)?.passphraseSet || draft.passphraseAction === "replace") && <button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setDraft({ ...draft, passphrase: "", passphraseAction: "clear" })}>{t("settings.remote.clearCredential")}</button>}</div></div>
        <label className="tauri-settings-checkbox"><input type="checkbox" checked={draft.useSSHConfig} disabled={busy} onChange={event => setDraft({ ...draft, useSSHConfig: event.target.checked })} />{t("settings.remote.useSSHConfig")}</label>
        <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => { setEditing(false); setDraft(EMPTY_HOST); }}>{t("common.cancel")}</button><button type="button" className="tauri-settings-button" disabled={busy || !draft.name.trim() || !draft.host.trim()} onClick={() => void save()}>{t("common.save")}</button></div>
      </section>}
      {pendingRemove && <div className="reasonix-confirm-backdrop" role="presentation"><section className="reasonix-confirm-dialog" role="alertdialog" aria-modal="true" aria-labelledby="remote-remove-title" aria-describedby="remote-remove-description">
        <h3 id="remote-remove-title">{t("settings.remote.removeConfirmTitle")}</h3>
        <p id="remote-remove-description">{t("settings.remote.removeConfirmMessage", { name: pendingRemove.name })}</p>
        <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={busy} onClick={() => setPendingRemove(null)}>{t("common.cancel")}</button><button type="button" className="tauri-settings-button tauri-settings-button--danger" disabled={busy} onClick={() => { void remove(pendingRemove.name).then(removed => { if (removed) setPendingRemove(null); }); }}>{t("common.delete")}</button></div>
      </section></div>}
      {browsingHost && <div className="reasonix-confirm-backdrop" role="presentation"><section className="reasonix-confirm-dialog tauri-remote-browser" role="dialog" aria-modal="true" aria-labelledby="remote-browser-title">
        <h3 id="remote-browser-title">{t("settings.remote.browserTitle", { host: browsingHost })}</h3>
        {browseListing && <><div className="tauri-remote-browser-toolbar"><code title={browseListing.path}>{browseListing.path}</code><div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={browseBusy || browseListing.parentPath === browseListing.path} onClick={() => void loadRemoteDirectory(browsingHost, browseListing.parentPath)}>{t("settings.remote.parentDirectory")}</button><button type="button" className="tauri-settings-button" disabled={browseBusy} onClick={chooseRemoteDirectory}>{t("settings.remote.selectDirectory")}</button></div></div>
          {browseListing.truncated && <p className="tauri-settings-muted">{t("settings.remote.listTruncated")}</p>}
          <div className="tauri-remote-browser-list" aria-busy={browseBusy}>{browseListing.entries.map(entry => entry.isDir ? <button key={entry.path} type="button" className="tauri-remote-browser-entry" disabled={browseBusy} onClick={() => void loadRemoteDirectory(browsingHost, entry.path)}><span>▸ {entry.name}</span><small>{entry.symlink ? t("settings.remote.symlink") : t("settings.remote.directory")}</small></button> : <button key={entry.path} type="button" className="tauri-remote-browser-entry tauri-remote-browser-entry--file" disabled={browseBusy || remotePreviewBusy} onClick={() => void loadRemotePreview(browsingHost, entry.path)}><span>{entry.name}</span><small>{t("settings.remote.fileSize", { size: entry.size })}</small></button>)}</div>
          {remotePreviewBusy && <p role="status">{t("settings.remote.previewLoading")}</p>}
          {remotePreviewError && <p className="tauri-diagnostic-error" role="alert">{t("settings.remote.previewFailed")}</p>}
          {remotePreview && <section className="tauri-remote-file-preview" aria-label={t("settings.remote.previewTitle")}><header><code title={remotePreview.path}>{remotePreview.path}</code><span>{remotePreview.kind === "binary" ? t("settings.remote.binaryFile") : t("settings.remote.textFile")}</span></header>{remotePreview.kind === "binary" ? <p>{t("settings.remote.binaryPreviewUnsupported")}</p> : <textarea aria-label={t("settings.remote.previewTitle")} value={remoteDraft} readOnly={remotePreview.truncated} onChange={event => setRemoteDraft(event.currentTarget.value)} />}{remotePreview.truncated && <p>{t("settings.remote.previewTruncated")}</p>}{remoteSaveError && <p className="tauri-diagnostic-error" role="alert">{t("settings.remote.saveFailed")}</p>}{remotePreview.kind === "text" && !remotePreview.truncated && <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={remoteSaveBusy || remoteDraft === remotePreview.content} onClick={() => void saveRemoteDraft()}>{remoteSaveBusy ? t("settings.remote.savingFile") : t("settings.remote.saveFile")}</button></div>}</section>}
          {browseListing.entries.length === 0 && <p className="tauri-settings-muted">{t("settings.remote.emptyDirectory")}</p>}</>}
        {browseBusy && <p role="status">{t("settings.remote.browsing")}</p>}
        {browseError && <p className="tauri-diagnostic-error" role="alert">{t("settings.remote.browseFailed")}</p>}
        <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" onClick={() => { if (!confirmDiscardRemoteDraft()) return; ++browseEpoch.current; ++previewEpoch.current; setBrowsingHost(""); setBrowseListing(null); setRemotePreview(null); }}>{t("common.cancel")}</button></div>
      </section></div>}
    </>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
  </div>;
}
