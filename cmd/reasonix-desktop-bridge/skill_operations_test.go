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

func TestPreviewSkillArchiveAndRestoreRespectRevision(t *testing.T) {
	for _, scope := range []string{"global", "project"} {
		t.Run(scope, func(t *testing.T) {
			home, project, source := t.TempDir(), t.TempDir(), t.TempDir()
			t.Setenv("REASONIX_HOME", home)
			if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: backed-up\ndescription: Backup test\n---\nUse this skill.\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			server := newBridgeServer(testToken, "instance-skill-archive").handler()
			post := func(path, id string, input any) *httptest.ResponseRecorder {
				body, _ := json.Marshal(input)
				r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Authorization", "Bearer "+testToken)
				r.Header.Set(requestIDHeader, id)
				w := httptest.NewRecorder()
				server.ServeHTTP(w, r)
				return w
			}
			root := ""
			if scope == "project" {
				root = project
			}
			installInput := previewSkillInstallRequest{Source: source, Scope: scope, WorkspaceRoot: root}
			var plan previewSkillInstallPlan
			if err := json.Unmarshal(post("/v1/settings/skills/plan", "", installInput).Body.Bytes(), &plan); err != nil || plan.PlanID == "" {
				t.Fatalf("plan: %+v, %v", plan, err)
			}
			installInput.PlanID = plan.PlanID
			var installed previewSkillInstallResult
			if err := json.Unmarshal(post("/v1/settings/skills/install", "install-"+scope, installInput).Body.Bytes(), &installed); err != nil || installed.Status != "done" {
				t.Fatalf("install: %+v, %v", installed, err)
			}
			var revision string
			for _, item := range installed.Settings.Skills {
				if item.Name == "backed-up" && item.Scope == scope {
					revision = item.ArchiveRevision
				}
			}
			if revision == "" {
				t.Fatalf("installed skill lacks archive revision: %+v", installed.Settings.Skills)
			}
			archiveInput := previewSkillArchiveRequest{Name: "backed-up", Scope: scope, WorkspaceRoot: root, Revision: "sha256:stale"}
			if got := post("/v1/settings/skills/archive", "stale-"+scope, archiveInput); got.Code != http.StatusConflict {
				t.Fatalf("stale revision accepted: %d %s", got.Code, got.Body.String())
			}
			archiveInput.Revision = revision
			var archived previewSkillArchiveResult
			resp := post("/v1/settings/skills/archive", "archive-"+scope, archiveInput)
			if err := json.Unmarshal(resp.Body.Bytes(), &archived); err != nil || resp.Code != http.StatusOK {
				t.Fatalf("archive: %d %+v, %v", resp.Code, archived, err)
			}
			if len(archived.Settings.ArchivedSkills) != 1 || archived.BackupPath == "" {
				t.Fatalf("backup not visible: %+v", archived)
			}
			if _, err := os.Stat(archived.BackupPath); err != nil {
				t.Fatalf("backup missing: %v", err)
			}
			backup := archived.Settings.ArchivedSkills[0]
			restoreInput := previewSkillArchiveRequest{Name: backup.Name, Scope: scope, WorkspaceRoot: root, ArchiveID: backup.ArchiveID, Revision: backup.Revision}
			if err := os.WriteFile(filepath.Join(archived.BackupPath, "SKILL.md"), []byte("---\nname: backed-up\ndescription: Backup test\n---\nUpdated while backed up.\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := post("/v1/settings/skills/restore", "restore-stale-"+scope, restoreInput); got.Code != http.StatusConflict {
				t.Fatalf("changed backup restored under stale revision: %d %s", got.Code, got.Body.String())
			}
			refreshed, err := loadSkillsSettings(root)
			if err != nil || len(refreshed.ArchivedSkills) != 1 {
				t.Fatalf("refresh backup: %+v, %v", refreshed.ArchivedSkills, err)
			}
			restoreInput.Revision = refreshed.ArchivedSkills[0].Revision
			target, _, err := previewSkillPaths(scope, root, backup.Name)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(target, 0o700); err != nil {
				t.Fatal(err)
			}
			if got := post("/v1/settings/skills/restore", "restore-conflict-"+scope, restoreInput); got.Code != http.StatusConflict {
				t.Fatalf("existing skill overwritten: %d %s", got.Code, got.Body.String())
			}
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
			resp = post("/v1/settings/skills/restore", "restore-"+scope, restoreInput)
			var restored previewSkillArchiveResult
			if err := json.Unmarshal(resp.Body.Bytes(), &restored); err != nil || resp.Code != http.StatusOK {
				t.Fatalf("restore: %d %+v, %v", resp.Code, restored, err)
			}
			if len(restored.Settings.ArchivedSkills) != 0 || len(restored.Settings.Skills) == 0 {
				t.Fatalf("skill did not return to inventory: %+v", restored.Settings)
			}
			if _, err := os.Stat(archived.BackupPath); !os.IsNotExist(err) {
				t.Fatalf("backup still present after restore: %v", err)
			}
		})
	}
}
