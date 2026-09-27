package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewSkillPlanAndInstallRespectScope(t *testing.T) {
	home, project, source := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: reviewed-skill\ndescription: A reviewed skill\n---\nUse this skill.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newBridgeServer(testToken, "instance-skills-install").handler()
	post := func(path, id string, input any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(input)
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+testToken)
		if id != "" {
			r.Header.Set(requestIDHeader, id)
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct{ scope, root, target string }{
		{"global", "", filepath.Join(home, "skills", "reviewed-skill", "SKILL.md")},
		{"project", project, filepath.Join(project, ".reasonix", "skills", "reviewed-skill", "SKILL.md")},
	} {
		input := previewSkillInstallRequest{Source: source, Scope: tc.scope, WorkspaceRoot: tc.root}
		planned := post("/v1/settings/skills/plan", "", input)
		if planned.Code != http.StatusOK {
			t.Fatalf("%s plan: %d %s", tc.scope, planned.Code, planned.Body.String())
		}
		var plan previewSkillInstallPlan
		if err := json.Unmarshal(planned.Body.Bytes(), &plan); err != nil {
			t.Fatal(err)
		}
		if len(plan.Actions) != 1 || plan.Actions[0].Name != "reviewed-skill" || plan.Actions[0].Target != tc.target || plan.PlanID == "" {
			t.Fatalf("%s plan: %+v", tc.scope, plan)
		}
		if _, err := os.Stat(tc.target); !os.IsNotExist(err) {
			t.Fatalf("%s plan wrote the skill: %v", tc.scope, err)
		}
		input.PlanID = "sha256:wrong"
		if got := post("/v1/settings/skills/install", tc.scope+"-wrong", input); got.Code != http.StatusConflict {
			t.Fatalf("%s stale plan: %d", tc.scope, got.Code)
		}
		input.PlanID = plan.PlanID
		installed := post("/v1/settings/skills/install", tc.scope+"-install", input)
		if installed.Code != http.StatusOK {
			t.Fatalf("%s install: %d %s", tc.scope, installed.Code, installed.Body.String())
		}
		var result previewSkillInstallResult
		if err := json.Unmarshal(installed.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Status != "done" || len(result.FailedNames) != 0 {
			t.Fatalf("%s result: %+v", tc.scope, result)
		}
		if _, err := os.Stat(tc.target); err != nil {
			t.Fatalf("%s installed skill missing: %v", tc.scope, err)
		}
	}
	if validPreviewSkillSource("https://secret@github.com/user/repo") {
		t.Fatal("credentialed GitHub source accepted")
	}
}
