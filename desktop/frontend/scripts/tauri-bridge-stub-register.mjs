// Registers the tauriBridge stub loader before tsx boots, so a component test
// can mount a Tauri view under jsdom without a Tauri host. Mirrors
// css-stub-register.mjs.
import { register } from "node:module";
register(new URL("./tauri-bridge-stub-loader.mjs", import.meta.url));
