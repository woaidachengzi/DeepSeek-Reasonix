package control

import (
	"fmt"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

func TestControllerHistoryDeltaSessionSwap(t *testing.T) {
	initial := agent.NewSession("initial")
	executor := agent.New(nil, nil, initial, agent.Options{}, event.Discard)
	controller := &Controller{executor: executor}
	_, cursor, _ := controller.HistoryDelta(agent.SessionHistoryCursor{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			session := agent.NewSession(fmt.Sprintf("session-%d", i))
			session.Add(provider.Message{ID: "question", Role: provider.RoleUser, Content: fmt.Sprintf("session-%d", i)})
			executor.SetSession(session)
		}
	}()
	for {
		delta, next, reset := controller.HistoryDelta(cursor)
		if reset && len(delta) == 2 && delta[0].Content != delta[1].Content {
			t.Fatal("history pointer and payload came from different sessions")
		}
		if !reset && len(delta) != 0 {
			t.Fatal("fresh session prefix was treated as an append")
		}
		cursor = next
		select {
		case <-done:
			delta, _, reset = controller.HistoryDelta(agent.SessionHistoryCursor{})
			if !reset || len(delta) != 2 || delta[0].Content != "session-99" {
				t.Fatal("final executor session was not projected")
			}
			return
		default:
		}
	}
}
