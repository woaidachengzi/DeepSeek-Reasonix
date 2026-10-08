import { invoke, isTauri } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import type { AppBindings } from "./bridge";
import { TerminalTransport } from "./terminalTransport";

export const nativeTerminals = new TerminalTransport(
  (command, request) => invoke(command, { request }),
  async (frame, reconnect) => {
    const offs: (() => void)[] = [];
    try {
      offs.push(await listen<Parameters<typeof frame>[0]>("bridge:terminal-event", event => frame(event.payload)));
      offs.push(await listen("bridge:connection-restored", () => reconnect(true)));
      offs.push(await listen("bridge:resync-required", () => reconnect(true)));
      offs.push(await listen("bridge:connection-error", () => reconnect(false)));
      return () => offs.forEach(off => off());
    } catch (error) { offs.forEach(off => off()); throw error; }
  },
);
type Bindings = Pick<AppBindings, "TerminalWorkspaceForTab" | "TerminalOutputForTab" | "CreateTerminalForTab" | "WriteTerminalForTab" | "ResizeTerminalForTab" | "CloseTerminalForTab" | "RenameTerminalForTab">;
const bindings: Bindings = {
  TerminalWorkspaceForTab: id => nativeTerminals.workspace(id),
  TerminalOutputForTab: (id, terminal) => nativeTerminals.text(id, terminal),
  CreateTerminalForTab: (id, path, shell) => nativeTerminals.create(id, path, shell),
  WriteTerminalForTab: (id, terminal, data) => nativeTerminals.write(id, terminal, data),
  ResizeTerminalForTab: (id, terminal, cols, rows) => nativeTerminals.mutate(id, terminal, "bridge_terminal_resize", { cols, rows }),
  CloseTerminalForTab: (id, terminal) => nativeTerminals.mutate(id, terminal, "bridge_terminal_close"),
  RenameTerminalForTab: (id, terminal, title) => nativeTerminals.mutate(id, terminal, "bridge_terminal_rename", { title }),
};
export function nativeTerminalBinding(method: string): Bindings[keyof Bindings] | undefined {
  if (!isTauri() || !Object.prototype.hasOwnProperty.call(bindings, method)) return undefined;
  return bindings[method as keyof Bindings];
}
