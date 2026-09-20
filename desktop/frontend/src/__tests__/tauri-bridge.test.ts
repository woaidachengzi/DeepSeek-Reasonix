import assert from "node:assert/strict";
import { isTauriDesktop } from "../lib/tauriBridge";

assert.equal(isTauriDesktop(), false, "Node test runner is not a Tauri WebView");
console.log("PASS: Tauri bridge adapter keeps browser/test runtime isolated");
