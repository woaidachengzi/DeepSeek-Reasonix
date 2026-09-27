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
)

func TestPreviewSkillsSettingsDiscoverAndPersist(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	configPath := filepath.Join(home, "config.toml")
	if err := os.WriteFile(configPath, []byte("future_setting = \"keep\"\n\n[skills]\nfuture_skill_setting = \"keep\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(project, ".reasonix", "skills", "preview-probe")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Preview probe skill\n---\nRun the probe.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := newBridgeServer(testToken, "instance-a")
	request := func(method, body, id string, authorized bool) *httptest.ResponseRecorder {
		target := "/v1/settings/skills?workspaceRoot=" + url.QueryEscape(project)
		r := httptest.NewRequest(method, target, strings.NewReader(body))
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
	read := func() skillsSettingsView {
		response := request(http.MethodGet, "", "", true)
		if response.Code != http.StatusOK {
			t.Fatalf("read: %d %s", response.Code, response.Body.String())
		}
		var view skillsSettingsView
		if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	containsSkill := func(view skillsSettingsView, name string) (previewSkillView, bool) {
		for _, item := range view.Skills {
			if item.Name == name {
				return item, true
			}
		}
		return previewSkillView{}, false
	}
	initial := read()
	if item, ok := containsSkill(initial, "preview-probe"); !ok || !item.Enabled {
		t.Fatalf("project skill not discovered: %#v", initial.Skills)
	}
	change := func(id, body string) {
		t.Helper()
		response := request(http.MethodPost, body, id, true)
		if response.Code != http.StatusOK {
			t.Fatalf("change %s: %d %s", id, response.Code, response.Body.String())
		}
	}
	change("skills-implicit", `{"action":"implicit","enabled":false,"workspaceRoot":"`+project+`"}`)
	change("skills-disable", `{"action":"skill","name":"preview-probe","enabled":false,"workspaceRoot":"`+project+`"}`)
	view := read()
	if view.AllowImplicitInvocation {
		t.Fatal("implicit invocation was not disabled")
	}
	if item, ok := containsSkill(view, "preview-probe"); !ok || item.Enabled {
		t.Fatalf("disabled skill not visible: %#v", item)
	}
	source := filepath.Join(project, ".reasonix", "skills")
	change("skills-source", `{"action":"source","path":"`+source+`","enabled":false,"workspaceRoot":"`+project+`"}`)
	view = read()
	if _, ok := containsSkill(view, "preview-probe"); ok {
		t.Fatal("excluded skill source still discovered")
	}
	var disabledSource bool
	for _, item := range view.Sources {
		if item.Path == source && !item.Enabled {
			disabledSource = true
		}
	}
	if !disabledSource {
		t.Fatal("disabled source not represented")
	}
	customSource := filepath.Join(t.TempDir(), "custom-skills")
	if err := os.MkdirAll(customSource, 0o755); err != nil {
		t.Fatal(err)
	}
	change("skills-add", `{"action":"add_source","path":"`+customSource+`","workspaceRoot":"`+project+`"}`)
	var foundCustom bool
	for _, item := range read().Sources {
		if item.Path == customSource && item.Configured && item.Enabled {
			foundCustom = true
		}
	}
	if !foundCustom {
		t.Fatal("custom source was not added")
	}
	change("skills-remove", `{"action":"remove_source","path":"`+customSource+`","workspaceRoot":"`+project+`"}`)
	for _, item := range read().Sources {
		if item.Path == customSource {
			t.Fatal("custom source still present after removal")
		}
	}
	raw, err := os.ReadFile(configPath)
	if err != nil || !strings.Contains(string(raw), "future_setting") || !strings.Contains(string(raw), "future_skill_setting") || !strings.Contains(string(raw), "preview-probe") {
		t.Fatalf("skill config lost fields: %v %s", err, raw)
	}
	if response := request(http.MethodPost, `{"action":"skill","name":"(invalid)","enabled":false}`, "skills-invalid", true); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid skill status = %d", response.Code)
	}
}
