package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"reasonix/internal/eventwire"
	"reasonix/internal/remote/controller"
)

func TestPreviewDesktopPendingUnifiedLocalRemoteAndOriginalCapture(t *testing.T) {
	var generation, reads atomic.Int32
	generation.Store(1)
	b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
		epoch := "remote-1"
		if generation.Load() != 1 {
			epoch = "remote-2"
		}
		rows := []controller.Session{{Path: "/remote/live.jsonl"}}
		if r.URL.Path == "/runtime-states" {
			_ = json.NewEncoder(w).Encode(catalogueFixtureStates(rows, epoch))
			return
		}
		reads.Add(1)
		if r.URL.Path != "/desktop/session-pending" {
			t.Error("pending reader used historical fallback")
			w.WriteHeader(400)
			return
		}
		var input struct {
			ProtocolVersion int `json:"protocolVersion"`
			controller.SessionPendingScope
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.SessionPath != rows[0].Path || input.RuntimeEpoch != epoch+"-"+rows[0].Path {
			w.WriteHeader(409)
			return
		}
		prompt := controller.SessionPendingPrompt{Scope: controller.SessionPromptScope{SessionPath: input.SessionPath, RuntimeEpoch: input.RuntimeEpoch, TurnID: "turn", PromptID: "private-prompt", PromptRuntimeEpoch: "routing", Kind: "ask"}, Ask: &eventwire.Ask{ID: "private-prompt", TurnID: "turn", Questions: []eventwire.AskQuestion{{ID: "q", Prompt: "private question"}}}}
		_ = json.NewEncoder(w).Encode(controller.SessionPendingView{ProtocolVersion: 1, SessionPendingScope: input.SessionPendingScope, Revision: 1, TurnID: "turn", Prompts: []controller.SessionPendingPrompt{prompt}})
	})
	remoteView := attachController(t, b, "/owned")
	manager := catalogueActualManager(t)
	c := newPreviewDesktopCatalogue(manager, b.remoteSessions)
	defer c.Close()
	entries, err := c.Refresh(context.Background())
	if err != nil || len(entries) != 2 {
		t.Fatal(entries, err)
	}
	var local, remote previewDesktopCatalogueEntry
	for _, entry := range entries {
		if entry.local != nil {
			local = entry
		} else {
			remote = entry
		}
	}
	got, err := c.ReadPending(context.Background(), local)
	if err != nil || len(got.Prompts) != 0 || got.RuntimeEpoch != local.local.Scope.RuntimeEpoch {
		t.Fatal(got, err)
	}
	got, err = c.ReadPending(context.Background(), remote)
	if err != nil || len(got.Prompts) != 1 || got.Prompts[0].Scope.RuntimeEpoch != remote.remoteEpoch {
		t.Fatal(got, err)
	}
	got.Prompts[0].Ask.Questions[0].Prompt = "mutated"
	again, err := c.ReadPending(context.Background(), remote)
	if err != nil || again.Prompts[0].Ask.Questions[0].Prompt == "mutated" {
		t.Fatal("consumer mutated snapshot", err)
	}
	generation.Store(2)
	if _, err := c.ReadPending(context.Background(), remote); err == nil {
		t.Fatal("same path adopted replacement prompt owner")
	}
	if _, err := manager.SetSessionModel(context.Background(), local.local.Scope.SessionID, "local/beta"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReadPending(context.Background(), local); err == nil {
		t.Fatal("same local ID adopted replacement")
	}
	b.remoteSessions.closeController(remoteView.ID)
	before := reads.Load()
	if _, err := c.ReadPending(context.Background(), remote); err == nil || reads.Load() != before {
		t.Fatal("closed connection dispatched read")
	}
	c.Close()
	if _, err := c.ReadPending(context.Background(), local); err == nil {
		t.Fatal("closed catalogue read admitted")
	}
}
