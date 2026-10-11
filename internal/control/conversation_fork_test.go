package control

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/provider"
)

func TestForkCurrentSessionRejectsUnsafeTargetExistingFileAndRunningTurn(t *testing.T) {
	c, _, parent := newSchemaTwoBranchController(t)
	defer c.Close()
	dir := filepath.Dir(parent)
	for _, target := range []string{parent, filepath.Join(t.TempDir(), "outside.jsonl"), filepath.Join(dir, "wrong.txt")} {
		if err := c.ForkCurrentSessionToPath(target, "child"); err == nil {
			t.Fatalf("accepted unsafe target %s", target)
		}
	}
	target := filepath.Join(dir, "child.jsonl")
	if err := os.WriteFile(target, []byte("keep existing file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.ForkCurrentSessionToPath(target, "child"); err == nil {
		t.Fatal("overwrote an existing child")
	}
	content, _ := os.ReadFile(target)
	if string(content) != "keep existing file" {
		t.Fatal("existing target changed")
	}
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()
	runningTarget := filepath.Join(dir, "running.jsonl")
	if err := c.ForkCurrentSessionToPath(runningTarget, "child"); err == nil {
		t.Fatal("accepted a running turn")
	}
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
	if _, err := os.Stat(runningTarget); !os.IsNotExist(err) {
		t.Fatal("running fork wrote a file")
	}
	if c.SessionPath() != parent {
		t.Fatal("rejected fork changed the parent")
	}
}

func TestMessageForkPreservesEarlierBoundaryAndRejectsChangedPrefix(t *testing.T) {
	c, session, parent := newSchemaTwoBranchController(t)
	defer c.Close()
	original := session.Snapshot()
	tokens, err := agent.ConversationPrefixTokens(original)
	if err != nil {
		t.Fatal(err)
	}
	answerToken := tokens[len(tokens)-1]
	session.Add(provider.Message{Role: provider.RoleUser, Content: "later prompt"})
	session.Add(provider.Message{Role: provider.RoleAssistant, Content: "later answer"})
	target := filepath.Join(filepath.Dir(parent), "earlier.jsonl")
	if err := c.ForkSessionToPath(target, "earlier", answerToken); err != nil {
		t.Fatal(err)
	}
	child, err := agent.LoadSession(target)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(child.Snapshot(), original) {
		t.Fatal("earlier fork included later messages")
	}
	// A token from a different revision of the same visible answer cannot select
	// an unrelated prefix, even when its message position and text match.
	changed := append([]provider.Message(nil), original...)
	changed[1].Content = "changed earlier user prompt"
	changedTokens, _ := agent.ConversationPrefixTokens(changed)
	for index, token := range []string{strings.Repeat("0", 64), tokens[1], changedTokens[len(changedTokens)-1]} {
		rejected := filepath.Join(filepath.Dir(parent), fmt.Sprintf("invalid-%d.jsonl", index))
		if err := c.ForkSessionToPath(rejected, "invalid", token); err == nil {
			t.Fatal("invalid boundary accepted")
		}
		if _, err := os.Stat(rejected); !os.IsNotExist(err) {
			t.Fatal("invalid boundary wrote a child")
		}
	}
	if c.SessionPath() != parent || session.Len() != len(original)+2 {
		t.Fatal("fork changed the source")
	}
}
