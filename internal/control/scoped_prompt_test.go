package control

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/mcpinteraction"
)

func scopedPromptIdentity(c *Controller, id PromptIdentity) PromptResolveScope {
	return PromptResolveScope{SessionPath: c.SessionPath(), RuntimeEpoch: c.RuntimeStateSnapshot().RuntimeEpoch, TurnID: id.TurnID}
}

func TestResolvePromptScopedUsesExistingDecisionAndDurableTransition(t *testing.T) {
	for _, kind := range []PromptKind{PromptAsk, PromptApproval, PromptMCP} {
		t.Run(string(kind), func(t *testing.T) {
			requests := make(chan event.Event, 2)
			answered := make(chan event.Event, 2)
			done := make(chan event.Event, 2)
			c := scopedCancelController(t, event.FuncSink(func(e event.Event) {
				switch e.Kind {
				case event.AskRequest, event.ApprovalRequest, event.MCPInteractionRequest:
					requests <- e
				case event.PromptAnswered:
					answered <- e
				case event.TurnDone:
					done <- e
				}
			}))
			// Controller epoch is always present; ordinary Serve prompt routing
			// remains empty. A desktop routing epoch must also work independently.
			if kind == PromptMCP {
				c.SetTurnEventRoutingMetadata("desktop-routing-epoch", "")
			}
			result := make(chan bool, 1)
			c.runGuarded(func(ctx context.Context) error {
				switch kind {
				case PromptAsk:
					answers, err := c.Ask(ctx, askProbeQuestions())
					result <- len(answers) == 1 && answers[0].Selected[0] == "A"
					return err
				case PromptApproval:
					allow, _, err := c.requestApprovalWithReason(ctx, "bash", "echo fixture", nil, "fixture")
					result <- allow
					return err
				default:
					answer, err := c.Interact(ctx, mcpinteraction.Request{Server: "fixture", Mode: "form", Message: "Confirm?"})
					result <- answer.Action == mcpinteraction.ActionAccept
					return err
				}
			})
			awaitPromptLedgerTest(t, requests, "actual prompt request")
			identities := c.PendingPromptIdentities()
			if len(identities) != 1 || identities[0].Kind != kind {
				t.Fatalf("unexpected identity: %+v", identities)
			}
			identity := identities[0]
			scope := scopedPromptIdentity(c, identity)
			payload, err := c.ReadPromptScopedContext(context.Background(), scope, identity)
			if err != nil || !json.Valid(payload) {
				t.Fatal("private scoped prompt snapshot unavailable", err)
			}
			payload[0] = '!'
			second, err := c.ReadPromptScopedContext(context.Background(), scope, identity)
			if err != nil || !json.Valid(second) {
				t.Fatal("snapshot aliases consumer", err)
			}
			if len(requests) != 0 || len(answered) != 0 || len(c.PendingPromptIdentities()) != 1 || c.PendingPromptIdentities()[0] != identity {
				t.Fatal("snapshot emitted or rebound prompt ownership")
			}
			answer := PromptAnswer{Allow: true, Questions: []event.AskAnswer{{QuestionID: "q1", Selected: []string{"A"}}}, Action: mcpinteraction.ActionAccept}
			wrongPath, wrongEpoch, wrongTurn := scope, scope, scope
			wrongPath.SessionPath += ".other"
			wrongEpoch.RuntimeEpoch += "-other"
			wrongTurn.TurnID += "-other"
			for _, wrong := range []PromptResolveScope{{}, wrongPath, wrongEpoch, wrongTurn} {
				if _, err := c.ReadPromptScopedContext(context.Background(), wrong, identity); !errors.Is(err, ErrPromptResolveScope) {
					t.Fatal("wrong snapshot scope accepted", err)
				}
				if err := c.ResolvePromptScopedContext(context.Background(), wrong, identity, answer); !errors.Is(err, ErrPromptResolveScope) {
					t.Fatalf("wrong scope: %v", err)
				}
			}
			stale := identity
			stale.RuntimeEpoch += "-other"
			if err := c.ResolvePromptScopedContext(context.Background(), scope, stale, answer); !errors.Is(err, ErrPromptStaleRuntime) {
				t.Fatalf("wrong routing epoch: %v", err)
			}
			select {
			case <-result:
				t.Fatal("rejected identity woke prompt")
			default:
			}
			request, revoke := context.WithCancel(context.Background())
			if err := c.ResolvePromptScopedContext(request, scope, identity, answer); err != nil {
				t.Fatal(err)
			}
			revoke()
			if !awaitPromptLedgerTest(t, result, "specialized decision result") {
				t.Fatal("existing resolver did not deliver captured answer")
			}
			transition := awaitPromptLedgerTest(t, answered, "durable prompt transition")
			if transition.TurnID != scope.TurnID || transition.ItemID != identity.PromptID {
				t.Fatalf("wrong decision transition: %+v", transition)
			}
			if err := c.ResolvePromptScopedContext(context.Background(), scope, identity, answer); err == nil {
				t.Fatal("duplicate decision accepted")
			}
			waitTurnDoneEvent(t, done)
		})
	}
}

func TestResolvePromptScopedRevocationAfterWaitingDoesNotDispatch(t *testing.T) {
	started := make(chan struct{}, 1)
	c := scopedCancelController(t, nil)
	c.runGuarded(func(ctx context.Context) error { started <- struct{}{}; <-ctx.Done(); return ctx.Err() })
	awaitPromptLedgerTest(t, started, "live turn")
	cancelScope := scopedCancelIdentity(c)
	scope := PromptResolveScope{cancelScope.SessionPath, cancelScope.RuntimeEpoch, cancelScope.TurnID}
	identity := PromptIdentity{PromptID: "fixture-prompt", TurnID: scope.TurnID, Kind: PromptPlan}
	calls := 0
	if err := c.promptOwner.RegisterPrompt(PendingPrompt{Identity: identity, Resolve: func(PromptAnswer) error { calls++; c.promptOwner.Remove(identity.PromptID); return nil }}); err != nil {
		t.Fatal(err)
	}
	request, revoke := context.WithCancel(context.Background())
	checked := &scopedSubmitCheckedContext{Context: request, checked: make(chan struct{})}
	returned := make(chan error, 1)
	c.promptResolveMu.Lock()
	go func() { returned <- c.ResolvePromptScopedContext(checked, scope, identity, PromptAnswer{}) }()
	awaitPromptLedgerTest(t, checked.checked, "initial context check")
	revoke()
	c.promptResolveMu.Unlock()
	if err := awaitPromptLedgerTest(t, returned, "revoked resolution"); !errors.Is(err, ErrPromptResolveScope) || calls != 0 {
		t.Fatalf("revoked decision dispatched: %v / %d", err, calls)
	}
	if _, ok := c.promptOwner.Identity(identity.PromptID); !ok {
		t.Fatal("revoked request removed pending decision")
	}
	c.turnEvents.mu.Lock()
	c.turnEvents.err = errors.New("fixture failed ledger")
	c.turnEvents.mu.Unlock()
	err := c.ResolvePromptScopedContext(context.Background(), scope, identity, PromptAnswer{})
	c.turnEvents.mu.Lock()
	c.turnEvents.err = nil
	c.turnEvents.mu.Unlock()
	if !errors.Is(err, ErrPromptResolveScope) || calls != 0 {
		t.Fatal("failed ledger admitted decision")
	}
	if err := c.CancelScoped(cancelScope); err != nil {
		t.Fatal(err)
	}
}

func TestResolvePromptScopedRejectsNilAndIncompleteIdentity(t *testing.T) {
	var c *Controller
	if err := c.ResolvePromptScopedContext(context.Background(), PromptResolveScope{}, PromptIdentity{}, PromptAnswer{}); !errors.Is(err, ErrPromptResolveScope) {
		t.Fatal(err)
	}
	if err := c.ResolvePromptExact(PromptIdentity{}, PromptAnswer{}); !errors.Is(err, ErrPromptNotPending) {
		t.Fatal(err)
	}
}
