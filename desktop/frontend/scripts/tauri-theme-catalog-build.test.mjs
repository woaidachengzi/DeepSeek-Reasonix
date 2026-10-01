import assert from "node:assert/strict";
import { mkdtemp, readdir, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createContext, SourceTextModule } from "node:vm";
import { build } from "vite";

// Exercise Vite's actual client transform. Direct tsx component imports do not
// expand glob imports and cannot prove that the production catalog is present.
const frontend = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const official = resolve(frontend, "../themes/official");
const output = await mkdtemp(join(tmpdir(), "reasonix-theme-catalog-"));
try {
  const input = "reasonix-theme-catalog-test";
  await build({
    configFile: false,
    root: frontend,
    base: "./",
    publicDir: false,
    logLevel: "error",
    plugins: [{
      name: input,
      resolveId(id) { if (id === input) return `\0${input}`; },
      load(id) {
        if (id === `\0${input}`) return `export * from ${JSON.stringify(join(frontend, "src/tauri/tauriThemeCatalog.ts"))};\nexport { isSafeBackgroundURL } from ${JSON.stringify(join(frontend, "src/lib/themePack.ts"))};`;
      },
    }],
    build: {
      outDir: output,
      assetsInlineLimit: 0,
      target: "es2021",
      minify: false,
      rolldownOptions: { input, preserveEntrySignatures: "strict", output: { entryFileNames: "catalog.mjs" } },
    },
  });
  // A client bundle resolves assets relative to its browser module URL, not a
  // Node file: URL. Preserve import.meta semantics without editing built code.
  const window = { location: new URL("https://reasonix.invalid/") };
  const module = new SourceTextModule(await readFile(join(output, "catalog.mjs"), "utf8"), {
    context: createContext({ URL, window }),
    initializeImportMeta(meta) { meta.url = new URL("catalog.mjs", window.location.href).href; },
  });
  await module.link(() => { throw new Error("Catalog test bundle unexpectedly requires another module"); });
  await module.evaluate();
  const catalog = module.namespace;
  const sources = await Promise.all((await readdir(official, { withFileTypes: true }))
    .filter(entry => entry.isDirectory() && entry.name.startsWith("official-"))
    .map(async entry => ({ directory: join(official, entry.name), manifest: JSON.parse(await readFile(join(official, entry.name, "theme.json"), "utf8")) })));
  assert(sources.length > 0, "official source fixtures must not be empty");
  assert.deepEqual(Array.from(catalog.TAURI_OFFICIAL_THEME_PACKS, pack => pack.id).sort(), sources.map(source => source.manifest.id).sort(), "built catalog must contain every official source theme");
  for (const { directory, manifest } of sources) {
    const pack = catalog.tauriThemePackById(manifest.id);
    assert(pack?.builtin && pack.kind === "official" && pack.hasBackground);
    assert.deepEqual(JSON.parse(JSON.stringify(pack.tokens)), manifest.tokens);
    assert.deepEqual(JSON.parse(JSON.stringify(pack.recipes)), manifest.recipes);
    for (const [url, filename] of [[pack.backgroundUrl, "background.webp"], [pack.previewUrl, "preview.webp"]]) {
      const emitted = new URL(url, window.location.href);
      assert.equal(emitted.origin, window.location.origin);
      assert.match(emitted.pathname, /^\/assets\/(background|preview)-[a-zA-Z0-9_-]+\.webp$/);
      assert.deepEqual(await readFile(join(output, emitted.pathname.slice(1))), await readFile(join(directory, filename)), "built asset bytes must match the source theme");
    }
    assert(catalog.isSafeBackgroundURL(new URL(pack.backgroundUrl, window.location.href).href), "built official background must be registered for display");
    const selected = catalog.tauriThemeCatalog("graphite", manifest.id);
    assert.deepEqual(Array.from(selected.filter(pack => pack.active), pack => pack.id), [manifest.id]);
  }
  assert.equal(catalog.tauriThemeCatalog("graphite", "").filter(pack => pack.kind === "base").length, 6);
  assert.deepEqual(Array.from(catalog.tauriThemeCatalog("graphite", "").filter(pack => pack.active), pack => pack.id), ["graphite"]);
  assert.equal(catalog.tauriThemePackById("official-missing"), null);
  assert(!catalog.isSafeBackgroundURL("https://foreign.invalid/assets/background-untrusted.webp"));
  assert(!catalog.isSafeBackgroundURL("https://reasonix.invalid/assets/background-unregistered.webp"));
  console.log(`Vite client theme catalog: ${sources.length} official themes, exact asset bytes, selection and URL boundaries passed`);
} finally {
  await rm(output, { recursive: true, force: true });
}
