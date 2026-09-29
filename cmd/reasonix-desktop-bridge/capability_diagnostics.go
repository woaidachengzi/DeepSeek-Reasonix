package main

import (
	"net/http"
	"strings"

	"reasonix/internal/capdiag"
	"reasonix/internal/desktopbridge"
)

func (b *bridgeServer) capabilityDiagnostics(w http.ResponseWriter, r *http.Request) {
	root, err := normalizeSkillsWorkspace(r.URL.Query().Get("workspaceRoot"))
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_workspace", "workspace root is invalid")
		return
	}
	includeRuntime := r.URL.Query().Get("includeSessionRuntime") == "true"
	opts := capdiag.Options{Root: root}
	if !includeRuntime {
		writeJSON(w, http.StatusOK, capdiag.Collect(opts))
		return
	}
	if b.runtimes == nil {
		writeJSON(w, http.StatusOK, capdiag.CollectWithRuntimeUnavailable(opts))
		return
	}
	servers, available := b.runtimes.MCPStatus(root)
	if !available {
		writeJSON(w, http.StatusOK, capdiag.CollectWithRuntimeUnavailable(opts))
		return
	}
	view := capdiag.Collect(opts)
	mergeBridgeMCPDiagnostics(&view, servers)
	writeJSON(w, http.StatusOK, view)
}

// mergeBridgeMCPDiagnostics adds only the bounded, display-safe Host metadata
// that the desktop bridge already exposes. It never includes schemas,
// arguments, results, raw startup errors, or credentials.
func mergeBridgeMCPDiagnostics(report *capdiag.Report, servers []desktopbridge.MCPRuntimeServer) {
	if report == nil {
		return
	}
	byName := make(map[string]int, len(report.MCP.Servers))
	for i := range report.MCP.Servers {
		byName[report.MCP.Servers[i].Name] = i
	}
	for _, server := range servers {
		name := strings.TrimSpace(server.Name)
		if name == "" {
			continue
		}
		index, ok := byName[name]
		if !ok {
			report.MCP.Servers = append(report.MCP.Servers, capdiag.MCPServerInfo{
				Name: name, Source: "host_session", Effective: true, Transport: "host",
				StartIntent: "automatic",
			})
			index = len(report.MCP.Servers) - 1
			byName[name] = index
		}
		entry := &report.MCP.Servers[index]
		entry.RuntimeStatus = bridgeMCPDiagnosticStatus(server.Status)
		entry.ToolCount = server.ToolCount
		entry.Tools = make([]capdiag.MCPToolInfo, 0, len(server.Tools))
		for _, tool := range server.Tools {
			if strings.TrimSpace(tool.Name) == "" {
				continue
			}
			entry.Tools = append(entry.Tools, capdiag.MCPToolInfo{Name: tool.Name})
		}
		if server.Status == "failed" {
			entry.StartupStage = server.ErrorKind
			report.Issues = append(report.Issues, capdiag.Issue{
				Severity: "error", Code: "mcp.start_failed", Subsystem: "mcp", Name: name,
				Message:     "MCP server failed in the current session",
				Remediation: "Inspect server configuration and authentication, then retry from Settings → MCP",
				SettingsTab: "mcp",
			})
		}
	}
}

func bridgeMCPDiagnosticStatus(status string) string {
	switch status {
	case "connected":
		return "connected"
	case "failed":
		return "failed"
	case "initializing":
		return "deferred"
	default:
		return ""
	}
}
