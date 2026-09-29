package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/capdiag"
	"reasonix/internal/desktopbridge"
)

func TestPreviewCapabilityDiagnosticsIsAuthenticatedAndReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	root := filepath.Join(home, "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServer(testToken, "instance-diagnostics")
	path := "/v1/settings/diagnostics/capabilities?workspaceRoot=" + url.QueryEscape(root)
	unauthorized := httptest.NewRecorder()
	bridge.handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, path, nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, path+"&includeSessionRuntime=true", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("diagnostics response: %d %s", response.Code, response.Body.String())
	}
	var report capdiag.Report
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != capdiag.SchemaVersion || report.Root != "<workspace>" || report.Issues == nil {
		t.Fatalf("unexpected Preview diagnostics: %+v", report)
	}
	if !strings.Contains(response.Body.String(), "mcp.runtime_unavailable") {
		t.Fatalf("missing explicit no-session-runtime diagnostic: %s", response.Body.String())
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "project" {
		t.Fatalf("read-only diagnostics created files in Preview home: %+v", entries)
	}
}

func TestMergeBridgeMCPDiagnosticsUsesOnlyDisplaySafeRuntimeFields(t *testing.T) {
	report := capdiag.Report{MCP: capdiag.MCPReport{Servers: []capdiag.MCPServerInfo{{Name: "configured", Transport: "stdio"}}}}
	mergeBridgeMCPDiagnostics(&report, []desktopbridge.MCPRuntimeServer{
		{Name: "configured", Status: "connected", ToolCount: 1, Tools: []desktopbridge.MCPRuntimeTool{{Name: "search", Description: "private description"}}},
		{Name: "session-only", Status: "failed", ErrorKind: "initialize", ToolCount: 0},
	})
	if report.MCP.Servers[0].RuntimeStatus != "connected" || report.MCP.Servers[0].Tools[0].Name != "search" {
		t.Fatalf("configured runtime metadata missing: %+v", report.MCP.Servers[0])
	}
	if len(report.MCP.Servers) != 2 || report.MCP.Servers[1].Source != "host_session" || report.MCP.Servers[1].RuntimeStatus != "failed" {
		t.Fatalf("session-only server not represented: %+v", report.MCP.Servers)
	}
	if len(report.Issues) != 1 || strings.Contains(report.Issues[0].Message, "private") {
		t.Fatalf("runtime diagnostic exposed data or omitted failure: %+v", report.Issues)
	}
}

func TestRuntimeDoctorEndpointIsAuthenticatedAndReturnsSharedDoctorReport(t *testing.T) {
	bridge := newBridgeServer(testToken, "instance-runtime-doctor")
	unauthorized := httptest.NewRecorder()
	bridge.handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/settings/diagnostics/runtime", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/settings/diagnostics/runtime", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	bridge.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("runtime doctor response: %d %s", response.Code, response.Body.String())
	}
	var report runtimeDoctorView
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Text == "" || !report.AllowResume {
		t.Fatalf("unexpected shared runtime doctor report: %+v", report)
	}
}
