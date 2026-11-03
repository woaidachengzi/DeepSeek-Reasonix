package agent

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"reasonix/internal/provider"
)

// compactionProgress is how compaction is faring in this session: whether a
// fold stopped reducing, how many ran back to back, and which retries already
// ran in the active turn. The fields are cleared together on lineage resets.
type compactionProgress struct {
	stuck          bool   // a fold landed above the trigger, so the same-view pressure retry is pointless
	stuckInputHash string // provider-visible view covered by stuck; changed input may retry
	consecutive    int    // back-to-back folds since one last helped
	// failedTurn backs off changed-view retries within one active tool loop.
	// A later user turn may retry, while hard-ceiling recovery bypasses it.
	failedTurn atomic.Int64
	// lastTurn stops the post-turn observer and the pre-send preflight from
	// paying for two summaries during one active tool loop.
	lastTurn atomic.Int64
}

// ContextManager is the sole owner of provider-visible context maintenance.
// Canonical session messages are immutable inputs; Prepare evolves only the
// durable projection and returns the exact visible view for one sampling round.
type ContextManager struct {
	agent *Agent
}

// ContextPreparePolicy describes one maintenance transaction.
type ContextPreparePolicy struct {
	Trigger      string
	Instructions string
	Force        bool
	// ObservedInputTokens is used by compatibility harnesses that invoke the
	// old post-turn shim directly. Production Prepare estimates the current view
	// from its calibrated final request shape.
	ObservedInputTokens int
	// AllowChunkedFallback enables fragment/tree-reduce recovery after a single
	// summary fails. Ordinary pressure/overflow leave this false.
	AllowChunkedFallback bool
}

// PreparedContext is the frozen result of a successful Prepare transaction.
type PreparedContext struct {
	Messages          []provider.Message
	InputTokens       int
	ProjectionVersion uint64
}

func (a *Agent) contextManager() ContextManager { return ContextManager{agent: a} }

// PrepareContext is the public automatic-maintenance entry used by smoke tools
// and controllers that need a one-shot Prepare without sampling.
func (a *Agent) PrepareContext(ctx context.Context) error {
	_, err := a.contextManager().Prepare(ctx, ContextPreparePolicy{Trigger: CompactionTriggerPressure})
	return err
}

// ObserveUsage is retained as a compatibility hook. Usage observations never
// mutate the provider-visible checkpoint.
func (m ContextManager) ObserveUsage(u *provider.Usage) {
	_ = u
}

// Prepare is the sole automatic maintenance entry. Below compact_ratio it does
// nothing. At or above the trigger it runs one single-flight prune/summary
// transaction. Ordinary pressure gets two summaries; over-ceiling recovery
// gets at most four and keeps each reducing checkpoint for the next batch.
func (m ContextManager) Prepare(ctx context.Context, policy ContextPreparePolicy) (PreparedContext, error) {
	// Legacy desktop callers can compact before their runtime context is installed.
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return PreparedContext{}, err
	}
	if policy.Trigger == "" {
		policy.Trigger = CompactionTriggerPressure
	}
	if m.agent == nil {
		return PreparedContext{}, nil
	}
	m.agent.sess.compactionRunMu.Lock()
	defer m.agent.sess.compactionRunMu.Unlock()
	// Cancellation may have arrived while another maintenance transaction held the lock.
	// Reject it before any fast path or projection maintenance can run.
	if err := ctx.Err(); err != nil {
		return PreparedContext{}, err
	}
	return m.prepareOnce(ctx, policy)
}

func (m ContextManager) prepareOnce(ctx context.Context, policy ContextPreparePolicy) (PreparedContext, error) {
	a := m.agent
	if a == nil || a.sess.conversation == nil {
		return PreparedContext{}, nil
	}
	visible := a.modelVisibleMessages()
	// Threshold uses the stable pre-interceptor request shape (messages + tools
	// + role projection). Extension interceptors run only on the real sampling
	// request so side-effecting plugins are not double-invoked; if they expand
	// the prompt past the hard ceiling, overflow recovery still fires.
	est := a.estimatedVisibleRequestTokens(visible)
	viewEst := est
	prepared := PreparedContext{
		Messages:          append([]provider.Message(nil), visible...),
		InputTokens:       est,
		ProjectionVersion: a.currentProjectionVersion(),
	}
	if a.contextWindow <= 0 || len(visible) == 0 {
		return prepared, nil
	}
	fold := a.compactTrigger()
	hard := a.hardInputCeiling()
	if policy.ObservedInputTokens > 0 {
		est = policy.ObservedInputTokens
		prepared.InputTokens = est
	}
	inputHash := a.contextMaintenanceInputHash(visible)
	// Receipts back off sub-critical retries only. Above the ceiling, continue
	// an incomplete summary checkpoint; when none exists, lossy rescue remains
	// available. Neither path may return an oversized view for sampling.
	if blocked, _ := a.contextMaintenanceBlocked(inputHash, viewEst); blocked && policy.Trigger != CompactionTriggerManual &&
		policy.Trigger != CompactionTriggerOverflow && est < hard {
		return prepared, nil
	}
	if est < fold {
		a.resetCompactionProgress()
	}
	if a.sess.compaction.stuck && a.sess.compaction.stuckInputHash != inputHash {
		// The previous projection could not reclaim enough from its exact view,
		// but newly appended messages create a new fold boundary and may retry.
		a.sess.compaction.stuck = false
		a.sess.compaction.stuckInputHash = ""
		a.sess.compaction.consecutive = 0
	}
	if a.sess.compaction.stuck && policy.Trigger == CompactionTriggerPressure && est < hard {
		return prepared, nil
	}
	// One user trigger. Overflow is a one-shot physical recovery path only.
	forceFold := policy.Force || policy.Trigger == CompactionTriggerManual || policy.Trigger == CompactionTriggerOverflow || est >= hard
	if est < fold && !forceFold {
		return prepared, nil
	}

	// A manual compact over the hard ceiling is a rescue, not a convenience:
	// prune first so the never-folded recent tail can shrink too.
	if shouldPruneBeforeFold(policy.Trigger, est >= hard) {
		applied, err := a.pruneToolResultsToProjectionLocked(policy.Trigger)
		if err != nil {
			return PreparedContext{}, err
		}
		if applied {
			prepared = m.currentPrepared()
			est = prepared.InputTokens
			inputHash = a.contextMaintenanceInputHash(prepared.Messages)
			if (policy.Trigger == CompactionTriggerPressure && est < fold && est < hard) ||
				(policy.Trigger == CompactionTriggerOverflow && est < hard) {
				return prepared, nil
			}
		}
	}

	return m.foldContext(ctx, prepared, policy, inputHash, est, fold, hard, forceFold)
}

func shouldPruneBeforeFold(trigger string, overHardCeiling bool) bool {
	switch trigger {
	case CompactionTriggerPressure, CompactionTriggerOverflow:
		return true
	case CompactionTriggerManual:
		return overHardCeiling
	default:
		return false
	}
}

// overCeilingRecoverySummaries bounds one Prepare above the hard input ceiling.
// Each fold takes the largest admissible prefix; later Prepare calls can resume
// from the saved checkpoint without unbounded summarizer spend in this batch.
const overCeilingRecoverySummaries = 4

func maxSummariesFor(policy ContextPreparePolicy, overCeiling bool) int {
	switch {
	case overCeiling:
		return overCeilingRecoverySummaries
	case policy.Trigger == CompactionTriggerPressure:
		return 2
	default:
		return 1
	}
}

func (m ContextManager) foldContext(ctx context.Context, prepared PreparedContext, policy ContextPreparePolicy, inputHash string, est, fold, hard int, forceFold bool) (PreparedContext, error) {
	a := m.agent
	maxSummaries := maxSummariesFor(policy, est >= hard)
	ladder := newSummaryLadder(maxSummaries)
	result := prepared
	for ladder.next() {
		if err := a.compactionStop(ctx); err != nil {
			return PreparedContext{}, err
		}
		mustFree := policy.Trigger == CompactionTriggerOverflow || result.InputTokens >= hard
		outcome, err := a.compactToProjectionLocked(ctx, policy.Trigger, policy.Instructions,
			ladder.request(forceFold, mustFree, policy.AllowChunkedFallback))
		if stop := a.compactionStop(ctx); stop != nil {
			return PreparedContext{}, stop
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return PreparedContext{}, err
			}
			if ladder.absorbOverflow(err) {
				continue
			}
			return m.summaryFailed(policy, inputHash, hard, err)
		}
		if outcome == CompactionNoop {
			return m.summaryNoop(policy, inputHash, hard)
		}

		result = m.currentPrepared()
		if foldLanded(policy, result.InputTokens, fold, hard) {
			a.resetCompactionProgress()
			return result, nil
		}
		forceFold = false
		inputHash = a.contextMaintenanceInputHash(result.Messages)
	}

	return m.compactionBatchExhausted(policy, result, maxSummaries, fold, hard)
}

func foldLanded(policy ContextPreparePolicy, tokens, fold, hard int) bool {
	if hard > 0 && tokens >= hard {
		return false
	}
	switch policy.Trigger {
	case CompactionTriggerManual, CompactionTriggerOverflow:
		return tokens < hard || tokens < fold
	default:
		return tokens < fold
	}
}

func (m ContextManager) summaryFailed(policy ContextPreparePolicy, inputHash string, hard int, err error) (PreparedContext, error) {
	a := m.agent
	if errors.Is(err, errCompressStaleContext) && policy.Trigger != CompactionTriggerManual {
		reason := "context changed during summary; automatic retry blocked for this generation"
		a.recordContextMaintenanceBlocked(inputHash, policy.Trigger, "summary", reason)
		return m.rescueOrFail(policy, hard, errors.New(reason))
	}
	status := "failed"
	if errors.Is(err, errSummaryOutputTruncated) || errors.Is(err, errCheckpointRejected) {
		status = "blocked"
	}
	a.recordContextMaintenanceOutcome(inputHash, policy.Trigger, "summary", status, fmt.Sprintf("context summary failed: %v", err))
	return m.rescueOrFail(policy, hard, err)
}

func (m ContextManager) summaryNoop(policy ContextPreparePolicy, inputHash string, hard int) (PreparedContext, error) {
	reason := "context is above the maintenance threshold but no foldable region remains"
	m.agent.recordContextMaintenanceBlocked(inputHash, policy.Trigger, "summary", reason)
	latest := m.currentPrepared()
	switch {
	case policy.Trigger == CompactionTriggerOverflow || latest.InputTokens >= hard:
		return m.rescueByTruncation(policy, hard, errors.New(reason))
	case policy.Force:
		return PreparedContext{}, fmt.Errorf("%w: %s", ErrCompactionRequired, reason)
	default:
		return latest, nil
	}
}

// Below the ceiling automatic maintenance waits for the next view and manual
// compact reports the error. Above it, preserve a partial summary checkpoint
// when available; otherwise attempt the lossy truncation rescue.
func (m ContextManager) rescueOrFail(policy ContextPreparePolicy, hard int, cause error) (PreparedContext, error) {
	latest := m.currentPrepared()
	if policy.Trigger != CompactionTriggerOverflow && latest.InputTokens < hard {
		if policy.Trigger == CompactionTriggerManual {
			return PreparedContext{}, cause
		}
		return latest, nil
	}
	return m.rescueByTruncation(policy, hard, cause)
}

// rescueByTruncation preserves an incomplete summary, or installs a lossy
// projection aimed at the fold trigger when no summary progress can resume.
// cause stays in the error when neither produces safe input.
func (m ContextManager) rescueByTruncation(policy ContextPreparePolicy, hard int, cause error) (PreparedContext, error) {
	a := m.agent
	if a.hasPartialSummaryCheckpoint(hard) {
		return m.retainCompactionProgress(policy, cause)
	}
	applied, err := a.truncateToProjectionLocked(policy.Trigger, a.compactTrigger())
	if err != nil {
		return PreparedContext{}, fmt.Errorf("%w: %w (truncation: %w)", ErrCompactionRequired, cause, err)
	}
	if !applied {
		return PreparedContext{}, fmt.Errorf("%w: %w", ErrCompactionRequired, cause)
	}
	latest := m.currentPrepared()
	if latest.InputTokens >= hard {
		return PreparedContext{}, fmt.Errorf("%w: truncated view still %d >= %d", ErrCompactionRequired, latest.InputTokens, hard)
	}
	a.resetCompactionProgress()
	return latest, nil
}

func (a *Agent) resetCompactionProgress() {
	a.sess.compaction.stuck = false
	a.sess.compaction.stuckInputHash = ""
	a.sess.compaction.consecutive = 0
	a.sess.compaction.failedTurn.Store(0)
}

func (m ContextManager) currentPrepared() PreparedContext {
	if m.agent == nil {
		return PreparedContext{}
	}
	visible := m.agent.modelVisibleMessages()
	return PreparedContext{
		Messages:          append([]provider.Message(nil), visible...),
		InputTokens:       m.agent.estimatedVisibleRequestTokens(visible),
		ProjectionVersion: m.agent.currentProjectionVersion(),
	}
}

// estimatedVisibleRequestTokens sizes the pre-interceptor sampling shape:
// ModelMessages + role projection + tool schemas. Extension interceptors are
// intentionally omitted here (see prepareOnce) to avoid double side effects.
func (a *Agent) estimatedVisibleRequestTokens(visible []provider.Message) int {
	if a == nil {
		return 0
	}
	msgs := a.normalizeModelRequestMessages(visible)
	tools := a.providerToolSchemas()
	return a.estimatedRequestTokens(provider.Request{
		Messages:    msgs,
		Tools:       tools,
		MaxTokens:   a.maxOutputTokens,
		Temperature: provider.OptionalTemperature(a.temperature),
	})
}
