package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"reasonix/internal/capability"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

type receiptVisibilityTool struct{ fail bool }

func (receiptVisibilityTool) Name() string            { return "computer_use" }
func (receiptVisibilityTool) Description() string     { return "fixture" }
func (receiptVisibilityTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (receiptVisibilityTool) ReadOnly() bool          { return false }
func (f receiptVisibilityTool) Execute(context.Context, json.RawMessage) (string, error) {
	if f.fail {
		return `{"dispatched":false,"blocked_reason":"observation_expired","next_action":"observe"}`, errors.New("observation expired; no input sent")
	}
	return `{"dispatched":true,"input_verification":"unconfirmed"}`, nil
}

func TestRepeatedDesktopOutcomesRemainVisibleThroughProxy(t *testing.T) {
	for _, fail := range []bool{false, true} {
		reg := tool.NewRegistry()
		reg.Add(receiptVisibilityTool{fail: fail})
		reg.Add(NewUseCapabilityTool(t.Context(), nil, nil, reg, capability.NewLedger(), nil, nil))
		a := New(&scriptedProvider{name: "fixture"}, reg, NewSession("sys"), Options{}, event.Discard)
		for _, id := range []string{"first", "second"} {
			result := a.executeOne(t.Context(), &a.turn, provider.ToolCall{ID: id, Name: "use_capability", Arguments: `{"action":"call","capability_id":"tool:computer_use","arguments":{}}`})
			if strings.Contains(result.output, "duplicate tool result omitted") || !strings.Contains(result.output, `"dispatched":`) {
				t.Fatalf("native outcome disappeared on %s (fail=%v): %+v", id, fail, result)
			}
			if fail && !strings.Contains(result.output, `"blocked_reason":"observation_expired"`) {
				t.Fatal("a refusal could be mistaken for dispatched input")
			}
			if result.blocked != fail || (fail && result.runState != provider.ToolRunNotStarted) {
				t.Fatal("a preserved zero-input refusal must remain blocked, not successful")
			}
			if result.computerUseInputProgress == fail {
				t.Fatal("proxy result lost its actual input progress classification")
			}
		}
	}
}
