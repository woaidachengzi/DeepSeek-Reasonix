import { hasDisallowedWindowsPathSyntax, localPathFromHref } from "./localFileUrl";
import { visit } from "unist-util-visit";
import type { Root, Link } from "mdast";

export interface SourceReference {
  path: string;
  line?: number;
}

const SOURCE_EXTENSION = /\.(?:vue|[cm]?[jt]sx?|go|rs|py|java|kt|swift|cpp|hpp|c|h|cs|rb|php|html|css|scss|json|toml|ya?ml|md|sh|ps1)$/i;

/** Parse file citations, not arbitrary URLs or editor commands. Decode once. */
export function parseSourceReference(href?: string): SourceReference | null {
  if (!href || href.length > 32768 || /[\u0000-\u001f\u007f]/.test(href)) return null;
  const suffix = /(?::([1-9]\d*)|#L([1-9]\d*))$/.exec(href);
  const line = suffix ? Number(suffix[1] ?? suffix[2]) : undefined;
  if (line !== undefined && (!Number.isSafeInteger(line) || line > 10_000_000)) return null;
  const raw = suffix ? href.slice(0, suffix.index) : href;
  if (/[?#]/.test(raw)) return null;
  let path: string;
  if (raw.startsWith("file://")) {
    const local = localPathFromHref(raw);
    if (local === null) return null;
    path = local;
  } else {
    try { path = decodeURIComponent(raw); } catch { return null; }
  }
  if (!path || /[\u0000-\u001f\u007f?#]/.test(path) || hasDisallowedWindowsPathSyntax(path)) return null;
  const normalized = path.replace(/\\/g, "/");
  if (normalized.replace(/^[A-Za-z]:\//, "/").includes(":")) return null;
  if (normalized.split("/").some(part => part === "..")) return null;
  // A filename extension distinguishes citations from anchors/routes/prose.
  if (!/\.[\w-]{1,24}$/.test(normalized) || /\/$/.test(normalized)) return null;
  // Preserve existing system-open behavior for PDFs/images/other documents.
  if (line === undefined && !SOURCE_EXTENSION.test(normalized)) return null;
  return { path, ...(line === undefined ? {} : { line }) };
}

/** Resolve against this session only; the native bridge rechecks real paths. */
export function workspaceSourcePath(reference: SourceReference, root: string, platform: string): string | null {
  if (!root) return null;
  const path = reference.path.replace(/\\/g, "/").replace(/^\.\//, "");
  const workspace = root.replace(/\\/g, "/").replace(/\/+$/, "");
  if (path.split("/").some(part => part === "..")) return null;
  if (path.startsWith("/") || /^[A-Za-z]:\//.test(path)) {
    const prefix = `${workspace}/`;
    const same = platform === "windows" ? path.toLowerCase().startsWith(prefix.toLowerCase()) : path.startsWith(prefix);
    return same ? path.slice(prefix.length) || null : null;
  }
  return path || null;
}

/** Inline file citations become links; fenced code and existing links stay intact. */
export function remarkSourceReferences() {
  return (tree: Root) => {
    visit(tree, "inlineCode", (node, index, parent) => {
      if (!parent || index === undefined || parent.type === "link" || parent.type === "linkReference") return;
      const reference = parseSourceReference(node.value);
      if (!reference || (!reference.line && !SOURCE_EXTENSION.test(reference.path))) return;
      const link: Link = {
        type: "link", title: null, position: node.position,
        url: encodeURI(reference.path.replace(/\\/g, "/")) + (reference.line ? `#L${reference.line}` : ""),
        children: [{ type: "text", value: node.value }],
      };
      parent.children[index] = link;
    });
  };
}
