package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPreviewSubagentSettingsPersistAndPreserveConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, "config.toml")
	initial := `default_model = "local/chat"
future_setting = "keep"

[agent]
max_subagent_concurrency = 6
max_parallel_writers = 3
future_agent_setting = "keep"

[agent.subagent_models]
security_review = "local/chat"

[[providers]]
name = "local"
kind = "openai"
base_url = "http://127.0.0.1:11434/v1"
models = ["chat"]
default = "chat"
supported_efforts = ["low", "medium", "high"]
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newBridgeServer(testToken, "instance-a")
	request := func(method, body, id string, authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1/settings/subagents", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if id != "" {
			r.Header.Set(requestIDHeader, id)
		}
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		server.handler().ServeHTTP(w, r)
		return w
	}
	if got := request(http.MethodGet, "", "", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", got.Code)
	}
	read := func() subagentSettingsView {
		t.Helper()
		response := request(http.MethodGet, "", "", true)
		if response.Code != http.StatusOK {
			t.Fatalf("read: %d %s", response.Code, response.Body.String())
		}
		var view subagentSettingsView
		if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	view := read()
	if got := view.ModelEfforts["local/chat"]; len(got) != 4 || got[0] != "auto" || got[1] != "low" {
		t.Fatalf("model effort choices = %v", got)
	}
	var securityReview bool
	for _, profile := range view.Profiles {
		if profile.Name == "security-review" && profile.ConfiguredModel == "local/chat" {
			securityReview = true
		}
	}
	if !securityReview {
		t.Fatalf("legacy alias override not shown: %#v", view.Profiles)
	}
	change := func(id, body string) {
		t.Helper()
		response := request(http.MethodPost, body, id, true)
		if response.Code != http.StatusOK {
			t.Fatalf("change %s: %d %s", id, response.Code, response.Body.String())
		}
	}
	change("subagents-total", `{"action":"concurrency","number":2}`)
	view = read()
	if view.MaxConcurrency != 2 || view.MaxParallelWriters != 2 {
		t.Fatalf("concurrency was not normalized: %#v", view)
	}
	change("subagents-depth", `{"action":"depth","number":1}`)
	change("subagents-model", `{"action":"model","value":"local/chat"}`)
	change("subagents-effort", `{"action":"effort","value":"low"}`)
	change("subagents-profile-effort", `{"action":"profile_effort","name":"security-review","value":"high"}`)
	change("subagents-profile-clear", `{"action":"profile_model","name":"security-review","value":""}`)
	view = read()
	if view.MaxDepth != 1 || view.SubagentModel != "local/chat" || view.SubagentEffort != "low" {
		t.Fatalf("saved defaults = %#v", view)
	}
	for _, profile := range view.Profiles {
		if profile.Name == "security-review" && profile.ConfiguredModel != "" {
			t.Fatalf("legacy alias survived clear: %#v", profile)
		}
		if profile.Name == "security-review" && profile.ConfiguredEffort != "high" {
			t.Fatalf("profile effort was not saved: %#v", profile)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), "future_setting") || !strings.Contains(string(raw), "future_agent_setting") {
		t.Fatalf("config lost fields: %v %s", err, raw)
	}
	if got := request(http.MethodPost, `{"action":"writers","number":33}`, "subagents-invalid", true); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid writer limit status = %d", got.Code)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(raw) {
		t.Fatalf("invalid change mutated config: %v", err)
	}
}

func TestPreviewSubagentProfileCRUDAndRevision(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	server := newBridgeServer(testToken, "instance-a")
	request := func(body, id string, authorized bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/v1/settings/subagents", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set(requestIDHeader, id)
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		server.handler().ServeHTTP(w, r)
		return w
	}
	create := `{"action":"create_profile","scope":"project","workspaceRoot":` + strconv.Quote(project) + `,"profile":{"name":"preview-review","description":"Review changes","systemPrompt":"Review carefully.","readOnly":true}}`
	if got := request(create, "profile-unauthorized", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", got.Code)
	}
	response := request(create, "profile-create", true)
	if response.Code != http.StatusOK {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	var view subagentSettingsView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	var profile previewSubagentProfile
	for _, item := range view.Profiles {
		if item.Name == "preview-review" {
			profile = item
		}
	}
	if !profile.Editable || profile.Scope != "project" || profile.Body != "Review carefully." || len(profile.Revision) != 64 || !profile.ReadOnly {
		t.Fatalf("created profile not visible: %+v", profile)
	}
	path := filepath.Join(project, ".reasonix", "skills", "preview-review", "SKILL.md")
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), "invocation: manual") {
		t.Fatalf("profile file: %v %s", err, raw)
	}
	update := `{"action":"update_profile","scope":"project","workspaceRoot":` + strconv.Quote(project) + `,"name":"preview-review","revision":` + strconv.Quote(profile.Revision) + `,"profile":{"name":"preview-review","description":"Inspect changes","systemPrompt":"Inspect deeply."}}`
	response = request(update, "profile-update", true)
	if response.Code != http.StatusOK {
		t.Fatalf("update: %d %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	for _, item := range view.Profiles {
		if item.Name == "preview-review" {
			profile = item
		}
	}
	if profile.Body != "Inspect deeply." || profile.Revision == "" || profile.ReadOnly {
		t.Fatalf("updated profile = %+v", profile)
	}
	if got := request(update, "profile-stale", true); got.Code != http.StatusBadRequest {
		t.Fatalf("stale update status = %d", got.Code)
	}
	if got := request(`{"action":"create_profile","scope":"project","workspaceRoot":`+strconv.Quote(project)+`,"profile":{"name":"review","description":"Collision","systemPrompt":"Prompt"}}`, "profile-collision", true); got.Code != http.StatusBadRequest {
		t.Fatalf("reserved name status = %d", got.Code)
	}
	deleteBody := `{"action":"delete_profile","scope":"project","workspaceRoot":` + strconv.Quote(project) + `,"name":"preview-review","revision":` + strconv.Quote(profile.Revision) + `}`
	extraPath := filepath.Join(filepath.Dir(path), "notes.txt")
	if err := os.WriteFile(extraPath, []byte("keep this file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := request(deleteBody, "profile-delete-with-sibling", true); got.Code != http.StatusBadRequest {
		t.Fatalf("profile directory with extra file was deleted: %d %s", got.Code, got.Body.String())
	}
	if raw, err := os.ReadFile(extraPath); err != nil || string(raw) != "keep this file" {
		t.Fatalf("profile sibling was changed: %v %q", err, raw)
	}
	if err := os.Remove(extraPath); err != nil {
		t.Fatal(err)
	}
	skillsDir := filepath.Dir(filepath.Dir(path))
	linkedTarget := skillsDir + "-real"
	if err := os.Rename(skillsDir, linkedTarget); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(linkedTarget, skillsDir); err != nil {
		t.Fatal(err)
	}
	if got := request(deleteBody, "profile-delete-linked-root", true); got.Code != http.StatusBadRequest {
		t.Fatalf("linked profile root was deleted: %d %s", got.Code, got.Body.String())
	}
	if _, err := os.Stat(filepath.Join(linkedTarget, "preview-review", "SKILL.md")); err != nil {
		t.Fatalf("linked profile target was removed: %v", err)
	}
	if err := os.Remove(skillsDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(linkedTarget, skillsDir); err != nil {
		t.Fatal(err)
	}
	if got := request(deleteBody, "profile-delete", true); got.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", got.Code, got.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("profile still exists after delete: %v", err)
	}
	if got := request(`{"action":"create_profile","scope":"global","profile":{"name":"global-helper","description":"Assist","systemPrompt":"Help."}}`, "profile-global", true); got.Code != http.StatusOK {
		t.Fatalf("global create: %d %s", got.Code, got.Body.String())
	}
	if _, err := os.Stat(filepath.Join(home, "skills", "global-helper", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}
