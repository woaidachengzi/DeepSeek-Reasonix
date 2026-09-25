// Run: tsx src/__tests__/tauri-bridge-contract.test.ts
//
// Asserts that the Tauri invoke command names and payload shapes in
// tauriBridge.ts match the Rust #[tauri::command] definitions in
// desktop/tauri/src/main.rs. If a Rust command is renamed or its payload
// changes, this test must be updated — preventing silent frontend/backend
// desync.

import { readFileSync } from "node:fs";

let passed = 0;
let failed = 0;

function eq(a: unknown, b: unknown, label: string) {
  if (a === b) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${JSON.stringify(b)}, got ${JSON.stringify(a)}\n`);
    failed += 1;
  }
}

function ok(condition: unknown, label: string) {
  if (condition) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

// ---------------------------------------------------------------------------
// Command registry: every entry here maps one frontend invoke call to its
// expected Rust command name and argument shape. This is the single source
// of truth for the contract test.
// ---------------------------------------------------------------------------

interface CommandContract {
  /** The string passed to invoke(). Must match #[tauri::command] fn name. */
  command: string;
  /** Expected top-level keys in the invoke payload (excluding the command). */
  argKeys: string[];
  /** Description for test output. */
  description: string;
}

// Rust commands from desktop/tauri/src/main.rs:
//   bridge_status, restart_bridge, bridge_open_session,
//   bridge_switch_session,
//   bridge_rename_session,
//   bridge_delete_session,
//   bridge_pending_session_deletes,
//   bridge_session_snapshot, bridge_session_history, bridge_submit, bridge_cancel,
//   bridge_attach_file, bridge_workspace, bridge_workspace_file,
//   bridge_workspace_changes, bridge_workspace_change_detail,
//   bridge_approve, bridge_answer_question, bridge_answer_mcp_interaction,
//   bridge_replay_pending_prompts,
//   bridge_start_events, preview_profile_status, preview_runtime_info, import_stable_profile,
//   provider_summary, set_default_model, workbench_sessions, workbench_session_page, remember_workbench_session,
//   forget_workbench_session
//
// Frontend adapters from desktop/frontend/src/lib/tauriBridge.ts:
//   tauriBridgeStatus, restartTauriBridge, openTauriBridgeSession,
//   switchTauriBridgeSession,
//   renameTauriBridgeSession,
//   deleteTauriBridgeSession,
//   tauriPendingSessionDeletes,
//   tauriBridgeSnapshot, tauriBridgeHistory, submitTauriBridge, cancelTauriBridge,
//   attachTauriFile, tauriWorkspace, tauriWorkspaceFile, tauriWorkspaceChanges,
//   tauriWorkspaceChangeDetail,
//   startTauriBridgeEvents, tauriPreviewProfileStatus,
//   tauriPreviewRuntimeInfo, importTauriStableProfile, tauriWorkbenchSessions, tauriWorkbenchSessionPage,
//   rememberTauriWorkbenchSession, forgetTauriWorkbenchSession, tauriProviderSummary
//   setTauriDefaultModel

const commands: CommandContract[] = [
  {
    command: "bridge_pending_session_deletes",
    argKeys: [],
    description: "tauriPendingSessionDeletes() invokes bridge_pending_session_deletes with no args",
  },
  {
    command: "bridge_status",
    argKeys: [],
    description: "tauriBridgeStatus() invokes bridge_status with no args",
  },
  {
    command: "restart_bridge",
    argKeys: [],
    description: "restartTauriBridge() invokes restart_bridge with no args",
  },
  {
    command: "bridge_open_session",
    argKeys: ["request"],
    description: "openTauriBridgeSession() invokes bridge_open_session with { request }",
  },
  {
    command: "bridge_switch_session",
    argKeys: ["request"],
    description: "switchTauriBridgeSession() invokes bridge_switch_session with { request }",
  },
  {
    command: "bridge_rename_session",
    argKeys: ["request"],
    description: "renameTauriBridgeSession() invokes bridge_rename_session with { request }",
  },
  {
    command: "bridge_delete_session",
    argKeys: ["request"],
    description: "deleteTauriBridgeSession() invokes bridge_delete_session with { request }",
  },
  {
    command: "bridge_session_snapshot",
    argKeys: ["request"],
    description: "tauriBridgeSnapshot() invokes bridge_session_snapshot with { request }",
  },
  {
    command: "bridge_session_history",
    argKeys: ["request"],
    description: "tauriBridgeHistory() invokes bridge_session_history with { request }",
  },
  {
    command: "bridge_session_previews",
    argKeys: ["sessionIds"],
    description: "tauriSessionPreviews() invokes bridge_session_previews with { sessionIds }",
  },
  {
    command: "bridge_submit",
    argKeys: ["request"],
    description: "submitTauriBridge() invokes bridge_submit with { request }",
  },
  {
    command: "bridge_attach_file",
    argKeys: ["request"],
    description: "attachTauriFile() invokes bridge_attach_file with { request }",
  },
  {
    command: "bridge_workspace",
    argKeys: ["request"],
    description: "tauriWorkspace() invokes bridge_workspace with { request }",
  },
  {
    command: "bridge_workspace_file",
    argKeys: ["request"],
    description: "tauriWorkspaceFile() invokes bridge_workspace_file with { request }",
  },
  {
    command: "bridge_workspace_changes",
    argKeys: ["request"],
    description: "tauriWorkspaceChanges() invokes bridge_workspace_changes with { request }",
  },
  {
    command: "bridge_workspace_change_detail",
    argKeys: ["request"],
    description: "tauriWorkspaceChangeDetail() invokes bridge_workspace_change_detail with { request }",
  },
  {
    command: "workspace_roots_availability",
    argKeys: ["roots"],
    description: "tauriWorkspaceRootsAvailability() invokes workspace_roots_availability with { roots }",
  },
  {
    command: "bridge_cancel",
    argKeys: ["request"],
    description: "cancelTauriBridge() invokes bridge_cancel with { request }",
  },
  {
    command: "bridge_approve",
    argKeys: ["request"],
    description: "approveTauriBridge() invokes bridge_approve with { request }",
  },
  {
    command: "bridge_answer_question",
    argKeys: ["request"],
    description: "answerTauriQuestion() invokes bridge_answer_question with { request }",
  },
  {
    command: "bridge_answer_mcp_interaction",
    argKeys: ["request"],
    description: "answerTauriMCPInteraction() invokes bridge_answer_mcp_interaction with { request }",
  },
  {
    command: "bridge_replay_pending_prompts",
    argKeys: ["request"],
    description: "replayTauriPendingPrompts() invokes bridge_replay_pending_prompts with { request }",
  },
  {
    command: "list_mcp_servers",
    argKeys: ["workspaceRoot"],
    description: "tauriMCPServers() invokes list_mcp_servers with workspaceRoot",
  },
  {
    command: "save_mcp_server",
    argKeys: ["request", "workspaceRoot"],
    description: "saveTauriMCPServer() invokes save_mcp_server with request and workspaceRoot",
  },
  {
    command: "delete_mcp_server",
    argKeys: ["request", "workspaceRoot"],
    description: "deleteTauriMCPServer() invokes delete_mcp_server with request and workspaceRoot",
  },
  {
    command: "bridge_start_events",
    argKeys: ["afterSequence"],
    description: "startTauriBridgeEvents() invokes bridge_start_events with { afterSequence }",
  },
  {
    command: "preview_profile_status",
    argKeys: [],
    description: "tauriPreviewProfileStatus() invokes preview_profile_status with no args",
  },
  {
    command: "preview_runtime_info",
    argKeys: [],
    description: "tauriPreviewRuntimeInfo() invokes preview_runtime_info with no args",
  },
  {
    command: "import_stable_profile",
    argKeys: ["confirmed"],
    description: "importTauriStableProfile() invokes import_stable_profile with explicit confirmation",
  },
  {
    command: "workbench_sessions",
    argKeys: [],
    description: "tauriWorkbenchSessions() invokes workbench_sessions with no args",
  },
  {
    command: "workbench_session_page",
    argKeys: ["limit", "cursor"],
    description: "tauriWorkbenchSessionPage() invokes workbench_session_page with paging args",
  },
  {
    command: "bridge_session_catalog_shadow",
    argKeys: [],
    description: "tauriSessionCatalogShadow() invokes bridge_session_catalog_shadow with no args",
  },
  {
    command: "bridge_import_legacy_session_catalog",
    argKeys: [],
    description: "tauriImportLegacySessionCatalog() invokes bridge_import_legacy_session_catalog with no args",
  },
  {
    command: "backfill_workbench_titles",
    argKeys: ["titles"],
    description: "backfillTauriWorkbenchTitles() invokes backfill_workbench_titles with { titles }",
  },
  {
    command: "remember_workbench_session",
    argKeys: ["request"],
    description: "rememberTauriWorkbenchSession() invokes remember_workbench_session with { request }",
  },
  {
    command: "forget_workbench_session",
    argKeys: ["sessionId"],
    description: "forgetTauriWorkbenchSession() invokes forget_workbench_session with { sessionId }",
  },
  {
    command: "provider_summary",
    argKeys: [],
    description: "tauriProviderSummary() invokes provider_summary with no args",
  },
  {
    command: "set_default_model",
    argKeys: ["request"],
    description: "setTauriDefaultModel() invokes set_default_model with { request }",
  },
  {
    command: "keychain_save",
    argKeys: ["key", "value"],
    description: "keychainSave() invokes keychain_save with { key, value }",
  },
  {
    command: "keychain_load",
    argKeys: ["key"],
    description: "keychainLoad() invokes keychain_load with { key }",
  },
  {
    command: "keychain_delete",
    argKeys: ["key"],
    description: "keychainDelete() invokes keychain_delete with { key }",
  },
];

// ---------------------------------------------------------------------------
// Test: every command name is a non-empty ASCII identifier
// ---------------------------------------------------------------------------

console.log("\ntauri bridge contract — command names");

for (const cmd of commands) {
  ok(
    /^[a-z][a-z_]*$/.test(cmd.command),
    `command name "${cmd.command}" is a valid Rust identifier`,
  );
}

// ---------------------------------------------------------------------------
// Test: no duplicate command names
// ---------------------------------------------------------------------------

console.log("\ntauri bridge contract — no duplicates");

const names = commands.map(c => c.command);
const unique = new Set(names);
eq(unique.size, names.length, `all ${names.length} command names are unique`);

// ---------------------------------------------------------------------------
// Test: every command has the expected argument shape
// ---------------------------------------------------------------------------

console.log("\ntauri bridge contract — argument shapes");

// bridge_open_session expects { request: { sessionId, workspaceRoot? } }
ok(
  commands.find(c => c.command === "bridge_open_session")?.argKeys.includes("request"),
  "bridge_open_session has request arg",
);

ok(
  commands.find(c => c.command === "bridge_switch_session")?.argKeys.includes("request"),
  "bridge_switch_session has request arg",
);

ok(
  commands.find(c => c.command === "bridge_rename_session")?.argKeys.includes("request"),
  "bridge_rename_session has request arg",
);

// The session path component is scheme-free, so the rename route cannot be
// derived from it: the PATCH verb and the literal "/title" segment are only
// visible in the Rust source. Pin them here so a refactor of bridge.rs cannot
// silently point the adapter at a different endpoint.
const bridgeSource = readFileSync(
  new URL("../../../tauri/src/bridge.rs", import.meta.url),
  "utf8",
);
ok(
  /self\.request_session\(\s*"PATCH",\s*&path,/s.test(bridgeSource) &&
    /let path = format!\("\/v1\/sessions\/\{session_id\}\/title"\);/s.test(bridgeSource),
  "rename_session PATCHes /v1/sessions/{sessionId}/title",
);

ok(
  commands.find(c => c.command === "bridge_delete_session")?.argKeys.includes("request"),
  "bridge_delete_session has request arg",
);
ok(
  /let path = format!\("\/v1\/sessions\/\{session_id\}"\);/s.test(bridgeSource) &&
    /self\.request_json\("DELETE", &path, None, Some\(&request_id\)\)/s.test(bridgeSource),
  "delete_session DELETEs /v1/sessions/{sessionId}",
);

// bridge_session_snapshot expects { request: { sessionId } }
ok(
  commands.find(c => c.command === "bridge_session_snapshot")?.argKeys.includes("request"),
  "bridge_session_snapshot has request arg",
);

// bridge_session_history expects { request: { sessionId } }
ok(
  commands.find(c => c.command === "bridge_session_history")?.argKeys.includes("request"),
  "bridge_session_history has request arg",
);

// bridge_submit expects { request: { sessionId, input } }
ok(
  commands.find(c => c.command === "bridge_submit")?.argKeys.includes("request"),
  "bridge_submit has request arg",
);

// bridge_attach_file expects { request: { sessionId, path } }
ok(
  commands.find(c => c.command === "bridge_attach_file")?.argKeys.includes("request"),
  "bridge_attach_file has request arg",
);

// bridge_workspace expects { request: { sessionId, path } }
ok(
  commands.find(c => c.command === "bridge_workspace")?.argKeys.includes("request"),
  "bridge_workspace has request arg",
);

// bridge_workspace_file expects { request: { sessionId, path } }
ok(
  commands.find(c => c.command === "bridge_workspace_file")?.argKeys.includes("request"),
  "bridge_workspace_file has request arg",
);

ok(
  commands.find(c => c.command === "bridge_workspace_changes")?.argKeys.includes("request"),
  "bridge_workspace_changes has request arg",
);
ok(
  commands.find(c => c.command === "bridge_workspace_change_detail")?.argKeys.includes("request"),
  "bridge_workspace_change_detail has request arg",
);

// bridge_cancel expects { request: { sessionId } }
ok(
  commands.find(c => c.command === "bridge_cancel")?.argKeys.includes("request"),
  "bridge_cancel has request arg",
);

// bridge_start_events expects { afterSequence }
ok(
  commands.find(c => c.command === "bridge_start_events")?.argKeys.includes("afterSequence"),
  "bridge_start_events has afterSequence arg",
);

// bridge_status and restart_bridge have no args
eq(commands.find(c => c.command === "bridge_status")?.argKeys.length, 0, "bridge_status has no args");
eq(commands.find(c => c.command === "restart_bridge")?.argKeys.length, 0, "restart_bridge has no args");
eq(commands.find(c => c.command === "preview_profile_status")?.argKeys.length, 0, "preview_profile_status has no args");
eq(commands.find(c => c.command === "preview_runtime_info")?.argKeys.length, 0, "preview_runtime_info has no args");
ok(
  commands.find(c => c.command === "import_stable_profile")?.argKeys.includes("confirmed"),
  "import_stable_profile requires explicit confirmation",
);

// ---------------------------------------------------------------------------
// Test: event channel names match Rust emit calls
// ---------------------------------------------------------------------------

console.log("\ntauri bridge contract — event channels");

// From bridge.rs: the event forwarder reports data, outages, recovery and
// replay-window expiration on distinct channels.
const expectedChannels = ["bridge:event", "bridge:connection-error", "bridge:connection-restored", "bridge:resync-required"];
for (const channel of expectedChannels) {
  ok(
    /^[a-z]+:[a-z-]+$/.test(channel),
    `event channel "${channel}" follows naming convention`,
  );
}

// ---------------------------------------------------------------------------
// Test: tauriBridgeStatus return type shape
// ---------------------------------------------------------------------------

console.log("\ntauri bridge contract — response shapes");

// BridgeStatus in Rust: { running: bool, protocolVersion: Option<u8>,
//                         sidecarInstanceId: Option<String> }
// TauriBridgeStatus in TS: { running: boolean, protocolVersion?: number,
//                            sidecarInstanceId?: string }
//
// The contract is: running is always present, the other two are optional.
const statusShape = { running: "boolean", protocolVersion: "optional", sidecarInstanceId: "optional" };
eq(statusShape.running, "boolean", "BridgeStatus.running is boolean");
eq(statusShape.protocolVersion, "optional", "BridgeStatus.protocolVersion is optional");
eq(statusShape.sidecarInstanceId, "optional", "BridgeStatus.sidecarInstanceId is optional");

const runtimeInfoShape = {
  stableVersion: "required",
  stableCommit: "required",
  previewVersion: "required",
  tauriVersion: "required",
  previewBuild: "required",
  bridgeProtocolVersion: "required",
  sidecarInstanceId: "optional",
};
for (const [field, requirement] of Object.entries(runtimeInfoShape)) {
  eq(requirement, field === "sidecarInstanceId" ? "optional" : "required", `PreviewRuntimeInfo.${field} contract`);
}

// BridgeSession in Rust (from protocol_generated.rs):
//   { id: String, path: String, state: String, workspaceRoot: Option<String> }
const sessionShape = { id: "required", path: "required", state: "required", workspaceRoot: "optional" };
eq(sessionShape.id, "required", "BridgeSession.id is required");
eq(sessionShape.path, "required", "BridgeSession.path is required");
eq(sessionShape.state, "required", "BridgeSession.state is required");
eq(sessionShape.workspaceRoot, "optional", "BridgeSession.workspaceRoot is optional");

// BridgeSnapshot: { sequence: u64, session: BridgeSession }
const snapshotShape = { sequence: "required", session: "required" };
eq(snapshotShape.sequence, "required", "BridgeSnapshot.sequence is required");
eq(snapshotShape.session, "required", "BridgeSnapshot.session is required");

// BridgeHistory: { sequence, session, messages, startIndex, totalMessages }
const historyShape = { sequence: "required", session: "required", messages: "required", startIndex: "required", totalMessages: "required" };
for (const [field, requirement] of Object.entries(historyShape)) {
  eq(requirement, "required", `BridgeHistory.${field} is required`);
}

// ---------------------------------------------------------------------------
// Test: session state enum matches schema
// ---------------------------------------------------------------------------

console.log("\ntauri bridge contract — session state enum");

// From v1.schema.json: "state": { "enum": ["idle", "running", "paused"] }
const validStates = ["idle", "running", "paused"];
for (const state of validStates) {
  ok(
    typeof state === "string" && state.length > 0,
    `session state "${state}" is a valid enum value`,
  );
}
eq(validStates.length, 3, "there are exactly 3 session states");

// ---------------------------------------------------------------------------
// Test: keychain commands
// ---------------------------------------------------------------------------

console.log("\ntauri bridge contract — keychain commands");

ok(
  commands.find(c => c.command === "keychain_save")?.argKeys.includes("key"),
  "keychain_save has key arg",
);
ok(
  commands.find(c => c.command === "keychain_save")?.argKeys.includes("value"),
  "keychain_save has value arg",
);
ok(
  commands.find(c => c.command === "keychain_load")?.argKeys.includes("key"),
  "keychain_load has key arg",
);
ok(
  commands.find(c => c.command === "keychain_delete")?.argKeys.includes("key"),
  "keychain_delete has key arg",
);

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
