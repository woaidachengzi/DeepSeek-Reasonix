package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/extension"
	"reasonix/internal/extension/dispatch"
	"reasonix/internal/extension/protocol"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

type progressSummaryProvider struct {
	requests      []provider.Request
	measure       func(provider.Request) int
	window        int
	usage         int
	failAt        int
	finishReason  string
	overflowFirst bool
}

func (*progressSummaryProvider) Name() string { return "bounded-progress-summary" }

func (*progressSummaryProvider) ContextBudgetPolicy() provider.ContextBudgetPolicy {
	return provider.ContextBudgetPolicy{WindowMode: provider.ContextWindowShared, LimitMode: provider.OutputLimitAlways}
}

func (p *progressSummaryProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	if p.overflowFirst {
		p.overflowFirst = false
		return nil, &provider.ContextLimitError{APIError: &provider.APIError{Status: 400, Body: "context overflow"}, WindowTokens: p.window, PromptTokens: 60_000}
	}
	p.requests = append(p.requests, req)
	if total := p.measure(req) + req.MaxTokens + protocolReserveTokens; total > p.window {
		return nil, fmt.Errorf("summary request exceeds window: %d > %d", total, p.window)
	}
	if p.failAt > 0 && len(p.requests) == p.failAt {
		return nil, errors.New("summary unavailable after partial progress")
	}
	return chunks(
		provider.Chunk{Type: provider.ChunkText, Text: "durable progress digest"},
		provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{PromptTokens: p.usage, TotalTokens: p.usage, FinishReason: p.finishReason}},
		provider.Chunk{Type: provider.ChunkDone},
	), nil
}

func progressCompactionFixture(t *testing.T, turns int) (*Agent, *progressSummaryProvider) {
	t.Helper()
	p := &progressSummaryProvider{window: 50_000}
	a := agentOverForceWindow(t, p, foldableSessionOverForce(turns), p.window)
	p.measure = a.estimatedRequestTokens
	return a, p
}

func TestPressureCompactionRecoversThroughReducingCheckpoints(t *testing.T) {
	a, p := progressCompactionFixture(t, 320)
	canonical := a.Session().Snapshot()
	if source, hard := a.ContextUsedTokens(), a.hardInputCeiling(); source < 2*hard {
		t.Fatalf("fixture is only %d tokens, need at least twice %d", source, hard)
	}
	prepared, err := a.contextManager().Prepare(t.Context(), ContextPreparePolicy{Trigger: CompactionTriggerPressure})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.requests) < 2 || len(p.requests) > overCeilingRecoverySummaries {
		t.Fatalf("summary requests=%d, want multiple bounded batches", len(p.requests))
	}
	if prepared.InputTokens >= a.hardInputCeiling() || truncatedRescue(a) {
		t.Fatal("bounded summaries did not restore a safe view without truncation")
	}
	if !reflect.DeepEqual(canonical, a.Session().Snapshot()) {
		t.Fatal("compaction changed canonical history")
	}
	if digest := latestDigest(a.modelVisibleMessages()); digest == "" {
		t.Fatal("recovery discarded the digest")
	}
}

func TestOverCeilingRecoveryKeepsProgressAcrossSamplingPreflights(t *testing.T) {
	a, p := progressCompactionFixture(t, 900)
	canonical := a.Session().Snapshot()
	previous := a.ContextUsedTokens()
	if previous < 8*a.hardInputCeiling() {
		t.Fatalf("fixture is %d tokens, need at least eight windows", previous)
	}
	for attempt := range 6 {
		before := len(p.requests)
		frozen, err := a.buildSamplingRequest(t.Context(), CompactionTriggerPressure)
		current := a.ContextUsedTokens()
		if calls := len(p.requests) - before; calls > overCeilingRecoverySummaries {
			t.Fatalf("attempt %d used %d batches", attempt, calls)
		}
		if current >= previous || truncatedRescue(a) || latestDigest(a.modelVisibleMessages()) == "" {
			t.Fatalf("attempt %d lost summary progress: before=%d after=%d", attempt, previous, current)
		}
		if !reflect.DeepEqual(canonical, a.Session().Snapshot()) {
			t.Fatal("partial recovery changed canonical history")
		}
		if err == nil {
			if a.estimatedRequestTokens(frozen.req) >= a.hardInputCeiling() {
				t.Fatal("sampling admitted a view above the physical ceiling")
			}
			return
		}
		if !errors.Is(err, ErrCompactionRequired) || len(frozen.req.Messages) != 0 {
			t.Fatalf("unsafe preflight result: err=%v messages=%d", err, len(frozen.req.Messages))
		}
		if calls := len(p.requests) - before; calls != overCeilingRecoverySummaries {
			t.Fatalf("attempt %d stopped after %d batches, want the bounded allowance", attempt, calls)
		}
		previous = current
	}
	t.Fatal("bounded recovery did not converge on later preflights")
}

func TestOverCeilingCompactionProgressSurvivesReopen(t *testing.T) {
	a, _ := progressCompactionFixture(t, 900)
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := a.Session().Save(path); err != nil {
		t.Fatal(err)
	}
	a.BindSessionPath(path, true)
	if _, err := a.buildSamplingRequest(t.Context(), CompactionTriggerPressure); !errors.Is(err, ErrCompactionRequired) {
		t.Fatalf("partial recovery=%v", err)
	}
	before := a.contextManager().currentPrepared()
	loaded, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	p := &progressSummaryProvider{window: 50_000}
	resumed := New(p, tool.NewRegistry(), loaded, Options{ContextWindow: p.window, CompactRatio: .5, SessionPath: path}, event.Discard)
	p.measure = resumed.estimatedRequestTokens
	after := resumed.contextManager().currentPrepared()
	if before.ProjectionVersion != after.ProjectionVersion || before.InputTokens != after.InputTokens || !reflect.DeepEqual(before.Messages, after.Messages) {
		t.Fatal("reopen lost the partial projection")
	}
	if _, err := resumed.buildSamplingRequest(t.Context(), CompactionTriggerPressure); !errors.Is(err, ErrCompactionRequired) {
		t.Fatalf("second bounded recovery=%v", err)
	}
	if resumed.ContextUsedTokens() >= before.InputTokens || len(p.requests) != overCeilingRecoverySummaries {
		t.Fatal("reopened recovery did not continue from the saved digest")
	}
}

func TestOverCeilingFailureKeepsThePriorBatchCheckpoint(t *testing.T) {
	for _, mode := range []string{"same agent", "reopen", "free prune"} {
		t.Run(mode, func(t *testing.T) {
			a, p := progressCompactionFixture(t, 900)
			path := filepath.Join(t.TempDir(), "session.jsonl")
			if err := a.Session().Save(path); err != nil {
				t.Fatal(err)
			}
			a.BindSessionPath(path, true)
			if _, err := a.buildSamplingRequest(t.Context(), CompactionTriggerPressure); !errors.Is(err, ErrCompactionRequired) {
				t.Fatalf("first batch=%v", err)
			}
			before := a.contextManager().currentPrepared()
			if mode == "reopen" {
				loaded, err := LoadSession(path)
				if err != nil {
					t.Fatal(err)
				}
				p = &progressSummaryProvider{window: 50_000}
				a = New(p, tool.NewRegistry(), loaded, Options{ContextWindow: p.window, CompactRatio: .5, SessionPath: path}, event.Discard)
				p.measure = a.estimatedRequestTokens
			}
			if mode == "free prune" {
				a.Session().AddBatch(
					provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "later", Name: "read_file", Arguments: "{}"}}},
					provider.Message{Role: provider.RoleTool, ToolCallID: "later", Name: "read_file", Content: strings.Repeat("x", 10_000)},
				)
				if err := a.Session().Save(path); err != nil {
					t.Fatal(err)
				}
				a.sess.compactionRunMu.Lock()
				applied, err := a.pruneToolResultsToProjectionLocked(CompactionTriggerPressure)
				a.sess.compactionRunMu.Unlock()
				if err != nil || !applied || a.sess.compactionState.Projection.SummaryHash != "" {
					t.Fatalf("free prune did not replace summary metadata: applied=%t err=%v", applied, err)
				}
				before = a.contextManager().currentPrepared()
			}
			canonical := a.Session().Snapshot()
			p.failAt = len(p.requests) + 1
			frozen, err := a.buildSamplingRequest(t.Context(), CompactionTriggerPressure)
			if !errors.Is(err, ErrCompactionRequired) || len(frozen.req.Messages) != 0 {
				t.Fatalf("failed recovery admitted input: err=%v messages=%d", err, len(frozen.req.Messages))
			}
			after := a.contextManager().currentPrepared()
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(canonical, a.Session().Snapshot()) {
				t.Fatalf("later failure replaced the saved checkpoint: before=%d v%d after=%d v%d", before.InputTokens, before.ProjectionVersion, after.InputTokens, after.ProjectionVersion)
			}
			if receipt := a.sess.compactionState.LastReceipt; receipt != nil && receipt.Action == maintenanceActionTruncate {
				t.Fatal("later failure installed a lossy truncation")
			}
		})
	}
}

func TestCompactionStopPreservesTheLastCommittedCheckpoint(t *testing.T) {
	for _, mode := range []string{"cancel", "tokens", "wall", "later failure"} {
		t.Run(mode, func(t *testing.T) {
			a, p := progressCompactionFixture(t, 900)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			canonical := a.Session().Snapshot()
			original := a.ContextUsedTokens()
			a.svc.sink = event.FuncSink(func(e event.Event) {
				if e.Kind != event.CompactionDone {
					return
				}
				switch mode {
				case "cancel":
					cancel()
				case "wall":
					a.task.budget.started = time.Now().Add(-time.Hour)
				}
			})
			switch mode {
			case "tokens":
				p.usage = 123
				ctx = WithTaskBudget(ctx, TaskBudget{Tokens: 123})
			case "wall":
				ctx = WithTaskBudget(ctx, TaskBudget{Wall: time.Minute})
			case "later failure":
				p.failAt = 2
			}
			prepared, err := a.contextManager().Prepare(ctx, ContextPreparePolicy{Trigger: CompactionTriggerPressure})
			var budget *taskBudgetPause
			switch mode {
			case "cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("stop=%v", err)
				}
			case "tokens", "wall":
				if !errors.As(err, &budget) {
					t.Fatalf("stop=%v", err)
				}
			case "later failure":
				if !errors.Is(err, ErrCompactionRequired) {
					t.Fatalf("stop=%v", err)
				}
			}
			wantCalls := 1
			if mode == "later failure" {
				wantCalls = 2
			}
			if len(p.requests) != wantCalls || len(prepared.Messages) != 0 || a.currentProjectionVersion() != 1 {
				t.Fatalf("calls=%d prepared=%d projection=%d", len(p.requests), len(prepared.Messages), a.currentProjectionVersion())
			}
			if remaining := a.ContextUsedTokens(); remaining >= original || remaining < a.hardInputCeiling() || truncatedRescue(a) {
				t.Fatalf("checkpoint changed or was replaced by truncation: %d -> %d", original, remaining)
			}
			if !reflect.DeepEqual(canonical, a.Session().Snapshot()) || latestDigest(a.modelVisibleMessages()) == "" {
				t.Fatal("stop lost canonical history or the committed digest")
			}
			if mode == "tokens" && a.task.budget.promptTokens != 123 {
				t.Fatalf("summary token accounting=%d", a.task.budget.promptTokens)
			}
		})
	}
}

func TestCompactionUsageAccountsForRetriesAndFailedAttemptsOnce(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprintf("retry=%t", retry), func(t *testing.T) {
			failure := error(errors.New("summary permanently failed"))
			if retry {
				failure = &provider.APIError{Status: 503}
			}
			p := &retryUsageProvider{
				failOnce: failure, reply: "digest",
				usage1: &provider.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
				usage2: &provider.Usage{PromptTokens: 11, CompletionTokens: 3, TotalTokens: 14},
			}
			a := New(p, tool.NewRegistry(), NewSession("sys"), Options{}, event.Discard)
			_, usage, err := a.summarize(t.Context(), []provider.Message{{Role: provider.RoleUser, Content: "fold me"}}, "")
			if retry && err != nil || !retry && err == nil {
				t.Fatalf("summary error=%v", err)
			}
			requests, prompt, output := 1, 10, 2
			if retry {
				requests, prompt, output = 2, 21, 5
			}
			budget := a.task.budget
			if usage == nil || usage.RequestCount != requests || budget.requests != requests || budget.promptTokens != prompt || budget.outputTokens != output || budget.rounds != 1 {
				t.Fatalf("usage=%+v task budget=%+v", usage, budget)
			}
		})
	}
}

func TestCompactionBudgetStopsChunkedAuxiliaryRequests(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprintf("manual-fallback=%t", fallback), func(t *testing.T) {
			a, p := progressCompactionFixture(t, 320)
			p.usage = 123
			canonical := a.Session().Snapshot()
			ctx := WithTaskBudget(t.Context(), TaskBudget{Tokens: 123})
			var err error
			if fallback {
				p.finishReason = "length"
				_, err = a.contextManager().Prepare(ctx, ContextPreparePolicy{Trigger: CompactionTriggerManual, AllowChunkedFallback: true})
			} else {
				_, err = a.chunkedFoldSummary(ctx, a.modelVisibleMessages(), "", nil)
			}
			var pause *taskBudgetPause
			if !errors.As(err, &pause) || pause.axis != "token" {
				t.Fatalf("chunked budget stop=%v", err)
			}
			if len(p.requests) != 1 || a.task.budget.promptTokens != 123 || a.task.budget.requests != 1 {
				t.Fatalf("chunked continued past budget: calls=%d budget=%+v", len(p.requests), a.task.budget)
			}
			if a.currentProjectionVersion() != 0 || !reflect.DeepEqual(canonical, a.Session().Snapshot()) {
				t.Fatal("budget pause installed a partial summary or truncation")
			}
		})
	}
}

func TestSamplingOverflowPreservesCompactionBudgetPause(t *testing.T) {
	for _, source := range []string{"interceptor", "provider"} {
		t.Run(source, func(t *testing.T) {
			a, p := progressCompactionFixture(t, 40)
			p.usage = 123
			canonical := a.Session().Snapshot()
			if a.ContextUsedTokens() >= a.compactTrigger() {
				t.Fatal("fixture must skip the initial pressure fold")
			}
			ctx := WithTaskBudget(t.Context(), TaskBudget{Tokens: 123})
			var err error
			if source == "interceptor" {
				client := &fakeDispatchClient{interceptFn: func(ev protocol.InterceptEvent, _ json.RawMessage) (protocol.InterceptResult, error) {
					if ev == protocol.EventContextPrepare {
						return replaceWith(t, dispatch.ContextPayload{Messages: []protocol.ProviderMessage{{Role: protocol.ProviderRoleUser, Content: strings.Repeat("inflate ", 50_000)}}}), nil
					}
					return protocol.InterceptResult{Decision: protocol.DecisionContinue}, nil
				}}
				a.SetExtensions(newExtDispatcher(client, true, nil, extension.PointContextPrepare))
				var frozen samplingRequest
				frozen, err = a.prepareSamplingRequest(ctx)
				if !errors.Is(err, ErrCompactionRequired) || len(frozen.req.Messages) != 0 {
					t.Fatalf("unsafe sampling admission: err=%v messages=%d", err, len(frozen.req.Messages))
				}
			} else {
				p.overflowFirst = true
				err = a.streamWithSamplingRecovery(ctx, 1).err
			}
			var pause *taskBudgetPause
			if !errors.As(err, &pause) || pause.axis != "token" {
				t.Fatalf("overflow swallowed compaction budget pause: %T %v", err, err)
			}
			if len(p.requests) != 1 || a.task.budget.promptTokens != 123 || !reflect.DeepEqual(canonical, a.Session().Snapshot()) {
				t.Fatalf("overflow continued after pause: summary calls=%d spent=%d", len(p.requests), a.task.budget.promptTokens)
			}
		})
	}
}

func TestCompactionAdmissionKeepsTheHardCeilingAboveTheSoftTrigger(t *testing.T) {
	for _, trigger := range []string{CompactionTriggerPressure, CompactionTriggerOverflow, CompactionTriggerManual} {
		policy := ContextPreparePolicy{Trigger: trigger}
		if foldLanded(policy, 9800, 9900, 9744) {
			t.Fatalf("%s admitted a below-trigger view over the hard ceiling", trigger)
		}
		if maxSummariesFor(policy, true) != 4 {
			t.Fatalf("%s over-ceiling recovery lost its bounded batch allowance", trigger)
		}
	}
	if maxSummariesFor(ContextPreparePolicy{Trigger: CompactionTriggerPressure}, false) != 2 ||
		maxSummariesFor(ContextPreparePolicy{Trigger: CompactionTriggerManual}, false) != 1 {
		t.Fatal("ordinary maintenance gained extra summary batches")
	}
}
