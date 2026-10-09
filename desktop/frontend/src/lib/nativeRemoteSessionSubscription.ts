import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import type { RemoteSubscriptionTransport } from "./remoteSessionSubscription";

// No Wails/browser fallback, renderer URL, cookie or native bearer argument.
export const nativeRemoteSessionTransport: RemoteSubscriptionTransport = {
  listen: (name,callback) => {
    if (name !== "bridge:remote-session-state" && name !== "bridge:remote-session-event") return Promise.reject(new Error("invalid remote subscription event"));
    return listen(name,event => callback(event.payload),{target:"main"});
  },
  subscribe: request => invoke("bridge_remote_controller_subscribe",{request:{controllerId:request.controllerId,sessionPath:request.sessionPath,surfaceId:request.surfaceId,generation:request.generation}}),
  unsubscribe: subscriptionId => invoke("bridge_remote_controller_unsubscribe",{request:{subscriptionId}}),
};
