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
	if len(planID) == 0 || len(planID) > 128 || strings.IndexFunc(planID, func(char rune) bool {
		return !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_')
	}) >= 0 {
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
	view := desktopbridge.WorkspaceFileRevertResult{
		OK: result.OK, TransactionID: result.TransactionID, UndoAvailable: result.UndoAvailable,
		WrittenCount: len(result.Written), DeletedCount: len(result.Deleted),
		Conflicts: fileRevertConflictReasons(result.Conflicts),
	}
	if err != nil {
		view.Error = "File changed or could not be restored. Preview it again before retrying."
	}
	return view, nil
}

func fileRevertConflictReasons(conflicts []checkpoint.RewindConflict) []string {
	reasons := make([]string, 0, len(conflicts))
	for _, conflict := range conflicts {
		reasons = append(reasons, string(conflict.Reason))
	}
	return reasons
}
