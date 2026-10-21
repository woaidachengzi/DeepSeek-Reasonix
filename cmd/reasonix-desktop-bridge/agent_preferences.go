package main

import (
	"net/http"

	appconfig "reasonix/internal/config"
)

func (b *bridgeServer) setAgentPreferences(w http.ResponseWriter, r *http.Request) {
	var request setAgentPreferenceRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid agent preference request")
		return
	}
	// Keep persistence and publication in the same order for concurrent edits.
	b.agentPreferencesMu.Lock()
	defer b.agentPreferencesMu.Unlock()
	if err := persistAgentPreferences(request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "agent preference could not be saved")
		return
	}
	if request.CompactRatioPercent != 0 {
		compactPreferenceRevision.Add(1)
	}
	if request.CompactRatioPercent != 0 && b.runtimes != nil {
		// Reload each resident project's effective configuration so a global
		// edit preserves project overrides and background conversations update
		// without being selected, restarted, or losing their current work.
		for _, session := range b.runtimes.Snapshots() {
			if effective, err := appconfig.LoadForRootWithoutCredentialsReadOnly(session.WorkspaceRoot); err == nil {
				b.runtimes.SetCompactRatio(session.ID, effective.Agent.CompactRatio)
			}
		}
	}
	if request.ReasoningLanguage != "" && request.Scope != "project" && b.runtimes != nil {
		if session, active := b.runtimes.Snapshot(); active && session.WorkspaceRoot != "" {
			if effective, err := appconfig.LoadForRootWithoutCredentialsReadOnly(session.WorkspaceRoot); err == nil {
				b.runtimes.SetReasoningLanguage(session.ID, effective.ReasoningLanguage())
			}
		}
	}
	summary, err := loadProviderSummaryForScope(request.Scope, request.WorkspaceRoot)
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read agent preferences")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
