import { existsSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";

// Mirror the JSON build flavors used in this repository, in Tauri merge order.
// Fail closed on unsupported syntax instead of verifying an old Preview app.
export function resolveMacOSProductName(directory, args) {
  const configs = [JSON.parse(readFileSync(join(directory, "tauri.conf.json"), "utf8"))];
  const platform = join(directory, "tauri.macos.conf.json");
  if (existsSync(platform)) configs.push(JSON.parse(readFileSync(platform, "utf8")));
  for (let i = 0; i < args.length && args[i] !== "--"; i++) {
    let source;
    if (args[i] === "--config" || args[i] === "-c") source = args[++i];
    else if (args[i].startsWith("--config=")) source = args[i].slice("--config=".length);
    else continue;
    if (!source || source.startsWith("-")) throw new Error("--config requires JSON or a configuration path");
    configs.push(JSON.parse(source.trimStart().startsWith("{") ? source : readFileSync(resolve(directory, source), "utf8")));
  }
  let productName;
  for (const config of configs) {
    if (Object.hasOwn(config, "productName")) productName = config.productName;
  }
  if (typeof productName !== "string" || !productName.trim() || /[/\\]/.test(productName)) {
    throw new Error("The merged Tauri configuration must define a valid productName for signature verification");
  }
  return productName;
}
