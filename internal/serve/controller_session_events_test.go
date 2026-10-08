package serve

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/remote/controller"
)

// Real Serve token gate, real catalogue/Controller and production broadcaster.
// Published runtime events are owned fixtures, not a claim of model/SSH/native
// lifecycle acceptance. Opening this reader must not resume the saved target.
func TestSharedControllerSessionEventsActualServeBackgroundIsolation(t *testing.T) {
	f, c := desktopViewFixture(t)
	selected := filepath.Join(f.dir, "background.jsonl")
	saveServeTestSession(t, selected)
	before, err := os.ReadFile(selected)
	if err != nil {
		t.Fatal(err)
	}
	path := agent.CanonicalSessionPath(selected)
	active := agent.CanonicalSessionPath(f.active)
	f.server.bc.SetCurrentSession(active)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stream, err := c.SessionEvents(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	f.server.bc.Emit(event.Event{Kind: event.Text, SessionPath: active, Text: "foreign foreground"})
	f.server.bc.Emit(event.Event{Kind: event.Text, Text: "untagged foreground"})
	f.server.bc.Emit(event.Event{Kind: event.Reasoning, SessionPath: path, TurnID: "owned-turn", Sequence: 1, Text: "owned reasoning"})
	f.server.bc.Emit(event.Event{Kind: event.Text, SessionPath: path, TurnID: "owned-turn", Sequence: 2, Text: "owned answer"})
	f.server.bc.Emit(event.Event{Kind: event.TurnDone, SessionPath: path, TurnID: "owned-turn", Sequence: 3})
	for i, kind := range []string{"reasoning", "text", "turn_done"} {
		frame, err := stream.Next()
		if err != nil || frame.Kind != kind || frame.SessionPath != path || frame.SessionCurrent || frame.TurnID != "owned-turn" || frame.Sequence != uint64(i+1) {
			t.Fatalf("background correlation failed: %+v %v", frame, err)
		}
	}
	if f.server.ctl().SessionPath() != f.active {
		t.Fatal("event subscription resumed selected history")
	}
	after, err := os.ReadFile(selected)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("event subscription rewrote saved session")
	}
	c.Close()
	if _, err := stream.Next(); !errors.Is(err, controller.ErrClosed) {
		t.Fatal("revoked owner still read production events")
	}
}
