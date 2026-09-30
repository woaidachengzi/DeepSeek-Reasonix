import { invoke } from "@tauri-apps/api/core";
import type { ExternalOpenerBridge } from "../components/ExternalOpener";
import { app } from "../lib/bridge";

/** Session identity crosses the renderer boundary; the host resolves its root. */
export const tauriExternalOpenerBridge: ExternalOpenerBridge = {
  ExternalOpenersForTab: sessionId => invoke("workspace_external_openers", { sessionId }),
  SetPreferredExternalOpener: id => app.SetPreferredExternalOpener(id),
  OpenWorkspaceInExternalOpenerForTab: (sessionId, id) => invoke("open_workspace_external", { sessionId, id }),
};
