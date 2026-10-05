import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Database, X } from "lucide-react";
import { CopyButton } from "../components/CopyButton";
import type { BridgeHistoryTurnUsage } from "../lib/bridgeProtocol.generated";
import { formatTokens } from "../lib/format";
import { useI18n } from "../lib/i18n";
import { formatTauriMessageClock } from "./historyPresentation";

function UsageDetails({ id, usage, trigger, onClose }: {
  id: string;
  usage: BridgeHistoryTurnUsage;
  trigger: HTMLButtonElement;
  onClose: (restoreFocus?: boolean) => void;
}) {
  const panel = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState<{ left: number; top: number }>();
  const { locale } = useI18n();
  const en = locale === "en";
  useLayoutEffect(() => {
    const element = panel.current;
    if (!element) return;
    const anchor = trigger.getBoundingClientRect();
    const box = element.getBoundingClientRect();
    const left = Math.max(8, Math.min(anchor.left, window.innerWidth - box.width - 8));
    const top = Math.max(8, Math.min(anchor.top - box.height - 8, window.innerHeight - box.height - 8));
    setPosition({ left, top });
    element.querySelector<HTMLButtonElement>("button")?.focus();
  }, [trigger]);
  useEffect(() => {
    const pointer = (event: PointerEvent) => {
      if (event.target instanceof Node && !panel.current?.contains(event.target) && !trigger.contains(event.target)) onClose();
    };
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); onClose(true); }
    };
    const resize = () => onClose();
    const scroll = (event: Event) => { if (!(event.target instanceof Node) || !panel.current?.contains(event.target)) onClose(); };
    document.addEventListener("pointerdown", pointer);
    document.addEventListener("keydown", key, true);
    window.addEventListener("resize", resize);
    window.addEventListener("scroll", scroll, true);
    return () => {
      document.removeEventListener("pointerdown", pointer);
      document.removeEventListener("keydown", key, true);
      window.removeEventListener("resize", resize);
      window.removeEventListener("scroll", scroll, true);
    };
  }, [trigger, onClose]);
  const exact = (value: number) => value.toLocaleString(locale === "en" ? "en-US" : "zh-CN");
  const absent = !usage.complete && usage.totalTokens === 0;
  return createPortal(<div id={id} ref={panel} role="dialog" aria-label={en ? "Turn usage" : "本轮用量"} className="tauri-turn-usage" style={{ ...position, visibility: position ? "visible" : "hidden" }}>
    <header><strong>{en ? "Turn usage" : "本轮用量"}</strong><button type="button" aria-label={en ? "Close" : "关闭"} onClick={() => onClose(true)}><X size={16} /></button></header>
    <dl>
      <dt>{en ? "Total tokens" : "总用量"}</dt><dd>{absent ? (en ? "Not recorded" : "未记录") : exact(usage.totalTokens)}</dd>
      <dt>{en ? "Input tokens" : "输入 tokens"}</dt><dd>{absent ? "—" : exact(usage.inputTokens)}</dd>
      <dt>{en ? "Output tokens" : "输出 tokens"}</dt><dd>{absent ? "—" : exact(usage.outputTokens)}</dd>
      {usage.reasoningTokens > 0 && <><dt>{en ? "Reasoning (included in output)" : "思考 tokens（已含在输出中）"}</dt><dd>{exact(usage.reasoningTokens)}</dd></>}
      {usage.cacheHitTokens !== undefined && <><dt>{en ? "Cached input" : "缓存命中 tokens"}</dt><dd>{exact(usage.cacheHitTokens)}</dd><dt>{en ? "Uncached input" : "缓存未命中 tokens"}</dt><dd>{exact(usage.cacheMissTokens ?? 0)}</dd></>}
      <dt>{en ? "Recorded requests" : "已记录请求次数"}</dt><dd>{exact(usage.requestCount)}</dd>
    </dl>
    <p>{en ? "Includes model requests between this question and answer, including tool steps." : "统计本轮提问至回答之间的模型请求，包含工具调用期间的请求。"}</p>
    {!usage.complete && <p className="tauri-turn-usage__note">{en ? "Some usage was not recorded; shown counts are partial." : "部分请求没有用量记录，显示的是已记录部分。"}</p>}
    {usage.estimated && <p className="tauri-turn-usage__note">{en ? "Includes estimated counts." : "包含估算用量。"}</p>}
  </div>, document.body);
}

export function TauriAnswerActions({ text, createdAtMs, usage }: { text: string; createdAtMs?: number; usage?: BridgeHistoryTurnUsage }) {
  const { locale } = useI18n();
  const en = locale === "en";
  const clock = formatTauriMessageClock(createdAtMs, Date.now(), locale);
  const trigger = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  useEffect(() => { setOpen(false); }, [text, createdAtMs]);
  const id = useId();
  const label = !usage ? (en ? "Usage not recorded" : "用量未记录") : !usage.complete && usage.totalTokens === 0 ? (en ? "Usage not recorded" : "用量未记录")
    : `${usage.complete ? (en ? "Usage" : "用量") : (en ? "Recorded" : "已记录")}${usage.estimated ? (en ? " ≈" : " 约") : " "}${usage.totalTokens === 0 ? "0" : formatTokens(usage.totalTokens)} tok`;
  return <div className="tauri-answer-actions">
    <CopyButton text={text} label={en ? "Copy answer" : "复制回答"} showInlineLabel={false} />
    {usage ? <button ref={trigger} type="button" className="tauri-answer-actions__usage" aria-haspopup="dialog" aria-expanded={open} aria-controls={open ? id : undefined} onClick={() => setOpen(value => !value)}><Database size={16} aria-hidden="true" />{label}</button>
      : <span className="tauri-answer-actions__missing" title={en ? "This older answer has no saved token accounting." : "这条回答没有已保存的用量数据。"}><Database size={16} aria-hidden="true" />{label}</span>}
    {clock && <time dateTime={new Date(createdAtMs!).toISOString()} title={new Date(createdAtMs!).toLocaleString()}>{clock}</time>}
    {open && usage && trigger.current && <UsageDetails id={id} usage={usage} trigger={trigger.current} onClose={restore => { setOpen(false); if (restore) trigger.current?.focus(); }} />}
  </div>;
}
