package control

import (
	"context"
	"reasonix/internal/agent"
	"reasonix/internal/sessioncontext"
	"strings"
)

// ContinueTurn is an explicit local action, not a new visible user message.
// Preserve the last user's raw task as the policy source; the host instruction
// cannot authorize repeating mutations whose effects are unknown.
func (c *Controller) ContinueTurn() bool {
	var raw string
	for _, message := range c.History() {
		if agent.IsUserAuthoredTurnMessage(message) && !agent.IsPinnedContextRevision(message) && !sessioncontext.IsContent(message.Content) {
			raw = message.RawContent
			if strings.TrimSpace(raw) == "" {
				raw = message.Content
			}
		}
	}
	if strings.TrimSpace(raw) == "" {
		return false
	}
	c.revokeDesktopDriving()
	result := c.runGuardedInbox(func(ctx context.Context) error {
		o := newTurnOrchestrator(c)
		token := c.goals.continuationToken()
		err := o.runSyntheticTurnWithRawDisplay(ctx, "Continue the existing task from the conversation history. Preserve completed work. Do not repeat actions with unknown outcomes; inspect their effects first. If the previous answer was complete, continue only as warranted by the user's existing request.", raw, "")
		if err != nil {
			return err
		}
		return o.continueGoal(ctx, token, nil)
	}, nil)
	return result == turnStarted
}
