package main

import (
	"net/http"
)

type botConnectionDiagnosticView struct {
	ID            string `json:"id"`
	ConfigStatus  string `json:"configStatus"`
	RuntimeStatus string `json:"runtimeStatus"`
}

type botDiagnosticsView struct {
	ProtocolVersion        int                           `json:"protocolVersion"`
	RuntimeObservationOnly bool                          `json:"runtimeObservationOnly"`
	Connections            []botConnectionDiagnosticView `json:"connections"`
}

// A configuration check and a runtime observation are separate facts. The
// latter does not prove remote authentication, delivery or that the latest
// saved configuration has already been applied. Never forward SDK errors,
// labels, credential identities, environment names or workspace paths here.
func projectBotConnectionDiagnostics(settings botSettingsView, runtime botRuntimeStatusView) botDiagnosticsView {
	view := botDiagnosticsView{ProtocolVersion: 1, RuntimeObservationOnly: true, Connections: make([]botConnectionDiagnosticView, 0, len(settings.Channels))}
	claims := make(map[string]int, len(settings.Channels))
	for _, channel := range settings.Channels {
		id, _ := botDiagnosticAdapterScope(channel)
		claims[id]++
	}
	for _, channel := range settings.Channels {
		item := botConnectionDiagnosticView{ID: channel.ID, ConfigStatus: "configured", RuntimeStatus: "not_observed"}
		switch {
		case !channel.Enabled:
			item.ConfigStatus = "disabled"
		case !settings.Enabled:
			item.ConfigStatus = "bot_disabled"
		case channel.CredentialMissing || !channel.CredentialsSet:
			item.ConfigStatus = "missing_credentials"
		case !settings.AccessControlConfigured:
			item.ConfigStatus = "access_blocked"
		}
		if runtime.Refreshing {
			item.RuntimeStatus = "refreshing"
		} else if id, _ := botDiagnosticAdapterScope(channel); claims[id] != 1 {
			// A custom ID can collide with a legacy platform adapter ID.
			// Neither row can establish which configuration owned this runtime.
			item.RuntimeStatus = "unknown"
		} else {
			id, domain := botDiagnosticAdapterScope(channel)
			matches := 0
			for _, health := range runtime.AdapterHealth {
				if health.ID != id || string(health.Platform) != channel.Platform || health.Domain != domain {
					continue
				}
				matches++
				switch health.Status {
				case "configured", "disabled", "running", "error", "closed", "degraded":
					item.RuntimeStatus = health.Status
				default:
					item.RuntimeStatus = "unknown"
				}
			}
			if matches > 1 {
				item.RuntimeStatus = "unknown"
			}
		}
		view.Connections = append(view.Connections, item)
	}
	return view
}

func botDiagnosticAdapterScope(channel botChannelSettings) (id, domain string) {
	id, domain = channel.ID, channel.Domain
	// Only the four real legacy slots map to platform adapter IDs;
	// arbitrary persisted IDs must not alias another connection.
	switch channel.ID {
	case "legacy:qq":
		id = "qq"
	case "legacy:feishu":
		id = "feishu"
	case "legacy:weixin":
		id, domain = "weixin", "weixin"
	case "legacy:dingtalk":
		id, domain = "dingtalk", "dingtalk"
	}
	return
}

func (b *bridgeServer) botConnectionDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "bot diagnostics does not accept query parameters")
		return
	}
	settings, err := readBotSettings()
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to inspect bot configuration; check the saved configuration and retry")
		return
	}
	runtime := botRuntimeStatusView{}
	if b.botRuntime != nil {
		runtime = b.botRuntime.snapshot()
	}
	writeJSON(w, http.StatusOK, projectBotConnectionDiagnostics(settings, runtime))
}
