package control

import (
	"context"
	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
	"testing"
	"time"
)

func TestContinueTurnUsesHostOriginAndOriginalPolicySource(t *testing.T) {
	sess := agent.NewSession("sys")
	prov := &scriptedTurns{turns: [][]provider.Chunk{textTurn("first answer"), textTurn("continued answer")}}
	exec := agent.New(prov, tool.NewRegistry(), sess, agent.Options{}, event.Discard)
	c := New(Options{Runner: exec, Executor: exec, Label: "continue-test"})
	defer c.Close()
	if c.ContinueTurn() {
		t.Fatal("empty conversation admitted continuation")
	}
	const raw = "Only inspect this project; do not edit files."
	if err := newTurnOrchestrator(c).runTurnWithRawDisplay(context.Background(), raw, raw, ""); err != nil {
		t.Fatal(err)
	}
	if !c.ContinueTurn() {
		t.Fatal("existing conversation rejected continuation")
	}
	deadline := time.Now().Add(5 * time.Second)
	for c.Running() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if c.Running() {
		t.Fatal("continuation did not finish")
	}
	users, hosts := 0, 0
	for _, message := range sess.Snapshot() {
		if message.Role != provider.RoleUser {
			continue
		}
		if message.Origin == provider.MessageOriginHost {
			hosts++
			if message.RawContent != raw {
				t.Fatal("continuation lost original policy source")
			}
		} else {
			users++
		}
	}
	if users != 1 || hosts != 1 {
		t.Fatal("continuation created a user turn", users, hosts)
	}
	if len(c.Checkpoints()) != 1 {
		t.Fatal("continuation created a visible rewind checkpoint")
	}
}
