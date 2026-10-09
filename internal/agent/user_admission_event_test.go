package agent

import (
	"context"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func TestUserAdmissionEventFollowsCanonicalAppend(t *testing.T) {
	session := NewSession("private system prompt")
	var admitted event.Event
	started := false
	sink := event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnStarted {
			started = true
		}
		if e.Kind != event.UserMessageAdmitted {
			return
		}
		if !started || e.MessageID == "" || e.Text != "" || e.ItemID != "" {
			t.Fatalf("invalid content-free admission: %+v", e)
		}
		found := false
		for _, message := range session.Snapshot() {
			if message.ID == e.MessageID {
				found = IsUserAuthoredTurnMessage(message) && UserMessageText(message) == "private user question"
			}
		}
		if !found {
			t.Fatal("admission was published before canonical user append")
		}
		if admitted.MessageID != "" {
			t.Fatal("one question published multiple admissions")
		}
		admitted = e
	})
	a := New(&fakeProvider{}, tool.NewRegistry(), session, Options{}, sink)
	a.beginRunTurn(context.Background(), "private user question", pinnedRevisionPlan{})
	if admitted.MessageID == "" {
		t.Fatal("missing canonical question identity")
	}
}

func TestSyntheticContinuationDoesNotAdmitUserQuestion(t *testing.T) {
	count := 0
	hostCount := 0
	session := NewSession("system")
	a := New(&fakeProvider{}, tool.NewRegistry(), session, Options{}, event.FuncSink(func(e event.Event) {
		if e.Kind == event.UserMessageAdmitted {
			count++
		}
		if e.Kind == event.HostInputAdmitted {
			hostCount++
			messages := session.Snapshot()
			last := messages[len(messages)-1]
			if e.MessageID == "" || last.ID != e.MessageID || last.Origin != provider.MessageOriginHost || e.Text != "" || e.ItemID != "" {
				t.Fatal("host admission preceded canonical append or carried content/authority")
			}
		}
	}))
	a.beginRunTurn(WithInputMessageOrigin(context.Background(), provider.MessageOriginHost), "synthetic continuation", pinnedRevisionPlan{})
	if count != 0 {
		t.Fatal("host continuation must not become a user question")
	}
	if hostCount != 1 {
		t.Fatal("explicit host input must publish exactly one independent readiness boundary")
	}
}
