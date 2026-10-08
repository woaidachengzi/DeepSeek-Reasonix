import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import type { RemoteSubscriptionTransport } from "./remoteSessionSubscription";

// No Wails/browser fallback, renderer URL, cookie or native bearer argument.
export const nativeRemoteSessionTransport: RemoteSubscriptionTransport = {
  listen: (name,callback) => listen(name,event => callback(event.payload)),
  subscribe: request => invoke("bridge_remote_controller_subscribe",{request}),
  unsubscribe: subscriptionId => invoke("bridge_remote_controller_unsubscribe",{request:{subscriptionId}}),
};
