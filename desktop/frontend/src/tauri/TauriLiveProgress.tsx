import { Check, ChevronDown, CircleAlert, LoaderCircle } from "lucide-react";
import { liveProgressLabel, type LiveProgress } from "./liveProgress";

export function TauriLiveProgress({ progress, paused }: { progress: LiveProgress; paused: boolean }) {
  const label = liveProgressLabel(progress, paused);
  return <section className="tauri-live-activity" aria-label="回答进度" data-transcript-block-key="live-activity">
    <div className="tauri-live-activity__status" role="status"><LoaderCircle size={14} aria-hidden="true" />{label}</div>
    {progress.tools.length > 0 && <details className="tauri-progress tauri-live-activity__tools" open>
      <summary><span>工具执行 · {progress.tools.length} 项</span><ChevronDown size={14} aria-hidden="true" /></summary>
      <ul>{progress.tools.map(tool => <li key={tool.id} data-tool-state={tool.state}>
        {tool.state === "running" ? <LoaderCircle size={14} aria-hidden="true" /> : tool.state === "failed" ? <CircleAlert size={14} aria-hidden="true" /> : <Check size={14} aria-hidden="true" />}
        <span>{tool.name}</span><small>{tool.state === "running" ? "执行中" : tool.state === "failed" ? "失败" : "已完成"}</small>
      </li>)}</ul>
    </details>}
  </section>;
}
