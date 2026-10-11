package control

import (
	"errors"
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
)

func TestPinnedMutationOwnsAdmissionGateAndReleasesOnError(t *testing.T) {
	exec := agent.New(nil, nil, agent.NewSession("base"), agent.Options{}, event.Discard)
	path := filepath.Join(t.TempDir(), "session.jsonl")
	c := New(Options{Executor: exec, Runner: exec, SessionPath: path, Sink: event.Discard})
	defer c.Close()
	failure := errors.New("save failed")
	if err := c.MutatePinnedContext(func(got string) error {
		if got != path {
			t.Fatalf("path: %q", got)
		}
		if err := c.beginRotation(); !IsSessionRotationBusy(err) {
			t.Fatalf("rotation was admitted: %v", err)
		}
		if err := c.MutatePinnedContext(func(string) error { t.Fatal("nested mutation admitted"); return nil }); !IsSessionRotationBusy(err) {
			t.Fatalf("nested: %v", err)
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatalf("lost save error: %v", err)
	}
	if err := c.beginRotation(); err != nil {
		t.Fatalf("gate leaked on error: %v", err)
	}
	c.endRotation()
	for _, state := range []string{"running", "finishing"} {
		c.mu.Lock()
		c.running, c.finishing = state == "running", state == "finishing"
		c.mu.Unlock()
		if err := c.MutatePinnedContext(func(string) error { t.Fatal("active turn allowed mutation"); return nil }); !IsSessionRotationBusy(err) {
			t.Fatalf("%s: %v", state, err)
		}
	}
	c.mu.Lock()
	c.running, c.finishing = false, false
	c.mu.Unlock()
	if err := c.MutatePinnedContext(func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	c.Close()
	if err := c.MutatePinnedContext(func(string) error { t.Fatal("closed controller allowed mutation"); return nil }); err == nil {
		t.Fatal("closed controller was accepted")
	}
}
