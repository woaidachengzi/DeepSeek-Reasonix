import { invoke } from "@tauri-apps/api/core";
import { RemoteControllerPool } from "./remoteControllerPool";
import type { BridgeRemoteControllerResponse, BridgeRemoteControllerSessionsResponse, BridgeRemoteControllerCloseResponse } from "./bridgeProtocol.generated";

export const nativeRemoteControllers = new RemoteControllerPool({
  attach:request => invoke<BridgeRemoteControllerResponse>("bridge_remote_controller_attach",{request}),
  sessions:controllerId => invoke<BridgeRemoteControllerSessionsResponse>("bridge_remote_controller_sessions",{request:{controllerId}}),
  close:controllerId => invoke<BridgeRemoteControllerCloseResponse>("bridge_remote_controller_close",{request:{controllerId}}),
});
