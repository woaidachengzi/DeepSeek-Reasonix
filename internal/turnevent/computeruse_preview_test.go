package turnevent

import (
	"bytes"
	"os"
	"testing"

	"reasonix/internal/event"
)

func TestPreviewImageIsLiveOnly(t *testing.T) {
	path := testSessionPath(t)
	l := openTestLedger(t, path, "preview")
	if _, err := l.Begin(); err != nil {
		t.Fatal(err)
	}
	e := event.Event{Kind: event.ToolProgress, Tool: event.Tool{ID: "call", Name: "computer_use", ComputerUsePreview: &event.ComputerUsePreview{Image: "PRIVATE-SCREENSHOT"}}}
	live, ok, err := l.Append(e, event.TurnInProgress)
	if err != nil || !ok || live.Tool.ComputerUsePreview == nil {
		t.Fatalf("live image missing: %v", err)
	}
	records, err := l.EventsAfter(0)
	if err != nil || len(records) != 1 || records[0].Event.Tool.ComputerUsePreview != nil {
		t.Fatalf("preview leaked to replay: %v", err)
	}
	raw, err := os.ReadFile(l.path)
	if err != nil || bytes.Contains(raw, []byte("PRIVATE-SCREENSHOT")) {
		t.Fatalf("preview persisted: %v", err)
	}
}
