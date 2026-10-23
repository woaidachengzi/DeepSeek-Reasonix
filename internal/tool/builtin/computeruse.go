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
	controller *computeruse.Controller
	confined   bool
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
	return "Observe and control a native macOS app. Start with status and apps, then observe a selected PID. Observation returns window bounds, accessibility elements, a screenshot, and a short-lived receipt. Use that receipt for exactly one activate/click/type/key/scroll action; changed state sends no input and returns a new observation. Coordinates are window points, not scaled image pixels. Input requires the observed window to be active. After dispatch inspect the returned state; dispatched means input sent, not that the task succeeded; dispatch_unknown means input may have been sent. Never automatically repeat an uncertain action. Requires Accessibility and Screen Recording permissions; never prompts for them automatically. Desktop automation is unavailable under the enforced workspace sandbox."
}
func (computerUse) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["status","apps","observe","activate","click","type","key","scroll"]},"pid":{"type":"integer","minimum":1},"window_id":{"type":"integer","minimum":1},"observation":{"type":"string"},"element":{"type":"string","description":"Observed accessibility element path for AX press or a vertical scroll area/bar"},"x":{"type":"number","description":"Window-local x in points"},"y":{"type":"number","description":"Window-local y in points"},"text":{"type":"string","minLength":1},"key":{"type":"string","enum":["return","tab","escape","backspace","space","left","right","up","down","a","c","v","x","z"]},"modifiers":{"type":"array","items":{"type":"string","enum":["command","shift","option","control"]}},"delta":{"type":"integer","minimum":-100,"maximum":100,"description":"Relative vertical amount: negative down, positive up; native controls may use page granularity"}},"required":["action"],"additionalProperties":false}`)
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
	result, err := c.controller.Execute(ctx, r)
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
