package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"reasonix/internal/bot"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/remote/controller"
)

const previewDesktopDisplayLimit = 16 << 10

// Command composition for the future host. It is deliberately not a legacy
// DesktopBridge: full host registration still requires driving/watch lifecycle.
// The gateway must authenticate the actual binding and check current admin on
// every invocation. Shared audiences receive only opaque handles/kinds/tickets.
type previewDesktopCommands struct {
	catalogue *previewDesktopCatalogue
	local     *previewDesktopPrompts
	remote    *previewDesktopRemotePrompts
	driving   *previewDesktopDriving
	watch     *previewDesktopWatchStore
	ctx       context.Context
	cancel    context.CancelFunc
}

func newPreviewDesktopCommands(catalogue *previewDesktopCatalogue) *previewDesktopCommands {
	ctx, cancel := context.WithCancel(catalogue.ctx)
	return &previewDesktopCommands{catalogue: catalogue, local: newPreviewDesktopPrompts(catalogue.manager), remote: newPreviewDesktopRemotePrompts(catalogue), driving: newPreviewDesktopDriving(catalogue), ctx: ctx, cancel: cancel}
}

func (h *previewDesktopCommands) Close() {
	h.cancel()
	h.local.Close()
	h.remote.Close()
	h.driving.Close()
}

// Shutdown stops all command ingress/ticket IO before the orderly driving
// cleanup. It does not own or close the shared RuntimeManager/SSH catalogue.
// The full host must stop gateway ingress/watch workers before calling this.
func (h *previewDesktopCommands) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errPreviewDesktopBinding
	}
	h.cancel()
	h.local.Close()
	h.remote.Close()
	return h.driving.Shutdown(ctx)
}

func (h *previewDesktopCommands) DesktopTakeoverActive(route bot.DesktopWatchRoute, actor string) bool {
	return h.ctx.Err() == nil && h.driving.DesktopTakeoverActive(route, actor)
}

func (h *previewDesktopCommands) ExecuteDesktopCommand(ctx context.Context, command bot.DesktopCommand) (string, error) {
	if ctx == nil || ctx.Err() != nil || h.ctx.Err() != nil || !validPreviewWatcher(previewDesktopWatcher{command.Route, command.ActorID}) {
		return "", errPreviewDesktopBinding
	}
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	stop := context.AfterFunc(h.ctx, cancel)
	defer func() { stop(); cancel() }()
	var result string
	var err error
	switch command.Action {
	case "status":
		if command.TargetID != "" || command.AnswerText != "" {
			return "", errPreviewDesktopBinding
		}
		result, err = h.status(bounded)
	case "pending":
		if command.AnswerText != "" {
			return "", errPreviewDesktopBinding
		}
		result, err = h.pending(bounded, command)
	case "takeover", "drive", "release":
		result, err = h.driving.ExecuteDesktopCommand(bounded, command)
	case "watch":
		result, err = h.watchCommand(bounded, command)
	case "approve", "deny", "answer", "plan", "recovery", "mcp":
		// A remote ticket cannot borrow a local resolver, or vice versa.
		if strings.HasPrefix(command.TargetID, "rp-") {
			result, err = h.remote.ExecuteDesktopCommand(bounded, command)
		} else if strings.HasPrefix(command.TargetID, "p-") {
			result, err = h.local.ExecuteDesktopCommand(bounded, command)
		} else {
			return "", errPreviewDesktopBinding
		}
	default:
		return "", errPreviewDesktopBinding
	}
	if err != nil || bounded.Err() != nil || h.ctx.Err() != nil {
		return "", errPreviewDesktopBinding
	}
	if len(result) > previewDesktopDisplayLimit {
		return "提示内容超过安全展示上限；请在 Preview 桌面查看并处理，不发送截断内容。", nil
	}
	return result, nil
}

func (h *previewDesktopCommands) status(ctx context.Context) (string, error) {
	entries, err := h.catalogue.Refresh(ctx)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "Preview 当前没有已发布的 live 会话。", nil
	}
	var out strings.Builder
	out.WriteString("Preview live 会话（观察快照，不是执行授权）：\n")
	for _, entry := range entries {
		origin, position, phase := "远程", "前台", "空闲"
		if entry.local != nil {
			origin = "本地"
		}
		if entry.detached {
			position = "后台"
		}
		if entry.running {
			phase = "处理中"
		}
		if entry.pending {
			phase = "待处理提示"
		}
		fmt.Fprintf(&out, "%s · %s/%s · %s\n", entry.handle, origin, position, phase)
	}
	out.WriteString("使用 /desktop pending <会话句柄> 查看当前提示，/desktop takeover <会话句柄> 接管，/desktop release 解除本聊天操作者的接管；群聊不展示提示正文。")
	return out.String(), nil
}

func (h *previewDesktopCommands) pending(ctx context.Context, command bot.DesktopCommand) (string, error) {
	entry, err := h.catalogue.Capture(ctx, command.TargetID)
	if err != nil {
		return "", err
	}
	snapshot, err := h.catalogue.ReadPending(ctx, entry)
	if err != nil {
		return "", err
	}
	if len(snapshot.Prompts) == 0 {
		return "该 Preview 会话当前没有待处理提示。", nil
	}
	private := command.Route.ChatType == bot.ChatDM || command.Route.ChatType == bot.ChatDirect
	var out strings.Builder
	fmt.Fprintf(&out, "Preview 当前提示（%s）：\n", entry.handle)
	for _, prompt := range snapshot.Prompts {
		var id string
		if entry.local != nil {
			s := prompt.Scope
			id, _, err = h.local.Issue(ctx, command.Route, command.ActorID, entry.local.Scope, desktopbridge.OwnedPrompt{ID: s.PromptID, Kind: s.Kind, TurnID: s.TurnID, RuntimeEpoch: s.PromptRuntimeEpoch})
		} else {
			id, _, err = h.remote.Issue(ctx, command.Route, command.ActorID, entry, prompt.Scope)
		}
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&out, "\n%s · %s\n", prompt.Scope.Kind, id)
		if private {
			writePreviewPromptDetail(&out, prompt)
		}
		out.WriteString(previewPromptInstructions(prompt, id))
		if out.Len() > previewDesktopDisplayLimit {
			return "提示内容超过安全展示上限；请在 Preview 桌面查看并处理，不发送截断内容。", nil
		}
	}
	if !private {
		out.WriteString("\n本聊天只展示类型与单次票据；提示正文请在桌面查看。票据仅供当前操作者在本聊天使用。")
	}
	// Reject a mixed pending cut instead of publishing old decisions as current.
	after, err := h.catalogue.ReadPending(ctx, entry)
	if err != nil || after.Revision != snapshot.Revision || after.TurnID != snapshot.TurnID || len(after.Prompts) != len(snapshot.Prompts) {
		return "", errPreviewDesktopBinding
	}
	for i := range after.Prompts {
		if after.Prompts[i].Scope != snapshot.Prompts[i].Scope {
			return "", errPreviewDesktopBinding
		}
	}
	return out.String(), nil
}

func previewPromptInstructions(prompt controller.SessionPendingPrompt, id string) string {
	switch prompt.Scope.Kind {
	case "ask":
		return "/desktop answer " + id + " <选项编号或文本；多题用 1=答案;2=答案>\n"
	case "approval":
		return "/desktop approve " + id + " 或 /desktop deny " + id + "\n"
	case "plan":
		return "/desktop plan " + id + " start_execution|revise_plan|exit_plan [说明]\n"
	case "recovery":
		actions := "continue|revise"
		if prompt.Approval != nil && prompt.Approval.Recovery != nil && prompt.Approval.Recovery.CanGrantTask {
			actions = "continue|continue_task|revise"
		}
		return "/desktop recovery " + id + " " + actions + " [说明]\n"
	case "mcp":
		if prompt.MCPInteraction != nil && prompt.MCPInteraction.Mode == "url" {
			return "/desktop mcp " + id + " accept|decline|cancel（不带 JSON；先在桌面核对外部流程）\n"
		}
		return "/desktop mcp " + id + " accept [表单 JSON 对象]；decline 或 cancel 不带 JSON\n"
	}
	return ""
}

// Prompt data is untrusted display text, not Markdown/commands or routing IDs.
// Keep each supplied field on one line and remove invisible control formatting.
func previewPromptText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, value)
}

func writePreviewPromptDetail(out *strings.Builder, prompt controller.SessionPendingPrompt) {
	if ask := prompt.Ask; ask != nil {
		for i, question := range ask.Questions {
			fmt.Fprintf(out, "问题 %d（%s）：%s\n", i+1, previewPromptText(question.Header), previewPromptText(question.Prompt))
			if question.Multi {
				out.WriteString("可多选。\n")
			}
			for j, option := range question.Options {
				fmt.Fprintf(out, "  %d. %s — %s\n", j+1, previewPromptText(option.Label), previewPromptText(option.Description))
			}
		}
	}
	if approval := prompt.Approval; approval != nil {
		fmt.Fprintf(out, "工具：%s\n提示：%s\n原因：%s\n", previewPromptText(approval.Tool), previewPromptText(approval.Subject), previewPromptText(approval.Reason))
		if approval.WriteAccess != nil {
			raw, _ := json.Marshal(approval.WriteAccess)
			fmt.Fprintf(out, "写入范围与授权条件：%s\n", previewPromptText(string(raw)))
		}
		if approval.Recovery != nil {
			raw, _ := json.Marshal(approval.Recovery)
			fmt.Fprintf(out, "恢复方案与授权条件：%s\n", previewPromptText(string(raw)))
		}
	}
	if mcp := prompt.MCPInteraction; mcp != nil {
		fmt.Fprintf(out, "MCP 服务：%s\n模式：%s\n提示：%s\n所需 JSON 结构：%s\n", previewPromptText(mcp.Server), previewPromptText(mcp.Mode), previewPromptText(mcp.Message), previewPromptText(string(mcp.RequestedSchema)))
		if mcp.URL != "" {
			out.WriteString("该提示含外部 URL；请在桌面核对和完成外部流程，聊天不会转发或自动打开该地址。\n")
		}
	}
}
