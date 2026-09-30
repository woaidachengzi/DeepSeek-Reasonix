import { invoke, isTauri } from "@tauri-apps/api/core";

/** Open an explicit user-selected link without navigating the app WebView. */
export async function openExternal(url: string): Promise<boolean> {
  try {
    if (isTauri()) {
      // Host validation is authoritative. A refusal must not fall through to
      // window.open, which would bypass the allowed schemes and leave the app.
      await invoke<void>("open_external_link", { url });
    } else if (typeof window !== "undefined" && window.runtime?.BrowserOpenURL) {
      window.runtime.BrowserOpenURL(url);
    } else if (typeof window !== "undefined") {
      window.open(url, "_blank", "noopener,noreferrer");
    } else {
      return false;
    }
    return true;
  } catch {
    // The boolean lets UI callers offer retry/copy without exposing OAuth
    // query strings or producing an unhandled rejection in older callers.
    return false;
  }
}
