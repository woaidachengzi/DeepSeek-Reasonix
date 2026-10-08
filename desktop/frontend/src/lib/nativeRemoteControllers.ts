import { invoke } from "@tauri-apps/api/core";
import { RemoteControllerPool } from "./remoteControllerPool";
import type { BridgeRemoteControllerResponse, BridgeRemoteControllerSessionsResponse, BridgeRemoteControllerCloseResponse, BridgeRemoteControllerSessionViewResponse } from "./bridgeProtocol.generated";
import type { BridgeRemoteControllerSessionImageResponse } from "./bridgeProtocol.generated";

export const nativeRemoteControllers = new RemoteControllerPool({
  attach:request => invoke<BridgeRemoteControllerResponse>("bridge_remote_controller_attach",{request}),
  sessions:controllerId => invoke<BridgeRemoteControllerSessionsResponse>("bridge_remote_controller_sessions",{request:{controllerId}}),
  sessionView:(controllerId,sessionPath) => invoke<BridgeRemoteControllerSessionViewResponse>("bridge_remote_controller_session_view",{request:{controllerId,sessionPath}}),
  sessionImage:(controllerId,sessionPath,source) => invoke<BridgeRemoteControllerSessionImageResponse>("bridge_remote_controller_session_image",{request:{controllerId,sessionPath,source}}),
  close:controllerId => invoke<BridgeRemoteControllerCloseResponse>("bridge_remote_controller_close",{request:{controllerId}}),
});
