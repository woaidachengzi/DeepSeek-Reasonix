package main

import (
	"context"
	"errors"
	"fmt"

	"reasonix/internal/boot"
	"reasonix/internal/desktopbridge"
)

func (f *controllerFactory) Rebuild(ctx context.Context, previous desktopbridge.Runtime, request desktopbridge.OpenRequest) (desktopbridge.SettingsRuntime, error) {
	old, ok := previous.(*controllerRuntime)
	if !ok || old.sessionID != request.SessionID {
		return nil, fmt.Errorf("settings rebuild requires the owned controller")
	}
	opts, sink, err := f.options(request)
	if err != nil {
		return nil, err
	}
	// Preflight/commit the narrow metadata delta before publishing a replacement
	// generation. A write refusal must leave the old controller live.
	undo := func() error { return nil }
	if request.Effort != nil {
		undo, err = prepareBridgeReasoning(old.controller.SessionPath(), opts.Model, selectedBridgeEffort(opts))
		if err != nil {
			return nil, errors.Join(err, sink.Close())
		}
	}
	opts.ForceFullRebuild = true
	result, err := boot.Rebuild(ctx, old.controller, opts)
	if err != nil {
		return nil, errors.Join(err, undo(), sink.Close())
	}
	// The boot layer migrates posture and grants; host prompt wiring belongs
	// to the replacement controller and must be reinstalled.
	result.Controller.EnableInteractiveApproval()
	next := &controllerRuntime{controller: result.Controller, sessionID: request.SessionID, lifecycleSink: sink, effort: selectedBridgeEffort(opts), terminalEvents: f.events, replacementRollback: undo}
	next.startTurnSnapshotMonitor()
	return next, nil
}

func (r *controllerRuntime) ReleaseForReplacement() error {
	terminalErr := r.closeTerminals()
	r.cancelMCPOAuthFlows()
	r.stopTurnSnapshotMonitor()
	r.controller.ReleaseResources()
	r.replacementMu.Lock()
	undo := r.replacementRollback
	r.replacementRollback = nil
	r.replacementMu.Unlock()
	var rollbackErr error
	if undo != nil {
		rollbackErr = undo()
	}
	return errors.Join(terminalErr, rollbackErr, r.lifecycleSink.Close())
}

func (r *controllerRuntime) CommitReplacement() {
	r.replacementMu.Lock()
	r.replacementRollback = nil
	r.replacementMu.Unlock()
}
