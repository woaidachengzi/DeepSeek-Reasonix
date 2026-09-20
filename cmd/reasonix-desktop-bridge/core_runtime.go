package main

import (
	"context"
	"strings"

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
	controller.EnsureSessionPath()
	return &controllerRuntime{controller: controller}, nil
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
