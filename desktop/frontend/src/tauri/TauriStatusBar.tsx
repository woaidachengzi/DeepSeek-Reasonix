import { useEffect, useState } from "react";
import { Bot, ChartLine, Coins, Database, FileOutput, FolderOpen, Gauge, HardDrive, Layers, MessageSquare, Minimize2, Percent, Receipt, RefreshCw, Server, Wallet, Zap, type LucideIcon } from "lucide-react";
import { useTauriStatusBarPreferences, type TauriStatusBarItemId } from "./tauriStatusBarPreferences";
import type { TauriObservedUsage, TauriUsageBillingMode } from "./tauriObservedUsage";
import type { TauriSessionMetrics } from "../lib/tauriBridge";
import { tauriSessionBalance, type TauriSessionBalance } from "../lib/tauriBridge";
import { useI18n, useT } from "../lib/i18n";
import { formatMoneyLocalized } from "../lib/money";

// Keep these Tauri-only labels out of the global startup dictionaries.
const COST_COPY = {
  en: {
    equivalent: { turn: "Turn plan equivalent", session: "Session plan equivalent" },
    mixed: { turn: "Turn mixed estimate", session: "Session mixed estimate" },
    estimateHint: "Estimated from token usage and configured rates; not an actual charge or remaining plan allowance.",
    equivalentHint: "Plan usage valued at pay-as-you-go rates; not the plan bill, an actual charge, or remaining allowance.",
    mixedHint: "Combines different billing estimates; not an actual charge or remaining plan allowance.",
  },
  zh: {
    equivalent: { turn: "本轮套餐折算", session: "会话套餐折算" },
    mixed: { turn: "本轮混合估算", session: "会话混合估算" },
    estimateHint: "根据 Token 用量和配置单价估算，不代表实际扣费或套餐剩余额度。",
    equivalentHint: "套餐用量按按量单价折算，不代表套餐账单、实际扣费或剩余额度。",
    mixedHint: "包含不同计费口径的参考估算，不代表实际扣费或套餐剩余额度。",
  },
  "zh-TW": {
    equivalent: { turn: "本輪方案折算", session: "工作階段方案折算" },
    mixed: { turn: "本輪混合估算", session: "工作階段混合估算" },
    estimateHint: "依 Token 用量與設定單價估算，不代表實際扣費或方案剩餘額度。",
    equivalentHint: "方案用量按隨用隨付單價折算，不代表方案帳單、實際扣費或剩餘額度。",
    mixedHint: "包含不同計費口徑的參考估算，不代表實際扣費或方案剩餘額度。",
  },
} as const;

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
  const costCopy = COST_COPY[locale];
  const costPresentation = (scope: "turn" | "session", mode?: TauriUsageBillingMode) => ({
    label: mode === "subscription_equivalent" ? costCopy.equivalent[scope]
      : mode === "mixed" ? costCopy.mixed[scope]
      : t(scope === "turn" ? "settings.statusBarItem.turnCost" : "settings.statusBarItem.sessionCost"),
    detail: mode === "subscription_equivalent" ? costCopy.equivalentHint
      : mode === "mixed" ? costCopy.mixedHint : costCopy.estimateHint,
  });
  const entries: Record<TauriStatusBarItemId, { label: string; value: string; icon: LucideIcon; detail?: string } | null> = {
    workspace: workspace ? { label: t("settings.statusBarItem.workspace"), value: workspace.split(/[\\/]/).filter(Boolean).pop() || workspace, icon: FolderOpen } : null,
    model: model ? { label: t("settings.statusBarItem.model"), value: model, icon: Bot } : null,
    balance: sessionBalance ? { label: t("settings.statusBarItem.balance"), value: sessionBalance.display, icon: Wallet } : null,
    session: sessionLabel ? { label: t("settings.statusBarItem.session"), value: sessionLabel, icon: MessageSquare } : null,
    observed_tokens: observedUsage ? { label: t("settings.statusBarItem.observedTokens"), value: observedUsage.tokens.toLocaleString(), icon: Database } : null,
    turn_tokens: observedUsage ? { label: t("settings.statusBarItem.turnTokens"), value: observedUsage.turnTokens.toLocaleString(), icon: Zap } : null,
    turn_tps: observedUsage ? { label: t("status.tpsLabel"), value: observedUsage.turnTps && observedUsage.turnTps > 0 ? observedUsage.turnTps < 1 ? "<1 t/s" : `${Math.round(observedUsage.turnTps)} t/s` : "-", icon: Gauge, detail: t("status.tpsTitle") } : null,
    turn_output_tokens: observedUsage && observedUsage.turnOutputTokens > 0 ? { label: t("settings.statusBarItem.turnOutputTokens"), value: observedUsage.turnOutputTokens.toLocaleString(), icon: FileOutput } : null,
    turn_cache_tokens: observedUsage && observedUsage.turnCacheTokens > 0 ? { label: t("settings.statusBarItem.turnCacheTokens"), value: observedUsage.turnCacheTokens.toLocaleString(), icon: HardDrive } : null,
    turn_cost: observedUsage?.turnCostComplete && observedUsage.turnCost > 0 && observedUsage.turnCurrency ? { ...costPresentation("turn", observedUsage.turnBillingMode), value: `≈${formatMoneyLocalized(observedUsage.turnCost, observedUsage.turnCurrency, { locale, empty: "dash" })}`, icon: Coins } : null,
    session_turns: observedUsage && observedUsage.turns > 0 ? { label: t("settings.statusBarItem.sessionTurns"), value: `${observedUsage.turns}`, icon: RefreshCw } : null,
    session_cost: observedUsage?.costComplete && observedUsage.sessionCost > 0 && observedUsage.sessionCurrency ? { ...costPresentation("session", observedUsage.sessionBillingMode), value: `≈${formatMoneyLocalized(observedUsage.sessionCost, observedUsage.sessionCurrency, { locale, empty: "dash" })}`, icon: Receipt } : null,
    context: context ? { label: t("settings.statusBarItem.context"), value: `${Math.round(context.contextUsedTokens / context.contextWindowTokens * 100)}%`, icon: Layers, detail: t("settings.statusBarItem.contextDetail", { used: context.contextUsedTokens.toLocaleString(), total: context.contextWindowTokens.toLocaleString() }) } : null,
    compact: context && context.compactThresholdPercent > 0 ? { label: t("settings.statusBarItem.compact"), value: `${context.compactThresholdPercent}%`, icon: Minimize2 } : null,
    cache: observedUsage?.latestCacheHitTokens !== null && observedUsage?.latestCacheHitTokens !== undefined && observedUsage.latestCacheMissTokens !== null
      ? { label: t("status.cacheLabel"), value: `${((observedUsage.latestCacheHitTokens / (observedUsage.latestCacheHitTokens + observedUsage.latestCacheMissTokens)) * 100).toFixed(2)}%`, icon: Percent, detail: t("status.cacheTitle") }
      : null,
    cache_avg: cache ? { label: t("status.cacheAvgLabel"), value: `${((cache.cacheHitTokens / (cache.cacheHitTokens + cache.cacheMissTokens)) * 100).toFixed(2)}%`, icon: ChartLine, detail: `${t("status.cacheAvgTitle")} · ${t("settings.statusBarItem.cacheDetail", { hit: cache.cacheHitTokens.toLocaleString(), miss: cache.cacheMissTokens.toLocaleString() })}` } : null,
    bridge: { label: t("settings.statusBarItem.bridge"), value: bridgeRunning ? t("settings.statusBarItem.bridge.connected") : t("settings.statusBarItem.bridge.connecting"), icon: Server },
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
