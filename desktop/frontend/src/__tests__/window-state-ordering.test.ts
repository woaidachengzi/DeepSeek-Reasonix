// Run: pnpm test:window-state

import { createWindowStateSaver, type WindowStateSnapshot } from "../lib/windowState";
import { getWailsWindowStateRuntime } from "../lib/wailsDesktopRuntime";

let failed = 0;

function ok(value: boolean, label: string) {
  process.stdout.write(`  ${value ? "PASS" : "FAIL"}  ${label}\n`);
  if (!value) failed += 1;
}

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => { resolve = done; });
  return { promise, resolve };
}

let nativeState = { width: 900, height: 600, x: 10, y: 20, maximised: false };
let sizeReads = 0;
const firstPersist = deferred();
const persisted: WindowStateSnapshot[] = [];
const save = createWindowStateSaver(
  {
    async WindowGetSize() {
      sizeReads += 1;
      return { w: nativeState.width, h: nativeState.height };
    },
    async WindowGetPosition() {
      return { x: nativeState.x, y: nativeState.y };
    },
    async WindowIsMaximised() {
      return nativeState.maximised;
    },
  },
  async (state) => {
    persisted.push(state);
    if (persisted.length === 1) await firstPersist.promise;
  },
);

const oldSave = save();
while (persisted.length === 0) await Promise.resolve();

nativeState = { width: 1280, height: 800, x: 40, y: 50, maximised: true };
const newSave = save();
await Promise.resolve();
ok(sizeReads === 1, "a newer request waits for the in-flight capture and persistence");

firstPersist.resolve();
await Promise.all([oldSave, newSave]);
ok(persisted.length === 2, "both distinct observations are persisted");
ok(persisted[0].width === 900 && persisted[1].width === 1280, "the newest observation is written last");
ok(persisted[1].maximised, "the newest maximised state is preserved");

await save();
ok(persisted.length === 2, "an unchanged observation is deduplicated after persistence succeeds");

// A disposed mount must not complete a delayed native read or drain queued
// requests after a replacement mount has already saved newer geometry.
const oldSize = deferred();
const disposed = new AbortController();
const lifecycleWrites: WindowStateSnapshot[] = [];
let disposedReads = 0;
let disposedPositionReads = 0;
const disposedSave = createWindowStateSaver({
  async WindowGetSize() {
    disposedReads += 1;
    await oldSize.promise;
    return { w: 900, h: 600 };
  },
  async WindowGetPosition() { disposedPositionReads += 1; return { x: 10, y: 20 }; },
  async WindowIsMaximised() { return false; },
}, async state => { lifecycleWrites.push(state); }, disposed.signal);
const delayedSave = disposedSave();
await Promise.resolve();
const queuedDisposedSave = disposedSave();
disposed.abort();
const mountedSave = createWindowStateSaver({
  async WindowGetSize() { return { w: 1280, h: 800 }; },
  async WindowGetPosition() { return { x: 40, y: 50 }; },
  async WindowIsMaximised() { return true; },
}, async state => { lifecycleWrites.push(state); });
await mountedSave();
oldSize.resolve();
await Promise.all([delayedSave, queuedDisposedSave]);
ok(lifecycleWrites.length === 1 && lifecycleWrites[0].width === 1280,
  "a disposed capture cannot overwrite the replacement mount's geometry");
ok(disposedReads === 1 && disposedPositionReads === 0,
  "disposal stops the remaining native reads and queued captures");
await disposedSave();
ok(disposedReads === 1, "a disposed saver ignores subsequent requests");

const originalWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
const lateWindow: { runtime?: object } = {};
Object.defineProperty(globalThis, "window", { value: lateWindow, configurable: true });
ok(getWailsWindowStateRuntime() === null, "browser without a host does not manufacture a geometry runtime");
lateWindow.runtime = { WindowGetSize: async () => ({ w: 1, h: 1 }) };
ok(getWailsWindowStateRuntime() === null, "incomplete geometry capability starts no capture");
const injectedHost = {
  geometry: { w: 1200, h: 700, x: 80, y: 90, maximised: true },
  async WindowGetSize() { return { w: this.geometry.w, h: this.geometry.h }; },
  async WindowGetPosition() { return { x: this.geometry.x, y: this.geometry.y }; },
  async WindowIsMaximised() { return this.geometry.maximised; },
};
lateWindow.runtime = injectedHost;
const injected = getWailsWindowStateRuntime();
ok(injected !== null, "late Wails runtime injection is discovered at use time");
if (injected) {
  ok((await injected.WindowGetSize()).w === 1200
    && (await injected.WindowGetPosition()).x === 80
    && await injected.WindowIsMaximised(), "geometry adapter preserves the host method receiver");
}
if (originalWindow) Object.defineProperty(globalThis, "window", originalWindow);
else Reflect.deleteProperty(globalThis, "window");

if (failed > 0) process.exit(1);
