import { parseAttachmentRefsForDisplay } from "../lib/attachmentDisplay";
import type { TauriWorkbenchSession } from "../lib/tauriBridge";

export interface WorkbenchProjectGroup {
  key: string;
  root?: string;
  label: string;
  sessions: TauriWorkbenchSession[];
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
export function groupWorkbenchSessions(sessions: readonly TauriWorkbenchSession[]): WorkbenchProjectGroup[] {
  const groups = new Map<string, WorkbenchProjectGroup>();
  for (const session of sessions) {
    const key = projectKey(session.workspaceRoot);
    let group = groups.get(key);
    if (!group) {
      group = { key, root: key || undefined, label: key ? projectName(key) : "", sessions: [] };
      groups.set(key, group);
    }
    group.sessions.push(session);
  }
  const result = [...groups.values()];
  const labelCounts = new Map<string, number>();
  for (const group of result) labelCounts.set(group.label, (labelCounts.get(group.label) ?? 0) + 1);
  for (const group of result) {
    if (group.root && (labelCounts.get(group.label) ?? 0) > 1) {
      const segments = group.root.split(/[/\\]/).filter(Boolean);
      const parent = segments[segments.length - 2];
      group.label = parent ? `${group.label} · ${parent}` : group.root;
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
