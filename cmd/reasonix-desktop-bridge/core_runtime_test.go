package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBridgeSessionPathIsDeterministicAndContained(t *testing.T) {
	dir := t.TempDir()
	path, err := bridgeSessionPath(dir, "preview_42-a")
	if err != nil {
		t.Fatalf("bridge session path: %v", err)
	}
	want := filepath.Join(dir, "tauri-preview_42-a.jsonl")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if _, err := bridgeSessionPath(dir, "../outside"); err == nil {
		t.Fatal("unsafe bridge session ID produced a path")
	}
}

func TestTruncateBridgeHistoryContentPreservesUnicodeAndMarksTruncation(t *testing.T) {
	content := strings.Repeat("界", bridgeHistoryMaxContentRunes+1)
	got, truncated := truncateBridgeHistoryContent(content)
	if !truncated || !strings.HasPrefix(got, strings.Repeat("界", bridgeHistoryMaxContentRunes)) || !strings.HasSuffix(got, "[Preview truncated this message]") {
		t.Fatalf("history truncation = %q, %v", got, truncated)
	}
}
