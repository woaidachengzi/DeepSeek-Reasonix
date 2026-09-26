package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"reasonix/internal/desktopbridge"
)

func TestProjectFoldersMissingFileReturnsEmptySnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)

	response := httptest.NewRecorder()
	(&bridgeServer{}).projectFolders(response, httptest.NewRequest(http.MethodGet, "/v1/projects", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var result projectFoldersResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.ProtocolVersion != desktopbridge.ProtocolVersion {
		t.Fatalf("protocol version = %d, want %d", result.ProtocolVersion, desktopbridge.ProtocolVersion)
	}
	if result.Projects == nil || len(result.Projects) != 0 {
		t.Fatalf("projects = %#v, want an empty array", result.Projects)
	}
	if _, err := os.Stat(filepath.Join(home, legacyProjectFoldersFile)); !os.IsNotExist(err) {
		t.Fatalf("missing catalog was created or stat failed: %v", err)
	}
}

func TestProjectFoldersReadsSanitizedSnapshotWithoutWriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, legacyProjectFoldersFile)
	contents := []byte(`{"projects":[{"root":"/work/alpha ","title":" Alpha "},{"root":"/work/alpha","title":"Duplicate"},{"root":"","title":"Invalid"},{"root":"/work/beta"}]}`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	(&bridgeServer{}).projectFolders(response, httptest.NewRequest(http.MethodGet, "/v1/projects", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var result projectFoldersResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := []projectFolder{
		{Root: "/work/alpha ", Title: "Alpha"},
		{Root: "/work/alpha", Title: "Duplicate"},
		{Root: "/work/beta"},
	}
	if !reflect.DeepEqual(result.Projects, want) {
		t.Fatalf("projects = %#v, want %#v", result.Projects, want)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, contents) {
		t.Fatal("read-only project endpoint modified the saved catalog")
	}
}

func TestProjectFoldersRejectsMalformedSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	path := filepath.Join(home, legacyProjectFoldersFile)
	contents := []byte(`{"projects":[`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	(&bridgeServer{}).projectFolders(response, httptest.NewRequest(http.MethodGet, "/v1/projects", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusInternalServerError, response.Body.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, contents) {
		t.Fatal("malformed saved catalog was modified")
	}
}
