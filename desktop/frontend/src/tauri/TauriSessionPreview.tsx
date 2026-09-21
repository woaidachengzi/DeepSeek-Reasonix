import { useEffect, useState } from "react";
import type { UnlistenFn } from "@tauri-apps/api/event";
import {
  cancelTauriBridge,
  chooseTauriWorkspaceRoot,
  importTauriStableProfile,
  newTauriSessionId,
  onTauriBridgeConnectionError,
  onTauriBridgeEvent,
  openTauriBridgeSession,
  rememberTauriWorkbenchSession,
  restartTauriBridge,
  setTauriDefaultModel,
  startTauriBridgeEvents,
  submitTauriBridge,
  switchTauriBridgeSession,
  tauriBridgeHistory,
  tauriBridgeSnapshot,
  tauriBridgeStatus,
  tauriEventSummary,
  tauriMessageFrom,
  tauriProviderSummary,
  tauriPreviewProfileStatus,
  tauriPreviewRuntimeInfo,
  tauriWorkbenchSessions,
  type TauriBridgeEvent,
  type TauriBridgeHistory,
  type TauriBridgeSession,
  type TauriBridgeStatus,
  type TauriPreviewProfileStatus,
  type TauriPreviewRuntimeInfo,
  type TauriProviderSummary,
} from "../lib/tauriBridge";
import "./tauriSessionPreview.css";

function newSessionId(): string {
  return newTauriSessionId();
}

function messageFrom(error: unknown): string {
  return tauriMessageFrom(error);
}

function eventSummary(event: TauriBridgeEvent): string {
  return tauriEventSummary(event);
}

interface WorkbenchSessionTab {
  sessionId: string;
  workspaceRoot?: string;
}

function workbenchSessionLabel(tab: WorkbenchSessionTab): string {
  return tab.sessionId.length > 28 ? `${tab.sessionId.slice(0, 25)}…` : tab.sessionId;
}

/**
 * A deliberately narrow, native-host-only vertical slice. It makes the bridge
 * executable and observable while the large Wails surface is migrated in
 * feature families, rather than letting unported calls silently fall back to
 * browser mocks.
 */
export function TauriSessionPreview() {
  const [sessionId, setSessionId] = useState(newSessionId);
  const [workspaceRoot, setWorkspaceRoot] = useState("");
  const [prompt, setPrompt] = useState("");
  const [status, setStatus] = useState<TauriBridgeStatus | null>(null);
  const [profile, setProfile] = useState<TauriPreviewProfileStatus | null>(null);
  const [runtimeInfo, setRuntimeInfo] = useState<TauriPreviewRuntimeInfo | null>(null);
  const [providerSummary, setProviderSummary] = useState<TauriProviderSummary | null>(null);
  const [profileNotice, setProfileNotice] = useState("");
  const [session, setSession] = useState<TauriBridgeSession | null>(null);
  const [tabs, setTabs] = useState<WorkbenchSessionTab[]>([]);
  const [events, setEvents] = useState<TauriBridgeEvent[]>([]);
  const [history, setHistory] = useState<TauriBridgeHistory | null>(null);
  const [sequence, setSequence] = useState(0);
  const [streamRevision, setStreamRevision] = useState(0);
  const [streamReady, setStreamReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const switchingBlocked = Boolean(session && session.state !== "idle");

  useEffect(() => {
    void tauriBridgeStatus().then(setStatus).catch(error => setError(messageFrom(error)));
    void tauriPreviewProfileStatus().then(setProfile).catch(error => setError(messageFrom(error)));
    void tauriPreviewRuntimeInfo().then(setRuntimeInfo).catch(error => setError(messageFrom(error)));
    void tauriProviderSummary().then(setProviderSummary).catch(error => setError(messageFrom(error)));
    void tauriWorkbenchSessions()
      .then(sessions => setTabs(previous => previous.length === 0 ? sessions : previous))
      .catch(error => setError(messageFrom(error)));
  }, []);

  useEffect(() => {
    if (!session) return;
    let active = true;
    let offEvent: UnlistenFn | undefined;
    let offError: UnlistenFn | undefined;

    void (async () => {
      try {
        const snapshot = await tauriBridgeSnapshot(session.id);
        if (!active) return;
        setSession(snapshot.session);
        setSequence(snapshot.sequence);
        offEvent = await onTauriBridgeEvent(event => {
          if (event.sessionId !== session.id) return;
          setSequence(previous => Math.max(previous, event.sequence));
          setEvents(previous => [event, ...previous].slice(0, 100));
          if (event.eventKind === "turn_done") {
            void tauriBridgeSnapshot(event.sessionId)
              .then(snapshot => {
                if (active) setSession(snapshot.session);
              })
              .catch(error => {
                if (active) setError(messageFrom(error));
              });
          }
        });
        offError = await onTauriBridgeConnectionError(message => {
          if (active) {
            setStreamReady(false);
            setStatus({ running: false });
            setError(`Bridge event stream: ${message}`);
          }
        });
        await startTauriBridgeEvents(snapshot.sequence);
        const history = await tauriBridgeHistory(session.id);
        if (!active) return;
        setHistory(history);
        if (active) {
          setError("");
          setStreamReady(true);
        }
      } catch (error) {
        if (active) {
          setStreamReady(false);
          setError(messageFrom(error));
        }
      }
    })();

    return () => {
      active = false;
      offEvent?.();
      offError?.();
    };
  }, [session?.id, streamRevision]);

  async function rememberSession(next: TauriBridgeSession) {
    try {
      setTabs(await rememberTauriWorkbenchSession(next.id, next.workspaceRoot ?? undefined));
    } catch (error) {
      setError(`Session opened, but it could not be saved to Recent conversations: ${messageFrom(error)}`);
    }
  }

  async function activateSession(id: string, root?: string) {
    if (!id) {
      setError("Session ID is required");
      return;
    }
    setBusy(true);
    setError("");
    setEvents([]);
    setHistory(null);
    setSequence(0);
    setStreamReady(false);
    try {
      // The bridge owns one Controller in this migration stage. Moving between
      // tabs is therefore an explicit, durable handoff; the bridge refuses to
      // replace a running or paused turn.
      const next = session && session.id !== id
        ? await switchTauriBridgeSession(id, root)
        : await openTauriBridgeSession(id, root);
      setSession(next);
      setSessionId(next.id);
      setWorkspaceRoot(next.workspaceRoot ?? "");
      await rememberSession(next);
      setStreamRevision(previous => previous + 1);
      setStatus(await tauriBridgeStatus());
    } catch (error) {
      setError(messageFrom(error));
    } finally {
      setBusy(false);
    }
  }

  async function openSession() {
    const id = sessionId.trim();
    await activateSession(id, workspaceRoot.trim() || undefined);
  }

  async function createSession() {
    const id = newSessionId();
    setSessionId(id);
    setWorkspaceRoot("");
    await activateSession(id);
  }

  async function chooseWorkspaceRoot() {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const selected = await chooseTauriWorkspaceRoot();
      if (selected) setWorkspaceRoot(selected);
    } catch (error) {
      setError(messageFrom(error));
    } finally {
      setBusy(false);
    }
  }

  async function submit() {
    if (!session || !streamReady || !prompt.trim()) return;
    setBusy(true);
    setError("");
    try {
      setSession(await submitTauriBridge(session.id, prompt.trim()));
      setPrompt("");
    } catch (error) {
      setError(messageFrom(error));
    } finally {
      setBusy(false);
    }
  }

  async function cancel() {
    if (!session) return;
    setBusy(true);
    setError("");
    try {
      setSession(await cancelTauriBridge(session.id));
    } catch (error) {
      setError(messageFrom(error));
    } finally {
      setBusy(false);
    }
  }

  async function restartBridge() {
    setBusy(true);
    setError("");
    setStreamReady(false);
    try {
      const restarted = await restartTauriBridge();
      setStatus(restarted);
      if (!session) return;
      // A restarted sidecar deliberately owns no inherited in-memory runtime.
      // Re-open the same session, then let the effect obtain its authoritative
      // snapshot before event forwarding or another submit becomes available.
      const reopened = await openTauriBridgeSession(session.id, session.workspaceRoot);
      setSession(reopened);
      await rememberSession(reopened);
      setEvents([]);
      setHistory(null);
      setSequence(0);
      setStreamRevision(previous => previous + 1);
    } catch (error) {
      setStatus({ running: false });
      setError(messageFrom(error));
    } finally {
      setBusy(false);
    }
  }

  async function importStableProfile() {
    if (!profile?.importAvailable || busy) return;
    const source = profile.stableConfig ?? "the stable Reasonix config";
    const confirmed = window.confirm(
      `Copy ${source} into this isolated Tauri Preview profile?\n\nA timestamped backup will be created first. Existing Preview config is never overwritten. Sessions, caches, plugins, and .env files are not imported.`,
    );
    if (!confirmed) return;
    setBusy(true);
    setError("");
    setProfileNotice("");
    try {
      const result = await importTauriStableProfile();
      setProfileNotice(`Imported config: ${result.importedConfig}\nBackup: ${result.backupConfig}`);
      setProfile(await tauriPreviewProfileStatus());
      setProviderSummary(await tauriProviderSummary());
    } catch (error) {
      setError(messageFrom(error));
    } finally {
      setBusy(false);
    }
  }

  async function refreshHistory() {
    if (!session || busy) return;
    setBusy(true);
    setError("");
    try {
      setHistory(await tauriBridgeHistory(session.id));
    } catch (error) {
      setError(messageFrom(error));
    } finally {
      setBusy(false);
    }
  }

  async function refreshProviderSummary() {
    setError("");
    try {
      setProviderSummary(await tauriProviderSummary());
    } catch (error) {
      setError(messageFrom(error));
    }
  }

  async function changeDefaultModel(model: string) {
    if (!model || busy) return;
    setBusy(true);
    setError("");
    try {
      setProviderSummary(await setTauriDefaultModel(model));
    } catch (error) {
      setError(messageFrom(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="tauri-workbench">
      <aside className="tauri-workbench__sidebar" aria-label="Session navigation">
        <div className="tauri-workbench__brand">
          <span>Reasonix</span>
          <small>Tauri Preview</small>
        </div>
        <button className="tauri-workbench__new" type="button" onClick={() => void createSession()} disabled={busy || switchingBlocked}>New conversation</button>
        <p className="tauri-workbench__heading">Recent conversations</p>
        <nav className="tauri-workbench__tabs">
          {tabs.length === 0 ? <p>No recent conversation yet.</p> : tabs.map(tab => <button
            key={tab.sessionId}
            type="button"
            className={session?.id === tab.sessionId ? "is-active" : ""}
            disabled={busy || session?.id === tab.sessionId || switchingBlocked}
            onClick={() => void activateSession(tab.sessionId, tab.workspaceRoot)}
            title={tab.workspaceRoot ? `${tab.sessionId}\n${tab.workspaceRoot}` : tab.sessionId}
          >
            <b>{workbenchSessionLabel(tab)}</b>
            <small>{tab.workspaceRoot || "Private preview profile"}</small>
          </button>)}
        </nav>
        <p className="tauri-workbench__notice">Switching is enabled only after the active turn is idle, so an in-flight response is never replaced.</p>
      </aside>
      <section className="tauri-preview">
      <section className="tauri-preview__card">
        <p className="tauri-preview__eyebrow">Reasonix Stable · Tauri Preview</p>
        <h1>Bridge session</h1>
        <p className="tauri-preview__intro">This preview uses the Rust host and Go bridge directly. The established Wails interface remains available outside Tauri while its feature families are migrated.</p>
        <p className={`tauri-preview__status ${status?.running ? "is-ready" : ""}`}>
          {status?.running ? `Bridge ready · protocol v${status.protocolVersion ?? "?"}` : "Connecting to bridge…"}
        </p>
        <button className="tauri-preview__restart" type="button" onClick={() => void restartBridge()} disabled={busy}>Restart bridge{session ? " and recover session" : ""}</button>

        <details className="tauri-preview__runtime">
          <summary>Runtime details</summary>
          {!runtimeInfo ? <p>Loading build and bridge information…</p> : <dl>
            <div><dt>Stable baseline</dt><dd>v{runtimeInfo.stableVersion} · {runtimeInfo.stableCommit.slice(0, 12)}</dd></div>
            <div><dt>Preview / Tauri</dt><dd>v{runtimeInfo.previewVersion} · v{runtimeInfo.tauriVersion}</dd></div>
            <div><dt>Bridge protocol</dt><dd>v{runtimeInfo.bridgeProtocolVersion}</dd></div>
            <div><dt>Live sidecar</dt><dd>{runtimeInfo.sidecarInstanceId ?? "not running"}</dd></div>
          </dl>}
        </details>

        <section className="tauri-preview__profile">
          <h2>Private preview profile</h2>
          {profile ? <>
            <p>This preview keeps its own config and sessions at:</p>
            <code>{profile.previewHome}</code>
            {profile.importAvailable ? <>
              <p>A stable config is available to copy once. The stable install is never changed.</p>
              <code>{profile.stableConfig}</code>
              <button type="button" onClick={() => void importStableProfile()} disabled={busy}>Copy stable config with backup</button>
            </> : <p>{profile.previewConfigExists ? "This Preview already has its own config; importing never overwrites it." : profile.managedProfile ? "No stable config was found at the default location." : "An explicit REASONIX_HOME was supplied, so automatic import is disabled."}</p>}
          </> : <p>Checking isolated profile…</p>}
          {profileNotice && <p className="tauri-preview__notice">{profileNotice}</p>}
        </section>

        <section className="tauri-preview__profile">
          <div className="tauri-preview__history-heading"><h2>Provider status</h2><button type="button" onClick={() => void refreshProviderSummary()} disabled={busy}>Refresh</button></div>
          {!providerSummary ? <p>Checking the private Preview configuration…</p> : <>
            <label>
              New-session default model
              <select
                value={providerSummary.defaultModel}
                onChange={event => void changeDefaultModel(event.target.value)}
                disabled={busy || !providerSummary.providers.some(provider => provider.configured && provider.models.length > 0)}
              >
                {!providerSummary.defaultModel && <option value="" disabled>Choose a default model…</option>}
                {providerSummary.defaultModel && <option value={providerSummary.defaultModel}>{providerSummary.defaultModel} · current</option>}
                {providerSummary.providers.filter(provider => provider.configured).flatMap(provider => provider.models.map(model => {
                  const ref = `${provider.name}/${model}`;
                  return ref === providerSummary.defaultModel ? null : <option key={ref} value={ref}>{provider.displayName || provider.name} / {model}</option>;
                }))}
              </select>
            </label>
            <p>Changing this only affects new conversations; existing conversations keep their selected model. A workspace&apos;s <code>reasonix.toml</code> may override the user default.</p>
            {providerSummary.providers.length === 0 ? <p>No providers are configured in the Preview profile.</p> : <ul>
              {providerSummary.providers.map(provider => <li key={provider.name}>
                <strong>{provider.displayName || provider.name}</strong>
                <span> · {provider.kind} · {provider.modelCount} model{provider.modelCount === 1 ? "" : "s"} · {provider.configured ? "Configured" : "API key missing"}</span>
              </li>)}
            </ul>}
            <p>This summary never sends API keys, credential variable names, or provider endpoints to the WebView.</p>
          </>}
        </section>

        <label>
          Session ID
          <div className="tauri-preview__inline">
            <input value={sessionId} onChange={event => setSessionId(event.target.value)} disabled={busy} />
            <button type="button" onClick={() => setSessionId(newSessionId())} disabled={busy}>New ID</button>
          </div>
        </label>
        <label>
          Workspace root <span>(optional)</span>
          <div className="tauri-preview__inline">
            <input value={workspaceRoot} onChange={event => setWorkspaceRoot(event.target.value)} placeholder="/path/to/workspace" disabled={busy} />
            <button type="button" onClick={() => void chooseWorkspaceRoot()} disabled={busy}>Choose…</button>
          </div>
        </label>
        <button className="tauri-preview__primary" type="button" onClick={() => void openSession()} disabled={busy}>Open session</button>

        {session && <section className="tauri-preview__session" aria-live="polite">
          <div><strong>{session.id}</strong><span>{session.state}</span></div>
          <code>{session.path}</code>
          <textarea value={prompt} onChange={event => setPrompt(event.target.value)} placeholder="Send a prompt to the bridge-owned session" disabled={busy} rows={5} />
          <div className="tauri-preview__actions">
            <button className="tauri-preview__primary" type="button" onClick={() => void submit()} disabled={busy || !streamReady || !prompt.trim()}>Send</button>
            <button type="button" onClick={() => void cancel()} disabled={busy || session.state !== "running"}>Cancel active turn</button>
          </div>
          <p className="tauri-preview__sequence">Event cursor: {sequence}</p>
          <section className="tauri-preview__history" aria-live="polite">
            <div className="tauri-preview__history-heading"><h2>Conversation preview</h2><button type="button" onClick={() => void refreshHistory()} disabled={busy}>Refresh</button></div>
            {!history ? <p>Loading display-safe conversation history…</p> : <>
              {history.startIndex > 0 && <p>Showing the latest {history.messages.length} of {history.totalMessages} visible messages.</p>}
              {history.messages.length === 0 ? <p>No user or assistant messages have been saved yet.</p> : <ol>{history.messages.map((message, index) => <li key={`${history.startIndex + index}-${message.role}`} className={`is-${message.role}`}><b>{message.role}</b><pre>{message.content}</pre>{message.truncated && <small>Preview truncated this message.</small>}</li>)}</ol>}
            </>}
          </section>
        </section>}

        {error && <p className="tauri-preview__error" role="alert">{error} {session && "Restart the bridge to recover this session."}</p>}
      </section>
      <section className="tauri-preview__events" aria-live="polite">
        <h2>Bridge events</h2>
        {events.length === 0 ? <p>Open a session to begin streaming events.</p> : <ol>{events.map(event => <li key={event.sequence}><b>#{event.sequence} · {event.eventKind}</b><pre>{eventSummary(event)}</pre></li>)}</ol>}
      </section>
      </section>
    </main>
  );
}
