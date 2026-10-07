import assert from "node:assert/strict";
import { attachPastedImages, pastedImageDataURL, pastedImageFiles } from "../tauri/pastedImages";
import type { TauriBridgeAttachment, TauriStagedImage } from "../lib/tauriBridge";

const image = new File(["test"], "screen.png", { type: "image/png" });
const data = (text: string, files: File[] = [], items: DataTransferItem[] = []) => ({ files, items, getData: () => text }) as unknown as DataTransfer;
const item = { kind: "file", type: image.type, getAsFile: () => image } as DataTransferItem;
assert.equal(pastedImageFiles(data("ordinary text")), null, "text remains native paste");
assert.deepEqual(pastedImageFiles(data("", [image], [item])), [image], "files and items do not duplicate images");
assert.deepEqual(pastedImageFiles(data("", [], [item])), [image], "image items work without files");
assert.deepEqual(pastedImageFiles(data("")), [], "native-only screenshots use fallback");
await assert.rejects(pastedImageDataURL(new File(["svg"], "a.svg", { type: "image/svg+xml" })), /格式/);
await assert.rejects(pastedImageDataURL(new File([], "a.png", { type: "image/png" })), /为空/);
await assert.rejects(pastedImageDataURL(new File([new Uint8Array(16 * 1024 * 1024 + 1)], "a.png", { type: "image/png" })), /16 MiB/);

const staged: TauriStagedImage = { token: "owned", path: "/private/tmp/pasted.png", size: 4 };
const attachment: TauriBridgeAttachment = { path: ".reasonix/attachments/pasted.png", name: "pasted.png", size: 4, isImage: true };
function fixture(sessionId?: string) {
  const calls: unknown[] = [];
  let current = true;
  const options = {
    sessionId, isCurrent: () => current,
    stage: async (url?: string) => { calls.push(["stage", url]); return staged as TauriStagedImage | null; },
    discard: async (token: string) => { calls.push(["discard", token]); },
    attach: async (id: string, path: string) => { calls.push(["attach", id, path]); return attachment; },
    add: (value: TauriBridgeAttachment) => { calls.push(["add", value]); },
    queue: (value: TauriStagedImage) => { calls.push(["queue", value]); },
    read: async () => "data:image/png;base64,dGVzdA==",
  };
  return { calls, options, invalidate: () => { current = false; } };
}
let f = fixture("s");
await attachPastedImages([image], f.options);
assert.deepEqual(f.calls, [["stage", "data:image/png;base64,dGVzdA=="], ["attach", "s", staged.path], ["add", attachment], ["discard", "owned"]]);
f = fixture();
await attachPastedImages([], f.options);
assert.deepEqual(f.calls, [["stage", undefined], ["queue", staged]], "draft keeps owned image until send/removal; no session created");
f = fixture("s");
f.options.stage = async () => null;
await attachPastedImages([], f.options);
assert.deepEqual(f.calls, [], "non-image clipboard creates no attachment");
f = fixture("s");
f.options.stage = async () => { f.invalidate(); return staged; };
await attachPastedImages([image], f.options);
assert.deepEqual(f.calls, [["discard", "owned"]], "late staging cannot add to a different workspace");
f = fixture("s");
f.options.attach = async () => { f.invalidate(); return attachment; };
await attachPastedImages([image], f.options);
assert.deepEqual(f.calls, [["stage", "data:image/png;base64,dGVzdA=="], ["discard", "owned"]], "late copy cannot update another workspace");
f = fixture("s");
f.options.attach = async () => { throw new Error("copy failed"); };
await assert.rejects(attachPastedImages([image], f.options), /copy failed/);
assert.deepEqual(f.calls[f.calls.length - 1], ["discard", "owned"], "failed attachment still releases temporary file");
f = fixture("s");
f.options.read = async () => { f.invalidate(); return "data"; };
await attachPastedImages([image], f.options);
assert.deepEqual(f.calls, [], "late FileReader result never stages an image");
console.log("tauri pasted images: OK");
