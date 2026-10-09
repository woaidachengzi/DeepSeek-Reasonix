import { invoke } from "@tauri-apps/api/core";
import { nativeRemoteSessionTransport } from "./nativeRemoteSessionSubscription";
import type { RemoteSnapshotTransport } from "./remoteSessionSnapshot";

export const nativeRemoteSnapshotTransport: RemoteSnapshotTransport = {
  ...nativeRemoteSessionTransport,
  snapshot: (subscriptionId, continuation) => invoke("bridge_remote_controller_snapshot", {
    request: continuation === undefined ? { subscriptionId } : { subscriptionId, continuation },
  }),
};
