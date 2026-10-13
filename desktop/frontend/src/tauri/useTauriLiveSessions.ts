import { useCallback, useEffect, useRef, useState } from "react";
import type { UnlistenFn } from "@tauri-apps/api/event";
import { onTauriBridgeConnectionRestored, onTauriBridgeEvent, tauriLiveSessions, type TauriBridgeSession } from "../lib/tauriBridge";

const activityEvents = new Set(["turn_started", "turn_done", "approval_request", "ask_request", "mcp_interaction", "prompt_answered"]);

/** One activity subscription; token events do not rerender the sidebar. */
export function useTauriLiveSessions(enabled: boolean, instanceId?: string) {
  const [sessions, setSessions] = useState<Record<string, TauriBridgeSession>>({});
  const generation = useRef(0);
  const inFlight = useRef(false);
  const dirty = useRef(false);
  const busyOwners = useRef(false);
  const refreshRef = useRef<() => Promise<void>>(async () => {});
  const refresh = useCallback(() => refreshRef.current(), []);

  useEffect(() => {
    const owner = ++generation.current;
    let active = true;
    let off: UnlistenFn | undefined;
    let offRestored: UnlistenFn | undefined;
    inFlight.current = false;
    dirty.current = false;
    busyOwners.current = false;
    setSessions({});
    if (!enabled) return;
    const read = async () => {
      if (!active) return;
      if (inFlight.current) { dirty.current = true; return; }
      inFlight.current = true;
      try {
        const inventory = await tauriLiveSessions();
        if (!active || generation.current !== owner) return;
        const next = Object.fromEntries(inventory.sessions.map(session => [session.id, session]));
        busyOwners.current = inventory.sessions.some(session => session.state !== "idle");
        setSessions(previous => JSON.stringify(previous) === JSON.stringify(next) ? previous : next);
      } catch {
        // Preserve observed busy owners on an outage; never invent idle state.
      } finally {
        if (active && generation.current === owner) {
          inFlight.current = false;
          if (dirty.current) { dirty.current = false; void read(); }
        }
      }
    };
    refreshRef.current = read;
    void onTauriBridgeEvent(event => {
      if (!active || !activityEvents.has(event.eventKind)) return;
      busyOwners.current = true;
      void read();
    }).then(unlisten => { if (active) off = unlisten; else unlisten(); });
    void onTauriBridgeConnectionRestored(() => { if (active) void read(); }).then(unlisten => { if (active) offRestored = unlisten; else unlisten(); });
    void read();
    const timer = setInterval(() => { if (busyOwners.current) void read(); }, 2000);
    return () => { active = false; off?.(); offRestored?.(); clearInterval(timer); refreshRef.current = async () => {}; };
  }, [enabled, instanceId]);
  return { sessions, refresh };
}
