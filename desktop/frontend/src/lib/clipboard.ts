import { isTauri } from "@tauri-apps/api/core";
import { readWailsClipboardText, writeWailsClipboardText } from "./wailsDesktopRuntime";

// Native Tauri clipboard access is limited to text in the main window. Browser
// and Wails keep their existing fallbacks; only explicit user actions read it.

export async function writeClipboardText(value: string): Promise<boolean> {
  if (isTauri()) {
    try {
      const { writeText } = await import("@tauri-apps/plugin-clipboard-manager");
      await writeText(value);
      return true;
    } catch {
      // Preserve the native capability boundary, including transient failures.
      // A user may retry; another transport must not bypass this refusal.
      return false;
    }
  }
  try {
    if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return true;
    }
  } catch {
    // Permission denied or unavailable — try the Wails bridge.
  }
  try {
    if (await writeWailsClipboardText(value)) {
      return true;
    }
  } catch {
    // Bridge missing or failed — fall through to execCommand.
  }
  return fallbackCopyText(value);
}

export async function readClipboardText(): Promise<string> {
  try { return await readClipboardTextOrThrow(); }
  catch { return ""; }
}

// Editors must distinguish a genuine empty clipboard from failed access so a
// denial cannot erase an existing draft. Existing input callers keep the
// forgiving empty-string contract above.
export async function readClipboardTextOrThrow(): Promise<string> {
  if (isTauri()) {
    const { readText } = await import("@tauri-apps/plugin-clipboard-manager");
    // A denial must propagate to strict editors. The forgiving wrapper above
    // still returns empty text without exposing fallback clipboard contents.
    return await readText();
  }
  try {
    if (typeof navigator !== "undefined" && navigator.clipboard?.readText) {
      return await navigator.clipboard.readText();
    }
  } catch {
    // Permission denied or unavailable.
  }
  try {
    return await readWailsClipboardText();
  } catch {
    // No readable clipboard source.
  }
  throw new Error("clipboard text is unavailable");
}

// execCommand("copy") needs a selected editable element, so this selects a
// hidden textarea and must hand the user's selection and focus back afterwards.
export function fallbackCopyText(value: string): boolean {
  const activeElement = document.activeElement;
  const selection = document.getSelection();
  const ranges: Range[] = [];
  if (selection) {
    for (let index = 0; index < selection.rangeCount; index += 1) {
      ranges.push(selection.getRangeAt(index));
    }
  }
  const textarea = document.createElement("textarea");
  textarea.value = value;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.inset = "0 auto auto 0";
  textarea.style.width = "1px";
  textarea.style.height = "1px";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.select();
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } catch {
    // Some WebViews reject execCommand("copy") with NotAllowedError instead of
    // returning false; treat that as a failed copy, never a thrown rejection, so
    // callers (and writeClipboardText's Promise<boolean> contract) stay honored.
    ok = false;
  } finally {
    textarea.remove();
    if (selection) {
      selection.removeAllRanges();
      for (const range of ranges) selection.addRange(range);
    }
    if (activeElement instanceof HTMLElement) activeElement.focus();
  }
  return ok;
}
