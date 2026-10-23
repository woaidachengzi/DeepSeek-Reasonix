package main

import (
	"errors"
	"os"

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/control"
)

func initializeBridgeApproval(controller *control.Controller, sessionID string) {
	// Cold opens need the same interactive services as settings rebuilds:
	// app-scoped consent, questions, and ordinary permission decisions.
	controller.EnableInteractiveApproval()
	// A composer choice can be durable before the first transcript turn.
	// Prefer saved per-session metadata; defaults only seed a new choice.
	path, err := bridgeSessionPath(controller.SessionDir(), sessionID)
	if err != nil {
		return
	}
	if meta, exists, loadErr := agent.LoadBranchMeta(path); loadErr == nil && exists && validBridgeApprovalMode(meta.ToolApprovalMode) {
		controller.SetToolApprovalMode(meta.ToolApprovalMode)
	} else if _, statErr := os.Lstat(path); errors.Is(statErr, os.ErrNotExist) {
		if cfg, loadErr := appconfig.LoadUserConfigReadOnly(); loadErr == nil {
			controller.SetToolApprovalMode(cfg.DesktopDefaultToolApprovalMode())
		}
	}
}
