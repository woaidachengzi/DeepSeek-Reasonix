import assert from "node:assert/strict";
import type { CapabilityDiagnosticsReport } from "../lib/types";
import { assessSkillRequirements } from "../tauri/tauriSkillReadiness";

function report(runtimeStatus = "connected"): CapabilityDiagnosticsReport {
  return {
    schema_version: 1,
    root: "<workspace>",
    live: false,
    summary: { errors: 0, warnings: 0, infos: 0, instructions: 0, skills: 0, commands: 0, hooks: 0, plugins: 0, mcp_servers: 1 },
    instructions: { docs: [] },
    skills: { roots: [], entries: [], winners: 0, shadowed: 0 },
    commands: { roots: [], entries: [], winners: 0, shadowed: 0 },
    hooks: { trusted_project: false, project_defines_hooks: false, sources: [], entries: [] },
    plugins: { packages: [] },
    mcp: { servers: [{ name: "github", effective: true, transport: "stdio", start_intent: "automatic", runtime_status: runtimeStatus, tools: [{ name: "search_issues" }] }] },
    issues: [],
  };
}

assert.deepEqual(
  assessSkillRequirements(["mcp-server:github", "mcp-tool:github/search_issues"], report()),
  [
    { requirement: "mcp-server:github", state: "ready" },
    { requirement: "mcp-tool:github/search_issues", state: "ready" },
  ],
);
assert.equal(assessSkillRequirements(["mcp-server:github"], null)[0]?.state, "unknown", "no matching runtime report never claims readiness");
assert.equal(assessSkillRequirements(["mcp-server:missing"], report())[0]?.state, "missing");
assert.equal(assessSkillRequirements(["mcp-server:github"], report("failed"))[0]?.state, "failed");
assert.equal(assessSkillRequirements(["mcp-server:github"], report(""))[0]?.state, "unknown", "static configuration is not reported as runtime-ready");
assert.equal(assessSkillRequirements(["mcp-tool:github/missing"], report())[0]?.state, "missing");
const disabled = report();
disabled.mcp.servers[0]!.effective = false;
assert.equal(assessSkillRequirements(["mcp-server:github"], disabled)[0]?.state, "disabled");
console.log("tauri skill readiness tests passed");
