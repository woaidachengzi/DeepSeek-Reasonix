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
const BrowserProfileApprovalTool = "browser_profile_access"

// One host per controller. Grants and denials belong to one live task and the
// exact native process incarnation. Never store them in generic session grants.
type computerUseApprover struct {
	c                *Controller
	mu               sync.Mutex
	turn             string
	decisions        map[tool.ComputerUseTarget]bool
	browserDecisions map[browserApprovalKey]bool
}

type browserApprovalKey struct {
	Target   tool.ComputerUseTarget
	WindowID int
}

func (a *computerUseApprover) ApproveBrowserProfile(ctx context.Context, target tool.ComputerUseTarget, windowID int) (bool, error) {
	return a.approve(ctx, target, windowID)
}

func (a *computerUseApprover) ApproveComputerUse(ctx context.Context, target tool.ComputerUseTarget) (bool, error) {
	return a.approve(ctx, target, 0)
}

func (a *computerUseApprover) approve(ctx context.Context, target tool.ComputerUseTarget, windowID int) (bool, error) {
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
		a.browserDecisions = make(map[browserApprovalKey]bool)
	}
	key := browserApprovalKey{target, windowID}
	allow, exists := a.decisions[target]
	if windowID != 0 {
		allow, exists = a.browserDecisions[key]
	}
	if exists {
		return allow, nil
	}
	args, _ := json.Marshal(target)
	approvalTool := ComputerUseAccessApprovalTool
	subject := fmt.Sprintf("%s (%s)", target.Name, target.BundleID)
	reason := "允许本次任务查看这个应用的窗口，并使用鼠标、键盘进行操作。窗口截图会提供给当前配置的模型处理。其他应用需要另行同意；停止或任务结束后授权失效。"
	if windowID != 0 {
		approvalTool = BrowserProfileApprovalTool
		subject = fmt.Sprintf("连接 %s 的网页 · 窗口 %d", target.Name, windowID)
		args, _ = json.Marshal(key)
		reason = "允许本次任务连接这个浏览器窗口的已登录标签页，读取网页内容并点击、填写。网页内容会提供给当前配置的模型处理。连接可能将浏览器带到前台、打开远程调试设置页并开启远程调试；该设置可能在任务结束后保留，浏览器也可能要求你再次确认。不会复制浏览器资料或重启浏览器。停止或任务结束后连接失效；其他窗口需要另行同意。"
	}
	reply, err := a.c.requestFreshApprovalDecision(ctx, approvalTool, subject, args, reason)
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
	if windowID == 0 {
		a.decisions[target] = reply.allow
	} else {
		a.browserDecisions[key] = reply.allow
	}
	return reply.allow, nil
}
