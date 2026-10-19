import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import type { TauriSpeechVoice } from "../lib/tauriBridge";

const dom = new JSDOM("<div id='root'></div>", { url: "http://localhost", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, FileReader: dom.window.FileReader, IS_REACT_ACT_ENVIRONMENT: true });
const React = await import("react");
const { act, useState } = React;
const { createRoot } = await import("react-dom/client");
const { TauriVoiceLibrary, useTauriVoiceLibrary } = await import("../tauri/TauriVoiceLibrary");
let profiles: TauriSpeechVoice[] = [];
let listGate: Promise<TauriSpeechVoice[]> | undefined;
let saveGate: Promise<TauriSpeechVoice[]> | undefined;
let imports = 0, stopped = 0;
const api = {
  list: async () => listGate ?? [...profiles],
  import: async (input: { name: string; format: "mp3" | "wav"; audio: string }) => {
    imports++;
    assert.equal(input.format, "wav"); assert.equal(input.audio, Buffer.from("reference-wave").toString("base64"));
    if (saveGate) return saveGate;
    profiles = [...profiles, { id: "clone:" + "a".repeat(32), name: input.name }]; return profiles;
  },
  remove: async (id: string) => { profiles = profiles.filter(voice => voice.id !== id); return profiles; },
};
function Fixture({ open = true, owner = "first" }) {
  const library = useTauriVoiceLibrary(open, owner, api);
  const [selected, setSelected] = useState("mimo_default");
  return <><output data-selected>{selected}</output><output data-profiles>{library.voices.map(voice => voice.name).join(",")}</output><TauriVoiceLibrary library={library} selected={selected} disabled={false} onSelect={setSelected} onBeforeChange={() => stopped++} /></>;
}
const root = createRoot(document.getElementById("root")!);
const tick = () => new Promise(resolve => setTimeout(resolve, 20));
const render = async (open = true, owner = "first") => { await act(async () => { root.render(<Fixture open={open} owner={owner} />); await tick(); }); };
const button = (text: string) => [...document.querySelectorAll<HTMLButtonElement>("button")].find(node => node.textContent === text)!;
const click = async (text: string) => { assert(button(text), text); await act(async () => { button(text).click(); await tick(); }); };
async function fill(name: string, fileName = "fixture.wav") {
  if (!document.querySelector('[aria-label="克隆音色名称"]')) await click("导入克隆音色");
  await act(async () => {
    const input = document.querySelector<HTMLInputElement>('[aria-label="克隆音色名称"]')!;
    Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")!.set!.call(input, name);
    input.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
    const file = document.querySelector<HTMLInputElement>('[aria-label="克隆声音样本"]')!;
    Object.defineProperty(file, "files", { configurable: true, value: [new dom.window.File(["reference-wave"], fileName)] });
    file.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
    await tick();
  });
}
try {
  await render(); assert.equal(imports, 0, "opening never uploads a sample or synthesizes");
  await fill("Fixture voice"); assert.equal(button("保存音色").disabled, false);
  await click("保存音色");
  assert.equal(imports, 1); assert.equal(document.querySelector("[data-selected]")?.textContent, "clone:" + "a".repeat(32));
  assert.equal(document.querySelector("[data-profiles]")?.textContent, "Fixture voice"); assert(stopped > 0);
  await click("删除音色"); assert(profiles.length === 1, "delete requires the explicit confirmation click"); await click("取消");
  await click("删除音色"); await click("确认删除"); assert.equal(profiles.length, 0); assert.equal(document.querySelector("[data-selected]")?.textContent, "mimo_default");
  await fill("Invalid", "recording.m4a"); await click("保存音色"); assert.equal(imports, 1); assert(document.querySelector('[role="alert"]')?.textContent?.includes("MP3 或 WAV"));

  let resolveList!: (voices: TauriSpeechVoice[]) => void;
  listGate = new Promise(resolve => { resolveList = resolve; }); await render(false); await render(true, "old");
  listGate = undefined; await render(true, "new");
  await act(async () => { resolveList([{ id: "clone:" + "b".repeat(32), name: "Stale" }]); await tick(); });
  assert.equal(document.querySelector("[data-profiles]")?.textContent, "", "late catalog cannot replace a new owner's catalog");

  let finishSave!: (voices: TauriSpeechVoice[]) => void;
  saveGate = new Promise(resolve => { finishSave = resolve; }); await fill("Pending"); await click("保存音色"); assert.equal(imports, 2);
  await render(false, "new");
  await act(async () => { finishSave([{ id: "clone:" + "c".repeat(32), name: "Pending" }]); await tick(); });
  assert.equal(document.querySelector("[data-selected]")?.textContent, "mimo_default", "late import cannot select a voice after closing"); saveGate = undefined;

  await render(); await fill("Cancelled read");
  const nativeRead = dom.window.FileReader.prototype.readAsDataURL;
  let pendingRead: FileReader | undefined;
  dom.window.FileReader.prototype.readAsDataURL = function () { pendingRead = this; };
  await click("保存音色"); await render(false);
  Object.defineProperty(pendingRead!, "result", { value: "data:audio/wav;base64,ZmFrZQ==" });
  await act(async () => { pendingRead!.onload?.call(pendingRead!, new dom.window.ProgressEvent("load") as unknown as ProgressEvent<FileReader>); await tick(); });
  assert.equal(imports, 2, "closing while reading revokes upload before the bridge call");
  dom.window.FileReader.prototype.readAsDataURL = nativeRead;
  console.log("PASS voice library: local import/name/selection, format errors, explicit delete, preview release, stale catalogs/imports and file-read cancellation");
} finally { await act(async () => root.unmount()); dom.window.close(); }
