import { Activity, Bot, FolderOpen, MessageSquare } from "lucide-react";
import { useTauriStatusBarPreferences, type TauriStatusBarItemId } from "./tauriStatusBarPreferences";
import type { TauriObservedUsage } from "./tauriObservedUsage";
import type { TauriSessionMetrics } from "../lib/tauriBridge";

interface TauriStatusBarProps {
  workspace: string;
  model?: string;
  sessionState?: "idle" | "running" | "paused";
  bridgeRunning?: boolean;
  observedUsage?: TauriObservedUsage | null;
  sessionMetrics?: TauriSessionMetrics | null;
}

export function TauriStatusBar({ workspace, model, sessionState, bridgeRunning, observedUsage, sessionMetrics }: TauriStatusBarProps) {
  const { style, items } = useTauriStatusBarPreferences();
  const sessionLabel = sessionState === "running" ? "运行中" : sessionState === "paused" ? "等待确认" : sessionState === "idle" ? "已就绪" : "";
  const context = sessionMetrics && sessionMetrics.contextWindowTokens > 0 ? sessionMetrics : null;
  const cache = sessionMetrics && sessionMetrics.cacheHitTokens + sessionMetrics.cacheMissTokens > 0 ? sessionMetrics : null;
  const entries: Record<TauriStatusBarItemId, { label: string; value: string; icon: typeof Activity; detail?: string } | null> = {
    workspace: workspace ? { label: "工作区", value: workspace.split(/[\\/]/).filter(Boolean).pop() || workspace, icon: FolderOpen } : null,
    model: model ? { label: "默认模型", value: model, icon: Bot } : null,
    session: sessionLabel ? { label: "会话", value: sessionLabel, icon: MessageSquare } : null,
    observed_tokens: observedUsage ? { label: "已观测 token", value: observedUsage.tokens.toLocaleString(), icon: Activity } : null,
    turn_tokens: observedUsage ? { label: "本轮 token", value: observedUsage.turnTokens.toLocaleString(), icon: Activity } : null,
    context: context ? { label: "上下文", value: `${Math.round(context.contextUsedTokens / context.contextWindowTokens * 100)}%`, icon: Activity, detail: `${context.contextUsedTokens.toLocaleString()} / ${context.contextWindowTokens.toLocaleString()} token` } : null,
    compact: context && context.compactThresholdPercent > 0 ? { label: "压缩阈值", value: `${context.compactThresholdPercent}%`, icon: Activity } : null,
    cache_hit: cache ? { label: "会话缓存命中", value: `${Math.round(cache.cacheHitTokens / (cache.cacheHitTokens + cache.cacheMissTokens) * 100)}%`, icon: Activity, detail: `命中 ${cache.cacheHitTokens.toLocaleString()} / 未命中 ${cache.cacheMissTokens.toLocaleString()} token` } : null,
    bridge: { label: "本地服务", value: bridgeRunning ? "已连接" : "连接中", icon: Activity },
  };
  const visible = items.flatMap(id => entries[id] ? [{ id, entry: entries[id] }] : []);
  if (visible.length === 0) return null;
  return <div className={`tauri-statusbar is-${style}`} role="status" aria-label="会话信息栏">
    {visible.map(({ id, entry }) => {
      if (!entry) return null;
      const Icon = entry.icon;
      const scope = id === "observed_tokens" || id === "turn_tokens" ? "（仅统计当前同步阶段收到的用量事件）" : "";
      return <span key={id} className="tauri-statusbar__item" title={`${entry.label}：${id === "workspace" ? workspace : entry.value}${scope}${entry.detail ? ` · ${entry.detail}` : ""}`} aria-label={`${entry.label}：${entry.value}${scope}`}><Icon size={13} aria-hidden="true" /><span>{style === "text" ? `${entry.label} · ${entry.value}` : entry.value}</span></span>;
    })}
  </div>;
}
