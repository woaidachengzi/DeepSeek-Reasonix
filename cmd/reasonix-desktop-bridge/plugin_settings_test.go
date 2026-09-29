package main

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "reasonix/internal/config"
	"reasonix/internal/pluginpkg"
)

func TestPreviewPluginSettingsReadToggleAndRejectStaleOrInvalid(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	root := filepath.Join(home, "plugins", "sample")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"apiVersion":"reasonix.io/plugin/v2","name":"sample","description":"A package","contributes":{"themes":["themes/*.reasonix-theme"],"hooks":{"PreToolUse":[{"match":"Bash","command":"./bin/private-command --token secret-value","description":"Review shell commands","contextFile":"hooks/policy.md"}]}}}`
	if err := os.WriteFile(filepath.Join(root, pluginpkg.NativeManifest), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	themeDir := filepath.Join(root, "themes")
	if err := os.MkdirAll(themeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	themeFile, err := os.Create(filepath.Join(themeDir, "neon.reasonix-theme"))
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(themeFile)
	entry, err := archive.Create("theme.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(`{"schemaVersion":2,"id":"neon","name":"Neon","baseStyle":"graphite"}`)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := themeFile.Close(); err != nil {
		t.Fatal(err)
	}
	if err := pluginpkg.Upsert(home, pluginpkg.InstalledPlugin{Name: "sample", Root: "plugins/sample", Source: "https://secret@example.com/plugin.git", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "instance-a").handler()
	request := func(method, body, id string, authorized bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1/settings/plugins", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if id != "" {
			r.Header.Set(requestIDHeader, id)
		}
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := request(http.MethodGet, "", "", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized read: %d", got.Code)
	}
	read := func() previewPluginSettings {
		t.Helper()
		response := request(http.MethodGet, "", "", true)
		if response.Code != http.StatusOK {
			t.Fatalf("read: %d %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "secret@") {
			t.Fatal("source credential leaked to renderer")
		}
		var view previewPluginSettings
		if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	initial := read()
	if len(initial.Plugins) != 1 || initial.Plugins[0].Status != "ready" || initial.Plugins[0].Source != "remote" || !initial.Plugins[0].Enabled {
		t.Fatalf("initial plugin = %+v", initial.Plugins)
	}
	if len(initial.Plugins[0].Themes) != 1 || initial.Plugins[0].Themes[0].Name != "neon" || !strings.HasPrefix(initial.Plugins[0].Themes[0].Path, root+string(filepath.Separator)) {
		t.Fatalf("enabled plugin themes = %+v", initial.Plugins[0].Themes)
	}
	if len(initial.Plugins[0].HookDetails) != 1 || initial.Plugins[0].HookDetails[0].Event != "PreToolUse" || initial.Plugins[0].HookDetails[0].Match != "Bash" || initial.Plugins[0].HookDetails[0].Description != "Review shell commands" || initial.Plugins[0].HookDetails[0].ContextFile != "hooks/policy.md" {
		t.Fatalf("plugin hook metadata = %+v", initial.Plugins[0].HookDetails)
	}
	encodedInitial, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedInitial), "private-command") || strings.Contains(string(encodedInitial), "secret-value") {
		t.Fatal("plugin hook command text leaked to renderer")
	}
	change := func(id, revision string, enabled bool) *httptest.ResponseRecorder {
		body, _ := json.Marshal(previewPluginChange{Name: "sample", Revision: revision, Enabled: enabled})
		return request(http.MethodPost, string(body), id, true)
	}
	if got := change("plugin-disable", initial.Plugins[0].Revision, false); got.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", got.Code, got.Body.String())
	}
	disabled := read().Plugins[0]
	if disabled.Enabled {
		t.Fatal("plugin remained enabled")
	}
	if len(disabled.Themes) != 0 {
		t.Fatalf("disabled plugin still contributed themes: %+v", disabled.Themes)
	}
	if got := change("plugin-stale", initial.Plugins[0].Revision, true); got.Code != http.StatusConflict {
		t.Fatalf("stale revision: %d", got.Code)
	}
	current := read().Plugins[0]
	if err := os.Remove(filepath.Join(root, pluginpkg.NativeManifest)); err != nil {
		t.Fatal(err)
	}
	if got := change("plugin-invalid", current.Revision, true); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid plugin enabled: %d", got.Code)
	}
	if read().Plugins[0].Status != "invalid" {
		t.Fatal("invalid plugin status not surfaced")
	}
	if got := request(http.MethodPost, `{"name":"sample","revision":"bad","enabled":true}`, "plugin-bad", true); got.Code != http.StatusBadRequest {
		t.Fatalf("bad request: %d", got.Code)
	}
}

func TestPreviewPluginDoctorReturnsCompatibilityWithoutExecutingPlugin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	root := filepath.Join(home, "plugins", "claude-sample")
	if err := os.MkdirAll(filepath.Join(root, ".claude-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "hooks"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, pluginpkg.ClaudeManifest), []byte(`{"name":"claude-sample"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hooks", "hooks.json"), []byte(`{"hooks":{"PreToolUse":[{"matcher":"WebFetch","hooks":[{"type":"command","command":"echo should-not-run"}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := pluginpkg.Upsert(home, pluginpkg.InstalledPlugin{Name: "claude-sample", Root: "plugins/claude-sample", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "instance-doctor").handler()
	request := func(authorized bool, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/v1/settings/plugins/doctor", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := request(false, `{"name":"claude-sample"}`); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized diagnostics: %d", got.Code)
	}
	got := request(true, `{"name":"claude-sample"}`)
	if got.Code != http.StatusOK {
		t.Fatalf("diagnostics: %d %s", got.Code, got.Body.String())
	}
	var view previewPluginDoctorView
	if err := json.Unmarshal(got.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Name != "claude-sample" || view.Compatibility != "partial" || len(view.SkippedCapabilities) == 0 || len(view.Warnings) == 0 {
		t.Fatalf("diagnostics missing compatibility details: %+v", view)
	}
}

func TestPreviewPluginPlanInstallAndRemove(t *testing.T) {
	home, source := t.TempDir(), t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	manifest := `{"apiVersion":"reasonix.io/plugin/v2","name":"planned","version":"1.0","contributes":{"skills":["skills"]}}`
	if err := os.WriteFile(filepath.Join(source, pluginpkg.NativeManifest), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(source, "skills", "sample")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Sample\n---\nSample"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "instance-b").handler()
	post := func(path, id string, input any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+testToken)
		if id != "" {
			r.Header.Set(requestIDHeader, id)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	planResponse := post("/v1/settings/plugins/plan", "", previewPluginInstallRequest{Source: source})
	if planResponse.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", planResponse.Code, planResponse.Body.String())
	}
	var plan previewPluginInstallPlan
	if err := json.Unmarshal(planResponse.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.PlanID == "" || len(plan.Actions) != 1 || plan.Actions[0].Name != "planned" || plan.Actions[0].Skills != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	if len(loadPluginNames(t, home)) != 0 {
		t.Fatal("plan wrote plugin registration")
	}
	if got := post("/v1/settings/plugins/install", "install-bad", previewPluginInstallRequest{Source: source, PlanID: "sha256:wrong"}); got.Code != http.StatusConflict {
		t.Fatalf("mismatched plan: %d", got.Code)
	}
	installResponse := post("/v1/settings/plugins/install", "install-good", previewPluginInstallRequest{Source: source, PlanID: plan.PlanID})
	if installResponse.Code != http.StatusOK {
		t.Fatalf("install: %d %s", installResponse.Code, installResponse.Body.String())
	}
	var installed previewPluginOperationResult
	if err := json.Unmarshal(installResponse.Body.Bytes(), &installed); err != nil {
		t.Fatal(err)
	}
	if installed.Status != "done" || len(installed.Settings.Plugins) != 1 || installed.Settings.Plugins[0].Name != "planned" {
		t.Fatalf("installed = %+v", installed)
	}
	if _, err := os.Stat(pluginpkg.InstallRoot(home, "planned")); err != nil {
		t.Fatalf("installed files missing: %v", err)
	}
	if installed.Settings.Plugins[0].UpdateSource != source || installed.Settings.Plugins[0].Linked {
		t.Fatalf("copied plugin update source = %+v", installed.Settings.Plugins[0])
	}
	if err := pluginpkg.SetEnabledIfRevision(home, "planned", installed.Settings.Plugins[0].Revision, false); err != nil {
		t.Fatal(err)
	}
	beforeUpdate, err := loadPreviewPluginSettings()
	if err != nil || len(beforeUpdate.Plugins) != 1 {
		t.Fatalf("load plugin before update: %+v %v", beforeUpdate, err)
	}
	manifest = `{"apiVersion":"reasonix.io/plugin/v2","name":"planned","version":"2.0","contributes":{"skills":["skills"]}}`
	if err := os.WriteFile(filepath.Join(source, pluginpkg.NativeManifest), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	updateRequest := previewPluginInstallRequest{Source: source, Replace: true, ExpectedName: "planned", ExpectedRevision: beforeUpdate.Plugins[0].Revision}
	updatePlanResponse := post("/v1/settings/plugins/plan", "", updateRequest)
	if updatePlanResponse.Code != http.StatusOK {
		t.Fatalf("update plan: %d %s", updatePlanResponse.Code, updatePlanResponse.Body.String())
	}
	var updatePlan previewPluginInstallPlan
	if err := json.Unmarshal(updatePlanResponse.Body.Bytes(), &updatePlan); err != nil {
		t.Fatal(err)
	}
	if updatePlan.PlanID == plan.PlanID || len(updatePlan.Actions) != 1 || updatePlan.Actions[0].Version != "2.0" {
		t.Fatalf("update plan = %+v", updatePlan)
	}
	updateRequest.PlanID = updatePlan.PlanID
	updatedResponse := post("/v1/settings/plugins/install", "update-good", updateRequest)
	if updatedResponse.Code != http.StatusOK {
		t.Fatalf("update plugin: %d %s", updatedResponse.Code, updatedResponse.Body.String())
	}
	var updated previewPluginOperationResult
	if err := json.Unmarshal(updatedResponse.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Settings.Plugins) != 1 || updated.Settings.Plugins[0].Version != "2.0" || updated.Settings.Plugins[0].Enabled {
		t.Fatalf("update did not replace package while preserving disabled state: %+v", updated.Settings.Plugins)
	}
	replacementSource := t.TempDir()
	replacementManifest := `{"apiVersion":"reasonix.io/plugin/v2","name":"planned","version":"3.0","contributes":{"skills":["skills"]}}`
	if err := os.WriteFile(filepath.Join(replacementSource, pluginpkg.NativeManifest), []byte(replacementManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	replacementRequest := previewPluginInstallRequest{Source: replacementSource, Replace: true, ExpectedName: "planned", ExpectedRevision: updated.Settings.Plugins[0].Revision}
	replacementPlanResponse := post("/v1/settings/plugins/plan", "", replacementRequest)
	if replacementPlanResponse.Code != http.StatusOK {
		t.Fatalf("replacement plan from a different source: %d %s", replacementPlanResponse.Code, replacementPlanResponse.Body.String())
	}
	var replacementPlan previewPluginInstallPlan
	if err := json.Unmarshal(replacementPlanResponse.Body.Bytes(), &replacementPlan); err != nil {
		t.Fatal(err)
	}
	if len(replacementPlan.Actions) != 1 || replacementPlan.Actions[0].Name != "planned" {
		t.Fatalf("replacement plan = %+v", replacementPlan)
	}
	replacementRequest.PlanID = replacementPlan.PlanID
	replacedResponse := post("/v1/settings/plugins/install", "replace-source", replacementRequest)
	if replacedResponse.Code != http.StatusOK {
		t.Fatalf("replace same-name plugin from selected source: %d %s", replacedResponse.Code, replacedResponse.Body.String())
	}
	var replaced previewPluginOperationResult
	if err := json.Unmarshal(replacedResponse.Body.Bytes(), &replaced); err != nil {
		t.Fatal(err)
	}
	if len(replaced.Settings.Plugins) != 1 || replaced.Settings.Plugins[0].Version != "3.0" || replaced.Settings.Plugins[0].Enabled {
		t.Fatalf("replacement did not preserve the disabled state: %+v", replaced.Settings.Plugins)
	}
	staleUpdate := previewPluginInstallRequest{Source: source, Replace: true, ExpectedName: "planned", ExpectedRevision: beforeUpdate.Plugins[0].Revision}
	if got := post("/v1/settings/plugins/plan", "", staleUpdate); got.Code != http.StatusConflict {
		t.Fatalf("stale update plan: %d %s", got.Code, got.Body.String())
	}
	if got := post("/v1/settings/plugins/remove", "remove-stale", previewPluginRemoveRequest{Name: "planned", Revision: strings.Repeat("a", 64)}); got.Code != http.StatusConflict {
		t.Fatalf("stale remove: %d", got.Code)
	}
	removed := post("/v1/settings/plugins/remove", "remove-good", previewPluginRemoveRequest{Name: "planned", Revision: replaced.Settings.Plugins[0].Revision})
	if removed.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", removed.Code, removed.Body.String())
	}
	if len(loadPluginNames(t, home)) != 0 {
		t.Fatal("plugin registration remained after removal")
	}
	if _, err := os.Stat(pluginpkg.InstallRoot(home, "planned")); !os.IsNotExist(err) {
		t.Fatalf("managed files remain: %v", err)
	}
	if validPreviewPluginSource("https://secret@github.com/user/repo") {
		t.Fatal("URL with embedded credential accepted")
	}
}

func TestPreviewPluginLinkedInstallUsesReviewedModeAndKeepsSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("REASONIX_HOME", filepath.Join(home, ".preview-profile"))
	source := filepath.Join(home, "dev", "linked-plugin")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"apiVersion":"reasonix.io/plugin/v2","name":"linked-plugin","version":"1.0"}`
	if err := os.WriteFile(filepath.Join(source, pluginpkg.NativeManifest), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "instance-link-plugin").handler()
	post := func(path, id string, input any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+testToken)
		if id != "" {
			r.Header.Set(requestIDHeader, id)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	linkedPlanResponse := post("/v1/settings/plugins/plan", "", previewPluginInstallRequest{Source: source, Mode: "link"})
	if linkedPlanResponse.Code != http.StatusOK {
		t.Fatalf("plan linked plugin: %d %s", linkedPlanResponse.Code, linkedPlanResponse.Body.String())
	}
	var linkedPlan previewPluginInstallPlan
	if err := json.Unmarshal(linkedPlanResponse.Body.Bytes(), &linkedPlan); err != nil {
		t.Fatal(err)
	}
	if linkedPlan.Mode != "link" || len(linkedPlan.Actions) != 1 {
		t.Fatalf("linked plan = %+v", linkedPlan)
	}
	copyPlanResponse := post("/v1/settings/plugins/plan", "", previewPluginInstallRequest{Source: source, Mode: "copy"})
	var copyPlan previewPluginInstallPlan
	if copyPlanResponse.Code != http.StatusOK || json.Unmarshal(copyPlanResponse.Body.Bytes(), &copyPlan) != nil {
		t.Fatalf("plan copy plugin: %d %s", copyPlanResponse.Code, copyPlanResponse.Body.String())
	}
	if copyPlan.PlanID == linkedPlan.PlanID {
		t.Fatal("copy and link install plans must have distinct plan IDs")
	}
	installed := post("/v1/settings/plugins/install", "install-linked-plugin", previewPluginInstallRequest{Source: source, Mode: "link", PlanID: linkedPlan.PlanID})
	if installed.Code != http.StatusOK {
		t.Fatalf("install linked plugin: %d %s", installed.Code, installed.Body.String())
	}
	var result previewPluginOperationResult
	if err := json.Unmarshal(installed.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Settings.Plugins) != 1 || !result.Settings.Plugins[0].Linked || filepath.Clean(result.Settings.Plugins[0].Root) != filepath.Clean(source) {
		t.Fatalf("installed linked plugin view = %+v", result.Settings.Plugins)
	}
	info, err := os.Lstat(pluginpkg.InstallRoot(appconfig.ReasonixHomeDir(), "linked-plugin"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("managed install pointer is not a symlink: info=%v err=%v", info, err)
	}
	if _, err := os.Stat(filepath.Join(source, pluginpkg.NativeManifest)); err != nil {
		t.Fatalf("linked source was modified or removed: %v", err)
	}
	removed := post("/v1/settings/plugins/remove", "remove-linked-plugin", previewPluginRemoveRequest{Name: "linked-plugin", Revision: result.Settings.Plugins[0].Revision})
	if removed.Code != http.StatusOK {
		t.Fatalf("remove linked plugin: %d %s", removed.Code, removed.Body.String())
	}
	if _, err := os.Lstat(pluginpkg.InstallRoot(appconfig.ReasonixHomeDir(), "linked-plugin")); !os.IsNotExist(err) {
		t.Fatalf("managed link stub remains after removal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(source, pluginpkg.NativeManifest)); err != nil {
		t.Fatalf("removing linked plugin changed its external source: %v", err)
	}
	if got := post("/v1/settings/plugins/plan", "", previewPluginInstallRequest{Source: "https://github.com/example/plugin", Mode: "link"}); got.Code != http.StatusBadRequest {
		t.Fatalf("remote link mode should be rejected: %d", got.Code)
	}
}

func TestPreviewPluginRuntimeRequiresRiskAcknowledgement(t *testing.T) {
	home, source := t.TempDir(), t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	manifest := `{"apiVersion":"reasonix.io/plugin/v2","name":"runtime-review","version":"1.0","runtime":{"command":"${REASONIX_PLUGIN_ROOT}/bin/run","required":true,"intercepts":["input.receive"]}}`
	if err := os.WriteFile(filepath.Join(source, pluginpkg.NativeManifest), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "bin", "run"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	handler := newBridgeServer(testToken, "instance-risk").handler()
	post := func(path, id string, input any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+testToken)
		if id != "" {
			r.Header.Set(requestIDHeader, id)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	planned := post("/v1/settings/plugins/plan", "", previewPluginInstallRequest{Source: source})
	if planned.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", planned.Code, planned.Body.String())
	}
	var plan previewPluginInstallPlan
	if err := json.Unmarshal(planned.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].RiskLevel != "high" || !plan.Actions[0].Runtime || !strings.Contains(plan.Actions[0].RuntimeCmd, "bin/run") {
		t.Fatalf("runtime plan = %+v", plan)
	}
	request := previewPluginInstallRequest{Source: source, PlanID: plan.PlanID}
	if got := post("/v1/settings/plugins/install", "without-risk", request); got.Code != http.StatusBadRequest {
		t.Fatalf("high-risk install without acknowledgement: %d %s", got.Code, got.Body.String())
	}
	if len(loadPluginNames(t, home)) != 0 {
		t.Fatal("unacknowledged runtime plugin was installed")
	}
	request.AcceptRisk = true
	if got := post("/v1/settings/plugins/install", "with-risk", request); got.Code != http.StatusOK {
		t.Fatalf("acknowledged install: %d %s", got.Code, got.Body.String())
	}
}

func loadPluginNames(t *testing.T, home string) []string {
	t.Helper()
	names, err := pluginpkg.InstalledNames(home)
	if err != nil {
		t.Fatal(err)
	}
	return names
}
