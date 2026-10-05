package main

import (
	"fmt"
	"reasonix/internal/agent"
	"reasonix/internal/boot"
	appconfig "reasonix/internal/config"
	"reasonix/internal/control"
)

func selectedBridgeEffort(opts boot.Options) string {
	if opts.EffortOverride != nil {
		if *opts.EffortOverride == "" {
			return "auto"
		}
		return *opts.EffortOverride
	}
	cfg, err := appconfig.LoadForRoot(opts.WorkspaceRoot)
	if err == nil {
		ref := opts.Model
		if ref == "" {
			ref = cfg.DefaultModel
		}
		if entry, ok := cfg.ResolveModel(ref); ok {
			return appconfig.EffortDisplay(entry)
		}
	}
	return "auto"
}
func persistBridgeReasoning(controller *control.Controller, effort string) error {
	return agent.UpdateBranchMeta(controller.SessionPath(), false, func(meta *agent.BranchMeta) error {
		meta.ReasoningModel = controller.ModelRef()
		meta.ReasoningEffort = &effort
		return nil
	})
}
func (r *controllerRuntime) Effort() string { return r.effort }

// Conditional rollback restores only the selection, preserving titles and other
// independent metadata edits. Existing identity/transcript fields are untouched.
func prepareBridgeReasoning(path, model, effort string) (func() error, error) {
	previous, exists, err := agent.LoadBranchMeta(path)
	if err != nil || !exists {
		return nil, fmt.Errorf("read session reasoning metadata: %v", err)
	}
	if err := agent.UpdateBranchMeta(path, false, func(meta *agent.BranchMeta) error {
		meta.ReasoningModel = model
		meta.ReasoningEffort = &effort
		return nil
	}); err != nil {
		return nil, err
	}
	return func() error {
		return agent.UpdateBranchMeta(path, false, func(meta *agent.BranchMeta) error {
			if meta.ReasoningModel != model || meta.ReasoningEffort == nil || *meta.ReasoningEffort != effort {
				return fmt.Errorf("session reasoning changed during rollback")
			}
			meta.ReasoningModel, meta.ReasoningEffort = previous.ReasoningModel, previous.ReasoningEffort
			return nil
		})
	}, nil
}
