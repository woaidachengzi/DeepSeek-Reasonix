import { useCallback, useLayoutEffect, useRef } from "react";
import { TerminalPanel } from "../components/TerminalPanel";
import { nativeTerminals } from "../lib/nativeTerminals";
import { formatTerminalOutputForComposer } from "../lib/terminalOutput";
import { useTerminalStore } from "../store/terminal";

/** Lazy boundary keeps xterm and the legacy App proxy out of initial load. */
export function TauriTerminalDock({ sessionId, cwd, open, onClose, onAddToChat }: {
  sessionId: string; cwd: string; open: boolean; onClose: () => void; onAddToChat: (text: string) => void;
}) {
  const owner = useRef<object | null>(null);
  const add = useRef(onAddToChat);
  useLayoutEffect(() => { add.current = onAddToChat; }, [onAddToChat]);
  useLayoutEffect(() => {
    const token = {};
    owner.current = token;
    const release = nativeTerminals.retain(sessionId);
    const offError = nativeTerminals.onError(error => {
      if (owner.current === token) useTerminalStore.setState({ error: error.message });
    });
    const offRefresh = nativeTerminals.onRefresh(() => {
      if (owner.current === token) void useTerminalStore.getState().syncWorkspace(sessionId).catch(() => {});
    });
    return () => { owner.current = null; offError(); offRefresh(); release(); };
  }, [sessionId]);
  const addOutput = useCallback((terminalId: string) => {
    const token = owner.current;
    void nativeTerminals.text(sessionId, terminalId).then(value => {
      if (owner.current === token && token) add.current(formatTerminalOutputForComposer(value));
    }).catch(error => {
      if (owner.current === token && token) useTerminalStore.setState({ error: error instanceof Error ? error.message : String(error) });
    });
  }, [sessionId]);
  return <TerminalPanel tabId={sessionId} cwd={cwd} readOnly={false} open={open} onClose={onClose} onAddOutput={addOutput} onAddToChat={onAddToChat} />;
}
