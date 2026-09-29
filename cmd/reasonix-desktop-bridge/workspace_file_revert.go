package main

import (
	"fmt"
	"strings"

	"reasonix/internal/checkpoint"
	"reasonix/internal/desktopbridge"
)

func (r *controllerRuntime) PrepareWorkspaceFileRevert(path string) (desktopbridge.WorkspaceFileRevertPlan, error) {
	_, relative, err := r.resolveWorkspaceChangePath(path)
	if err != nil {
		return desktopbridge.WorkspaceFileRevertPlan{}, err
	}
	plan, err := r.controller.PrepareFileRevert(relative)
	if err != nil {
		return desktopbridge.WorkspaceFileRevertPlan{}, err
	}
	return desktopbridge.WorkspaceFileRevertPlan{
		PlanID: plan.PlanID, Path: relative, CanFiles: plan.CanFiles,
		DisabledReason: plan.DisabledReason, Conflicts: fileRevertConflictReasons(plan.Conflicts), Legacy: plan.Legacy,
	}, nil
}

func (r *controllerRuntime) CommitWorkspaceFileRevert(planID, resolution string) (desktopbridge.WorkspaceFileRevertResult, error) {
	if !validRevertID(planID) {
		return desktopbridge.WorkspaceFileRevertResult{}, fmt.Errorf("invalid file revert plan ID")
	}
	var choice checkpoint.ConflictResolution
	switch resolution {
	case "":
	case string(checkpoint.ResolveOverwriteCheckpoint):
		choice = checkpoint.ResolveOverwriteCheckpoint
	default:
		return desktopbridge.WorkspaceFileRevertResult{}, fmt.Errorf("invalid file revert resolution")
	}
	result, err := r.controller.CommitFileRevert(planID, choice)
	return fileRevertResultView(result, err), nil
}

func (r *controllerRuntime) UndoWorkspaceFileRevert(transactionID string) (desktopbridge.WorkspaceFileRevertResult, error) {
	if !validRevertID(transactionID) {
		return desktopbridge.WorkspaceFileRevertResult{}, fmt.Errorf("invalid file revert transaction ID")
	}
	result, err := r.controller.UndoRewind(transactionID)
	return fileRevertResultView(result, err), nil
}

func validRevertID(id string) bool {
	return len(id) > 0 && len(id) <= 128 && strings.IndexFunc(id, func(char rune) bool {
		return !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_')
	}) < 0
}

func fileRevertResultView(result checkpoint.RewindResult, err error) desktopbridge.WorkspaceFileRevertResult {
	view := desktopbridge.WorkspaceFileRevertResult{
		OK: result.OK, TransactionID: result.TransactionID, UndoAvailable: result.UndoAvailable,
		WrittenCount: len(result.Written), DeletedCount: len(result.Deleted),
		Conflicts: fileRevertConflictReasons(result.Conflicts),
	}
	if err != nil {
		view.Error = "The file changed or the operation is unavailable. Refresh workspace changes before retrying."
	}
	return view
}

func fileRevertConflictReasons(conflicts []checkpoint.RewindConflict) []string {
	reasons := make([]string, 0, len(conflicts))
	for _, conflict := range conflicts {
		reasons = append(reasons, string(conflict.Reason))
	}
	return reasons
}
