package control

import (
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

func TestControllerHistoryReaderCapturesOneSession(t *testing.T) {
	executor := agent.New(nil, nil, agent.NewSession("old"), agent.Options{}, event.Discard)
	controller := &Controller{executor: executor}
	reader := controller.HistoryReader()
	target, _, _ := reader.Begin(agent.SessionHistoryCursor{})
	executor.SetSession(agent.NewSession("new"))
	var scratch [1]provider.Message
	if chunk, valid := reader.Chunk(target, 0, scratch[:]); !valid || len(chunk) != 1 || chunk[0].Content != "old" {
		t.Fatal("captured reader switched executor sources between chunks")
	}
	newReader := controller.HistoryReader()
	newTarget, start, reset := newReader.Begin(target)
	if !reset || start != 0 {
		t.Fatal("a later request reused the old session prefix")
	}
	if chunk, valid := newReader.Chunk(newTarget, start, scratch[:]); !valid || len(chunk) != 1 || chunk[0].Content != "new" {
		t.Fatal("later request failed to capture the replacement session")
	}
	var absent *Controller
	empty, start, reset := absent.HistoryReader().Begin(target)
	if !reset || start != 0 || empty.MessageCount() != 0 {
		t.Fatal("absent controller retained another session's transcript")
	}
}
