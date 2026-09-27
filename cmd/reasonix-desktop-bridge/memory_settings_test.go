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

	appconfig "reasonix/internal/config"
	"reasonix/internal/memory"
)

func TestPreviewMemorySettingsRoundTripAndScopeBoundaries(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	docPath := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(docPath, []byte("Old standing instructions.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := newBridgeServer(testToken, "instance-memory")
	target := "/v1/settings/memory?workspaceRoot=" + url.QueryEscape(root)
	request := func(method, path, body, id string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if id != "" {
			r.Header.Set(requestIDHeader, id)
		}
		if auth {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}
		w := httptest.NewRecorder()
		server.handler().ServeHTTP(w, r)
		return w
	}
	if got := request(http.MethodGet, target, "", "", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized read = %d", got.Code)
	}
	read := func() previewMemorySettingsView {
		t.Helper()
		w := request(http.MethodGet, target, "", "", true)
		if w.Code != http.StatusOK {
			t.Fatalf("read = %d: %s", w.Code, w.Body.String())
		}
		var view previewMemorySettingsView
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	post := func(id string, change previewMemorySettingsChange) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(change)
		if err != nil {
			t.Fatal(err)
		}
		return request(http.MethodPost, "/v1/settings/memory", string(body), id, true)
	}
	view := read()
	if view.WorkspaceRoot != root || view.StoreDir == "" || view.GlobalStoreDir == "" {
		t.Fatalf("memory view metadata = %#v", view)
	}
	var current previewMemoryDoc
	for _, doc := range view.Docs {
		if doc.Path == docPath {
			current = doc
		}
	}
	if current.Body != "Old standing instructions.\n" || current.Revision == "" {
		t.Fatalf("project doc missing: %#v", view.Docs)
	}
	change := previewMemorySettingsChange{WorkspaceRoot: root, Action: "save_doc", Path: docPath, Revision: current.Revision, Body: "New standing instructions."}
	if w := post("memory-save", change); w.Code != http.StatusOK {
		t.Fatalf("save doc = %d: %s", w.Code, w.Body.String())
	}
	if raw, err := os.ReadFile(docPath); err != nil || string(raw) != "New standing instructions.\n" {
		t.Fatalf("project doc = %s, %v", raw, err)
	}
	if w := post("memory-stale", change); w.Code != http.StatusConflict {
		t.Fatalf("stale save = %d: %s", w.Code, w.Body.String())
	}
	outside := filepath.Join(t.TempDir(), "other.md")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, revision, err := readPreviewMemoryDoc(outside)
	if err != nil {
		t.Fatal(err)
	}
	if w := post("memory-outside", previewMemorySettingsChange{WorkspaceRoot: root, Action: "save_doc", Path: outside, Revision: revision, Body: "overwrite"}); w.Code != http.StatusBadRequest {
		t.Fatalf("outside doc save = %d: %s", w.Code, w.Body.String())
	}
	if raw, _ := os.ReadFile(outside); string(raw) != "keep" {
		t.Fatalf("outside doc changed: %s", raw)
	}
	if w := post("memory-note", previewMemorySettingsChange{WorkspaceRoot: root, Action: "quick_add", Scope: "local", Body: "Keep a local note"}); w.Code != http.StatusOK {
		t.Fatalf("quick add = %d: %s", w.Code, w.Body.String())
	}
	if raw, err := os.ReadFile(filepath.Join(root, "AGENTS.local.md")); err != nil || !strings.Contains(string(raw), "Keep a local note") {
		t.Fatalf("local note = %s, %v", raw, err)
	}
	set := memory.Load(memory.Options{CWD: root, UserDir: appconfig.MemoryUserDir()})
	saved, err := set.Store.Save(memory.Memory{Name: "package-manager", Description: "Package manager", Type: memory.TypeProject, Body: "Use pnpm."})
	if err != nil {
		t.Fatal(err)
	}
	view = read()
	if len(view.Facts) != 1 || saved == "" || view.Facts[0].ID == "" {
		t.Fatalf("facts = %#v", view.Facts)
	}
	if w := post("memory-archive", previewMemorySettingsChange{WorkspaceRoot: root, Action: "archive", FactID: view.Facts[0].ID, FactRevision: view.Facts[0].Revision}); w.Code != http.StatusOK {
		t.Fatalf("archive = %d: %s", w.Code, w.Body.String())
	}
	view = read()
	if len(view.Facts) != 0 || len(view.Archives) != 1 || view.Archives[0].ID == "" {
		t.Fatalf("archived view = %#v", view)
	}
	if w := post("memory-restore", previewMemorySettingsChange{WorkspaceRoot: root, Action: "restore", Path: view.Archives[0].Path}); w.Code != http.StatusOK {
		t.Fatalf("restore = %d: %s", w.Code, w.Body.String())
	}
	view = read()
	if len(view.Facts) != 1 || len(view.Archives) != 0 {
		t.Fatalf("restored view = %#v", view)
	}
}
