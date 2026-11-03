package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"reasonix/internal/provider"
)

func (a *Agent) compactionStop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if axis, detail := a.task.budget.exceeded(a.taskBudgetLimit(ctx)); axis != "" {
		return &taskBudgetPause{axis: axis, detail: detail}
	}
	return nil
}

// Receipts can describe a later failed attempt, and free prune rewrites clear
// SummaryHash. Recognize resumable work from the still-valid reducing body and
// its host digest instead, including checkpoints restored from disk.
func (a *Agent) hasPartialSummaryCheckpoint(hard int) bool {
	canonical, _ := a.sess.conversation.snapshotMessagesVersion()
	a.sess.compactionMu.Lock()
	state := a.sess.compactionState
	a.sess.compactionMu.Unlock()
	projection := state.Projection
	if hard <= 0 || projection.ProjectionTokens < hard || projection.SourceTokens <= projection.ProjectionTokens ||
		!projectionValid(state, canonical, a.currentPromptCacheKey()) {
		return false
	}
	for _, message := range projection.Messages {
		if message.Origin == provider.MessageOriginHost && isCompactionSummary(message) {
			return true
		}
	}
	return false
}

// A successful partial fold is resumable work, even above the provider's
// ceiling. Do not replace it with a lossy rescue when the batch allowance ends
// or a later summary fails. Returning no prepared input prevents sampling it.
func (m ContextManager) retainCompactionProgress(policy ContextPreparePolicy, cause error) (PreparedContext, error) {
	latest := m.currentPrepared()
	reason := fmt.Sprintf("summary progress retained at %d tokens; recovery must continue before sampling: %v", latest.InputTokens, cause)
	m.agent.recordContextMaintenanceBlocked(m.agent.contextMaintenanceInputHash(latest.Messages), policy.Trigger, "summary", reason)
	return PreparedContext{}, fmt.Errorf("%w: %s", ErrCompactionRequired, reason)
}

func (m ContextManager) compactionBatchExhausted(policy ContextPreparePolicy, result PreparedContext, attempts, fold, hard int) (PreparedContext, error) {
	a := m.agent
	reason := fmt.Sprintf("summary result remains above fold trigger after %d attempts (%d >= %d)", attempts, result.InputTokens, fold)
	if policy.Trigger == CompactionTriggerOverflow || result.InputTokens >= hard {
		return m.retainCompactionProgress(policy, errors.New(reason))
	}
	inputHash := a.contextMaintenanceInputHash(result.Messages)
	a.recordContextMaintenanceBlocked(inputHash, policy.Trigger, "summary", reason)
	a.sess.compaction.stuck = true
	a.sess.compaction.stuckInputHash = inputHash
	a.sess.compaction.consecutive += attempts
	slog.Info("agent: context maintenance paused below hard ceiling", "reason", reason)
	return result, nil
}
