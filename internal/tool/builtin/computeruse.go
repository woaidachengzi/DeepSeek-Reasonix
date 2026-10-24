package builtin

import (
	"context"
	"encoding/json"
	"errors"

	"reasonix/internal/computeruse"
	"reasonix/internal/sandbox"
	"reasonix/internal/tool"
)

type computerUse struct {
	controller computerUseController
	confined   bool
}

type computerUseController interface {
	Target(context.Context, int) (computeruse.Target, error)
	Execute(context.Context, computeruse.Request) (computeruse.Result, error)
	ExecuteForTarget(context.Context, computeruse.Request, computeruse.Target) (computeruse.Result, error)
}

func init() {
	if computeruse.Available() {
		tool.RegisterBuiltin(newComputerUse(false))
	}
}
func newComputerUse(confined bool) computerUse {
	return computerUse{controller: computeruse.New(), confined: confined}
}

// ConfineComputerUse keeps the non-workspace boot path under the same desktop
// capability boundary as Workspace.Tools.
func ConfineComputerUse(spec sandbox.Spec) tool.Tool { return newComputerUse(spec.Mode == "enforce") }

func (computerUse) Name() string { return "computer_use" }
func (computerUse) Description() string {
	return "Observe and control a native macOS app. Start with status and apps, then observe a selected PID. Observation returns window bounds, accessibility elements, a screenshot, and a 120-second receipt (observation_ttl_ms). Every action re-observes and validates current native state before input. Use that receipt for exactly one activate/move/confirm_click/click/fill/type/key/scroll action; state_changed means no input was sent and a new observation was returned. Inspect blocked_reason: window_changed, window_not_active, focus_changed, controls_changed, visual_changed, invalid_observation, or observation_expired. For observation_expired, dispatched=false and next_action=observe mean the host sent no input: obtain a fresh observation and inspect it before acting. Do not infer a broken input channel, permissions failure, or multi-monitor coordinate bug from an expired receipt. For screenshot targets, always set coordinate_space=image and use x/y in the actual image_geometry.width/height pixels; the host converts to window-local points. Do not manually scale or add global screen origins. Use coordinate_space=window only for window-local AX point coordinates; omission preserves legacy window points. A coordinate_mapping_unavailable refusal sends no input; obtain a fresh observation and use its exact screenshot dimensions. Normal windows are captured without upscaling; images larger than 1920 pixels may be downscaled. Input requires the observed window to be active. Prefer fill with an observed editable element or text-field coordinates to set its complete value and read it back in one local action; it sends no Return or submit-button press and refuses unverified, read-only or secure fields. fill is idempotent: unchanged=true and input_verification=confirmed means the field already matched. If fill is unsupported, inspect a fresh observation and use click/type; do not repeatedly retry fill. Old desktop screenshots/AX trees are superseded in model input after two observations; full results remain in local history. Three input refusals without a dispatched action pause this turn. For the first screenshot-coordinate click in an app, after display/zoom/window geometry changes, or after a missed/refused click, use move with screenshot x/y first. move posts only mouse movement and returns a fresh screenshot with a red crosshair at the actual OS cursor position (pointer_preview), not the requested point. Inspect the crosshair against the intended target. If it is correct, use confirm_click with only PID and the move observation receipt; do not supply coordinates or an element. The host binds the click to that actual position and refuses pointer_moved, stale/changed window/page, or missing preview. confirm_click posts down/up only and never moves the pointer back. If the marker is wrong, move to a corrected target; do not confirm. After two unsuccessful corrected previews, stop and report the coordinates and refusal reason instead of looping. A movement is not task success. Use ordinary click for subsequent reliable targets to avoid an extra model round on every action. Coordinate clicks move the pointer through WindowServer only after confirming the point belongs to the approved, uncovered window. pointer_target_changed, pointer_target_unverified, pointer_busy, and input_permission_missing refuse input; observe again and resolve the reported condition rather than repeating blindly. dispatch_method=window_server_mouse identifies that route. Do not infer caret blink from blocked_reason alone: controls_changed reports an actual AX state change. observation_validation=caret_only means the AX state was identical and pixels changed only at a verified text caret; pointer targets additionally remain outside the focused field and caret region. After dispatch inspect the returned state; dispatched means input sent, not that the task succeeded; dispatch_unknown means input may have been sent. For type, input_verification=confirmed means field content matched the expected edit; unconfirmed means inspect the returned field/value or observe again, never repeat typing automatically. A recipient converted to a chip can require visual verification. Never automatically repeat an uncertain action. Requires Accessibility and Screen Recording permissions; never prompts for them automatically. Desktop automation is unavailable under the enforced workspace sandbox."
}
func (computerUse) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["status","apps","observe","activate","move","confirm_click","click","fill","type","key","scroll"]},"pid":{"type":"integer","minimum":1},"window_id":{"type":"integer","minimum":1},"observation":{"type":"string"},"element":{"type":"string","description":"Observed accessibility element path for AX press, verified text fill, or a vertical scroll area/bar"},"coordinate_space":{"type":"string","enum":["image","window"],"description":"For screenshot x/y choose image: host converts exact image_geometry pixels. Window selects window-local AX points; omission is legacy window points. Not valid with element when image is selected."},"x":{"type":"number","description":"X from screenshot pixels when coordinate_space=image; otherwise window-local points"},"y":{"type":"number","description":"Y from screenshot pixels when coordinate_space=image; otherwise window-local points"},"text":{"type":"string","minLength":1,"description":"For fill, the complete desired field value; for type, text inserted at the current selection"},"key":{"type":"string","enum":["return","tab","escape","backspace","space","left","right","up","down","a","c","v","x","z"]},"modifiers":{"type":"array","items":{"type":"string","enum":["command","shift","option","control"]}},"delta":{"type":"integer","minimum":-100,"maximum":100,"description":"Relative vertical amount: negative down, positive up; native controls may use page granularity"}},"required":["action"],"additionalProperties":false}`)
}

// All desktop operations pass the ordinary write-capability approval gate and
// remain unavailable to read-only agents. They must not bypass workspace scope.
func (computerUse) ReadOnly() bool                         { return false }
func (c computerUse) ProviderVisible(context.Context) bool { return !c.confined }
func (c computerUse) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	text, _, err := c.ExecuteWithImages(ctx, args)
	return text, err
}
func (c computerUse) ExecuteWithImages(ctx context.Context, args json.RawMessage) (string, []string, error) {
	if c.confined {
		return "", nil, errors.New("computer_use cannot operate outside the enforced workspace sandbox")
	}
	var r computeruse.Request
	if err := json.Unmarshal(args, &r); err != nil {
		return "", nil, err
	}
	if err := computeruse.ValidateRequest(r); err != nil {
		return "", nil, &computerUseError{cause: err}
	}
	var result computeruse.Result
	var err error
	if r.Action == "status" || r.Action == "apps" {
		result, err = c.controller.Execute(ctx, r)
	} else {
		approver := tool.ComputerUseApproverFrom(ctx)
		if approver == nil {
			return "", nil, &computerUseError{cause: tool.Blocked("computer control is unavailable: the interactive host did not install its app access approval handler. This is a host configuration failure, not revoked macOS permission. Stop desktop input and report it; do not fall back to Bash, osascript/JXA, CGEvent, or clipboard probes")}
		}
		target, resolveErr := c.controller.Target(ctx, r.PID)
		if resolveErr != nil {
			return "", nil, &computerUseError{cause: resolveErr}
		}
		allow, approveErr := approver.ApproveComputerUse(ctx, tool.ComputerUseTarget(target))
		if approveErr != nil {
			return "", nil, &computerUseError{cause: approveErr}
		}
		if !allow {
			return "", nil, &computerUseError{cause: errors.New("user declined access to this app; do not retry in this task")}
		}
		result, err = c.controller.ExecuteForTarget(ctx, r, target)
		if result.Image != "" {
			var state struct {
				WindowID int `json:"window_id"`
			}
			_ = json.Unmarshal(result.State, &state)
			if thumbnail := computeruse.PreviewImage(result.Image); thumbnail != "" {
				tool.PublishComputerUsePreview(ctx, tool.ComputerUsePreview{Target: tool.ComputerUseTarget(target), WindowID: state.WindowID, Action: r.Action, Image: thumbnail})
			}
		}
	}
	// A changed frame is a new observation, not a dispatch failure. Returning
	// it normally preserves its image through the engine's result channel.
	if result.StateChanged {
		err = nil
	}
	body, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return "", nil, marshalErr
	}
	var images []string
	if result.Image != "" {
		images = []string{result.Image}
	}
	if err != nil {
		err = &computerUseError{cause: err}
	}
	return string(body), images, err
}

// Desktop dispatch can have an uncertain outcome. The model must explicitly
// inspect it; generic transient-error retries must not replay any input.
type computerUseError struct{ cause error }

func (e *computerUseError) Error() string          { return e.cause.Error() }
func (e *computerUseError) Unwrap() error          { return e.cause }
func (*computerUseError) RetryableToolError() bool { return false }
