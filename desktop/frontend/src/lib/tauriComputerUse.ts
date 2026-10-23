import { invoke } from "@tauri-apps/api/core";

export interface ComputerUseStatus {
  protocolVersion: number;
  platform: string;
  available: boolean;
  accessibility: boolean;
  screenRecording: boolean;
}

export type ComputerUsePermission = "accessibility" | "screen_recording";

export function tauriComputerUseStatus(): Promise<ComputerUseStatus> {
  return invoke<ComputerUseStatus>("computer_use_status");
}

export function openComputerUsePermission(permission: ComputerUsePermission): Promise<void> {
  return invoke<void>("open_computer_use_permission", { permission });
}
