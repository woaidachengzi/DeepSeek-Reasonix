import { useLayoutEffect, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import { useT } from "../lib/i18n";
import { writeClipboardText } from "../lib/clipboard";
import { useToast } from "../lib/toast";

// CopyButton copies text to the clipboard on click and briefly flips to a check.
// Only acknowledge a successful clipboard write; keep failures retryable.
export function CopyButton({
  text,
  getText,
  className,
  label,
  showInlineLabel = true,
}: {
  text?: string;
  getText?: () => string | Promise<string>;
  className?: string;
  label?: string;
  showInlineLabel?: boolean;
}) {
  const t = useT();
  const { showToast } = useToast();
  const [copied, setCopied] = useState(false);
  const [failed, setFailed] = useState(false);
  const [busy, setBusy] = useState(false);
  const timerRef = useRef<number | null>(null);
  const requestRef = useRef<object | null>(null);
  const generation = useRef(0);
  const actionLabel = label ?? t("msg.copy");
  const stateLabel = copied ? t("msg.copied") : actionLabel;

  useLayoutEffect(() => {
    generation.current += 1;
    requestRef.current = null;
    setCopied(false);
    setFailed(false);
    setBusy(false);
    return () => {
      generation.current += 1;
      requestRef.current = null;
      if (timerRef.current != null) window.clearTimeout(timerRef.current);
      timerRef.current = null;
    };
  }, [text, getText]);

  const copy = async () => {
    if (requestRef.current) return;
    const request = {};
    const source = generation.current;
    const current = () => generation.current === source && requestRef.current === request;
    requestRef.current = request;
    setBusy(true);
    setCopied(false);
    setFailed(false);
    if (timerRef.current != null) window.clearTimeout(timerRef.current);
    timerRef.current = null;
    try {
      const value = getText ? await getText() : text ?? "";
      if (!current()) return;
      const success = await writeClipboardText(value);
      if (!current()) return;
      if (!success) throw new Error("clipboard unavailable");
      setCopied(true);
      timerRef.current = window.setTimeout(() => {
        setCopied(false);
        timerRef.current = null;
      }, 1200);
    } catch {
      if (current()) {
        setFailed(true);
        showToast(t("msg.copyFailed"), "error");
      }
    } finally {
      if (current()) {
        requestRef.current = null;
        setBusy(false);
      }
    }
  };
  return (
    <button
      className={[
        "copybtn",
        copied ? "copybtn--copied" : "",
        className ?? "",
      ].filter(Boolean).join(" ")}
      onClick={copy}
      aria-label={failed ? t("msg.copyFailed") : stateLabel}
      title={failed ? t("msg.copyFailed") : actionLabel}
      disabled={busy}
      type="button"
    >
      {copied ? <Check size={13} /> : <Copy size={13} />}
      {showInlineLabel && (
        <span className="copybtn__label-inline">{stateLabel}</span>
      )}
    </button>
  );
}
