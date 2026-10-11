package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/pinnedcontext"
	"reasonix/internal/provider"
	"reasonix/internal/sessionidentity"
	"reasonix/internal/store"
)

func TestConversationForkRejectsCorruptPinnedManifestBeforeCreatingChild(t *testing.T) {
	state := t.TempDir()
	t.Setenv("REASONIX_HOME", state)
	t.Setenv("REASONIX_STATE_HOME", state)
	dir := appconfig.SessionDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path, err := bridgeSessionPath(dir, "tauri-parent")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"role\":\"assistant\",\"content\":\"answer\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := store.SessionPinnedContext(path)
	if err := os.WriteFile(manifest, []byte("broken manifest"), 0600); err != nil {
		t.Fatal(err)
	}
	ag := agent.New(nil, nil, agent.NewSession("base"), agent.Options{}, event.Discard)
	ctrl := control.New(control.Options{Executor: ag, Runner: ag, Sink: event.Discard, SessionDir: dir, SessionPath: path})
	defer ctrl.Close()
	result, err := (&controllerRuntime{controller: ctrl}).ForkConversation("")
	if err == nil || result.SessionID != "" || !strings.Contains(err.Error(), "load source pinned files") {
		t.Fatalf("fork did not reject corrupt context: %+v %v", result, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed preflight created child files: %+v %v", entries, err)
	}
	// Controller initialization creates parent-owned inbox/event sidecars.
	// A rejected fork must create no files under a new session identity.
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), strings.TrimSuffix(filepath.Base(path), ".jsonl")+".") {
			t.Fatalf("failed preflight created child artifact: %s", entry.Name())
		}
	}
	raw, err := os.ReadFile(manifest)
	if err != nil || string(raw) != "broken manifest" {
		t.Fatalf("failed preflight changed source: %q %v", raw, err)
	}
}

func TestConversationForkCopiesVisibleHistoryAndRegistersIndependentChild(t *testing.T) {
	for _, format := range []string{"legacy", "schema2-selected-head", "legacy-early", "schema2-selected-head-early"} {
		t.Run(format, func(t *testing.T) {
			state, workspace := t.TempDir(), t.TempDir()
			t.Setenv("REASONIX_HOME", state)
			t.Setenv("REASONIX_STATE_HOME", state)
			dir := appconfig.SessionDir()
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			parentID := "tauri-parent"
			parentPath, err := bridgeSessionPath(dir, parentID)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(parentPath, []byte("{\"role\":\"system\",\"content\":\"sys\"}\n{\"role\":\"user\",\"content\":\"first\"}\n{\"role\":\"assistant\",\"content\":\"answer\"}\n{\"role\":\"user\",\"content\":\"later question\"}\n{\"role\":\"assistant\",\"content\":\"later answer\"}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			session, err := agent.LoadSession(parentPath)
			if err != nil {
				t.Fatal(err)
			}
			ag := agent.New(nil, nil, session, agent.Options{}, event.Discard)
			ctrl := control.New(control.Options{Executor: ag, Runner: ag, Sink: event.Discard, SessionDir: dir, SessionPath: parentPath, WorkspaceRoot: workspace})
			t.Cleanup(ctrl.Close)
			if strings.HasPrefix(format, "schema2") {
				if err := ctrl.SnapshotRewrite(); err != nil {
					t.Fatal(err)
				}
				if _, err := ctrl.Branch("selected variant"); err != nil {
					t.Fatal(err)
				}
				session.Add(provider.Message{Role: provider.RoleUser, Content: "selected branch prompt"})
				session.Add(provider.Message{Role: provider.RoleAssistant, Content: "selected branch answer"})
				if err := ctrl.Snapshot(); err != nil {
					t.Fatal(err)
				}
			}
			if err := agent.RenameSession(parentPath, "原始对话"); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
			if err != nil {
				t.Fatal(err)
			}
			if err := identities.ImportLegacyCatalog(ctx, dir, []sessionidentity.Candidate{{ID: parentID, Path: parentPath, WorkspaceRoot: workspace, Title: "原始对话"}}); err != nil {
				t.Fatal(err)
			}
			identities.Close()
			before, err := os.ReadFile(parentPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("pinned context"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := pinnedcontext.SaveState(parentPath, []string{"notes.txt"}); err != nil {
				t.Fatal(err)
			}
			sourceHead, hasHead := ctrl.SessionHead()
			if (strings.HasPrefix(format, "schema2")) != hasHead {
				t.Fatalf("unexpected fixture format: %s head=%v", format, hasHead)
			}
			sourceMessages := session.Snapshot()
			runtime := &controllerRuntime{controller: ctrl, sessionID: parentID}
			expected := sourceMessages
			forkToken := ""
			if strings.HasSuffix(format, "-early") {
				projected := runtime.History()
				forkToken = projected[1].ForkToken
				if forkToken == "" {
					t.Fatal("completed reply has no fork token")
				}
				expected = sourceMessages[:3] // Includes hidden system prompt, excludes later turns.
			}
			result, err := runtime.ForkConversation(forkToken)
			if err != nil || !result.OK || result.SessionID == "" || result.SessionID == parentID {
				t.Fatalf("fork=%+v err=%v", result, err)
			}
			currentHead, _ := ctrl.SessionHead()
			if currentHead != sourceHead || ctrl.SessionPath() != parentPath {
				t.Fatal("source path/head changed")
			}
			current, _ := os.ReadFile(parentPath)
			if !reflect.DeepEqual(current, before) {
				t.Fatal("fork changed the source file")
			}
			childPath, err := bridgeSessionPath(dir, result.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			child, err := agent.LoadSession(childPath)
			if err != nil {
				t.Fatal(err)
			}
			pins, err := pinnedcontext.LoadState(childPath)
			if err != nil || pins.SessionID != agent.BranchID(childPath) || !reflect.DeepEqual(pins.Files, []string{"notes.txt"}) {
				t.Fatalf("child pins=%+v err=%v", pins, err)
			}
			if _, err := pinnedcontext.Change(workspace, childPath, "notes.txt", false); err != nil {
				t.Fatal(err)
			}
			pins, err = pinnedcontext.LoadState(parentPath)
			if err != nil || !reflect.DeepEqual(pins.Files, []string{"notes.txt"}) {
				t.Fatalf("child unpin changed source pins=%+v err=%v", pins, err)
			}
			if !reflect.DeepEqual(child.Snapshot(), expected) {
				t.Fatalf("forked visible chain differs: %v vs %v", child.Snapshot(), expected)
			}
			meta, ok, err := agent.LoadBranchMeta(childPath)
			if err != nil || !ok || meta.CustomTitle != "原始对话（分叉）" || meta.ParentID != agent.BranchID(parentPath) {
				t.Fatalf("metadata=%+v err=%v", meta, err)
			}
			reopened, err := newControllerFactory(nil).Open(ctx, desktopbridge.OpenRequest{SessionID: result.SessionID, WorkspaceRoot: workspace})
			if err != nil {
				t.Fatalf("identity cannot reopen: %v", err)
			}
			reopened.Shutdown()
			child, err = agent.LoadSession(childPath)
			if err != nil {
				t.Fatal(err)
			}
			child.Add(provider.Message{Role: provider.RoleUser, Content: "child-only continuation"})
			if err := child.Save(childPath); err != nil {
				t.Fatal(err)
			}
			original, err := agent.LoadSession(parentPath)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(original.Snapshot(), sourceMessages) {
				t.Fatal("writing child changed parent")
			}
			original.Add(provider.Message{Role: provider.RoleUser, Content: "parent-only continuation"})
			if err := original.Save(parentPath); err != nil {
				t.Fatal(err)
			}
			child, err = agent.LoadSession(childPath)
			if err != nil {
				t.Fatal(err)
			}
			last := child.Snapshot()
			if len(last) != len(expected)+1 || last[len(last)-1].Content != "child-only continuation" {
				t.Fatal("writing parent changed child")
			}
		})
	}
}

func TestConversationForkRouteScopesBusyStateAndDeduplicates(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/parent", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	handler := newBridgeServer(testToken, "instance", manager).handler()
	call := func(path, body, id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(requestIDHeader, id)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	if r := call("/v1/sessions:open", `{"sessionId":"parent"}`, ""); r.Code != http.StatusOK {
		t.Fatal(r.Body.String())
	}
	if r := call("/v1/sessions/other:fork", `{}`, "other"); r.Code != http.StatusNotFound {
		t.Fatalf("wrong source=%d", r.Code)
	}
	if r := call("/v1/sessions/parent:fork", `{}`, ""); r.Code != http.StatusBadRequest {
		t.Fatalf("missing request ID=%d", r.Code)
	}
	runtime.state = "running"
	if r := call("/v1/sessions/parent:fork", `{}`, "running"); r.Code != http.StatusConflict {
		t.Fatalf("busy=%d body=%s", r.Code, r.Body.String())
	}
	runtime.state = "idle"
	body := `{"forkToken":"` + strings.Repeat("a", 64) + `"}`
	first, second := call("/v1/sessions/parent:fork", body, "fork-1"), call("/v1/sessions/parent:fork", body, "fork-1")
	if first.Code != http.StatusOK || second.Code != http.StatusOK || first.Body.String() != second.Body.String() || runtime.conversationForkCalls != 1 {
		t.Fatalf("fork idempotency: %d %d calls=%d", first.Code, second.Code, runtime.conversationForkCalls)
	}
	if runtime.conversationForkToken != strings.Repeat("a", 64) {
		t.Fatal("route discarded the reply boundary token")
	}

}

func TestConversationForkRejectsEmptyHistoryWithoutLeavingReservedIdentity(t *testing.T) {
	state := t.TempDir()
	t.Setenv("REASONIX_HOME", state)
	t.Setenv("REASONIX_STATE_HOME", state)
	dir := appconfig.SessionDir()
	os.MkdirAll(dir, 0700)
	ag := agent.New(nil, nil, agent.NewSession("sys"), agent.Options{}, event.Discard)
	ctrl := control.New(control.Options{Executor: ag, Runner: ag, Sink: event.Discard, SessionDir: dir, SessionPath: filepath.Join(dir, "parent.jsonl")})
	defer ctrl.Close()
	runtime := &controllerRuntime{controller: ctrl, sessionID: "parent"}
	if _, err := runtime.ForkConversation(""); err == nil {
		t.Fatal("empty fork accepted")
	}
	transcripts, _ := filepath.Glob(filepath.Join(dir, "tauri-*.jsonl"))
	if len(transcripts) != 0 {
		t.Fatal("failed fork created a transcript")
	}
	identities, err := sessionidentity.Open(context.Background(), appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	page, err := identities.ListVisible(context.Background(), 200, nil, "")
	if err != nil || len(page.Records) != 0 {
		t.Fatalf("failed fork left a visible reserved identity: %+v, %v", page, err)
	}

}
