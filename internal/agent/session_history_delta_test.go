package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"reasonix/internal/provider"
)

func TestSessionHistoryDeltaAppendAndMarkers(t *testing.T) {
	s := NewSession("system")
	initial, cursor, reset := s.HistoryDelta(SessionHistoryCursor{})
	if !reset || len(initial) != 1 || cursor.MessageCount() != 1 {
		t.Fatal("initial projection must receive the whole transcript")
	}
	initial[0].Content = "reader changed value"
	if s.Snapshot()[0].Content != "system" {
		t.Fatal("delta shares its message values with the session")
	}
	if delta, next, reset := s.HistoryDelta(cursor); reset || len(delta) != 0 || next != cursor {
		t.Fatal("unchanged transcript should reuse its cursor without copying history")
	}
	if allocations := testing.AllocsPerRun(100, func() { s.HistoryDelta(cursor) }); allocations != 0 {
		t.Fatalf("unchanged history allocated %g times; expected no transcript copy", allocations)
	}
	s.head.dag = true // initialized before concurrent use
	s.QueueTurnBegin("turn", false)
	s.QueueTurnEnd("turn")
	if delta, next, reset := s.HistoryDelta(cursor); reset || len(delta) != 0 || next.MessageCount() != 1 {
		t.Fatal("turn markers must not manufacture transcript changes")
	}
	s.AddBatch(provider.Message{ID: "question", Role: provider.RoleUser, Content: "q"}, provider.Message{ID: "answer", Role: provider.RoleAssistant, Content: "a"})
	delta, cursor, reset := s.HistoryDelta(cursor)
	if reset || len(delta) != 2 || delta[0].ID != "question" || delta[1].ID != "answer" || cursor.MessageCount() != 3 {
		t.Fatal("append should copy only the atomic suffix")
	}
}

func TestSessionHistoryDeltaFencesSameLengthAndGrowingReplacement(t *testing.T) {
	for _, grow := range []bool{false, true} {
		t.Run(fmt.Sprintf("grow=%t", grow), func(t *testing.T) {
			s := NewSession("")
			s.Add(provider.Message{ID: "same", Role: provider.RoleUser, Content: "old"})
			_, cursor, _ := s.HistoryDelta(SessionHistoryCursor{})
			changed := []provider.Message{{ID: "same", Role: provider.RoleUser, Content: "edited"}}
			if grow {
				changed = append(changed, provider.Message{ID: "new", Role: provider.RoleAssistant, Content: "new"})
			}
			s.Replace(changed)
			delta, _, reset := s.HistoryDelta(cursor)
			if !reset || !reflect.DeepEqual(delta, s.Snapshot()) || s.RewriteVersion() != 0 {
				t.Fatal("Replace must invalidate the cached prefix without changing persistence rewrite classification")
			}
		})
	}
}

func TestSessionHistoryDeltaFencesLocalMetadataAndGuard(t *testing.T) {
	for _, mutation := range []string{"preview", "resolution", "receipt", "intent", "guard", "rewrite", "local_metadata", "system"} {
		t.Run(mutation, func(t *testing.T) {
			s := NewSession("system")
			s.Add(provider.Message{ID: "assistant", Role: provider.RoleAssistant, Content: "a", ToolCalls: []provider.ToolCall{{ID: "call", Name: "read", Diff: "old"}}, RequestUsage: &provider.Usage{PromptTokens: 20, CompletionTokens: 2}})
			s.Add(provider.Message{ID: "result", Role: provider.RoleTool, Name: "read", ToolCallID: "call", Content: "old result"})
			before, cursor, _ := s.HistoryDelta(SessionHistoryCursor{})
			switch mutation {
			case "preview":
				s.UpdateToolCallPreview(provider.ToolCall{ID: "call", Diff: "new", Added: 1})
			case "resolution":
				readOnly := true
				s.UpdateToolCallResolution(provider.ToolCall{ID: "call", ResolvedName: "read_file", ResolvedReadOnly: &readOnly})
			case "receipt":
				s.AddDecisionReceipt(&provider.DecisionReceipt{ID: "approval", Kind: "tool", Outcome: "allowed"})
			case "intent":
				s.addWriteIntent("call", json.RawMessage(`{"path":"local"}`))
			case "guard":
				s.updateBatchGuardResults([]provider.ToolCall{{ID: "call", Name: "read"}}, []string{"guarded result"})
				if s.RewriteVersion() != 0 {
					t.Fatal("unpersisted guard must keep its existing save classification")
				}
			case "rewrite", "local_metadata":
				changed := s.Snapshot()
				changed[1].Edited = true
				changed[1].RequestUsage = &provider.Usage{PromptTokens: 30, CompletionTokens: 3}
				if mutation == "rewrite" {
					s.Rewrite(changed, "edit")
				} else {
					s.ReplaceLocalMetadata(changed)
				}
			case "system":
				s.SetLeadingSystemPrompt("changed system")
			}
			delta, next, reset := s.HistoryDelta(cursor)
			if !reset || !reflect.DeepEqual(delta, s.Snapshot()) || next.MessageCount() != 3 {
				t.Fatal("an existing-prefix change escaped the history fence")
			}
			if before[1].ToolCalls[0].Diff != "old" || before[1].ToolCalls[0].ResolvedName != "" || len(before[1].ToolCalls[0].WriteIntents) != 0 || before[1].RequestUsage.PromptTokens != 20 {
				t.Fatal("published nested records changed after a session mutation")
			}
		})
	}
}

func TestSessionHistoryDeltaFencesRecoveryRewriteDuringAppend(t *testing.T) {
	s := NewSession("")
	const pending = `{"version":1,"id":"incident","state":"pending","prefix":1,"fingerprint":"owned"}`
	s.Add(provider.Message{ID: "recovery", Role: provider.RoleTool, LocalOnly: true, ProtocolRecovery: json.RawMessage(pending)})
	before, cursor, _ := s.HistoryDelta(SessionHistoryCursor{})
	s.Add(provider.Message{ID: "question", Role: provider.RoleUser, Content: "new turn"})
	delta, _, reset := s.HistoryDelta(cursor)
	if !reset || len(delta) != 2 || string(delta[0].ProtocolRecovery) == string(before[0].ProtocolRecovery) {
		t.Fatal("append that expires a prior recovery must rebuild the cached prefix")
	}
	if string(before[0].ProtocolRecovery) != pending {
		t.Fatal("recovery rewrite changed a published byte slice")
	}
}

func TestSessionHistoryDeltaFencesRecoveryConsumption(t *testing.T) {
	for _, recovery := range []string{"readiness", "protocol"} {
		t.Run(recovery, func(t *testing.T) {
			s := NewSession("")
			s.Add(provider.Message{ID: "recovery", Role: provider.RoleTool, LocalOnly: true,
				FinalReadinessRecovery: &provider.FinalReadinessRecovery{Pending: true, Missing: []string{"deliverable"}},
				ProtocolRecovery:       json.RawMessage(`{"version":1,"id":"incident","state":"pending","prefix":1,"fingerprint":"owned"}`),
			})
			before, cursor, _ := s.HistoryDelta(SessionHistoryCursor{})
			if recovery == "readiness" {
				if !s.ConsumeFinalReadinessRecovery() {
					t.Fatal("fixture did not consume pending recovery")
				}
			} else {
				s.storeProtocolRecord("incident", json.RawMessage(`{"state":"consumed"}`))
			}
			if delta, _, reset := s.HistoryDelta(cursor); !reset || !reflect.DeepEqual(delta, s.Snapshot()) {
				t.Fatal("recovery consumption retained a stale prefix")
			}
			if !before[0].FinalReadinessRecovery.Pending || string(before[0].ProtocolRecovery) != `{"version":1,"id":"incident","state":"pending","prefix":1,"fingerprint":"owned"}` {
				t.Fatal("recovery consumption mutated already published nested records")
			}
		})
	}
}

func TestSessionHistoryDeltaFencesSaveIDChanges(t *testing.T) {
	s := &Session{Messages: []provider.Message{{Role: provider.RoleUser, Content: "legacy"}}}
	_, cursor, _ := s.HistoryDelta(SessionHistoryCursor{})
	s.ensureMessageIDsForSave(s.Snapshot())
	delta, cursor, reset := s.HistoryDelta(cursor)
	if !reset || len(delta) != 1 || delta[0].ID == "" {
		t.Fatal("save-time ID mint must invalidate fork token inputs")
	}
	plan := dagWritePlan{renames: map[string]string{delta[0].ID: "fresh"}}
	plan.applyIDRenames(s)
	delta, _, reset = s.HistoryDelta(cursor)
	if !reset || len(delta) != 1 || delta[0].ID != "fresh" {
		t.Fatal("save-time ID rename must invalidate fork token inputs")
	}
	if s.TranscriptVersion() != 0 || s.RewriteVersion() != 0 {
		t.Fatal("display fencing changed save-time transcript counters")
	}
}

func TestSessionHistoryDeltaFencesSessionReplacement(t *testing.T) {
	s := NewSession("same")
	_, cursor, _ := s.HistoryDelta(SessionHistoryCursor{})
	replacement := s.CloneWithMessages(s.Snapshot())
	if replacement.TranscriptVersion() != s.TranscriptVersion() || replacement.RewriteVersion() != s.RewriteVersion() {
		t.Fatal("fixture needs equal counters")
	}
	if delta, _, reset := replacement.HistoryDelta(cursor); !reset || len(delta) != 1 {
		t.Fatal("same-sized replacement with equal versions reused another session's cursor")
	}
	_, replacementCursor, _ := replacement.HistoryDelta(SessionHistoryCursor{})
	if cursor.identity == nil || cursor.identity == replacementCursor.identity || reflect.TypeOf(sessionHistoryIdentity{}).Size() == 0 {
		t.Fatal("replacement sessions need distinct nonzero-sized identity tokens")
	}
	cursorType := reflect.TypeOf(cursor)
	for i := 0; i < cursorType.NumField(); i++ {
		if cursorType.Field(i).Type == reflect.TypeOf((*Session)(nil)) {
			t.Fatal("a retained cursor must not keep the private Session transcript alive")
		}
	}
	var absent *Session
	if delta, next, reset := absent.HistoryDelta(cursor); !reset || len(delta) != 0 || next != (SessionHistoryCursor{}) {
		t.Fatal("absent executor retained old history")
	}
}

func TestSessionHistoryDeltaConcurrentPrefixChanges(t *testing.T) {
	s := NewSession("")
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			id := fmt.Sprintf("call-%d", i)
			s.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: id, Name: "read"}}})
			s.UpdateToolCallPreview(provider.ToolCall{ID: id, Diff: fmt.Sprintf("diff-%d", i)})
			s.Add(provider.Message{Role: provider.RoleTool, Name: "read", ToolCallID: id, Content: "old"})
			s.updateBatchGuardResults([]provider.ToolCall{{ID: id, Name: "read"}}, []string{"guarded"})
		}
	}()
	var cursor SessionHistoryCursor
	var materialized []provider.Message
	read := func() {
		delta, next, reset := s.HistoryDelta(cursor)
		if reset {
			materialized = delta
		} else {
			materialized = append(materialized, delta...)
		}
		cursor = next
		if len(materialized) != cursor.MessageCount() {
			t.Fatal("cursor count and delta came from different session versions")
		}
		// Hash readers traverse nested records after the lock is released.
		if _, err := json.Marshal(materialized); err != nil {
			t.Fatal(err)
		}
	}
	for {
		read()
		select {
		case <-done:
			read()
			if !reflect.DeepEqual(materialized, s.Snapshot()) {
				t.Fatal("incremental materialization differs from the final transcript")
			}
			return
		default:
		}
	}
}
