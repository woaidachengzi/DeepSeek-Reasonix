const configStatuses = ["disabled", "bot_disabled", "missing_credentials", "access_blocked", "configured"] as const;
const runtimeStatuses = ["not_observed", "refreshing", "unknown", "configured", "disabled", "running", "error", "closed", "degraded"] as const;
export type BotConfigDiagnosticStatus = typeof configStatuses[number];
export type BotRuntimeDiagnosticStatus = typeof runtimeStatuses[number];
export interface BotConnectionDiagnostic {
  id: string;
  configStatus: BotConfigDiagnosticStatus;
  runtimeStatus: BotRuntimeDiagnosticStatus;
}
export interface BotDiagnostics {
  protocolVersion: 1;
  runtimeObservationOnly: true;
  connections: BotConnectionDiagnostic[];
}

const invalid = "Bot diagnostics response is unsupported; update the Preview host and bridge together";
const configSet = new Set<string>(configStatuses);
const runtimeSet = new Set<string>(runtimeStatuses);
const encoder = new TextEncoder();
function recordWithKeys(value: unknown, keys: readonly string[]): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    && Object.keys(value).length === keys.length && keys.every(key => Object.prototype.hasOwnProperty.call(value, key));
}

/** Do not turn configuration into delivery evidence or render raw SDK diagnostics. */
export function parseBotDiagnostics(value: unknown): BotDiagnostics {
  if (!recordWithKeys(value, ["protocolVersion", "runtimeObservationOnly", "connections"])
    || value.protocolVersion !== 1 || value.runtimeObservationOnly !== true
    || !Array.isArray(value.connections) || value.connections.length > 10_000) throw new Error(invalid);
  const ids = new Set<string>();
  const connections = value.connections.map((row: unknown): BotConnectionDiagnostic => {
    if (!recordWithKeys(row, ["id", "configStatus", "runtimeStatus"])
      || typeof row.id !== "string" || !row.id || encoder.encode(row.id).length > 256
      || /[\p{Cc}]/u.test(row.id) || ids.has(row.id)
      || typeof row.configStatus !== "string" || !configSet.has(row.configStatus)
      || typeof row.runtimeStatus !== "string" || !runtimeSet.has(row.runtimeStatus)) throw new Error(invalid);
    ids.add(row.id);
    return { id: row.id, configStatus: row.configStatus as BotConfigDiagnosticStatus,
      runtimeStatus: row.runtimeStatus as BotRuntimeDiagnosticStatus };
  });
  return { protocolVersion: 1, runtimeObservationOnly: true, connections };
}
