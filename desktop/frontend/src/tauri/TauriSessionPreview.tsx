import { useEffect, useState } from "react";
import type { UnlistenFn } from "@tauri-apps/api/event";
import {
  cancelTauriBridge,
  onTauriBridgeConnectionError,
  onTauriBridgeEvent,
  openTauriBridgeSession,
  restartTauriBridge,
  startTauriBridgeEvents,
  submitTauriBridge,
  tauriBridgeSnapshot,
  tauriBridgeStatus,
  type TauriBridgeEvent,
  type TauriBridgeSession,
  type TauriBridgeStatus,
} from "../lib/tauriBridge";
import "./tauriSessionPreview.css";

function newSessionId(): string {
  return `tauri-${crypto.randomUUID()}`;
}

function messageFrom(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function eventSummary(event: TauriBridgeEvent): string {
  const payload = JSON.stringify(event.payload);
  return payload.length > 500 ? `${payload.slice(0, 497)}...` : payload;
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
  const [session, setSession] = useState<TauriBridgeSession | null>(null);
  const [events, setEvents] = useState<TauriBridgeEvent[]>([]);
  const [sequence, setSequence] = useState(0);
  const [streamRevision, setStreamRevision] = useState(0);
  const [streamReady, setStreamReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    void tauriBridgeStatus().then(setStatus).catch(error => setError(messageFrom(error)));
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
        });
        offError = await onTauriBridgeConnectionError(message => {
          if (active) {
            setStreamReady(false);
            setStatus({ running: false });
            setError(`Bridge event stream: ${message}`);
          }
        });
        await startTauriBridgeEvents(snapshot.sequence);
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

  async function openSession() {
    const id = sessionId.trim();
    if (!id) {
      setError("Session ID is required");
      return;
    }
    setBusy(true);
    setError("");
    setEvents([]);
    setSequence(0);
    setStreamReady(false);
    try {
      setSession(await openTauriBridgeSession(id, workspaceRoot.trim() || undefined));
      setStreamRevision(previous => previous + 1);
      setStatus(await tauriBridgeStatus());
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
      setEvents([]);
      setSequence(0);
      setStreamRevision(previous => previous + 1);
    } catch (error) {
      setStatus({ running: false });
      setError(messageFrom(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="tauri-preview">
      <section className="tauri-preview__card">
        <p className="tauri-preview__eyebrow">Reasonix Stable · Tauri Preview</p>
        <h1>Bridge session</h1>
        <p className="tauri-preview__intro">This preview uses the Rust host and Go bridge directly. The established Wails interface remains available outside Tauri while its feature families are migrated.</p>
        <p className={`tauri-preview__status ${status?.running ? "is-ready" : ""}`}>
          {status?.running ? `Bridge ready · protocol v${status.protocolVersion ?? "?"}` : "Connecting to bridge…"}
        </p>
        <button className="tauri-preview__restart" type="button" onClick={() => void restartBridge()} disabled={busy}>Restart bridge{session ? " and recover session" : ""}</button>

        <label>
          Session ID
          <div className="tauri-preview__inline">
            <input value={sessionId} onChange={event => setSessionId(event.target.value)} disabled={busy} />
            <button type="button" onClick={() => setSessionId(newSessionId())} disabled={busy}>New ID</button>
          </div>
        </label>
        <label>
          Workspace root <span>(optional)</span>
          <input value={workspaceRoot} onChange={event => setWorkspaceRoot(event.target.value)} placeholder="/path/to/workspace" disabled={busy} />
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
        </section>}

        {error && <p className="tauri-preview__error" role="alert">{error} {session && "Restart the bridge to recover this session."}</p>}
      </section>
      <section className="tauri-preview__events" aria-live="polite">
        <h2>Bridge events</h2>
        {events.length === 0 ? <p>Open a session to begin streaming events.</p> : <ol>{events.map(event => <li key={event.sequence}><b>#{event.sequence} · {event.eventKind}</b><pre>{eventSummary(event)}</pre></li>)}</ol>}
      </section>
    </main>
  );
}
