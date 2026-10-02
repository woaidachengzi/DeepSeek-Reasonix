import { useEffect, useLayoutEffect, useRef } from "react";
import {
  acknowledgeTauriNotificationClick, onTauriNotificationClick, pendingTauriNotificationClicks,
  resolveTauriNotificationClick, type TauriNotificationClick, type TauriNotificationTarget,
} from "../lib/tauriBridge";

export interface NotificationClickDependencies {
  pending(): Promise<TauriNotificationClick[]>;
  resolve(token: string): Promise<TauriNotificationTarget | null>;
  acknowledge(token: string): Promise<void>;
  canOpen(click: TauriNotificationClick): boolean;
  open(target: TauriNotificationTarget): Promise<boolean>;
  unavailable(): void;
  failed(): void;
}

/** A single consumer. Native clicks stay queued until navigation is accepted. */
export class NotificationClickPump {
  private stopped = false;
  private draining = false;
  private wakeAgain = false;
  constructor(private readonly dependencies: NotificationClickDependencies) {}
  stop(): void { this.stopped = true; }
  async wake(): Promise<void> {
    if (this.stopped) return;
    if (this.draining) { this.wakeAgain = true; return; }
    this.draining = true;
    let failed = false;
    try {
      // Native bounds the queue to 32; take a fresh head after each ack so
      // signals arriving during navigation cannot restore stale targets.
      for (let attempt = 0; attempt < 32 && !this.stopped; attempt += 1) {
        this.wakeAgain = false;
        const [click] = await this.dependencies.pending();
        if (this.stopped || !click || !this.dependencies.canOpen(click)) return;
        const target = await this.dependencies.resolve(click.token);
        if (this.stopped) return;
        if (target) {
          if (target.sessionId !== click.sessionId) throw new Error("notification target mismatch");
          if (!this.dependencies.canOpen(click) || !await this.dependencies.open(target) || this.stopped) return;
        } else this.dependencies.unavailable();
        await this.dependencies.acknowledge(click.token);
        if (this.stopped) return;
      }
    } catch {
      if (!this.stopped) this.dependencies.failed();
      failed = true; // A bridge/storage failure needs a later real wake.
    } finally {
      this.draining = false;
      // A signal received during an empty snapshot or a readiness check still
      // owns a fresh query, even when this drain acknowledged nothing.
      if (!this.stopped && !failed && this.wakeAgain) void this.wake();
    }
  }
}

export function useTauriNotificationClicks(options: Pick<NotificationClickDependencies, "canOpen" | "open" | "unavailable" | "failed">, readiness: { busy: boolean; blocked: boolean; source: string; sessionId?: string }): void {
  const committedOptions = useRef(options);
  const pump = useRef<NotificationClickPump | null>(null);
  useLayoutEffect(() => { committedOptions.current = options; });
  useEffect(() => {
    let active = true;
    let unlisten: (() => void) | undefined;
    const consumer = new NotificationClickPump({
      pending: pendingTauriNotificationClicks, resolve: resolveTauriNotificationClick, acknowledge: acknowledgeTauriNotificationClick,
      canOpen: click => committedOptions.current.canOpen(click),
      open: target => committedOptions.current.open(target),
      unavailable: () => committedOptions.current.unavailable(), failed: () => committedOptions.current.failed(),
    });
    void onTauriNotificationClick(() => { if (active) void consumer.wake(); }).then(off => {
      if (!active) { off(); return; }
      unlisten = off;
      pump.current = consumer;
      // Register first, then query. A cold-start callback before hydration
      // remains in native state instead of being lost as an early JS event.
      void consumer.wake();
    }).catch(() => { if (active) committedOptions.current.failed(); });
    return () => { active = false; consumer.stop(); unlisten?.(); if (pump.current === consumer) pump.current = null; };
  }, []);
  useEffect(() => { void pump.current?.wake(); }, [readiness.busy, readiness.blocked, readiness.source, readiness.sessionId]); // Readiness can unblock a queued click.
}
