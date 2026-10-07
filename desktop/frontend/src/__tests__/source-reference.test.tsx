import assert from "node:assert/strict";
import React from "react";
import ReactMarkdown from "react-markdown";
import { renderToStaticMarkup } from "react-dom/server";
import { JSDOM } from "jsdom";
import { parseSourceReference, workspaceSourcePath } from "../lib/sourceReference";
import { reasonixRemarkPlugins } from "../components/markdownRemarkPlugins";
import { markdownUrlTransform, parseMarkdownToBlocks } from "../lib/markdownPipeline";
import { hastBlockToJsx } from "../lib/hastJsx";
import { SourceReferenceContext } from "../components/SourceReferenceContext";
import { RichMarkdownLink } from "../components/githubLink";

for (const href of ["src/main.ts:188", "src/main.ts#L188", "file:///repo/src/main.ts#L188", "C:\\repo\\src\\main.ts:188"]) {
  assert.equal(parseSourceReference(href)?.line, 188, href);
}
assert.deepEqual(parseSourceReference("src/%E4%B8%AD%E6%96%87%20file.vue#L2"), { path: "src/中文 file.vue", line: 2 });
assert.equal(parseSourceReference("file:///repo/report.pdf"), null, "ordinary documents keep their native opener");
for (const href of ["javascript:alert(1).ts", "https://host/main.ts#L2", "../secret.ts:2", "src/%2e%2e/secret.ts:2", "C:/repo/main.ts:payload", "C:/repo/NUL.ts:2", "src/main.ts#L0", "main.ts:0", "main.ts:10000001", "main.ts?line=2", "main.ts#section", "main%00.ts", "main%zz.ts", "file://./PhysicalDrive0"]) {
  assert.equal(parseSourceReference(href), null, href);
}
for (const platform of ["darwin", "linux"]) {
  assert.equal(workspaceSourcePath({ path: "/repo/src/main.ts" }, "/repo", platform), "src/main.ts");
  assert.equal(workspaceSourcePath({ path: "/repo-other/main.ts" }, "/repo", platform), null);
  assert.equal(workspaceSourcePath({ path: "/Repo/main.ts" }, "/repo", platform), null);
}
assert.equal(workspaceSourcePath({ path: "c:\\REPO\\src\\main.ts" }, "C:/repo", "windows"), "src/main.ts");
assert.equal(workspaceSourcePath({ path: "D:/repo/main.ts" }, "C:/repo", "windows"), null);
assert.equal(workspaceSourcePath({ path: "//nas/share/repo/main.ts" }, "//NAS/share/repo", "windows"), "main.ts");
assert.equal(workspaceSourcePath({ path: "./src/main.ts" }, "/repo", "linux"), "src/main.ts");
assert.equal(workspaceSourcePath({ path: "src/../main.ts" }, "/repo", "linux"), null);
assert.equal(workspaceSourcePath({ path: "src/main.ts" }, "", "linux"), null);

const content = "已修正 `src/WorkProcessTasks.vue:188`，见 [代码](src/中文%20file.ts#L2)。\n\n`handleAddr`\n\n```ts\nsrc/main.ts:5\n```";
const components = { a: RichMarkdownLink };
const wrap = (child: React.ReactNode) => <SourceReferenceContext.Provider value={() => {}}>{child}</SourceReferenceContext.Provider>;
const live = renderToStaticMarkup(wrap(<ReactMarkdown remarkPlugins={reasonixRemarkPlugins} urlTransform={markdownUrlTransform} components={components}>{content}</ReactMarkdown>));
const history = renderToStaticMarkup(wrap(<>{parseMarkdownToBlocks(content).map(block => <React.Fragment key={block.key}>{hastBlockToJsx(block, components)}</React.Fragment>)}</>));
assert.equal(live, history, "streaming and worker history preserve identical citations");
assert.match(live, /href="src\/WorkProcessTasks.vue#L188"/);
assert.match(live, /<code>handleAddr<\/code>/, "non-file inline code stays code");
assert.match(live, /<pre><code class="language-ts">src\/main.ts:5/, "fenced code stays code");

const dom = new JSDOM("<div id='root'></div>", { url: "https://reasonix.local" });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, HTMLElement: dom.window.HTMLElement, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
const { createRoot } = await import("react-dom/client");
const root = createRoot(document.getElementById("root")!);
const received: unknown[] = [];
const nativeCalls: unknown[] = [];
await React.act(async () => root.render(<RichMarkdownLink href="./docs/GUIDE.md">Guide</RichMarkdownLink>));
assert.equal(document.querySelector("a")?.getAttribute("href"), "./docs/GUIDE.md", "ordinary relative documents retain links without a workspace source handler");
await React.act(async () => root.render(<RichMarkdownLink href="src/main.ts#L188">main.ts (line 188)</RichMarkdownLink>));
assert.ok(document.querySelector("code"), "explicit code-line citations remain non-navigating without a source handler");
Object.assign(globalThis, { isTauri: true });
Object.assign(window, { __TAURI_INTERNALS__: { invoke: async (command: string, args: unknown) => {
  nativeCalls.push({ command, args });
} } });
const renderLink = async (href: string) => {
  await React.act(async () => root.render(<SourceReferenceContext.Provider value={reference => received.push(reference)}><RichMarkdownLink href={href}>main.ts (line 188)</RichMarkdownLink></SourceReferenceContext.Provider>));
};
await renderLink("file:///outside/report.md");
assert.ok(document.querySelector(".md-rich-link--local"), "existing file links keep their native opener even inside a source context");
assert.equal(document.querySelector(".md-rich-link--source"), null);
await React.act(async () => document.querySelector<HTMLAnchorElement>("a")!.click());
assert.deepEqual(nativeCalls, [{ command: "open_local_path", args: { path: "/outside/report.md" } }]);
assert.deepEqual(received, [], "ordinary documents never enter the workspace-only preview");
await renderLink("file:///repo/main.ts#L188");
assert.ok(document.querySelector(".md-rich-link--source"), "file URLs with a line suffix opt into the source preview");
await renderLink("src/main.ts#L188");
const anchor = document.querySelector("a")!;
for (const [event, button] of [["click", 0], ["auxclick", 1]] as const) {
  const click = new dom.window.MouseEvent(event, { bubbles: true, cancelable: true, button });
  await React.act(async () => { anchor.dispatchEvent(click); });
  assert.equal(click.defaultPrevented, true, "file citation never navigates the WebView");
}
assert.deepEqual(received, [{ path: "src/main.ts", line: 188 }, { path: "src/main.ts", line: 188 }]);
assert.equal(nativeCalls.length, 1, "source citations never invoke the system opener");
await React.act(async () => root.unmount());
dom.window.close();
console.log("source citations: platform boundaries, URL safety, live/history parity and clicks OK");
