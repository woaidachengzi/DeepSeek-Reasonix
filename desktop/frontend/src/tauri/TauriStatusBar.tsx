import { Activity, Bot, FolderOpen, MessageSquare } from "lucide-react";
import { useTauriStatusBarPreferences, type TauriStatusBarItemId } from "./tauriStatusBarPreferences";

interface TauriStatusBarProps {
  workspace: string;
  model?: string;
  sessionState?: "idle" | "running" | "paused";
  bridgeRunning?: boolean;
}

export function TauriStatusBar({ workspace, model, sessionState, bridgeRunning }: TauriStatusBarProps) {
  const { style, items } = useTauriStatusBarPreferences();
  const sessionLabel = sessionState === "running" ? "运行中" : sessionState === "paused" ? "等待确认" : sessionState === "idle" ? "已就绪" : "";
  const entries: Record<TauriStatusBarItemId, { label: string; value: string; icon: typeof Activity } | null> = {
    workspace: workspace ? { label: "工作区", value: workspace.split(/[\\/]/).filter(Boolean).pop() || workspace, icon: FolderOpen } : null,
    model: model ? { label: "默认模型", value: model, icon: Bot } : null,
    session: sessionLabel ? { label: "会话", value: sessionLabel, icon: MessageSquare } : null,
    bridge: { label: "本地服务", value: bridgeRunning ? "已连接" : "连接中", icon: Activity },
  };
  const visible = items.flatMap(id => entries[id] ? [{ id, entry: entries[id] }] : []);
  if (visible.length === 0) return null;
  return <div className={`tauri-statusbar is-${style}`} role="status" aria-label="会话信息栏">
    {visible.map(({ id, entry }) => {
      if (!entry) return null;
      const Icon = entry.icon;
      return <span key={id} className="tauri-statusbar__item" title={`${entry.label}：${id === "workspace" ? workspace : entry.value}`} aria-label={`${entry.label}：${entry.value}`}><Icon size={13} aria-hidden="true" /><span>{style === "text" ? `${entry.label} · ${entry.value}` : entry.value}</span></span>;
    })}
  </div>;
}
