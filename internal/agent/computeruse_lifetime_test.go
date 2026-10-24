package agent

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

type desktopLifetimeFixture struct{ seen chan context.Context }

func (desktopLifetimeFixture) Name() string            { return "computer_use" }
func (desktopLifetimeFixture) Description() string     { return "local lifetime probe" }
func (desktopLifetimeFixture) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (desktopLifetimeFixture) ReadOnly() bool          { return true }
func (f desktopLifetimeFixture) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	f.seen <- tool.ComputerUseLifetimeFrom(ctx)
	return `{"dispatched":false}`, nil
}

func TestComputerUseRunOwnsLifetimeEvenWhenCallerStaysAlive(t *testing.T) {
	probe := desktopLifetimeFixture{seen: make(chan context.Context, 1)}
	registry := tool.NewRegistry()
	registry.Add(probe)
	p := &scriptedProvider{name: "fixture", turns: [][]provider.Chunk{
		{toolCallChunk("desktop", "computer_use", `{}`), {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "done"}, {Type: provider.ChunkDone}},
	}}
	a := New(p, registry, NewSession(""), Options{}, event.Discard)
	if err := a.Run(t.Context(), "probe local lifetime"); err != nil {
		t.Fatal(err)
	}
	lifetime := <-probe.seen
	if lifetime == nil || lifetime.Err() != context.Canceled || t.Context().Err() != nil {
		t.Fatal("Run did not end its private desktop lifetime independently of its caller")
	}
}
