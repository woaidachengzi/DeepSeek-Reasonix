const STORAGE_KEY = "reasonix.tauri.default-workspace.v1";

function validWorkspacePath(value: string): boolean {
  return value.length > 0 && value.length <= 4096 && !/[\x00-\x1f\x7f]/.test(value) &&
    (value.startsWith("/") || /^[a-zA-Z]:[\\/]/.test(value) || value.startsWith("\\\\"));
}

export function getTauriDefaultWorkspace(): string {
  try {
    const stored = localStorage.getItem(STORAGE_KEY) ?? "";
    return validWorkspacePath(stored) ? stored : "";
  } catch { return ""; }
}

export function setTauriDefaultWorkspace(path: string): string {
  if (path && !validWorkspacePath(path)) throw new Error("默认工作区必须是有效的绝对路径。");
  if (path) localStorage.setItem(STORAGE_KEY, path);
  else localStorage.removeItem(STORAGE_KEY);
  return path;
}
