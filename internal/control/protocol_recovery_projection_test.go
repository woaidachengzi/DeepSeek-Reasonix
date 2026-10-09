package control

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

func TestProtocolRecoveryProjectionRejectsUnattestedChanges(t *testing.T) {
	isolateControlConfigHome(t)
	previous := json.RawMessage(`{"version":1,"id":"incident","state":"pending","scope":"owned","fingerprint":"owned","prefix":1,"count":2,"anchor":"owned","run":1,"future":"preserved"}`)
	current := json.RawMessage(`{"version":1,"id":"incident","state":"consumed","projected":true,"scope":"owned","fingerprint":"owned","prefix":1,"count":2,"anchor":"owned","run":1,"future":"preserved"}`)
	cases := []struct {
		name   string
		mutate func(*event.ProtocolRecoveryRewrite, []provider.Message) []provider.Message
	}{
		{"wrong identity", func(e *event.ProtocolRecoveryRewrite, rows []provider.Message) []provider.Message {
			e.MessageID = "foreign"
			return rows
		}},
		{"different current payload", func(e *event.ProtocolRecoveryRewrite, rows []provider.Message) []provider.Message {
			e.Current = previous
			return rows
		}},
		{"unrelated visible edit", func(_ *event.ProtocolRecoveryRewrite, rows []provider.Message) []provider.Message {
			rows[1].Content = "changed question"
			return rows
		}},
		{"same row visible edit", func(_ *event.ProtocolRecoveryRewrite, rows []provider.Message) []provider.Message {
			rows[2].Content = "unexpected text"
			return rows
		}},
		{"not local only", func(_ *event.ProtocolRecoveryRewrite, rows []provider.Message) []provider.Message {
			rows[2].LocalOnly = false
			return rows
		}},
		{"duplicate identity", func(_ *event.ProtocolRecoveryRewrite, rows []provider.Message) []provider.Message {
			return append(rows, rows[2])
		}},
		{"changed incident fields", func(e *event.ProtocolRecoveryRewrite, rows []provider.Message) []provider.Message {
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(e.Current, &fields)
			fields["scope"] = json.RawMessage(`"foreign"`)
			e.Current, _ = json.Marshal(fields)
			rows[2].ProtocolRecovery = e.Current
			return rows
		}},
		{"changed unknown field", func(e *event.ProtocolRecoveryRewrite, rows []provider.Message) []provider.Message {
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(e.Current, &fields)
			fields["future"] = json.RawMessage(`"changed"`)
			e.Current, _ = json.Marshal(fields)
			rows[2].ProtocolRecovery = e.Current
			return rows
		}},
		{"different old payload", func(e *event.ProtocolRecoveryRewrite, rows []provider.Message) []provider.Message {
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(e.Previous, &fields)
			fields["future"] = json.RawMessage(`"invented"`)
			e.Previous, _ = json.Marshal(fields)
			return rows
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, session := recoveryProjectionFixture(t, previous)
			rows := session.Snapshot()
			rows[2].ProtocolRecovery = current
			change := &event.ProtocolRecoveryRewrite{MessageID: "record", Previous: previous, Current: current}
			session.Replace(tc.mutate(change, rows))
			if err := c.acceptProtocolRecoveryRewrite(change); !errors.Is(err, ErrTurnProjectionChanged) {
				t.Fatalf("unsafe rewrite accepted: %v", err)
			}
			if _, err := c.TurnProjectionView(); !errors.Is(err, ErrTurnProjectionChanged) {
				t.Fatalf("rejected rewrite advanced display fence: %v", err)
			}
		})
	}
	t.Run("exact consumption and replay rejection", func(t *testing.T) {
		c, session := recoveryProjectionFixture(t, previous)
		rows := session.Snapshot()
		rows[2].ProtocolRecovery = current
		session.Replace(rows)
		change := &event.ProtocolRecoveryRewrite{MessageID: "record", Previous: previous, Current: current}
		if err := c.acceptProtocolRecoveryRewrite(change); err != nil {
			t.Fatal(err)
		}
		if _, err := c.TurnProjectionView(); err != nil {
			t.Fatalf("exact hidden consumption failed to recover: %v", err)
		}
		if err := c.acceptProtocolRecoveryRewrite(change); !errors.Is(err, ErrTurnProjectionChanged) {
			t.Fatalf("replayed attestation accepted: %v", err)
		}
	})
	t.Run("unavailable display budget does not reject engine recovery", func(t *testing.T) {
		c, session := recoveryProjectionFixture(t, previous)
		rows := session.Snapshot()
		rows[2].ProtocolRecovery = current
		session.Replace(rows)
		c.turnEvents.mu.Lock()
		c.turnEvents.projectionPrefix = nil
		c.turnEvents.projectionPrefixDigest = ""
		c.turnEvents.mu.Unlock()
		if err := c.acceptProtocolRecoveryRewrite(&event.ProtocolRecoveryRewrite{MessageID: "record", Previous: previous, Current: current}); err != nil {
			t.Fatalf("display limit rejected engine recovery: %v", err)
		}
		if _, err := c.TurnProjectionView(); !errors.Is(err, ErrTurnProjectionChanged) {
			t.Fatalf("display limit silently minted a readable cut: %v", err)
		}
	})
}

func recoveryProjectionFixture(t *testing.T, previous json.RawMessage) (*Controller, *agent.Session) {
	t.Helper()
	session := agent.NewSession("system")
	session.Add(provider.Message{ID: "question", Role: provider.RoleUser, Origin: provider.MessageOriginUser, Content: "actual question"})
	session.Add(provider.Message{ID: "record", Role: provider.RoleTool, Name: provider.LocalOnlyToolName, ToolCallID: provider.LocalOnlyToolID, LocalOnly: true, ProtocolRecovery: previous})
	executor := agent.New(nil, nil, session, agent.Options{}, event.Discard)
	dir := t.TempDir()
	c := New(Options{Executor: executor, SessionDir: dir, SessionPath: filepath.Join(dir, "owned.jsonl"), Sink: event.Discard})
	t.Cleanup(c.Close)
	if err := c.beginProjectionTurn(); err != nil {
		t.Fatal(err)
	}
	return c, session
}
