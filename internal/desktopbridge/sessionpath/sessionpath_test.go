package sessionpath

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTranscriptPathIsFlatAndDeterministic(t *testing.T) {
	dir := filepath.Join("/state", "sessions")
	path, err := TranscriptPath(dir, "tauri-abc")
	if err != nil {
		t.Fatalf("TranscriptPath: %v", err)
	}
	if want := filepath.Join(dir, "tauri-tauri-abc.jsonl"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestTranscriptPathRejectsUnusableInput(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name      string
		sessionID string
	}{
		{"empty", ""},
		{"path separator", "../outside"},
		{"slash", "a/b"},
		{"dot", "a.b"},
		{"space", "a b"},
		{"too long", strings.Repeat("x", 129)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := TranscriptPath(dir, tc.sessionID); err == nil {
				t.Fatalf("session ID %q produced a path", tc.sessionID)
			}
			if ValidID(tc.sessionID) {
				t.Fatalf("ValidID(%q) = true", tc.sessionID)
			}
		})
	}
	if _, err := TranscriptPath("  ", "tauri-abc"); err == nil {
		t.Fatal("an empty session directory produced a path")
	}
	if _, err := TranscriptPath(dir, strings.Repeat("x", 128)); err != nil {
		t.Fatalf("a 128-byte ID must be accepted: %v", err)
	}
}

// The shape is load-bearing: the importer recovers an ID from a file name, so a
// transcript name must round-trip through the name prefix.
func TestTranscriptPathRoundTripsThroughItsNamePrefix(t *testing.T) {
	dir := t.TempDir()
	path, err := TranscriptPath(dir, "tauri-abc")
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(path)
	if !strings.HasPrefix(name, "tauri-") || !strings.HasSuffix(name, ".jsonl") {
		t.Fatalf("transcript name = %q", name)
	}
	recovered := strings.TrimSuffix(strings.TrimPrefix(name, "tauri-"), ".jsonl")
	if recovered != "tauri-abc" {
		t.Fatalf("recovered ID = %q", recovered)
	}
}
