package agent

import (
	"context"

	"reasonix/internal/nilutil"
	"reasonix/internal/tool"
)

// Run owns the private desktop worker even when callers never cancel their
// context. All return/panic paths invalidate its snapshots and stop the child.
func (a *Agent) Run(ctx context.Context, input string) error {
	ctx, closeDesktop := context.WithCancel(ctx)
	defer closeDesktop()
	return a.run(tool.WithComputerUseLifetime(ctx, ctx), input)
}

func (a *Agent) SetComputerUseApprover(g tool.ComputerUseApprover) {
	if nilutil.IsNil(g) {
		g = nil
	}
	a.svc.computerUse = g
}

func (c *Coordinator) SetComputerUseApprover(g tool.ComputerUseApprover) {
	if c == nil {
		return
	}
	if c.plannerAgent != nil {
		c.plannerAgent.SetComputerUseApprover(g)
	}
	if c.executor != nil {
		c.executor.SetComputerUseApprover(g)
	}
}
