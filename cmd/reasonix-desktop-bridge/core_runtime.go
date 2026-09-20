package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/boot"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
)

// controllerFactory builds the established Go core only after a bridge client
// opens a session. It lives with the host because the layering rule keeps
// internal/desktopbridge free of boot and control imports.
type controllerFactory struct {
	base   boot.Options
	events *desktopbridge.EventStream
}

func newControllerFactory(events *desktopbridge.EventStream) *controllerFactory {
	return &controllerFactory{events: events}
}

func (f *controllerFactory) Open(ctx context.Context, request desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
	opts := f.base
	opts.WorkspaceRoot = request.WorkspaceRoot
	if f.events != nil {
		opts.Sink = f.events.Sink(request.SessionID)
	} else if opts.Sink == nil {
		opts.Sink = event.Discard
	}
	if strings.TrimSpace(opts.StatsSource) == "" {
		opts.StatsSource = "desktop-tauri"
	}
	controller, err := boot.Build(ctx, opts)
	if err != nil {
		return nil, err
	}
	if err := resumeBridgeSession(controller, request.SessionID); err != nil {
		controller.Close()
		return nil, err
	}
	return &controllerRuntime{controller: controller}, nil
}

// resumeBridgeSession gives a bridge ID one deterministic transcript path.
// Existing Wails sessions are intentionally not discovered or migrated here:
// a Tauri preview creates its own explicitly named session on first open and
// can only resume the same ID it created earlier.
func resumeBridgeSession(controller *control.Controller, sessionID string) error {
	path, err := bridgeSessionPath(controller.SessionDir(), sessionID)
	if err != nil {
		return err
	}
	loaded, err := agent.LoadSession(path)
	if errors.Is(err, os.ErrNotExist) {
		controller.SetFreshSessionPath(path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("load desktop bridge session: %w", err)
	}
	controller.Resume(loaded, path)
	return nil
}

func bridgeSessionPath(sessionDir, sessionID string) (string, error) {
	if strings.TrimSpace(sessionDir) == "" {
		return "", fmt.Errorf("desktop bridge session directory is unavailable")
	}
	if sessionID == "" || len(sessionID) > 128 {
		return "", fmt.Errorf("desktop bridge session identifier is invalid")
	}
	for _, byte := range []byte(sessionID) {
		if !(byte >= 'a' && byte <= 'z') && !(byte >= 'A' && byte <= 'Z') && !(byte >= '0' && byte <= '9') && byte != '-' && byte != '_' {
			return "", fmt.Errorf("desktop bridge session identifier is invalid")
		}
	}
	return filepath.Join(sessionDir, "tauri-"+sessionID+".jsonl"), nil
}

// controllerRuntime adapts the established controller to the bridge's minimal
// Runtime surface.
type controllerRuntime struct {
	controller *control.Controller
}

func (r *controllerRuntime) SessionPath() string { return r.controller.SessionPath() }

func (r *controllerRuntime) State() string {
	status := r.controller.RuntimeStatus()
	switch {
	case status.Running:
		return "running"
	case status.PendingPrompt:
		return "paused"
	default:
		return "idle"
	}
}

func (r *controllerRuntime) Submit(input string) { r.controller.SubmitHTTP(input) }

func (r *controllerRuntime) Cancel() { r.controller.Cancel() }

func (r *controllerRuntime) Shutdown() error {
	err := r.controller.SnapshotForShutdown()
	r.controller.Close()
	return err
}
