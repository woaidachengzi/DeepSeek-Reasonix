package serve

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/remote/controller"
)

func TestDesktopRuntimeCatalogueActualClientIncludesDetachedNotSavedFiles(t *testing.T) {
	closeDesktopReadTestUsage(t)
	t.Setenv("REASONIX_HOME", t.TempDir())
	t.Setenv("REASONIX_STATE_HOME", t.TempDir())
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	t.Cleanup(func() { closeDesktopReadTestUsage(t) })
	dir := t.TempDir()
	foreground := runtimeStateServeController(t, dir, "foreground", nil)
	detached := runtimeStateServeController(t, dir, "detached", nil)
	server := New(foreground, nil, config.ServeConfig{})
	server.auth = newAuthGate(config.ServeConfig{AuthMode: "token", Token: "isolated-runtime-token"})
	path := agent.CanonicalSessionPath(detached.SessionPath())
	server.detached[path] = &detachedSession{path: path, ctrl: detached}
	// Many invalid saved records must not be loaded by the memory-only route.
	for i := range 256 {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("saved-%d.jsonl", i)), []byte("invalid saved body"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(detached.SessionPath()); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client, err := controller.Connect(context.Background(), context.Background(), httpServer.URL, "isolated-runtime-token")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.RuntimeStates(context.Background())
	if err != nil || len(result.Sessions) != 2 {
		t.Fatal(result, err)
	}
	for _, row := range result.Sessions {
		if row.SessionPath == path {
			if row.Current || row.State.RuntimeEpoch != detached.RuntimeStateSnapshot().RuntimeEpoch {
				t.Fatal("detached identity lost")
			}
		} else if row.SessionPath != agent.CanonicalSessionPath(foreground.SessionPath()) || !row.Current {
			t.Fatal("saved history adopted")
		}
	}
	var foreign atomic.Bool
	foreign.Store(true)
	server.bindMu.Lock()
	server.runtimeLeaseProbe = func(candidate string) bool { return foreign.Load() && agent.CanonicalSessionPath(candidate) == path }
	server.bindMu.Unlock()
	result, err = client.RuntimeStates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range result.Sessions {
		if row.SessionPath == path && row.Ownership != "external" {
			t.Fatal("foreign writer presented as owned")
		}
	}
	server.detachedMu.Lock()
	server.detached[path].retiring = true
	server.detachedMu.Unlock()
	result, err = client.RuntimeStates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range result.Sessions {
		if row.SessionPath == path && row.Ownership != "retiring" {
			t.Fatal("retiring writer presented as owned")
		}
	}
}
