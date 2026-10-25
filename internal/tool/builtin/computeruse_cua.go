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

const cuaDescription = "Control one approved macOS app through Reasonix's MIT Cua Driver Rust runtime. This build uses Cua actions, not legacy observation receipts. Start with apps, then windows with a selected PID; windows and every screenshot/input require fresh task-scoped app consent. Use observe with exact pid/window_id for a screenshot and accessibility element_tokens. Prefer click using a returned element_token; stale tokens require observe again. Coordinates are pixels of that exact screenshot, never global screen coordinates; do not manually scale. type inserts text in the exact window, key sends one key, hotkey sends keys such as [command,enter]. Background delivery is the default; if explicitly refused, inspect the reason and request foreground for the intended window. Cua effect=confirmed verifies only the reported evidence (such as a field value), not the whole task. Inspect the returned state or observe after uncertain clicks. Never infer site-wide synthetic-click rejection from an unchanged page or shortcut success. Before another send/submit, verify the prior one did not complete. No automatic retry after interrupted/uncertain input. Tokens and runtime end with the host task; re-observe in a new task. For Chromium web pages prefer browser_observe: first pid/window_id to bind and list tabs, then the returned target_id/tab_id to read semantic DOM refs. browser_prepare attaches the exact browser window only after separate interactive profile approval; it may enable remote debugging and show browser consent. If declined or unavailable, keep using native observe/click; never use shell or legacy page to bypass it. browser_click requires a current DOM ref and uses trusted CDP input; browser_type requires an editable ref, inserts text by default, replace=true replaces the field (empty text clears it). browser_navigate navigates the observed target/tab to an absolute http/https URL or about:blank; all old DOM refs become invalid, so observe again. browser_select requires a current native HTML select ref and exactly one exact label or value (empty value is allowed); its input/change events are synthetic, so verify application acceptance. browser_pointer uses a current ref and gesture hover, right_click, double_click, scroll or drag with trusted CDP input only; scroll needs nonzero delta_x/delta_y in CSS pixels, drag needs destination_ref from the same frame. Read the declared ref actions before using pointer/scroll. Background is the default; if explicitly refused, inspect before requesting foreground. Native element_token and screenshot coordinates are not DOM refs. browser_observe supports query, ref for a scoped read, continuation and optional include_screenshot; DOM-only reads avoid unnecessary images. Request include_screenshot=true for the first tab snapshot and meaningful visual milestones to keep the PiP current; use DOM-only reads between them. After input inspect the postcondition before claiming success or repeating a submission. Do not call browser_prepare repeatedly when it asks for human/browser confirmation. This adapter does not expose arbitrary JavaScript, shell, clipboard, file paths, full-desktop capture, or automatic logged-in browser-profile attachment without separate approval. Upstream hints can name tools this adapter does not expose, such as page or run_steps; use only the current computer_use schema. get_window_state means observe; get_browser_state means browser_observe. Workspace sandbox restrictions and existing app consent still apply."

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
	browserApproved := false
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
		if request.Action == "browser_prepare" {
			browserApprover, ok := approver.(tool.BrowserProfileApprover)
			if !ok {
				return "", nil, &computerUseError{cause: tool.Blocked("browser attachment requires separate interactive profile approval; use native observe instead")}
			}
			browserApproved, err = browserApprover.ApproveBrowserProfile(ctx, tool.ComputerUseTarget(resolved), request.WindowID)
			if err != nil || !browserApproved {
				if err == nil {
					err = errors.New("user declined browser profile attachment; do not retry in this task")
				}
				return "", nil, &computerUseError{cause: err}
			}
		}
		current, err := c.controller.Target(ctx, request.PID)
		if err != nil || current != resolved || ctx.Err() != nil || lifetime.Err() != nil {
			return "", nil, &computerUseError{cause: errors.New("approved app or task changed before Cua dispatch")}
		}
		target = &resolved
	}
	var reply computeruse.CuaReply
	var err error
	if browserApproved {
		reply, err = c.cua.CallWithBrowserApproval(ctx, lifetime, request, target)
	} else {
		reply, err = c.cua.Call(ctx, lifetime, request, target)
	}
	if err != nil {
		var uncertain *computeruse.CuaTransportError
		body, _ := json.Marshal(map[string]any{"engine": "cua", "dispatched": false, "dispatch_unknown": errors.As(err, &uncertain), "next_action": "observe_before_retry"})
		return string(body), nil, &computerUseError{cause: err}
	}
	return c.cuaResult(ctx, request, target, reply)
}

func (c computerUse) cuaResult(ctx context.Context, request computeruse.CuaRequest, target *computeruse.Target, reply computeruse.CuaReply) (string, []string, error) {
	if request.IsBrowser() {
		reply.Result = cuaBrowserHints(reply.Result, request)
		reply.Action = cuaBrowserHints(reply.Action, request)
	}

	summary := reply.Text
	if request.Action == "observe" || request.Action == "browser_observe" {
		summary = "Cua window observation; inspect state and image."
	}
	body := map[string]any{"engine": "cua", "driver_version": reply.Version, "source_revision": reply.SourceRevision, "state": reply.Result, "summary": summary, "dispatched": false, "cua_action": reply.Action}
	if request.Action == "browser_observe" {
		// Give DOM snapshots the same bounded-history contract as native frames.
		var state map[string]json.RawMessage
		if json.Unmarshal(reply.Result, &state) == nil && state != nil {
			state["pid"], _ = json.Marshal(request.PID)
			state["window_id"], _ = json.Marshal(request.WindowID)
			body["state"] = state
		}
	}
	cuaInputFacts(request, reply, body)

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
				if previewAction == "browser_observe" {
					previewAction = "observe"
				}
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

// A browser refusal may follow partial delivery or failed focus cleanup.
// Preserve uncertainty even though the upstream envelope is named refused.
func cuaBrowserDeliveryUnknown(reply computeruse.CuaReply, action cuaAction) bool {
	if reply.Completion == "not_started" {
		return false
	}
	if action.Effect == "partial" {
		return true
	}
	var result struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
		Refusal struct {
			Code   string `json:"code"`
			Detail struct {
				Delivery string `json:"delivery"`
			} `json:"detail"`
		} `json:"refusal"`
	}
	_ = json.Unmarshal(reply.Result, &result)
	codes := []string{reply.ErrorCode, result.Refusal.Code, result.Error.Code}
	if action.Error != nil {
		codes = append(codes, action.Error.Code)
	}
	for _, code := range codes {
		// The pinned producer uses trust_unavailable for failures both
		// before input and after a mouse press. Neither proves unsent input.
		if code == "browser_input_incomplete" || code == "browser_input_trust_unavailable" {
			return true
		}
	}
	return result.Refusal.Detail.Delivery == "unknown"
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

type cuaAction struct {
	Effect   string `json:"effect"`
	Route    string `json:"route"`
	Delivery *struct {
		Count *int   `json:"delivered_count"`
		Mode  string `json:"mode"`
	} `json:"delivery"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func cuaInputFacts(request computeruse.CuaRequest, reply computeruse.CuaReply, body map[string]any) {
	var action cuaAction
	_ = json.Unmarshal(reply.Action, &action)
	input := request.Action == "click" || request.Action == "type" || request.Action == "key" || request.Action == "hotkey" || request.Action == "scroll" || request.Action == "move" || request.Action == "browser_click" || request.Action == "browser_type" || request.Action == "browser_navigate" || request.Action == "browser_select" || request.Action == "browser_pointer"
	if input {
		refusal := cuaRefusal(reply, action.Effect)
		if action.Effect == "refused" && action.Error != nil {
			refusal = action.Error.Code
		}
		switch {
		case request.IsBrowser() && cuaBrowserDeliveryUnknown(reply, action):
			body["dispatch_unknown"] = true
			if action.Delivery != nil && action.Delivery.Count != nil && *action.Delivery.Count > 0 {
				body["dispatched"], body["dispatch_method"] = true, "cua_"+action.Route
			}
		case action.Delivery != nil && action.Delivery.Count != nil && *action.Delivery.Count > 0:
			body["dispatched"], body["dispatch_method"] = true, "cua_"+action.Route
		case cuaBrowserInputAcknowledged(request, reply, action):
			// The pinned browser producer returns this only after CDP accepts
			// input and restores focus emulation. It is not page-effect proof.
			body["dispatched"], body["dispatch_method"] = true, "cua_cdp_input"
		case cuaBrowserMutationAcknowledged(request, reply):
			if request.Action == "browser_select" {
				var selection struct {
					Changed bool `json:"changed"`
				}
				_ = json.Unmarshal(reply.Result, &selection)
				if selection.Changed {
					body["dispatched"], body["dispatch_method"] = true, "cua_dom_select"
				} else {
					body["no_change"] = true
				}
			} else {
				body["dispatched"], body["dispatch_method"] = true, "cua_browser_navigation"
			}
		case refusal != "":
			body["blocked_reason"] = refusal
		default:
			body["dispatch_unknown"] = true
		}
	}
}

// These SDK tools return structured acknowledgements without an input action
// record. Acknowledging navigation/selection still does not prove page success.
func cuaBrowserMutationAcknowledged(request computeruse.CuaRequest, reply computeruse.CuaReply) bool {
	if reply.IsError || reply.Completion == "unknown" || request.TargetID == "" || request.TabID == "" {
		return false
	}
	var state struct {
		Status          string  `json:"status"`
		TargetID        string  `json:"target_id"`
		TabID           string  `json:"tab_id"`
		URL             string  `json:"url"`
		Ref             string  `json:"ref"`
		RefsInvalidated bool    `json:"refs_invalidated"`
		Changed         *bool   `json:"changed"`
		Label           *string `json:"label"`
		Value           *string `json:"value"`
	}
	if json.Unmarshal(reply.Result, &state) != nil || state.Status != "ok" || state.TargetID != request.TargetID || state.TabID != request.TabID {
		return false
	}
	switch request.Action {
	case "browser_navigate":
		return request.URL != "" && state.URL == request.URL && state.RefsInvalidated
	case "browser_select":
		return request.Ref != "" && state.Ref == request.Ref && state.Changed != nil &&
			(request.Label != nil && request.Value == nil && state.Label != nil && *state.Label == *request.Label ||
				request.Value != nil && request.Label == nil && state.Value != nil && *state.Value == *request.Value)
	}
	return false
}

func cuaBrowserInputAcknowledged(request computeruse.CuaRequest, reply computeruse.CuaReply, action cuaAction) bool {
	return request.IsBrowser() && !reply.IsError && action.Route == "trusted_input" && action.Effect != "refused" && action.Delivery != nil && (action.Delivery.Mode == "background" || action.Delivery.Mode == "foreground")
}
