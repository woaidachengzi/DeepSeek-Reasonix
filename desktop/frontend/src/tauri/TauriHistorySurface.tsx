import { createContext, useCallback, useContext, useLayoutEffect, useRef, useState, type ReactNode } from "react";

export interface HistoryMarkdownEntry { index: number; text: string }
const HistoryParsedContext = createContext<(index: number, text: string) => void>(() => {});
export const useTauriHistoryParsed = () => useContext(HistoryParsedContext);

/** Keep the real layout measurable while the first history view is prepared.
 * Each session gets a new instance; late parse results cannot reveal another
 * session. Once revealed, streamed updates never hide the conversation again. */
export function TauriHistorySurface({ entries, loaded, onPrepared, children }: {
  entries: readonly HistoryMarkdownEntry[];
  loaded: boolean;
  onPrepared: () => void;
  children: ReactNode;
}) {
  const [parsed, setParsed] = useState<ReadonlyMap<number, string>>(() => new Map());
  const [presented, setPresented] = useState(false);
  const presentedRef = useRef(false);
  const activeRef = useRef(true);
  useLayoutEffect(() => {
    activeRef.current = true;
    return () => { activeRef.current = false; };
  }, []);
  const markParsed = useCallback((index: number, text: string) => {
    if (!activeRef.current || presentedRef.current) return;
    setParsed(previous => {
      if (previous.get(index) === text) return previous;
      const next = new Map(previous);
      next.set(index, text);
      return next;
    });
  }, []);
  const ready = !presented && loaded && entries.every(entry => parsed.get(entry.index) === entry.text);
  const preparing = loaded && !presented && !ready;
  useLayoutEffect(() => {
    if (!ready || presentedRef.current) return;
    // The caller commits through TranscriptKernel before revealing the body.
    // No scroll position is written by this presentation boundary.
    onPrepared();
    presentedRef.current = true;
    setPresented(true);
    setParsed(new Map());
  }, [ready, onPrepared]);
  return <HistoryParsedContext.Provider value={markParsed}>
    <div className="tauri-conversation-frame" aria-busy={preparing || undefined}>
      <div className={`tauri-history-surface${preparing ? " is-preparing" : ""}`} aria-hidden={preparing || undefined}>{children}</div>
      {preparing && <div className="tauri-history-preparing tauri-loading" role="status"><span aria-hidden="true" /><p>正在载入对话…</p></div>}
    </div>
  </HistoryParsedContext.Provider>;
}
