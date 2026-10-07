// Registers the tauriBridge stub loader before tsx boots, so a component test
// can mount a Tauri view under jsdom without a Tauri host. Mirrors
// css-stub-register.mjs.
import { register } from "node:module";
register(new URL("./tauri-bridge-stub-loader.mjs", import.meta.url));
// Delegate to the test's visual jsdom window once it is installed. The real
// viewport kernel now runs in Tauri component tests, just as it does in WebView.
globalThis.requestAnimationFrame ??= callback => window.requestAnimationFrame(callback);
globalThis.cancelAnimationFrame ??= handle => window.cancelAnimationFrame(handle);
