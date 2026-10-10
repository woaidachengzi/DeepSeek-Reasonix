package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"reasonix/internal/event"
)

// DesktopCommand carries only authenticated routing/actor identity and the
// explicit user operation. It conveys no Controller pointer or execution lease.
// A scoped host must capture its exact owner/prompt before parsing AnswerText,
// then use the same capture for admission; it must not re-find by prompt ID.
type DesktopCommand struct {
	Route      DesktopWatchRoute
	ActorID    string
	Action     string
	TargetID   string
	AnswerText string
}

// DesktopScopedBridge is an opt-in host boundary. Once implemented, commands
// and continuing takeover input never fall back to the ID-only legacy methods,
// including after errors, cancellation, or an unknown dispatch outcome.
// The host remains responsible for current owner, audience, watch persistence,
// bounded IO, and local reclaim. A gateway role check is not a Controller lease.
// Release must permit revoking this route's own binding after admin loss; it
// must never require current admin to relinquish or affect another route.
type DesktopScopedBridge interface {
	ExecuteDesktopCommand(context.Context, DesktopCommand) (string, error)
	// Probe must match both route and authenticated actor; another group member
	// cannot drive or trigger permission-loss release of the owner's binding.
	DesktopTakeoverActive(DesktopWatchRoute, string) bool
}

const desktopOperationUnknown = "桌面操作未确认；请刷新桌面会话状态后检查结果，不要自动重试。"
const desktopCommandBytes = 64 << 10
const desktopScopedCommandUsage = "用法:\n" +
	"/desktop status - 查看已发布的 live 会话句柄\n" +
	"/desktop watch on|off|status - 订阅/退订桌面事件推送\n" +
	"/desktop pending <会话句柄> - 查看当前提示与单次操作票据\n" +
	"/desktop approve <ticket> - 批准该票据对应的操作\n" +
	"/desktop deny <ticket> - 拒绝该票据对应的操作\n" +
	"/desktop answer <ticket> <选项编号或文本> - 回答该票据对应的问题\n" +
	"/desktop plan <ticket> start_execution|revise_plan|exit_plan [说明]\n" +
	"/desktop recovery <ticket> continue|continue_task|revise [说明]\n" +
	"/desktop mcp <ticket> accept|decline|cancel [JSON 对象]\n" +
	"/desktop takeover <会话句柄> - 接管该 live 会话，后续文本直接驱动它\n" +
	"/desktop release - 解除本聊天操作者的接管"

var ErrDesktopAskAnswer = errors.New("desktop question answer is invalid")

// ParseDesktopAskAnswers parses against an already captured prompt. Unlike the
// legacy best-effort parser, it rejects unknown/duplicate/missing assignments
// and ambiguous numbered aliases instead of silently discarding user input.
// It grants no authority; callers must resolve that same exact prompt capture.
func ParseDesktopAskAnswers(questions []event.AskQuestion, raw string) ([]event.AskAnswer, error) {
	if len(questions) == 0 || len(questions) > 32 || len(raw) > desktopCommandBytes {
		return nil, ErrDesktopAskAnswer
	}
	aliases := make(map[string]int, len(questions)*2)
	ids := make(map[string]bool, len(questions))
	for i, q := range questions {
		if strings.TrimSpace(q.ID) == "" || len(q.ID) > 4096 || ids[q.ID] {
			return nil, ErrDesktopAskAnswer
		}
		ids[q.ID] = true
		for _, alias := range []string{q.ID, fmt.Sprint(i + 1)} {
			if previous, ok := aliases[alias]; ok && previous != i {
				return nil, ErrDesktopAskAnswer
			}
			aliases[alias] = i
		}
	}
	selections := make(map[int]string, len(questions))
	if strings.Contains(raw, "=") {
		for part := range strings.SplitSeq(raw, ";") {
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				return nil, ErrDesktopAskAnswer
			}
			index, ok := aliases[strings.TrimSpace(key)]
			if !ok {
				return nil, ErrDesktopAskAnswer
			}
			if _, duplicate := selections[index]; duplicate {
				return nil, ErrDesktopAskAnswer
			}
			selections[index] = strings.TrimSpace(value)
		}
	} else if len(questions) == 1 {
		selections[0] = strings.TrimSpace(raw)
	} else {
		return nil, ErrDesktopAskAnswer
	}
	answers := make([]event.AskAnswer, 0, len(questions))
	for i, q := range questions {
		value, present := selections[i]
		if !present || value == "" {
			return nil, ErrDesktopAskAnswer
		}
		selected := normalizeAskSelection(q, value)
		if len(selected) == 0 {
			return nil, ErrDesktopAskAnswer
		}
		if len(selected) > 64 {
			return nil, ErrDesktopAskAnswer
		}
		seen := make(map[string]bool, len(selected))
		for _, label := range selected {
			if len(label) > 8192 || strings.ContainsRune(label, 0) {
				return nil, ErrDesktopAskAnswer
			}
			if seen[label] {
				return nil, ErrDesktopAskAnswer
			}
			seen[label] = true
		}
		answers = append(answers, event.AskAnswer{QuestionID: q.ID, Selected: selected})
	}
	return answers, nil
}

func desktopActor(msg InboundMessage) string {
	if msg.OperatorID != "" {
		return msg.OperatorID
	}
	return msg.UserID
}

func desktopScopedIdentity(msg InboundMessage) bool {
	route := desktopRouteFromMessage(msg)
	if strings.TrimSpace(desktopActor(msg)) == "" || strings.TrimSpace(route.ConnectionID) == "" || strings.TrimSpace(route.ChatID) == "" {
		return false
	}
	switch route.Platform {
	case PlatformQQ, PlatformFeishu, PlatformWeixin, PlatformDingtalk:
	default:
		return false
	}
	switch route.ChatType {
	case ChatDM, ChatDirect, ChatGroup, ChatGuild, ChatThread:
		return true
	}
	return false
}

func parseDesktopCommand(msg InboundMessage) (DesktopCommand, bool) {
	command := DesktopCommand{Route: desktopRouteFromMessage(msg), ActorID: desktopActor(msg)}
	if len(msg.Text) > desktopCommandBytes || !desktopScopedIdentity(msg) {
		return command, false
	}
	fields := strings.Fields(msg.Text)
	if len(fields) == 0 || fields[0] != "/desktop" {
		return command, false
	}
	if len(fields) == 1 {
		command.Action = "status"
		return command, true
	}
	command.Action = strings.ToLower(fields[1])
	switch command.Action {
	case "status", "sessions":
		command.Action = "status"
		return command, len(fields) == 2
	case "release":
		return command, len(fields) == 2
	case "pending":
		if len(fields) != 3 {
			return command, false
		}
		command.TargetID = fields[2]
		return command, true
	case "watch":
		command.TargetID = "status"
		if len(fields) == 3 {
			command.TargetID = strings.ToLower(fields[2])
			if command.TargetID == "state" {
				command.TargetID = "status"
			}
		}
		return command, len(fields) <= 3 && (command.TargetID == "status" || command.TargetID == "on" || command.TargetID == "off")
	case "approve", "deny", "takeover":
		if len(fields) != 3 {
			return command, false
		}
		command.TargetID = fields[2]
		return command, true
	case "answer", "plan", "recovery", "mcp":
		if len(fields) < 4 {
			return command, false
		}
		command.TargetID = fields[2]
		remainder := strings.TrimSpace(msg.Text)
		for _, token := range fields[:3] {
			remainder = strings.TrimSpace(remainder[len(token):])
		}
		command.AnswerText = remainder
		return command, true
	}
	return command, false
}

func (gw *BotGateway) handleDesktopCommandContext(ctx context.Context, msg InboundMessage) string {
	scoped, ok := gw.cfg.Desktop.(DesktopScopedBridge)
	if !ok {
		return gw.handleDesktopCommand(msg)
	}
	if ctx == nil || ctx.Err() != nil {
		return desktopOperationUnknown
	}
	// Recheck here too: a scoped dispatch must never acquire authority merely
	// because a caller bypassed the normal slash-command role gate.
	if !gw.checkCommandRole(msg.Platform, msg, "admin") {
		return "抱歉，你没有执行此 bot 命令的权限。"
	}
	command, valid := parseDesktopCommand(msg)
	if !valid {
		return desktopScopedCommandUsage
	}
	feedback, err := scoped.ExecuteDesktopCommand(ctx, command)
	if err != nil || ctx.Err() != nil {
		// Raw errors may contain private provider/workspace details. Neither an
		// error nor cancellation proves that dispatch did not take effect.
		return desktopOperationUnknown
	}
	return feedback
}
