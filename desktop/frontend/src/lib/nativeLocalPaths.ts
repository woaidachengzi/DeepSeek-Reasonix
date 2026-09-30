import { invoke, isTauri } from "@tauri-apps/api/core";
import type { AppBindings } from "./bridge";

type LocalPathBindings = Pick<AppBindings, "OpenLocalPath" | "RevealPath" | "SaveLocalPathAs" | "ExternalOpeners" | "SetPreferredExternalOpener" | "OpenLocalPathInExternalOpener">;

const bindings: LocalPathBindings = {
  OpenLocalPath: path => invoke<void>("open_local_path", { path }),
  RevealPath: path => invoke<void>("reveal_local_path", { path }),
  SaveLocalPathAs: path => invoke<string>("save_local_path_as", { path }),
  SetPreferredExternalOpener: id => invoke<void>("set_preferred_external_opener", { id }),
  ExternalOpeners: () => invoke("local_path_openers"),
  OpenLocalPathInExternalOpener: (path, id) => invoke<void>("open_local_path_with", { path, id }),
};

/** Resolve host methods before the browser mock, including late injection. */
export function nativeLocalPathBinding(method: string): LocalPathBindings[keyof LocalPathBindings] | undefined {
  if (!isTauri() || !Object.prototype.hasOwnProperty.call(bindings, method)) return undefined;
  return bindings[method as keyof LocalPathBindings];
}
