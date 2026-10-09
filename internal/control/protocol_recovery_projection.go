package control

import (
	"bytes"
	"encoding/json"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// Only the engine's checked pending->consumed checkpoint may advance this
// fence. Reconstructing the exact old prefix catches simultaneous edits and
// leaves arbitrary same-ID rewrites fail-closed. No visible history is rebased.
func (c *Controller) acceptProtocolRecoveryRewrite(change *event.ProtocolRecoveryRewrite) error {
	if change == nil {
		return nil
	}
	c.turnEvents.mu.Lock()
	defer c.turnEvents.mu.Unlock()
	ledger := c.turnEvents.ledger
	if ledger == nil || ledger.ActiveTurnID() == "" {
		return nil
	}
	if c.turnEvents.err != nil {
		return c.turnEvents.err
	}
	prefix := c.turnEvents.projectionPrefix
	if prefix == nil {
		// The optional display snapshot exceeded its admission budget. Keep it
		// unavailable rather than turning a display limit into an engine failure.
		return nil
	}
	if change.MessageID == "" || !validProtocolRecoveryConsumption(change.Previous, change.Current) {
		return ErrTurnProjectionChanged
	}
	history := c.History()
	if ledger.ActiveTurnID() != c.turnEvents.projectionTurnID || len(history) < len(prefix) {
		return ErrTurnProjectionChanged
	}
	index, matches := -1, 0
	for i, message := range history {
		if message.ID == change.MessageID {
			index, matches = i, matches+1
		}
	}
	if matches != 1 || index < 0 || index >= len(prefix) {
		return ErrTurnProjectionChanged
	}
	message := history[index]
	if !message.LocalOnly || message.Role != provider.RoleTool || message.Name != provider.LocalOnlyToolName || message.ToolCallID != provider.LocalOnlyToolID || !bytes.Equal(message.ProtocolRecovery, change.Current) {
		return ErrTurnProjectionChanged
	}
	for i, id := range prefix {
		if history[i].ID != id {
			return ErrTurnProjectionChanged
		}
	}
	currentDigest, err := agent.ContentDigestForMessages(history[:len(prefix)])
	if err != nil {
		return err
	}
	history[index].ProtocolRecovery = change.Previous
	previousDigest, err := agent.ContentDigestForMessages(history[:len(prefix)])
	if err != nil || previousDigest != c.turnEvents.projectionPrefixDigest {
		return ErrTurnProjectionChanged
	}
	c.turnEvents.projectionPrefixDigest = currentDigest
	return nil
}

func validProtocolRecoveryConsumption(previous, current json.RawMessage) bool {
	before, ok := provider.DecodeProtocolRecovery(previous)
	if !ok || before.State != "pending" || before.Projected {
		return false
	}
	after, ok := provider.DecodeProtocolRecovery(current)
	if !ok || after.State != "consumed" || !after.Projected || before.ID == "" || before.ID != after.ID {
		return false
	}
	var oldFields, newFields map[string]json.RawMessage
	if json.Unmarshal(previous, &oldFields) != nil || json.Unmarshal(current, &newFields) != nil {
		return false
	}
	// Preserve every known and unknown incident field; exactly these two
	// fields can change. Canonical JSON comparison is independent of key order.
	delete(oldFields, "state")
	delete(newFields, "state")
	delete(oldFields, "projected")
	delete(newFields, "projected")
	oldJSON, oldErr := json.Marshal(oldFields)
	newJSON, newErr := json.Marshal(newFields)
	return oldErr == nil && newErr == nil && bytes.Equal(oldJSON, newJSON)
}
