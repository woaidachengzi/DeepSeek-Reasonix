import type { TauriMCPServer, TauriMCPServerInput } from "../lib/tauriBridge";

// MCP server form draft. Credential fields stay as the user typed them and are
// never populated from the bridge, because no credential value is ever returned
// to the renderer.
export interface MCPDraft {
  name: string;
  scope: "project" | "global";
  type: "stdio" | "http" | "sse";
  command: string;
  args: string;
  url: string;
  /** KEY=value per line; empty keeps the stored credentials untouched. */
  env: string;
  headers: string;
  editing: boolean;
  managedByPackage: boolean;
}

export function emptyMCPDraft(scope: "project" | "global"): MCPDraft {
  return {
    name: "",
    scope,
    type: "stdio",
    command: "",
    args: "",
    url: "",
    env: "",
    headers: "",
    editing: false,
    managedByPackage: false,
  };
}

export function mcpDraftForEditing(server: TauriMCPServer): MCPDraft {
  return {
    name: server.name,
    scope: server.scope === "project" ? "project" : "global",
    type: server.type === "http" || server.type === "sse" ? server.type : "stdio",
    command: server.command ?? "",
    args: (server.args ?? []).join(" "),
    url: server.url ?? "",
    // Empty on purpose: the stored values are not readable, so the field shows
    // which keys exist and waits for a replacement.
    env: "",
    headers: "",
    editing: true,
    managedByPackage: Boolean(server.managedByPackage),
  };
}

/** What the user should be told about credentials they cannot see. */
export function mcpCredentialHint(server: TauriMCPServer): string {
  const parts: string[] = [];
  if (server.envKeys?.length) parts.push(`环境变量 ${server.envKeys.join("、")}`);
  if (server.headerKeys?.length) parts.push(`请求头 ${server.headerKeys.join("、")}`);
  if (parts.length === 0) return "";
  return `${parts.join("；")}（值不回传，留空保持原值）`;
}

export function mcpTransportSummary(server: TauriMCPServer): string {
  if (server.type === "stdio") {
    const args = (server.args ?? []).join(" ");
    return args ? `${server.command ?? ""} ${args}` : server.command ?? "";
  }
  return server.url ?? "";
}

function parseCredentialLines(input: string): Record<string, string> | undefined {
  const lines = input
    .split("\n")
    .map(line => line.trim())
    .filter(Boolean);
  if (lines.length === 0) return undefined;
  const values: Record<string, string> = {};
  for (const line of lines) {
    const separator = line.indexOf("=");
    if (separator <= 0) {
      throw new Error(`凭据需要用 KEY=value 的格式：${line}`);
    }
    values[line.slice(0, separator).trim()] = line.slice(separator + 1).trim();
  }
  return values;
}

/** Turns a draft into the bridge request. Omitted credential fields keep the
 *  stored values; an explicitly submitted map replaces them. */
export function mcpDraftToInput(draft: MCPDraft): TauriMCPServerInput {
  const name = draft.name.trim();
  if (!name) throw new Error("服务器名称不能为空");
  if (draft.type === "stdio" && !draft.command.trim()) {
    throw new Error("stdio 服务器需要填写命令");
  }
  if (draft.type !== "stdio" && !draft.url.trim()) {
    throw new Error("http/sse 服务器需要填写 URL");
  }
  const input: TauriMCPServerInput = {
    scope: draft.scope,
    name,
    type: draft.type,
  };
  if (draft.type === "stdio") {
    input.command = draft.command.trim();
    const args = draft.args.split(/\s+/).filter(Boolean);
    if (args.length > 0) input.args = args;
  } else {
    input.url = draft.url.trim();
  }
  const env = parseCredentialLines(draft.env);
  if (env) input.env = env;
  const headers = parseCredentialLines(draft.headers);
  if (headers) input.headers = headers;
  return input;
}
