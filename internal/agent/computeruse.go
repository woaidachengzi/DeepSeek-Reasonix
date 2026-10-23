package agent

import (
	"reasonix/internal/nilutil"
	"reasonix/internal/tool"
)

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
