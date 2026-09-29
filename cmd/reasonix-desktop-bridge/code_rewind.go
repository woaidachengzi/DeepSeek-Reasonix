package main

import (
	"fmt"

	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
)

const maxBridgeCheckpoints = 50
const maxBridgeRewindFiles = 60

func (r *controllerRuntime) WorkspaceCheckpoints() []desktopbridge.WorkspaceCheckpointView {
	metas := r.controller.Checkpoints()
	if len(metas) > maxBridgeCheckpoints {
		metas = metas[len(metas)-maxBridgeCheckpoints:]
	}
	views := make([]desktopbridge.WorkspaceCheckpointView, 0, len(metas))
	for _, meta := range metas {
		prompt := []rune(meta.Prompt)
		if len(prompt) > 240 {
			prompt = append(prompt[:240], '…')
		}
		views = append(views, desktopbridge.WorkspaceCheckpointView{
			Turn: meta.Turn, Prompt: string(prompt), Time: meta.Time.UnixMilli(), TurnFileCount: len(meta.Paths),
		})
	}
	return views
}

func (r *controllerRuntime) PrepareCodeRewind(turn int) (desktopbridge.WorkspaceCodeRewindPlan, error) {
	if turn < 0 {
		return desktopbridge.WorkspaceCodeRewindPlan{}, fmt.Errorf("invalid checkpoint turn")
	}
	plan, err := r.controller.PrepareRewind(turn, control.RewindCode)
	if err != nil {
		return desktopbridge.WorkspaceCodeRewindPlan{}, err
	}
	files := append([]string{}, plan.Files...)
	if len(files) > maxBridgeRewindFiles {
		files = files[:maxBridgeRewindFiles]
	}
	gaps := make([]string, 0, len(plan.CoverageGaps))
	for _, gap := range plan.CoverageGaps {
		gaps = append(gaps, gap.Reason)
	}
	return desktopbridge.WorkspaceCodeRewindPlan{
		PlanID: plan.PlanID, Turn: plan.Turn, CanFiles: plan.CanFiles,
		FileCount: plan.FileCount, Files: files, FilesTruncated: len(files) < plan.FileCount,
		Coverage: string(plan.Coverage), CoverageGaps: gaps,
		RequiresCoverageConfirmation: control.RewindPlanRequiresConfirmation(plan),
		DisabledReason:               plan.DisabledReason, Conflicts: fileRevertConflictReasons(plan.Conflicts),
	}, nil
}

func (r *controllerRuntime) CommitCodeRewind(planID string, confirmPartialCoverage bool) (desktopbridge.WorkspaceFileRevertResult, error) {
	if !validRevertID(planID) {
		return desktopbridge.WorkspaceFileRevertResult{}, fmt.Errorf("invalid code rewind plan ID")
	}
	result, err := r.controller.CommitCodeRewind(planID, confirmPartialCoverage)
	return fileRevertResultView(result, err), nil
}
