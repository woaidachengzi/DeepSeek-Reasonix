import { useEffect, useRef, useState } from "react";
import { Download } from "lucide-react";
import { useI18n } from "../lib/i18n";
import { tauriMessageFrom } from "../lib/tauriBridge";
import { exportTauriAITrace } from "../lib/tauriAITrace";
import { SettingsSection } from "../components/SettingsForm";

const preferenceKey = "reasonix-ai-trace-detailed";
const COPY = {
  zh: { title: "AI 诊断日志", description: "导出当前会话中已保存的记录，核对模型判断和工具实际返回的结果。", detail: "导出详细内容", hint: "包含聊天、模型返回的思考、工具参数和完整结果；关闭后导出简洁记录。此选项只影响导出内容。", privacy: "文件仅保存到本机，已遮盖常见密钥，仍含对话和个人信息。截图不包含在内，部分模型不返回思考内容。", export: "导出当前会话日志", exporting: "正在导出…", saved: "日志已保存。", cancelled: "已取消导出。", missing: "请先打开需要分析的会话，再返回这里导出。" },
  en: { title: "AI diagnostic logs", description: "Export saved records for the current conversation to compare model statements with actual tool results.", detail: "Include detailed content", hint: "Include chat, model-returned reasoning, tool arguments and full results. Turn off for a compact report. This changes exports only.", privacy: "Saved locally. Common credential patterns are masked, but conversations and personal information remain. Screenshots are excluded; some models return no reasoning.", export: "Export current conversation logs", exporting: "Exporting…", saved: "Logs saved.", cancelled: "Export cancelled.", missing: "Open the conversation to investigate, then return here to export." },
  "zh-TW": { title: "AI 診斷日誌", description: "匯出目前對話已儲存的記錄，核對模型判斷與工具實際回傳的結果。", detail: "匯出詳細內容", hint: "包含對話、模型回傳的思考、工具參數與完整結果；關閉後匯出簡潔記錄。此選項只影響匯出內容。", privacy: "檔案僅儲存於本機，已遮蔽常見金鑰，仍含對話與個人資訊。不包含截圖，部分模型不回傳思考內容。", export: "匯出目前對話日誌", exporting: "正在匯出…", saved: "日誌已儲存。", cancelled: "已取消匯出。", missing: "請先開啟需要分析的對話，再返回此處匯出。" },
};

function readDetailed() {
  try { return localStorage.getItem(preferenceKey) !== "false"; } catch { return true; }
}

export function TauriAITraceSettings({ sessionId, api = exportTauriAITrace }: {
  sessionId?: string;
  api?: typeof exportTauriAITrace;
}) {
  const { locale } = useI18n();
  const copy = COPY[locale];
  const [detailed, setDetailed] = useState(readDetailed);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<"saved" | "cancelled" | "">("");
  const [error, setError] = useState("");
  const epoch = useRef(0);
  const pending = useRef<number | null>(null);
  useEffect(() => {
    epoch.current++;
    pending.current = null;
    setBusy(false); setNotice(""); setError("");
    return () => { epoch.current++; };
  }, [sessionId]);
  const exportLogs = async () => {
    if (!sessionId || pending.current !== null) return;
    const request = epoch.current;
    pending.current = request;
    setBusy(true); setNotice(""); setError("");
    try {
      const saved = await api(sessionId, detailed);
      if (epoch.current === request) setNotice(saved ? "saved" : "cancelled");
    } catch (cause) {
      if (epoch.current === request) setError(tauriMessageFrom(cause));
    } finally {
      if (epoch.current === request && pending.current === request) { pending.current = null; setBusy(false); }
    }
  };
  return <SettingsSection title={copy.title} description={copy.description}>
    <label className="tauri-settings-field">
      <span className="tauri-settings-field-label">{copy.detail}<small>{copy.hint}</small></span>
      <input type="checkbox" checked={detailed} disabled={busy} onChange={event => {
        const next = event.target.checked;
        setDetailed(next);
        try { localStorage.setItem(preferenceKey, String(next)); } catch { /* export still works */ }
      }} />
    </label>
    <p>{copy.privacy}</p>
    {!sessionId && <p role="status">{copy.missing}</p>}
    <div className="tauri-settings-actions"><button type="button" className="tauri-settings-button" disabled={!sessionId || busy} onClick={() => void exportLogs()}><Download size={14} aria-hidden="true" />{busy ? copy.exporting : copy.export}</button></div>
    {notice && <p role="status">{copy[notice]}</p>}
    {error && <p className="tauri-diagnostic-error" role="alert">{error}</p>}
  </SettingsSection>;
}
