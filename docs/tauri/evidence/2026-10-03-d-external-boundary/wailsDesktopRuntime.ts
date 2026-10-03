// Narrow legacy shell boundary. No mock runtime or Tauri fallback: callers
// choose their host transport before entering this module. Keep it free of
// bridge initialization so shared clipboard utilities stay lightweight.
export interface WindowStateRuntime {
  WindowGetSize(): Promise<{ w: number; h: number }>;
  WindowGetPosition(): Promise<{ x: number; y: number }>;
  WindowIsMaximised(): Promise<boolean>;
}

export function getWailsWindowStateRuntime(): WindowStateRuntime | null {
  const runtime = typeof window === "undefined" ? undefined : window.runtime;
  const size = runtime?.WindowGetSize;
  const position = runtime?.WindowGetPosition;
  const maximised = runtime?.WindowIsMaximised;
  if (typeof size !== "function" || typeof position !== "function" || typeof maximised !== "function") return null;
  return {
    WindowGetSize: () => size.call(runtime),
    WindowGetPosition: () => position.call(runtime),
    WindowIsMaximised: () => maximised.call(runtime),
  };
}

export async function writeWailsClipboardText(value: string): Promise<boolean> {
  const runtime = typeof window === "undefined" ? undefined : window.runtime;
  const write = runtime?.ClipboardSetText;
  return typeof write === "function" && Boolean(await write.call(runtime, value));
}

export async function readWailsClipboardText(): Promise<string> {
  const runtime = typeof window === "undefined" ? undefined : window.runtime;
  const read = runtime?.ClipboardGetText;
  if (typeof read !== "function") throw new Error("clipboard text is unavailable");
  return read.call(runtime);
}

export function openWailsExternalLink(url: string): boolean {
  const runtime = typeof window === "undefined" ? undefined : window.runtime;
  const open = runtime?.BrowserOpenURL;
  if (typeof open !== "function") return false;
  open.call(runtime, url);
  return true;
}
