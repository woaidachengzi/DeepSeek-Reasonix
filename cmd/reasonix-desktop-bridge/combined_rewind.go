package main

import (
	"fmt"

	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
)

func (r *controllerRuntime) PrepareCombinedRewind(turn int) (desktopbridge.WorkspaceCombinedRewindPlan, error) {
	if turn < 0 {
		return desktopbridge.WorkspaceCombinedRewindPlan{}, fmt.Errorf("invalid checkpoint turn")
	}
	if !r.controller.CanRewindConversationInPlace() {
		return desktopbridge.WorkspaceCombinedRewindPlan{
			Turn: turn, Files: []string{}, CoverageGaps: []string{}, Conflicts: []string{},
			DisabledReason: "this session format cannot switch conversation heads within its transcript",
		}, nil
	}
	plan, err := r.controller.PrepareRewind(turn, control.RewindBoth)
	if err != nil {
		return desktopbridge.WorkspaceCombinedRewindPlan{}, err
	}
	files := append([]string{}, plan.Files...)
	if len(files) > maxBridgeRewindFiles {
		files = files[:maxBridgeRewindFiles]
	}
	gaps := make([]string, 0, len(plan.CoverageGaps))
	for _, gap := range plan.CoverageGaps {
		gaps = append(gaps, gap.Reason)
	}
	return desktopbridge.WorkspaceCombinedRewindPlan{
		PlanID: plan.PlanID, Turn: plan.Turn, CanFiles: plan.CanFiles,
		CanConversation: plan.CanConversation, FileCount: plan.FileCount,
		Files: files, FilesTruncated: len(files) < plan.FileCount,
		CoverageGaps: gaps, RequiresCoverageConfirmation: control.RewindPlanRequiresConfirmation(plan),
		Conflicts: fileRevertConflictReasons(plan.Conflicts), DisabledReason: plan.DisabledReason,
	}, nil
}

func (r *controllerRuntime) CommitCombinedRewind(planID string, confirmed bool) (desktopbridge.WorkspaceCombinedRewindResult, error) {
	if !validRevertID(planID) {
		return desktopbridge.WorkspaceCombinedRewindResult{}, fmt.Errorf("invalid combined rewind plan ID")
	}
	result, err := r.controller.CommitCombinedRewindInPlace(planID, confirmed)
	view := desktopbridge.WorkspaceCombinedRewindResult{
		OK: result.OK, Partial: result.Partial, ConversationForked: result.ConversationForked,
		HeadID: result.Branch, FilesRestored: result.OK && !result.Partial,
		TransactionID: result.TransactionID, UndoAvailable: result.UndoAvailable,
		WrittenCount: len(result.Written), DeletedCount: len(result.Deleted),
		Conflicts: fileRevertConflictReasons(result.Conflicts),
	}
	if result.Partial {
		view.Error = "File restore did not complete after the conversation version was created. Check workspace files and refresh checkpoints before retrying."
	} else if err != nil {
		view.Error = "The files or conversation changed after preview. Refresh checkpoints and review the plan again."
	}
	return view, nil
}
