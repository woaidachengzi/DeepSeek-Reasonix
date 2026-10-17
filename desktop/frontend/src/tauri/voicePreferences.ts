type VoicePreferences = { recognizer: string; speaker: string; voice: string; spaceEnabled: boolean };
const KEY = "reasonix-tauri-voice-preferences-v1";

export function readVoicePreferences(): VoicePreferences {
  try {
    const value = JSON.parse(localStorage.getItem(KEY) ?? "{}");
    return {
      recognizer: typeof value?.recognizer === "string" ? value.recognizer : "",
      speaker: typeof value?.speaker === "string" ? value.speaker : "",
      voice: typeof value?.voice === "string" ? value.voice : "mimo_default",
      spaceEnabled: value?.spaceEnabled !== false,
    };
  } catch {
    return { recognizer: "", speaker: "", voice: "mimo_default", spaceEnabled: true };
  }
}

export function saveVoicePreferences(patch: Partial<VoicePreferences>) {
  try {
    localStorage.setItem(KEY, JSON.stringify({ ...readVoicePreferences(), ...patch }));
  } catch { /* Device preferences must not prevent voice use. */ }
}
