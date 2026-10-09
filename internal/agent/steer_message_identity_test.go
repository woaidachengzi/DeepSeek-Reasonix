package agent

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/agent/testutil"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

type queueIdentitySteerTool struct{ queue func() bool }

func (t *queueIdentitySteerTool) Name() string        { return "queue_identity_steer" }
func (t *queueIdentitySteerTool) Description() string { return "queues owned test guidance" }
func (t *queueIdentitySteerTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (t *queueIdentitySteerTool) ReadOnly() bool { return true }
func (t *queueIdentitySteerTool) Execute(context.Context, json.RawMessage) (string, error) {
	if t.queue() {
		return "queued", nil
	}
	return "rejected", nil
}

func TestAppliedSteerEventNamesAlreadySavedMessage(t *testing.T) {
	seen := map[string]bool{}
	for _, durable := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy intake", true: "durable intake"}[durable], func(t *testing.T) {
			mp := testutil.NewMock("m", testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "owned-call", Name: "queue_identity_steer", Arguments: `{}`}}}, testutil.Turn{Text: "owned answer"})
			queued := &queueIdentitySteerTool{}
			reg := tool.NewRegistry()
			reg.Add(queued)
			session := NewSession("owned system")
			var steers []event.Event
			sink := event.FuncSink(func(e event.Event) {
				if e.Kind != event.Steer {
					return
				}
				steers = append(steers, e)
				matches := 0
				for _, message := range session.Snapshot() {
					if message.ID == e.MessageID {
						text, ok := SteerText(message.Content)
						if !ok || text != "same guidance" || message.RawContent != text || message.Role != provider.RoleUser || message.Origin != provider.MessageOriginUser {
							t.Fatalf("saved association: %+v", message)
						}
						matches++
					}
				}
				if e.MessageID == "" || matches != 1 || e.MessageID == e.ItemID {
					t.Fatalf("message not saved before publication: %+v", e)
				}
			})
			a := New(mp, reg, session, Options{}, sink)
			queued.queue = func() bool {
				if durable {
					return a.SteerItem("owned-inbox", func() (string, error) { return "same guidance", nil })
				}
				return a.Steer("same guidance")
			}
			if err := a.Run(context.Background(), "owned question"); err != nil {
				t.Fatal(err)
			}
			if len(steers) != 1 || steers[0].Text != "same guidance" || (durable && steers[0].ItemID != "owned-inbox") || (!durable && steers[0].ItemID != "") {
				t.Fatalf("steers: %+v", steers)
			}
			if seen[steers[0].MessageID] {
				t.Fatal("same text reused positional identity")
			}
			seen[steers[0].MessageID] = true
		})
	}
}
