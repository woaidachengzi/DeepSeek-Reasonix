package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/sessionidentity"
)

func commandResult(t *testing.T, args ...string) map[string]any {
	t.Helper()
	var output bytes.Buffer
	if err := run(context.Background(), args, &output); err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode command output %q: %v", output.String(), err)
	}
	return result
}

func TestOfflineScanImportOnlyWritesStagedProfile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	sessions := filepath.Join(source, "sessions")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(sessions, "tauri-orphan.jsonl")
	if err := os.WriteFile(transcript, []byte("{\"role\":\"user\",\"content\":\"hello\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(root, "catalog", "workbench-sessions.json")
	if err := os.MkdirAll(filepath.Dir(catalog), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalog, []byte("[]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupParent := filepath.Join(root, "backups")
	stageParent := filepath.Join(root, "stages")
	for _, dir := range []string{backupParent, stageParent} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := run(ctx, []string{"snapshot", "--profile", source, "--catalog", catalog, "--out-parent", backupParent}, &output); err == nil {
		t.Fatal("snapshot without writer-stop confirmation succeeded")
	}
	snapshot := commandResult(t, "snapshot", "--profile", source, "--catalog", catalog, "--out-parent", backupParent, "--writers-stopped")["snapshot"].(string)
	stage := commandResult(t, "stage", "--snapshot", snapshot, "--out-parent", stageParent)["stage"].(string)
	output.Reset()
	if err := run(ctx, []string{"list", "--stage", stage, "--selection-out", filepath.Join(source, "selection.json")}, &output); err == nil {
		t.Fatal("list wrote a review template into the source profile")
	}
	output.Reset()
	if err := run(ctx, []string{"list", "--stage", stage, "--selection-out", filepath.Join(snapshot, "selection.json")}, &output); err == nil {
		t.Fatal("list changed the verified snapshot")
	}
	selectionPath := filepath.Join(root, "selection.json")
	commandResult(t, "list", "--stage", stage, "--selection-out", selectionPath)
	var selection selectionDocument
	if err := readStrictJSON(selectionPath, &selection); err != nil {
		t.Fatal(err)
	}
	if len(selection.Candidates) != 1 || selection.Candidates[0].ID != "orphan" || selection.Candidates[0].Title != nil || selection.Candidates[0].Selected {
		t.Fatalf("read-only selection template = %#v", selection)
	}
	selection.Candidates[0].Selected = true
	title, workspace := "Confirmed title", "/work/confirmed"
	selection.Candidates[0].Title = &title
	selection.Candidates[0].WorkspaceRoot = &workspace
	encoded, err := json.Marshal(selection)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(selectionPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(root, "plan.json")
	approval := commandResult(t, "review", "--stage", stage, "--selection", selectionPath, "--plan-out", planPath)["approve"].(string)
	output.Reset()
	if err := run(ctx, []string{"apply", "--stage", stage, "--plan", planPath, "--approve", "wrong"}, &output); err == nil {
		t.Fatal("unapproved plan applied")
	}
	result := commandResult(t, "apply", "--stage", stage, "--plan", planPath, "--approve", approval)
	if result["applied"] != float64(1) {
		t.Fatalf("apply result = %#v", result)
	}
	if _, err := os.Stat(identityPath(source)); !os.IsNotExist(err) {
		t.Fatalf("source identity database was written: %v", err)
	}
	identities, err := sessionidentity.OpenReadOnly(ctx, identityPath(filepath.Join(stage, "profile")), filepath.Join(stage, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	record, found, err := identities.Get(ctx, "orphan")
	if err != nil || !found || record.Title != title || record.WorkspaceRoot != workspace {
		t.Fatalf("staged identity = %#v, found=%v, err=%v", record, found, err)
	}
	got, err := os.ReadFile(transcript)
	if err != nil || string(got) != "{\"role\":\"user\",\"content\":\"hello\"}\n" {
		t.Fatalf("source transcript changed: %q, %v", got, err)
	}
}
