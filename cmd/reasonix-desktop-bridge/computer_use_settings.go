package main

import (
	"encoding/json"
	"net/http"
	"runtime"

	"reasonix/internal/computeruse"
	"reasonix/internal/desktopbridge"
)

type computerUseStatusView struct {
	ProtocolVersion int             `json:"protocolVersion"`
	Platform        string          `json:"platform"`
	Available       bool            `json:"available"`
	Accessibility   bool            `json:"accessibility"`
	ScreenRecording bool            `json:"screenRecording"`
	Linux           json.RawMessage `json:"linux,omitempty"`
}

// Read permission readiness from the same process that dispatches desktop input.
// Status never prompts, captures a screen, lists apps, or sends input.
func (b *bridgeServer) computerUseStatus(w http.ResponseWriter, r *http.Request) {
	view := computerUseStatusView{ProtocolVersion: desktopbridge.ProtocolVersion, Platform: runtime.GOOS, Available: computeruse.Available()}
	if view.Available {
		result, err := computeruse.ReadReadiness(r.Context())
		var state struct {
			Accessibility   bool `json:"accessibility"`
			ScreenRecording bool `json:"screen_recording"`
		}
		if err != nil || json.Unmarshal(result, &state) != nil {
			writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read desktop control permissions")
			return
		}
		view.Accessibility, view.ScreenRecording = state.Accessibility, state.ScreenRecording
		if runtime.GOOS == "linux" {
			// An X11 connection does not prove native Wayland capture/input.
			view.Linux = result
		}
	}
	writeJSON(w, http.StatusOK, view)
}
