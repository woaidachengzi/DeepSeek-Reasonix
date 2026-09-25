import { parseAttachmentRefsForDisplay } from "../lib/attachmentDisplay";
import type { TauriWorkbenchSession } from "../lib/tauriBridge";

export interface WorkbenchProjectGroup {
  key: string;
  root?: string;
  label: string;
  title?: string;
  sessions: TauriWorkbenchSession[];
  savedTitle?: boolean;
}

export interface WorkbenchProjectFolder {
  root: string;
  title?: string;
}

function projectKey(root?: string): string {
  const trimmed = (root ?? "").trim();
  return trimmed.replace(/[/\\]+$/, "") || (trimmed ? trimmed[0] : "");
}

function projectName(root: string): string {
  const segments = root.split(/[/\\]/).filter(Boolean);
  return segments[segments.length - 1] || root;
}

/** Preserve catalog order both within projects and among projects. New
 *  sessions prepend; reopening an existing row never reorders folders.
 *  A blank root stays rootless so the sidebar can list it as a bare chat
 *  instead of inventing an "unspecified project" folder. */
export function groupWorkbenchSessions(
  sessions: readonly TauriWorkbenchSession[],
  folders: readonly WorkbenchProjectFolder[] = [],
): WorkbenchProjectGroup[] {
  const groups = new Map<string, WorkbenchProjectGroup>();
  for (const folder of folders) {
    const root = folder.root.trim();
    const key = projectKey(root);
    if (!key || groups.has(key)) continue;
    const title = folder.title?.trim();
    groups.set(key, {
      key,
      root,
      label: title || projectName(key),
      title: title || undefined,
      sessions: [],
      savedTitle: Boolean(title),
    });
  }
  for (const session of sessions) {
    const root = (session.workspaceRoot ?? "").trim();
    const key = projectKey(root);
    let group = groups.get(key);
    if (!group) {
      group = { key, root: root || undefined, label: key ? projectName(key) : "", sessions: [] };
      groups.set(key, group);
    }
    group.sessions.push(session);
  }
  const result = [...groups.values()];
  const duplicateLabels = new Map<string, WorkbenchProjectGroup[]>();
  for (const group of result) {
    const sameLabel = duplicateLabels.get(group.label) ?? [];
    sameLabel.push(group);
    duplicateLabels.set(group.label, sameLabel);
  }
  for (const [label, duplicates] of duplicateLabels) {
    if (duplicates.length < 2) continue;
    const parentCounts = new Map<string, number>();
    for (const group of duplicates) {
      const segments = (group.root ?? "").split(/[/\\]/).filter(Boolean);
      const parent = segments[segments.length - 2] ?? "";
      parentCounts.set(parent, (parentCounts.get(parent) ?? 0) + 1);
    }
    for (const group of duplicates) {
      const segments = (group.root ?? "").split(/[/\\]/).filter(Boolean);
      const parent = segments[segments.length - 2] ?? "";
      const suffix = parent && parentCounts.get(parent) === 1 ? parent : group.root || label;
      group.label = `${label} · ${suffix}`;
    }
  }
  return result;
}

/** A deterministic, local title from the first user turn. Attachment paths
 * and line breaks are not shown as conversation names. Manual titles win. */
export function titleFromFirstUser(content: string): string | undefined {
  const display = parseAttachmentRefsForDisplay(content);
  const text = display.text.replace(/\p{Cc}+/gu, " ").replace(/\s+/g, " ").trim();
  if (!text) return display.attachments.length ? "文件对话" : undefined;
  const runes = Array.from(text);
  return runes.length > 48 ? `${runes.slice(0, 48).join("")}…` : text;
}
