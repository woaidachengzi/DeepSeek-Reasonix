package pinnedcontext

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/store"
)

func TestLoaderReadsCurrentSessionFiles(t *testing.T) {
	root := t.TempDir()
	session := filepath.Join(root, "owned.jsonl")
	file := filepath.Join(root, "owned.txt")
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(store.SessionPinnedContext(session), `{"schemaVersion":1,"sessionId":"owned","files":["owned.txt"]}`)
	load := Loader(root, nil)
	for _, content := range []string{"initial 中文", "changed 中文"} {
		write(file, content)
		snapshot, err := load(context.Background(), session)
		if err != nil || len(snapshot.Files) != 1 || snapshot.Files[0].Content != content {
			t.Fatalf("current snapshot: %+v, %v", snapshot, err)
		}
	}
	write(store.SessionPinnedContext(session), `{"schemaVersion":1,"sessionId":"owned","files":[]}`)
	snapshot, err := load(context.Background(), session)
	if err != nil || len(snapshot.Files) != 0 || len(snapshot.Issues) != 0 {
		t.Fatalf("revoked snapshot: %+v, %v", snapshot, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := load(ctx, session); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled loader performed work", err)
	}
}

func TestLoadStateRejectsInvalidManifest(t *testing.T) {
	for name, content := range map[string]string{
		"wrong_session": `{"schemaVersion":1,"sessionId":"another","files":[]}`,
		"schema":        `{"schemaVersion":999,"sessionId":"owned","files":[]}`,
		"traversal":     `{"schemaVersion":1,"sessionId":"owned","files":["../outside.txt"]}`,
		"bad_json":      `{`,
		"oversize":      strings.Repeat(" ", maxPinnedContextStateBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			session := filepath.Join(t.TempDir(), "owned.jsonl")
			if err := os.WriteFile(store.SessionPinnedContext(session), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadState(session); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
}

func TestLoadStateRejectsOutsideSymlink(t *testing.T) {
	session := filepath.Join(t.TempDir(), "owned.jsonl")
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"schemaVersion":1,"sessionId":"owned","files":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, store.SessionPinnedContext(session)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := LoadState(session); err == nil {
		t.Fatal("outside manifest symlink accepted")
	}
}

func TestLoadStateAcceptsRelativeSessionPath(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(store.SessionPinnedContext("owned.jsonl"), []byte(`{"schemaVersion":1,"sessionId":"owned","files":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState("owned.jsonl"); err != nil {
		t.Fatal(err)
	}
}

func TestBuildConfinesReadsAndBoundsGrowth(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside-private-marker"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	snapshot := Build(root, []string{"link.txt"}, nil).Snapshot
	if len(snapshot.Files) != 0 || len(snapshot.Issues) != 1 {
		t.Fatal("outside symlink accepted")
	}
	file := filepath.Join(root, "growth.txt")
	if err := os.WriteFile(file, []byte("initial"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot = Build(root, []string{"growth.txt"}, func() {
		if err := os.WriteFile(file, []byte(strings.Repeat("x", agent.MaxPinnedContextFileBytes+1)), 0600); err != nil {
			t.Fatal(err)
		}
	}).Snapshot
	if len(snapshot.Files) != 0 || len(snapshot.Issues) != 1 || snapshot.Issues[0].Reason != agent.PinnedContextIssueFileTooLarge {
		t.Fatal("growing file escaped limit")
	}
}
