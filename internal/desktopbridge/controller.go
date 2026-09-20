package desktopbridge

import (
	"context"
	"strings"

	"reasonix/internal/boot"
	"reasonix/internal/control"
	"reasonix/internal/event"
)

// ControllerFactory builds the established Go core only after a bridge client
// opens a session. Its Base options allow the desktop host to add tightly scoped
// process-local policy later without reimplementing boot.Build.
type ControllerFactory struct {
	Base  boot.Options
	build func(context.Context, boot.Options) (*control.Controller, error)
}

func NewControllerFactory(base boot.Options) *ControllerFactory {
	return &ControllerFactory{Base: base, build: boot.Build}
}

func (f *ControllerFactory) Open(ctx context.Context, request OpenRequest) (Runtime, error) {
	opts := f.Base
	opts.WorkspaceRoot = request.WorkspaceRoot
	if opts.Sink == nil {
		opts.Sink = event.Discard
	}
	if strings.TrimSpace(opts.StatsSource) == "" {
		opts.StatsSource = "desktop-tauri"
	}
	controller, err := f.build(ctx, opts)
	if err != nil {
		return nil, err
	}
	controller.EnsureSessionPath()
	return &controllerRuntime{controller: controller}, nil
}

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
