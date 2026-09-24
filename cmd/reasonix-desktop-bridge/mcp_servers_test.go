package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const mcpTestSecret = "mcp-test-secret-value"

// mcpTestBridge builds a bridge bound to a private home so the test never reads
// or writes the developer's own MCP configuration.
func mcpTestBridge(t *testing.T) (http.Handler, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	return newBridgeServer(testToken, "instance", nil).handler(), home
}

var mcpRequestCounter atomic.Int32

func mcpRequest(t *testing.T, handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")
	// The bridge bounds the request ID length, so keep it short and unique.
	request.Header.Set("X-Reasonix-Request-ID", fmt.Sprintf("mcp-%d", mcpRequestCounter.Add(1)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeMCPMutation(t *testing.T, response *httptest.ResponseRecorder) mcpServerMutationResponse {
	t.Helper()
	var body mcpServerMutationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode MCP response %q: %v", response.Body.String(), err)
	}
	return body
}

func decodeMCPList(t *testing.T, response *httptest.ResponseRecorder) mcpServerListResponse {
	t.Helper()
	var body mcpServerListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode MCP list %q: %v", response.Body.String(), err)
	}
	return body
}

// Credentials are write-only: the listing names the keys a server expects but
// never returns a value.
func TestMCPServersNeverReturnCredentialValues(t *testing.T) {
	handler, home := mcpTestBridge(t)

	added := mcpRequest(t, handler, http.MethodPost, "/v1/mcp/servers",
		`{"scope":"global","name":"time","type":"stdio","command":"uvx","args":["mcp-server-time"],"env":{"TIMEZONE":"Asia/Shanghai","API_TOKEN":"`+mcpTestSecret+`"}}`)
	if added.Code != http.StatusOK {
		t.Fatalf("add status = %d, body = %s", added.Code, added.Body.String())
	}
	if strings.Contains(added.Body.String(), mcpTestSecret) {
		t.Fatalf("the add response leaked the credential: %s", added.Body.String())
	}
	body := decodeMCPMutation(t, added)
	if body.Server == nil || body.Server.Name != "time" {
		t.Fatalf("add response server = %#v", body.Server)
	}
	if got := strings.Join(body.Server.EnvKeys, ","); got != "API_TOKEN,TIMEZONE" {
		t.Fatalf("env keys = %q, want the key names in order", got)
	}
	if body.Server.Scope != "global" || body.Server.Type != "stdio" {
		t.Fatalf("server view = %#v", body.Server)
	}

	// The value is stored in the config file, not lost by the redaction.
	configPath := filepath.Join(home, "config.toml")
	stored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), mcpTestSecret) {
		t.Fatalf("the credential was not persisted to %s", configPath)
	}

	listed := mcpRequest(t, handler, http.MethodGet, "/v1/mcp/servers", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d", listed.Code)
	}
	if strings.Contains(listed.Body.String(), mcpTestSecret) {
		t.Fatalf("the listing leaked the credential: %s", listed.Body.String())
	}
	list := decodeMCPList(t, listed)
	if len(list.Servers) != 1 || list.Servers[0].Name != "time" {
		t.Fatalf("listing = %#v", list.Servers)
	}
}

// Editing must not require retyping a credential: an omitted field keeps the
// stored value, and an explicit empty map clears it.
func TestMCPServerEditPreservesOmittedCredentials(t *testing.T) {
	handler, home := mcpTestBridge(t)
	configPath := filepath.Join(home, "config.toml")

	if response := mcpRequest(t, handler, http.MethodPost, "/v1/mcp/servers",
		`{"scope":"global","name":"time","type":"stdio","command":"uvx","env":{"API_TOKEN":"`+mcpTestSecret+`"}}`); response.Code != http.StatusOK {
		t.Fatalf("add status = %d, body = %s", response.Code, response.Body.String())
	}

	// Change only the command: env is absent, so the stored token survives.
	edited := mcpRequest(t, handler, http.MethodPost, "/v1/mcp/servers",
		`{"scope":"global","name":"time","command":"uvx","args":["--local-timezone=Asia/Tokyo"]}`)
	if edited.Code != http.StatusOK {
		t.Fatalf("edit status = %d, body = %s", edited.Code, edited.Body.String())
	}
	body := decodeMCPMutation(t, edited)
	if body.Server == nil {
		t.Fatal("edit response carried no server")
	}
	if len(body.Server.EnvKeys) != 1 || body.Server.EnvKeys[0] != "API_TOKEN" {
		t.Fatalf("edit dropped the stored credential: %#v", body.Server.EnvKeys)
	}
	stored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), mcpTestSecret) {
		t.Fatalf("edit rewrote the config without the credential: %s", stored)
	}
	if !strings.Contains(string(stored), "Asia/Tokyo") {
		t.Fatalf("edit did not persist the new arguments: %s", stored)
	}

	// An explicit empty map is how a caller clears a credential.
	cleared := mcpRequest(t, handler, http.MethodPost, "/v1/mcp/servers",
		`{"scope":"global","name":"time","env":{}}`)
	if cleared.Code != http.StatusOK {
		t.Fatalf("clear status = %d, body = %s", cleared.Code, cleared.Body.String())
	}
	if body := decodeMCPMutation(t, cleared); len(body.Server.EnvKeys) != 0 {
		t.Fatalf("explicit empty env did not clear the credential: %#v", body.Server.EnvKeys)
	}
	if stored, err := os.ReadFile(configPath); err == nil && strings.Contains(string(stored), mcpTestSecret) {
		t.Fatal("the cleared credential is still on disk")
	}
}

// The scope decides which file owns the server, and the project file lives in
// the workspace the caller names.
func TestMCPServerScopeSelectsTheOwningFile(t *testing.T) {
	handler, home := mcpTestBridge(t)
	projectRoot := t.TempDir()

	project := mcpRequest(t, handler, http.MethodPost, "/v1/mcp/servers?workspaceRoot="+projectRoot,
		`{"scope":"project","name":"proj","type":"http","url":"https://example.test/mcp"}`)
	if project.Code != http.StatusOK {
		t.Fatalf("project add status = %d, body = %s", project.Code, project.Body.String())
	}
	projectBody := decodeMCPMutation(t, project)
	if projectBody.Server == nil || projectBody.Server.Scope != "project" {
		t.Fatalf("project server = %#v", projectBody.Server)
	}
	if want := filepath.Join(projectRoot, "reasonix.toml"); projectBody.ConfigPath != want {
		t.Fatalf("project config path = %q, want %q", projectBody.ConfigPath, want)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, "reasonix.toml")); err != nil {
		t.Fatalf("project config was not written into the workspace: %v", err)
	}

	global := mcpRequest(t, handler, http.MethodPost, "/v1/mcp/servers",
		`{"scope":"global","name":"glob","command":"uvx"}`)
	if global.Code != http.StatusOK {
		t.Fatalf("global add status = %d, body = %s", global.Code, global.Body.String())
	}
	if want := filepath.Join(home, "config.toml"); decodeMCPMutation(t, global).ConfigPath != want {
		t.Fatalf("global config path = %q, want %q", decodeMCPMutation(t, global).ConfigPath, want)
	}

	// Both scopes are visible together, each labelled with its own scope.
	list := decodeMCPList(t, mcpRequest(t, handler, http.MethodGet, "/v1/mcp/servers?workspaceRoot="+projectRoot, ""))
	scopes := map[string]string{}
	for _, server := range list.Servers {
		scopes[server.Name] = server.Scope
	}
	if scopes["proj"] != "project" || scopes["glob"] != "global" {
		t.Fatalf("scopes = %#v", scopes)
	}
}

func TestMCPServerDeleteReportsMissingServers(t *testing.T) {
	handler, _ := mcpTestBridge(t)

	if response := mcpRequest(t, handler, http.MethodPost, "/v1/mcp/servers",
		`{"scope":"global","name":"doomed","command":"uvx"}`); response.Code != http.StatusOK {
		t.Fatalf("add status = %d", response.Code)
	}
	removed := mcpRequest(t, handler, http.MethodDelete, "/v1/mcp/servers", `{"name":"doomed"}`)
	if removed.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", removed.Code, removed.Body.String())
	}
	body := decodeMCPMutation(t, removed)
	if body.Status != "removed" {
		t.Fatalf("delete status field = %q", body.Status)
	}
	if len(body.Servers) != 0 {
		t.Fatalf("servers after delete = %#v", body.Servers)
	}

	again := mcpRequest(t, handler, http.MethodDelete, "/v1/mcp/servers", `{"name":"doomed"}`)
	if again.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, body = %s", again.Code, again.Body.String())
	}
}

func TestMCPServerMutationsValidateInput(t *testing.T) {
	handler, _ := mcpTestBridge(t)

	cases := []struct {
		name   string
		method string
		target string
		body   string
		status int
	}{
		{"missing name", http.MethodPost, "/v1/mcp/servers", `{"scope":"global","command":"uvx"}`, http.StatusBadRequest},
		{"unknown scope", http.MethodPost, "/v1/mcp/servers", `{"scope":"planet","name":"x","command":"uvx"}`, http.StatusBadRequest},
		{"stdio without command", http.MethodPost, "/v1/mcp/servers", `{"scope":"global","name":"x","type":"stdio"}`, http.StatusBadRequest},
		{"http without url", http.MethodPost, "/v1/mcp/servers", `{"scope":"global","name":"x","type":"http"}`, http.StatusBadRequest},
		{"unknown fields", http.MethodPost, "/v1/mcp/servers", `{"scope":"global","name":"x","command":"uvx","surprise":true}`, http.StatusBadRequest},
		{"delete without name", http.MethodDelete, "/v1/mcp/servers", `{}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := mcpRequest(t, handler, tc.method, tc.target, tc.body)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d (body %s)", response.Code, tc.status, response.Body.String())
			}
		})
	}

	// Every mutation endpoint requires the bridge token.
	for _, tc := range []struct {
		method string
		target string
		body   string
	}{
		{http.MethodGet, "/v1/mcp/servers", ""},
		{http.MethodPost, "/v1/mcp/servers", `{"scope":"global","name":"x","command":"uvx"}`},
		{http.MethodDelete, "/v1/mcp/servers", `{"name":"x"}`},
	} {
		request := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without a token = %d", tc.method, tc.target, response.Code)
		}
	}
}
