import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><div id='root'></div>", { url: "http://localhost/", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, IS_REACT_ACT_ENVIRONMENT: true });
Object.defineProperty(globalThis, "navigator", { value: dom.window.navigator, configurable: true });
const React = await import("react");
const { act } = React;
const { createRoot } = await import("react-dom/client");
const { useTauriPinnedFiles } = await import("../tauri/useTauriPinnedFiles");
const { TauriPinnedFilesShelf } = await import("../tauri/TauriPinnedFilesShelf");
type Files = Array<{ path: string; sizeBytes: number; tokenEstimate: number; error?: string }>;
const fixture = globalThis as unknown as {
  __pinnedFilesHandler?: (id: string) => Promise<Files>;
  __changePinnedFileHandler?: (id: string, path: string, pin: boolean) => Promise<Files>;
  __tauriBridgeCalls: Array<{ name: string; args: unknown }>;
};
const file = (path: string, error?: string): Files => [{ path, sizeBytes: 12, tokenEstimate: 3, error }];
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: Error) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; }); return { promise, resolve, reject }; }
type Options = Parameters<typeof useTauriPinnedFiles>[0];
let latest!: ReturnType<typeof useTauriPinnedFiles>;
function Harness({ options }: { options: Options }) {
  latest = useTauriPinnedFiles(options);
  return React.createElement(TauriPinnedFilesShelf, { files: latest.files, error: latest.error, disabled: latest.disabled, english: false, onPreview: () => {}, onRemove: path => { void latest.change(path, false); }, onRetry: latest.retry });
}
const root = createRoot(document.getElementById("root")!);
const options = (sessionId: string, extra: Partial<Options> = {}): Options => ({ sessionId, ready: true, idle: true, readOnly: false, busy: false, ...extra });
const render = async (value: Options) => { await act(async () => { root.render(React.createElement(Harness, { options: value })); }); };
const calls = () => fixture.__tauriBridgeCalls.filter(call => call.name === "bridge_change_pinned_file");

// A slow read for a former session must never replace the new session's shelf.
const readA = deferred<Files>();
fixture.__pinnedFilesHandler = id => id === "a" ? readA.promise : Promise.resolve(file("b.txt"));
await render(options("a"));
await act(async () => { await latest.change("early.txt", true); });
assert.equal(calls().length, 0, "mutation must wait for the initial authoritative list");
await render(options("b"));
await act(async () => { readA.resolve(file("a.txt")); });
assert.deepEqual(latest.files.map(file => file.path), ["b.txt"]);
assert.ok(!document.body.textContent?.includes("a.txt"));

// Duplicate clicks admit one write; a late former-session write cannot leak.
const changeB = deferred<Files>();
fixture.__changePinnedFileHandler = () => changeB.promise;
const before = calls().length;
let pending!: Promise<void>;
await act(async () => { pending = latest.change("notes.txt", true); void latest.change("notes.txt", true); });
assert.equal(calls().length, before + 1);
assert.equal(latest.busy, true);
await render(options("c"));
await act(async () => { changeB.resolve(file("old-write.txt")); await pending; });
assert.deepEqual(latest.files.map(file => file.path), ["b.txt"]);
assert.equal(latest.busy, false);

for (const extra of [{ idle: false }, { busy: true }, { readOnly: true }, { ready: false }]) {
  await render(options("c", extra));
  const count = calls().length;
  await act(async () => { await latest.change("notes.txt", true); });
  assert.equal(calls().length, count, `blocked mutation ${JSON.stringify(extra)}`);
}

// Failed mutations keep the original list, surface a solution, and can retry.
fixture.__pinnedFilesHandler = () => Promise.resolve(file("missing.txt", "file not found"));
await render(options("c"));
fixture.__changePinnedFileHandler = () => Promise.reject(new Error("disk unavailable"));
await act(async () => { await latest.change("missing.txt", false); });
assert.ok(document.querySelector('[role="alert"]')?.textContent?.includes("请重试读取清单"));
assert.deepEqual(latest.files.map(file => file.path), ["missing.txt"]);
await act(async () => { latest.retry(); });
assert.equal(latest.error, "");
assert.ok(document.body.textContent?.includes("文件失效"));
fixture.__changePinnedFileHandler = () => Promise.resolve([]);
await act(async () => { document.querySelector<HTMLButtonElement>('[aria-label="取消固定 missing.txt"]')!.click(); });
assert.equal(latest.files.length, 0);
assert.equal(document.querySelector(".tauri-pinned-files"), null);

// Unmount retires all pending work rather than publishing into a dead surface.
const late = deferred<Files>();
fixture.__pinnedFilesHandler = () => late.promise;
await render(options("d"));
await act(async () => { root.unmount(); late.resolve(file("late.txt")); });
dom.window.close();
console.log("PASS: Tauri pinned files enforce ownership, admission, duplicate-click and stale-completion fences");
