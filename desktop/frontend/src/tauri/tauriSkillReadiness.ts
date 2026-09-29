import type { CapabilityDiagnosticsReport } from "../lib/types";

export type SkillRequirementState = "ready" | "missing" | "disabled" | "failed" | "unknown";

export type SkillRequirementReadiness = {
  requirement: string;
  state: SkillRequirementState;
};

export function assessSkillRequirements(
  requirements: string[],
  report: CapabilityDiagnosticsReport | null,
): SkillRequirementReadiness[] {
  return requirements.map(requirement => ({ requirement, state: assessOne(requirement.trim(), report) }));
}

function assessOne(requirement: string, report: CapabilityDiagnosticsReport | null): SkillRequirementState {
  if (!report || !requirement) return "unknown";
  if (requirement.startsWith("mcp-server:")) {
    const name = requirement.slice("mcp-server:".length).trim();
    if (!name) return "missing";
    const server = report.mcp.servers.find(candidate => candidate.name === name);
    if (!server) return "missing";
    if (!server.effective || server.start_intent === "off" || server.runtime_status === "disabled") return "disabled";
    if (server.runtime_status === "failed") return "failed";
    return server.runtime_status === "connected" ? "ready" : "unknown";
  }
  if (requirement.startsWith("mcp-tool:")) {
    const capability = requirement.slice("mcp-tool:".length);
    const separator = capability.indexOf("/");
    if (separator <= 0 || separator === capability.length - 1) return "missing";
    const name = capability.slice(0, separator).trim();
    const tool = capability.slice(separator + 1).trim();
    const server = report.mcp.servers.find(candidate => candidate.name === name);
    if (!server) return "missing";
    if (!server.effective || server.start_intent === "off" || server.runtime_status === "disabled") return "disabled";
    if (server.runtime_status === "failed") return "failed";
    if (server.runtime_status !== "connected") return "unknown";
    return server.tools?.some(candidate => candidate.name === tool) ? "ready" : "missing";
  }
  return "unknown";
}
