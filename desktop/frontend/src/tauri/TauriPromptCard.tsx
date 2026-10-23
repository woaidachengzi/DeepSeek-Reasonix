import { Sparkles } from "lucide-react";
import { tauriSafeMCPURL, type TauriPendingPrompt } from "../lib/tauriBridge";

interface PromptCardProps {
  prompt: TauriPendingPrompt;
  busy: boolean;
  selections: Record<string, string[]>;
  onApproval: (allow: boolean) => void;
  onAskSelection: (questionId: string, label: string, multi: boolean) => void;
  onAskSubmit: () => void;
  onMCPAction: (action: "accept" | "decline" | "cancel") => void;
  onOpenExternalURL: (url: string) => void;
}

export function TauriPromptCard({ prompt, busy, selections, onApproval, onAskSelection, onAskSubmit, onMCPAction, onOpenExternalURL }: PromptCardProps) {
  if (prompt.kind === "approval") {
    const appAccess = prompt.tool === "computer_use_access";
    return <section className="tauri-prompt-card" aria-live="polite" aria-label="等待权限确认">
      <div className="tauri-prompt-card__heading"><Sparkles size={17} /><div><strong>{appAccess ? "允许查看并控制这个应用？" : "需要你的确认"}</strong><span>{appAccess ? "电脑控制 · 仅本次任务" : prompt.tool}</span></div></div>
      {prompt.subject && <p className="tauri-prompt-card__subject">{prompt.subject}</p>}
      {prompt.reason && <p className="tauri-prompt-card__reason">{prompt.reason}</p>}
      <div className="tauri-prompt-card__actions"><button type="button" className="tauri-prompt-card__allow" onClick={() => onApproval(true)} disabled={busy}>{appAccess ? "允许本次任务" : "允许一次"}</button><button type="button" className="tauri-prompt-card__deny" onClick={() => onApproval(false)} disabled={busy}>拒绝</button></div>
    </section>;
  }
  if (prompt.kind === "ask") {
    return <section className="tauri-prompt-card" aria-live="polite" aria-label="回答助手提问">
      <div className="tauri-prompt-card__heading"><Sparkles size={17} /><div><strong>请回答一个问题</strong><span>回答后助手会继续工作</span></div></div>
      <div className="tauri-prompt-card__questions">{prompt.questions.map(question => {
        const selected = selections[question.id] ?? [];
        return <fieldset key={question.id} className="tauri-prompt-card__question"><legend>{question.header || "问题"}</legend><p>{question.prompt}</p><div className="tauri-prompt-card__options">{question.options.map(option => {
          const active = selected.includes(option.label);
          return <button key={option.label} type="button" className={active ? "is-selected" : ""} onClick={() => onAskSelection(question.id, option.label, question.multi)} disabled={busy} aria-pressed={active}><b>{option.label}</b>{option.description && <small>{option.description}</small>}</button>;
        })}</div></fieldset>;
      })}</div>
      <div className="tauri-prompt-card__actions"><button type="button" className="tauri-prompt-card__allow" onClick={onAskSubmit} disabled={busy}>提交回答</button><button type="button" className="tauri-prompt-card__deny" onClick={onAskSubmit} disabled={busy}>跳过</button></div>
    </section>;
  }
  const safeURL = tauriSafeMCPURL(prompt.url);
  const linkHint = prompt.mode !== "url" ? "这是一个外部服务请求" : safeURL ? "打开链接完成后再继续" : "链接不可安全打开，请检查服务配置";
  return <section className="tauri-prompt-card" aria-live="polite" aria-label="MCP 服务请求">
    <div className="tauri-prompt-card__heading"><Sparkles size={17} /><div><strong>{prompt.server} 请求你的操作</strong><span>{linkHint}</span></div></div>
    {prompt.message && <p className="tauri-prompt-card__subject">{prompt.message}</p>}
    {safeURL && <a className="tauri-prompt-card__link" href={safeURL} onClick={event => { event.preventDefault(); onOpenExternalURL(safeURL); }}>打开外部链接（{new URL(safeURL).host}）</a>}
    <div className="tauri-prompt-card__actions"><button type="button" className="tauri-prompt-card__allow" onClick={() => onMCPAction("accept")} disabled={busy}>接受并继续</button><button type="button" className="tauri-prompt-card__deny" onClick={() => onMCPAction("decline")} disabled={busy}>拒绝</button><button type="button" className="tauri-prompt-card__cancel" onClick={() => onMCPAction("cancel")} disabled={busy}>取消</button></div>
  </section>;
}
