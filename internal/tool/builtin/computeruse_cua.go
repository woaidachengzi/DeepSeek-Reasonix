package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"reasonix/internal/computeruse"
	"reasonix/internal/tool"
)

const cuaDescription = "Control one approved macOS app through Reasonix's MIT Cua Driver Rust runtime. This build uses Cua actions, not legacy observation receipts. Start with apps, then windows with a selected PID; windows and every screenshot/input require fresh task-scoped app consent. Use observe with exact pid/window_id for a screenshot and accessibility element_tokens. Prefer click using a returned element_token; stale tokens require observe again. Coordinates are pixels of that exact screenshot, never global screen coordinates; do not manually scale. type inserts text in the exact window, key sends one key, hotkey sends keys such as [command,enter]. Background delivery is the default; if explicitly refused, inspect the reason and request foreground for the intended window. Cua effect=confirmed verifies only the reported evidence (such as a field value), not the whole task. Inspect the returned state or observe after uncertain clicks. Never infer site-wide synthetic-click rejection from an unchanged page or shortcut success. Before another send/submit, verify the prior one did not complete. No automatic retry after interrupted/uncertain input. Tokens and runtime end with the host task; re-observe in a new task. This adapter does not expose arbitrary JavaScript, shell, clipboard, file paths, full-desktop capture, or automatic logged-in browser-profile attachment. Upstream hints can name tools this adapter does not expose, such as page or run_steps; use only the current computer_use schema. get_window_state means observe. Workspace sandbox restrictions and existing app consent still apply."

func (c computerUse) executeCua(ctx context.Context, args json.RawMessage) (string, []string, error) {
	var request computeruse.CuaRequest
	decoder := json.NewDecoder(bytes.NewReader(args))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return "", nil, &computerUseError{cause: err}
	}
	if decoder.Decode(new(any)) != io.EOF {
		return "", nil, &computerUseError{cause: errors.New("expected one Cua request")}
	}
	if err := request.Validate(); err != nil {
		return "", nil, &computerUseError{cause: err}
	}
	lifetime := tool.ComputerUseLifetimeFrom(ctx)
	if lifetime == nil || lifetime.Done() == nil || lifetime.Err() != nil {
		return "", nil, &computerUseError{cause: tool.Blocked("Cua requires an active host-owned task")}
	}
	var target *computeruse.Target
	if request.Action != "status" && request.Action != "apps" {
		approver := tool.ComputerUseApproverFrom(ctx)
		if approver == nil {
			return "", nil, &computerUseError{cause: tool.Blocked("computer control requires explicit app access approval from an interactive host")}
		}
		resolved, err := c.controller.Target(ctx, request.PID)
		if err != nil {
			return "", nil, &computerUseError{cause: err}
		}
		allow, err := approver.ApproveComputerUse(ctx, tool.ComputerUseTarget(resolved))
		if err != nil || !allow {
			if err == nil {
				err = errors.New("user declined this app; do not retry in this task")
			}
			return "", nil, &computerUseError{cause: err}
		}
		current, err := c.controller.Target(ctx, request.PID)
		if err != nil || current != resolved || ctx.Err() != nil || lifetime.Err() != nil {
			return "", nil, &computerUseError{cause: errors.New("approved app or task changed before Cua dispatch")}
		}
		target = &resolved
	}
	reply, err := c.cua.Call(ctx, lifetime, request, target)
	if err != nil {
		var uncertain *computeruse.CuaTransportError
		body, _ := json.Marshal(map[string]any{"engine": "cua", "dispatched": false, "dispatch_unknown": errors.As(err, &uncertain), "next_action": "observe_before_retry"})
		return string(body), nil, &computerUseError{cause: err}
	}
	return c.cuaResult(ctx, request, target, reply)
}

func (c computerUse) cuaResult(ctx context.Context, request computeruse.CuaRequest, target *computeruse.Target, reply computeruse.CuaReply) (string, []string, error) {
	var action struct {
		Effect   string `json:"effect"`
		Route    string `json:"route"`
		Delivery *struct {
			Count *int `json:"delivered_count"`
		} `json:"delivery"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(reply.Action, &action)
	summary := reply.Text
	if request.Action == "observe" {
		summary = "Cua window observation; inspect state and image."
	}
	body := map[string]any{"engine": "cua", "driver_version": reply.Version, "source_revision": reply.SourceRevision, "state": reply.Result, "summary": summary, "dispatched": false, "cua_action": reply.Action}
	input := request.Action == "click" || request.Action == "type" || request.Action == "key" || request.Action == "hotkey" || request.Action == "scroll" || request.Action == "move"
	if input {
		refusal := cuaRefusal(reply, action.Effect)
		if action.Effect == "refused" && action.Error != nil {
			refusal = action.Error.Code
		}
		switch {
		case action.Delivery != nil && action.Delivery.Count != nil && *action.Delivery.Count > 0:
			body["dispatched"], body["dispatch_method"] = true, "cua_"+action.Route
		case refusal != "":
			body["blocked_reason"] = refusal
		default:
			body["dispatch_unknown"] = true
		}
	}
	var images []string
	for _, image := range reply.Images {
		if (image.MIME != "image/png" && image.MIME != "image/jpeg") || len(image.Data) > 5<<20 {
			continue
		}
		data := "data:" + image.MIME + ";base64," + image.Data
		images = append(images, data)
		if target != nil {
			if thumbnail := computeruse.PreviewImage(data); thumbnail != "" {
				previewAction := request.Action
				if previewAction == "hotkey" {
					previewAction = "key"
				}
				tool.PublishComputerUsePreview(ctx, tool.ComputerUsePreview{Target: tool.ComputerUseTarget(*target), WindowID: request.WindowID, Action: previewAction, Image: thumbnail})
			}
		}
		break
	}
	encoded, marshalErr := json.Marshal(body)
	if marshalErr != nil {
		return "", nil, marshalErr
	}
	if reply.IsError {
		return string(encoded), images, &computerUseError{cause: errors.New("Cua refused or could not confirm this action: " + reply.ErrorCode + " " + reply.Error)}
	}
	return string(encoded), images, nil
}

func cuaRefusal(reply computeruse.CuaReply, effect string) string {
	if reply.Completion == "not_started" || effect == "refused" {
		if reply.ErrorCode != "" {
			return reply.ErrorCode
		}
		return "cua_refused"
	}
	var result struct {
		Status  string `json:"status"`
		Refusal struct {
			Code string `json:"code"`
		} `json:"refusal"`
	}
	if reply.IsError && reply.Completion != "unknown" && json.Unmarshal(reply.Result, &result) == nil && result.Status == "refused" {
		return result.Refusal.Code
	}
	return ""
}
