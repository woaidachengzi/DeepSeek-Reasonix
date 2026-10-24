package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/imageinput"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

type refusingDesktop struct{ inputs, observations int }

func (*refusingDesktop) Name() string            { return "computer_use" }
func (*refusingDesktop) Description() string     { return "test desktop" }
func (*refusingDesktop) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (*refusingDesktop) ReadOnly() bool          { return false }
func (d *refusingDesktop) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	body, _, err := d.ExecuteWithImages(ctx, args)
	return body, err
}
func (d *refusingDesktop) ExecuteWithImages(_ context.Context, args json.RawMessage) (string, []string, error) {
	var request struct{ Action string }
	_ = json.Unmarshal(args, &request)
	if request.Action == "observe" {
		d.observations++
		return `{"state":{"window_id":11},"observation":"new-receipt","dispatched":false}`, []string{"fresh-frame"}, nil
	}
	d.inputs++
	reason := "visual_changed"
	if d.inputs%2 == 0 {
		reason = "controls_changed"
	}
	return fmt.Sprintf(`{"state":{"window_id":11},"observation":"another-receipt","dispatched":false,"state_changed":true,"blocked_reason":%q}`, reason), []string{"refused-frame"}, nil
}

func TestComputerUseRefusalsPauseRealRunDespiteFreshObservations(t *testing.T) {
	desktop := &refusingDesktop{}
	reg := tool.NewRegistry()
	reg.Add(desktop)
	p := &scriptedProvider{name: "desktop-model"}
	for i := range 9 {
		action := "click"
		if i%2 == 1 {
			action = "observe"
		}
		p.turns = append(p.turns, []provider.Chunk{toolCallChunk(fmt.Sprintf("d%d", i), "computer_use", fmt.Sprintf(`{"action":%q}`, action)), {Type: provider.ChunkDone}})
	}
	p.turns = append(p.turns, []provider.Chunk{{Type: provider.ChunkText, Text: "Stopped after repeated refusals."}, {Type: provider.ChunkDone}})
	a := New(p, reg, NewSession("system"), Options{}, event.Discard)
	err := a.Run(t.Context(), "fill the local test form")
	var pause *RecoveryPauseError
	if !errors.As(err, &pause) || pause.StopReason != "computer_use_blocked" || desktop.inputs != 3 || desktop.observations != 2 || p.call != 6 {
		t.Fatalf("unbounded desktop retries: inputs=%d observes=%d rounds=%d err=%v", desktop.inputs, desktop.observations, p.call, err)
	}
	for _, message := range a.Session().Snapshot() {
		if message.Role == provider.RoleTool && (message.ToolCallID == "d0" || message.ToolCallID == "d2" || message.ToolCallID == "d4") {
			if len(message.Images) != 1 || !strings.Contains(message.Content, `"dispatched":false`) || message.ToolRunState != provider.ToolRunNotStarted {
				t.Fatal("refusal lost its current frame, dispatch fact or recovery classification")
			}
		}
	}
}

func TestComputerUseProgressResetsOnlyOnProvenInput(t *testing.T) {
	a := New(nil, tool.NewRegistry(), NewSession(""), Options{}, event.Discard)
	call := []provider.ToolCall{{Name: "use_capability", CapabilityID: "tool:computer_use"}}
	refusal := toolOutcome{blocked: true, errMsg: computerUseRefusalPrefix + "visual_changed"}
	a.applyComputerUseProgress(t.Context(), call, []toolOutcome{refusal}, 0)
	a.applyComputerUseProgress(t.Context(), call, []toolOutcome{{output: `{"dispatched":false,"observation":"fresh"}`}}, 0)
	if a.turn.computerUseRefusals != 1 {
		t.Fatal("observation counted as input progress")
	}
	a.applyComputerUseProgress(t.Context(), call, []toolOutcome{{computerUseInputProgress: true, output: `{"dispatched":true}` + "\n[image summary]"}}, 0)
	if a.turn.computerUseRefusals != 0 {
		t.Fatal("actual dispatch did not clear the refusal streak")
	}
	for _, body := range []string{`{"dispatched":true,"blocked_reason":"visual_changed"}`, `{"dispatched":false,"dispatch_unknown":true,"blocked_reason":"visual_changed"}`, `{"dispatched":null,"blocked_reason":"visual_changed"}`} {
		if computerUseRefusal("computer_use", body) != "" {
			t.Fatal("uncertain/performed input treated as a zero-input refusal")
		}
	}
}

func TestComputerUseRefusalKeepsImageUnderstandingFallback(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(&refusingDesktop{})
	vision := &summaryProvider{}
	config := &imageinput.Config{Model: "vision/model", Resolve: func(string) (provider.Provider, error) { return vision, nil }}
	a := New(&scriptedProvider{name: "text"}, reg, NewSession(""), Options{ImageInput: config, ModelRef: "text/model"}, event.Discard)
	out := a.executeOne(t.Context(), &a.turn, provider.ToolCall{ID: "refused", Name: "computer_use", Arguments: `{"action":"click"}`})
	if !out.blocked || out.runState != provider.ToolRunNotStarted || len(out.images) != 1 || out.visionSummary == nil || !strings.Contains(out.output, `"dispatched":false`) || vision.calls.Load() != 1 {
		t.Fatalf("a blocked desktop result lost observation/image fallback: %+v", out)
	}
}

func TestComputerUsePointerMoveIsNotInputProgress(t *testing.T) {
	if computerUseInputProgress("computer_use", `{"dispatched":true,"dispatch_method":"window_server_move","pointer_preview":{"screen_x":10,"screen_y":20}}`) {
		t.Fatal("cursor preview reset refused-click budget")
	}
	if !computerUseInputProgress("computer_use", `{"dispatched":true,"dispatch_method":"window_server_mouse"}`) {
		t.Fatal("confirmed click was not input progress")
	}
}
