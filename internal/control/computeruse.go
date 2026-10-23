package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"reasonix/internal/tool"
)

const ComputerUseAccessApprovalTool = "computer_use_access"

// One host per controller. Grants and denials belong to one live task and the
// exact native process incarnation. Never store them in generic session grants.
type computerUseApprover struct {
	c         *Controller
	mu        sync.Mutex
	turn      string
	decisions map[tool.ComputerUseTarget]bool
}

func (a *computerUseApprover) ApproveComputerUse(ctx context.Context, target tool.ComputerUseTarget) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	turn, status, _, _ := a.c.turnEventRuntimeStatus()
	if turn == "" || status.Terminal() {
		return false, errors.New("computer control requires an active task")
	}
	if a.turn != turn {
		a.turn = turn
		a.decisions = make(map[tool.ComputerUseTarget]bool)
	}
	if allow, exists := a.decisions[target]; exists {
		return allow, nil
	}
	args, _ := json.Marshal(target)
	reply, err := a.c.requestFreshApprovalDecision(ctx, ComputerUseAccessApprovalTool,
		fmt.Sprintf("%s (%s)", target.Name, target.BundleID), args,
		"允许本次任务查看这个应用的窗口，并使用鼠标、键盘进行操作。窗口截图会提供给当前配置的模型处理。其他应用需要另行同意；停止或任务结束后授权失效。")
	current, currentStatus, _, _ := a.c.turnEventRuntimeStatus()
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err != nil {
		return false, err
	}
	if current != turn || currentStatus.Terminal() {
		return false, errors.New("computer control task has ended")
	}
	a.decisions[target] = reply.allow
	return reply.allow, nil
}
