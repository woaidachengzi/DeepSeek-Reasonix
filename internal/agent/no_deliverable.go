package agent

import (
	"context"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

func (a *Agent) recoverNoDeliverable(ctx context.Context, state *turnRuntime, reasoning string, usage *provider.Usage) (bool, error) {
	// Transport completion does not deliver an answer. Recovery is bounded
	// across the whole Run, including intervening successful tool rounds.
	if axis, detail := a.task.budget.exceeded(a.taskBudgetLimit(ctx)); axis != "" {
		return false, &taskBudgetPause{axis: axis, detail: detail}
	}
	state.terminal.emptyFinalBlocks++
	if state.terminal.emptyFinalBlocks >= maxEmptyFinalBlocks {
		a.recordInterruptedDisplay("", "", nil, true, provider.ErrNoDeliverableOutput, state.workDurationMs())
		return false, provider.ErrNoDeliverableOutput
	}
	a.svc.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Code: event.NoticeCodeEmptyFinal, Text: emptyFinalNotice(), Detail: emptyFinalNoticeDetail(a.svc.prov.Name(), usage, len(reasoning))})
	a.sess.conversation.Add(HostGeneratedUserMessage(a.withTurnPreferences(emptyFinalRetryMessage())))
	a.contextManager().ObserveUsage(usage)
	return true, nil
}
