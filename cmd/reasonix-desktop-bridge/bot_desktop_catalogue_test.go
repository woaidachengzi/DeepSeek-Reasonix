package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/remote/controller"
)

func catalogueFixtureView(path, epoch string) controller.SessionView {
	v := controller.SessionView{ProtocolVersion: 1, SessionPath: path, ReadOnly: true, Ownership: "serve", Current: path == "/remote/live.jsonl", History: []controller.HistoryMessage{}}
	switch path {
	case "/remote/saved.jsonl":
		v.Ownership = "saved"
	case "/remote/external.jsonl":
		v.Ownership = "external"
	default:
		v.RuntimeState = &event.RuntimeStateSnapshot{SchemaVersion: 1, RuntimeEpoch: epoch + "-" + path, Revision: 1, Phase: "idle"}
		if path == "/remote/closed.jsonl" {
			v.RuntimeState.Phase = "closed"
		}
		if path == "/remote/detached.jsonl" {
			v.RuntimeState.Running = true
			v.RuntimeState.Phase = "executing"
			v.RuntimeState.PendingPrompt = true
		}
	}
	return v
}

func catalogueFixtureStates(rows []controller.Session, epoch string) controller.RuntimeStates {
	result := controller.RuntimeStates{SchemaVersion: 1, Epoch: "serve-instance", Revision: 1, Sessions: []controller.RuntimeSessionState{}}
	for _, row := range rows {
		if row.Path == "/remote/saved.jsonl" || row.Name == "history" {
			continue
		}
		view := catalogueFixtureView(row.Path, epoch)
		state := view.RuntimeState
		if state == nil {
			state = &event.RuntimeStateSnapshot{SchemaVersion: 1, RuntimeEpoch: epoch + "-" + row.Path, Revision: 1, Phase: "idle"}
		}
		result.Sessions = append(result.Sessions, controller.RuntimeSessionState{SessionPath: row.Path, Ownership: view.Ownership, Current: view.Current, State: *state})
	}
	return result
}

func catalogueActualManager(t *testing.T) *desktopbridge.RuntimeManager {
	t.Helper()
	t.Setenv("REASONIX_STATE_HOME", t.TempDir())
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	_, endpoint := newOwnedCommandFixtureProvider(t)
	if err := appconfig.Default().SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf("default_model=\"local/alpha\"\n[desktop]\nprovider_access=[\"local\"]\n[[providers]]\nname=\"local\"\nkind=\"preview-owned-command-test\"\nbase_url=%q\nmodels=[\"alpha\",\"beta\"]\ndefault=\"alpha\"\n", endpoint)
	if err := os.WriteFile(appconfig.UserConfigPath(), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	m := desktopbridge.NewRuntimeManager(newControllerFactory(nil))
	t.Cleanup(func() { _ = m.Shutdown() })
	if _, err := m.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "private-local-owner", WorkspaceRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPreviewDesktopCatalogueActualLocalAndAllAttachedRemoteOwners(t *testing.T) {
	var reads, generation atomic.Int32
	generation.Store(1)
	rows := []controller.Session{}
	for _, name := range []string{"live", "detached", "saved", "external", "closed"} {
		rows = append(rows, controller.Session{Name: name, Path: "/remote/" + name + ".jsonl"})
	}
	for i := range 256 {
		rows = append(rows, controller.Session{Name: "history", Path: fmt.Sprintf("/remote/history-%d.jsonl", i)})
	}
	b, _, _, _ := controllerFixture(t, func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(rows) }, func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		if r.URL.Path == "/runtime-states" {
			_ = json.NewEncoder(w).Encode(catalogueFixtureStates(rows, fmt.Sprintf("remote-%d", generation.Load())))
			return
		}
		t.Error("live directory read historical view instead of runtime metadata")
		if strings.Contains(r.URL.Query().Get("session"), "history-") {
			t.Error("saved history scanned for live directory")
		}
		_ = json.NewEncoder(w).Encode(catalogueFixtureView(r.URL.Query().Get("session"), fmt.Sprintf("remote-%d", generation.Load())))
	})
	first := attachController(t, b, "/first")
	second := attachController(t, b, "/second")
	manager := catalogueActualManager(t)
	c := newPreviewDesktopCatalogue(manager, b.remoteSessions)
	defer c.Close()
	entries, err := c.Refresh(context.Background())
	if err != nil || len(entries) != 5 {
		t.Fatal("not all actual owners", len(entries), err)
	}
	byKey := make(map[previewDesktopCatalogueKey]string)
	localCount, detachedCount := 0, 0
	for _, entry := range entries {
		if !strings.HasPrefix(entry.handle, "s-") || len(entry.handle) != 34 || strings.Contains(entry.handle, "private") {
			t.Fatal("raw owner identity exposed")
		}
		byKey[entry.key()] = entry.handle
		if entry.local != nil {
			localCount++
			entry.local.Scope.RuntimeEpoch = "consumer mutation"
		} else if entry.detached {
			detachedCount++
			if !entry.running || !entry.pending {
				t.Fatal("detached state missing")
			}
		}
	}
	if localCount != 1 || detachedCount != 2 || len(byKey) != 5 {
		t.Fatal("owners conflated", localCount, detachedCount)
	}
	again, err := c.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range again {
		if byKey[entry.key()] != entry.handle {
			t.Fatal("identity changed or returned clone mutated catalogue")
		}
	}
	before := reads.Load()
	if _, err := c.Capture(context.Background(), "/remote/live.jsonl"); err == nil || reads.Load() != before {
		t.Fatal("raw path reached lookup")
	}
	oldLocal, oldRemote := "", ""
	for _, entry := range again {
		if entry.local != nil {
			oldLocal = entry.handle
		} else {
			oldRemote = entry.handle
		}
	}
	generation.Add(1)
	if _, err := c.Capture(context.Background(), oldRemote); err == nil {
		t.Fatal("same remote path adopted replacement epoch")
	}
	if _, err := manager.SetSessionModel(context.Background(), "private-local-owner", "local/beta"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Capture(context.Background(), oldLocal); err == nil {
		t.Fatal("same local ID adopted replacement owner")
	}
	b.remoteSessions.closeController(first.ID)
	remaining, err := c.Refresh(context.Background())
	if err != nil || len(remaining) != 3 {
		t.Fatal("closed remote retained", len(remaining), err)
	}
	for _, entry := range remaining {
		if entry.remote != nil && entry.remote.view.ID != second.ID {
			t.Fatal("wrong surviving transport")
		}
	}
}

func TestPreviewDesktopCataloguePendingReadRevokedAndFailedCutRetiresHandles(t *testing.T) {
	for _, mode := range []string{"connection", "catalogue"} {
		t.Run(mode, func(t *testing.T) {
			var block atomic.Bool
			entered, abort := make(chan struct{}, 1), make(chan struct{})
			b, _, _, _ := controllerFixture(t, nil, func(w http.ResponseWriter, r *http.Request) {
				if block.Load() {
					entered <- struct{}{}
					select {
					case <-r.Context().Done():
						return
					case <-abort:
						return
					}
				}
				if r.URL.Path == "/runtime-states" {
					_ = json.NewEncoder(w).Encode(catalogueFixtureStates([]controller.Session{{Path: "/remote/session.jsonl"}}, "exact-epoch"))
					return
				}
				_ = json.NewEncoder(w).Encode(catalogueFixtureView("/remote/session.jsonl", "exact-epoch"))
			})
			defer close(abort)
			view := attachController(t, b, "/remote-workspace")
			c := newPreviewDesktopCatalogue(b.runtimes, b.remoteSessions)
			defer c.Close()
			if entries, err := c.Refresh(context.Background()); err != nil || len(entries) != 1 {
				t.Fatal(entries, err)
			}
			block.Store(true)
			done := make(chan error, 1)
			go func() { _, err := c.Refresh(context.Background()); done <- err }()
			notificationWait(t, entered)
			if mode == "connection" {
				b.remoteSessions.closeController(view.ID)
			} else {
				c.Close()
			}
			if err := notificationWait(t, done); err == nil {
				t.Fatal("late cut published")
			}
			c.mu.Lock()
			count := len(c.entries)
			c.mu.Unlock()
			if count != 0 {
				t.Fatal("failed read retained handles")
			}
			if _, exists := b.runtimes.CommandSnapshot(); exists {
				t.Fatal("remote catalogue restored local Controller")
			}
		})
	}
}

func TestPreviewDesktopCatalogueLimitAndEmptyDoNotClaimPartialDirectory(t *testing.T) {
	var views atomic.Int32
	rows := make([]controller.Session, previewDesktopCatalogueLimit+1)
	for i := range rows {
		rows[i] = controller.Session{Name: fmt.Sprint(i), Path: fmt.Sprintf("/remote/%d.jsonl", i)}
	}
	b, _, _, _ := controllerFixture(t, func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(rows) }, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runtime-states" {
			_ = json.NewEncoder(w).Encode(catalogueFixtureStates(rows, "bounded"))
			return
		}
		views.Add(1)
	})
	attachController(t, b, "/large")
	c := newPreviewDesktopCatalogue(b.runtimes, b.remoteSessions)
	defer c.Close()
	if entries, err := c.Refresh(context.Background()); err == nil || len(entries) != 0 || views.Load() != 0 {
		t.Fatal("truncated list represented as complete", len(entries), err)
	}
	empty := newPreviewDesktopCatalogue(nil, nil)
	defer empty.Close()
	if entries, err := empty.Refresh(context.Background()); err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := empty.Refresh(ctx); err == nil {
		t.Fatal("pre-cancel accepted")
	}
}
