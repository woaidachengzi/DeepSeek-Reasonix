import { useEffect, useState } from "react";
import { Activity, Bot, Coins, FolderOpen, MessageSquare } from "lucide-react";
import { useTauriStatusBarPreferences, type TauriStatusBarItemId } from "./tauriStatusBarPreferences";
import type { TauriObservedUsage } from "./tauriObservedUsage";
import type { TauriSessionMetrics } from "../lib/tauriBridge";
import { tauriSessionBalance, type TauriSessionBalance } from "../lib/tauriBridge";
import { useI18n, useT } from "../lib/i18n";
import { formatMoneyLocalized } from "../lib/money";

interface TauriStatusBarProps {
  workspace: string;
  sessionId?: string;
  model?: string;
  sessionState?: "idle" | "running" | "paused";
  bridgeRunning?: boolean;
  observedUsage?: TauriObservedUsage | null;
  sessionMetrics?: TauriSessionMetrics | null;
}

export function TauriStatusBar({ workspace, sessionId, model, sessionState, bridgeRunning, observedUsage, sessionMetrics }: TauriStatusBarProps) {
  const { style, items } = useTauriStatusBarPreferences();
  const { locale } = useI18n();
  const t = useT();
  const [balanceState, setBalanceState] = useState<{ sessionId: string; balance: TauriSessionBalance } | null>(null);
  const showBalance = items.includes("balance");
  useEffect(() => {
    let active = true;
    setBalanceState(null);
    if (!showBalance || !sessionId) return () => { active = false; };
    const refresh = () => {
      void tauriSessionBalance(sessionId).then(response => {
        if (active && response.balance?.display) setBalanceState({ sessionId, balance: response.balance });
        else if (active) setBalanceState(null);
      }).catch(() => { if (active) setBalanceState(null); });
    };
    refresh();
    const timer = window.setInterval(refresh, 5 * 60 * 1000);
    return () => { active = false; window.clearInterval(timer); };
  }, [sessionId, showBalance, model]);
  const sessionLabel = sessionState === "running" ? t("settings.statusBarItem.state.running") : sessionState === "paused" ? t("settings.statusBarItem.state.paused") : sessionState === "idle" ? t("settings.statusBarItem.state.idle") : "";
  const sessionBalance = balanceState && balanceState.sessionId === sessionId ? balanceState.balance : null;
  const context = sessionMetrics && sessionMetrics.contextWindowTokens > 0 ? sessionMetrics : null;
  const cache = sessionMetrics && sessionMetrics.cacheHitTokens + sessionMetrics.cacheMissTokens > 0 ? sessionMetrics : null;
  const entries: Record<TauriStatusBarItemId, { label: string; value: string; icon: typeof Activity; detail?: string } | null> = {
    workspace: workspace ? { label: t("settings.statusBarItem.workspace"), value: workspace.split(/[\\/]/).filter(Boolean).pop() || workspace, icon: FolderOpen } : null,
    model: model ? { label: t("settings.statusBarItem.model"), value: model, icon: Bot } : null,
    balance: sessionBalance ? { label: t("settings.statusBarItem.balance"), value: sessionBalance.display, icon: Coins } : null,
    session: sessionLabel ? { label: t("settings.statusBarItem.session"), value: sessionLabel, icon: MessageSquare } : null,
    observed_tokens: observedUsage ? { label: t("settings.statusBarItem.observedTokens"), value: observedUsage.tokens.toLocaleString(), icon: Activity } : null,
    turn_tokens: observedUsage ? { label: t("settings.statusBarItem.turnTokens"), value: observedUsage.turnTokens.toLocaleString(), icon: Activity } : null,
    turn_output_tokens: observedUsage && observedUsage.turnOutputTokens > 0 ? { label: t("settings.statusBarItem.turnOutputTokens"), value: observedUsage.turnOutputTokens.toLocaleString(), icon: Activity } : null,
    turn_cache_tokens: observedUsage && observedUsage.turnCacheTokens > 0 ? { label: t("settings.statusBarItem.turnCacheTokens"), value: observedUsage.turnCacheTokens.toLocaleString(), icon: Activity } : null,
    turn_cost: observedUsage?.turnCostComplete && observedUsage.turnCost > 0 && observedUsage.turnCurrency ? { label: t("settings.statusBarItem.turnCost"), value: `≈${formatMoneyLocalized(observedUsage.turnCost, observedUsage.turnCurrency, { locale, empty: "dash" })}`, icon: Coins } : null,
    session_turns: observedUsage && observedUsage.turns > 0 ? { label: t("settings.statusBarItem.sessionTurns"), value: `${observedUsage.turns}`, icon: MessageSquare } : null,
    session_cost: observedUsage?.costComplete && observedUsage.sessionCost > 0 && observedUsage.sessionCurrency ? { label: t("settings.statusBarItem.sessionCost"), value: `≈${formatMoneyLocalized(observedUsage.sessionCost, observedUsage.sessionCurrency, { locale, empty: "dash" })}`, icon: Coins } : null,
    context: context ? { label: t("settings.statusBarItem.context"), value: `${Math.round(context.contextUsedTokens / context.contextWindowTokens * 100)}%`, icon: Activity, detail: t("settings.statusBarItem.contextDetail", { used: context.contextUsedTokens.toLocaleString(), total: context.contextWindowTokens.toLocaleString() }) } : null,
    compact: context && context.compactThresholdPercent > 0 ? { label: t("settings.statusBarItem.compact"), value: `${context.compactThresholdPercent}%`, icon: Activity } : null,
    cache_hit: cache ? { label: t("settings.statusBarItem.cacheHit"), value: `${Math.round(cache.cacheHitTokens / (cache.cacheHitTokens + cache.cacheMissTokens) * 100)}%`, icon: Activity, detail: t("settings.statusBarItem.cacheDetail", { hit: cache.cacheHitTokens.toLocaleString(), miss: cache.cacheMissTokens.toLocaleString() }) } : null,
    bridge: { label: t("settings.statusBarItem.bridge"), value: bridgeRunning ? t("settings.statusBarItem.bridge.connected") : t("settings.statusBarItem.bridge.connecting"), icon: Activity },
  };
  const visible = items.flatMap(id => entries[id] ? [{ id, entry: entries[id] }] : []);
  if (visible.length === 0) return null;
  return <div className={`tauri-statusbar is-${style}`} role="status" aria-label="会话信息栏">
    {visible.map(({ id, entry }) => {
      if (!entry) return null;
      const Icon = entry.icon;
      const scope = id === "observed_tokens" || id === "turn_tokens" || id === "turn_output_tokens" || id === "turn_cache_tokens" ? t("settings.statusBarItem.scope.usage") : id === "turn_cost" || id === "session_cost" ? t("settings.statusBarItem.scope.cost") : id === "session_turns" ? t("settings.statusBarItem.scope.turns") : "";
      return <span key={id} className="tauri-statusbar__item" title={`${entry.label}: ${id === "workspace" ? workspace : entry.value}${scope ? ` (${scope})` : ""}${entry.detail ? ` · ${entry.detail}` : ""}`} aria-label={`${entry.label}: ${entry.value}${scope ? ` (${scope})` : ""}`}><Icon size={13} aria-hidden="true" /><span>{style === "text" ? `${entry.label} · ${entry.value}` : entry.value}</span></span>;
    })}
  </div>;
}
