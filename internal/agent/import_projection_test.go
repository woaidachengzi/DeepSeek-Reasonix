package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

func TestImportedHistoryRetainsCompactedModelProjectionAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "imported.jsonl")
	history := []provider.Message{
		{Role: provider.RoleSystem, Content: "original system"},
		{Role: provider.RoleUser, Content: "earlier question"},
		{Role: provider.RoleAssistant, Content: "earlier answer"},
		{Role: provider.RoleUser, Content: "latest question"},
		{Role: provider.RoleAssistant, Content: "latest answer"},
	}
	model := []provider.Message{
		{Role: provider.RoleSystem, Content: "original system"},
		{Role: provider.RoleUser, Content: "retained summary"},
		{Role: provider.RoleAssistant, Content: "latest answer"},
	}
	if err := SaveImportedProjection(path, history, model, "workspace", "provider/model"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		loaded, err := LoadSession(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := loaded.Snapshot(); len(got) != len(history) || got[2].Content != "earlier answer" {
			t.Fatalf("lost display history: %+v", got)
		}
		a := New(nil, nil, loaded, Options{SessionPath: path, WorkspaceID: "workspace", ModelRef: "provider/model"}, event.Discard)
		if !projectionContentValid(a.sess.compactionState, loaded.Snapshot()) {
			t.Fatal("import checkpoint invalid after restart")
		}
		got := provider.ModelMessages(modelVisibleFromProjection(a.sess.compactionState.Projection, loaded.Snapshot()))
		expected := append([]provider.Message(nil), model...)
		// Leading system identity is refreshed from canonical storage; it is
		// local metadata, while the provider-visible prompt/body stays identical.
		expected[0].ID = loaded.Snapshot()[0].ID
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("model workset replayed full history: %+v", got)
		}
		loaded.Add(provider.Message{Role: provider.RoleUser, Content: "continue"})
		got = modelVisibleFromProjection(a.sess.compactionState.Projection, loaded.Snapshot())
		if len(got) != len(model)+1 || got[len(got)-1].Content != "continue" {
			t.Fatal("new turn not spliced into model context")
		}
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveImportedProjection(path, history, model, "workspace", "provider/model"); err == nil {
		t.Fatal("overwrote existing import")
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("retry changed existing history")
	}
}
