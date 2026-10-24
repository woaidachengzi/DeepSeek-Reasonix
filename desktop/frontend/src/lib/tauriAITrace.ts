import { invoke } from "@tauri-apps/api/core";

export function exportTauriAITrace(sessionId: string, detailed: boolean): Promise<boolean> {
  return invoke<boolean>("export_ai_trace", { request: { sessionId }, detailed });
}
