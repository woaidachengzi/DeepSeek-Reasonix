package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"reasonix/internal/provider"
)

func TestSessionHistoryReaderFixedPrefixAndBufferReuse(t *testing.T) {
	s := NewSession("")
	s.head.dag = true
	for i := 0; i < 85; i++ {
		s.Add(provider.Message{ID: fmt.Sprintf("message-%d", i), Role: provider.RoleUser, Content: fmt.Sprintf("content-%d", i)})
	}
	want := s.Snapshot()
	reader := s.HistoryReader()
	target, start, reset := reader.Begin(SessionHistoryCursor{})
	if !reset || start != 0 || target.MessageCount() != len(want) {
		t.Fatal("initial read did not fix the full transcript prefix")
	}
	scratch := make([]provider.Message, 0, 32)
	var got []provider.Message
	for start < target.MessageCount() {
		chunk, valid := reader.Chunk(target, start, scratch)
		if !valid || len(chunk) == 0 || len(chunk) > cap(scratch) || &chunk[0] != &scratch[:cap(scratch)][0] {
			t.Fatal("chunk must reuse the caller's bounded buffer")
		}
		got = append(got, chunk...)
		start += len(chunk)
		if start == 32 {
			s.Add(provider.Message{ID: "later", Role: provider.RoleAssistant, Content: "outside the fixed target"})
			s.QueueTurnBegin("later-turn", false)
			s.QueueTurnEnd("later-turn")
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("append or turn markers changed the fixed read prefix")
	}
	next, start, reset := reader.Begin(target)
	if reset || start != len(want) || next.MessageCount() != len(want)+1 {
		t.Fatal("the next read must see only the previously excluded append")
	}
	chunk, valid := reader.Chunk(next, start, scratch)
	if !valid || len(chunk) != 1 || chunk[0].ID != "later" {
		t.Fatal("later append was lost or included in the earlier target")
	}
	chunk[0].Content = "reader-local change"
	if s.Snapshot()[len(want)].Content == chunk[0].Content {
		t.Fatal("chunk values alias canonical session messages")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		reader.Begin(next)
		reader.Chunk(next, 0, scratch)
	}); allocations != 0 {
		t.Fatalf("warmed bounded reads allocated %g times", allocations)
	}
}

func TestSessionHistoryReaderInvalidatesChangedPrefixes(t *testing.T) {
	for _, mutation := range []string{"rewrite", "replace", "truncate", "local_metadata", "guard", "system", "preview", "id_mint", "id_rename", "recovery_expire"} {
		t.Run(mutation, func(t *testing.T) {
			s := NewSession("system")
			s.Add(provider.Message{ID: "assistant", Role: provider.RoleAssistant, Content: "answer", ToolCalls: []provider.ToolCall{{ID: "call", Name: "read", Diff: "before"}}})
			s.Add(provider.Message{ID: "tool", Role: provider.RoleTool, Name: "read", ToolCallID: "call", Content: "before"})
			if mutation == "id_mint" {
				s.Messages[1].ID = "" // legacy fixture, before publishing a cursor
			}
			if mutation == "recovery_expire" {
				s.Add(provider.Message{Role: provider.RoleTool, LocalOnly: true, ProtocolRecovery: json.RawMessage(`{"version":1,"id":"incident","state":"pending","prefix":1,"fingerprint":"owned"}`)})
			}
			reader := s.HistoryReader()
			target, _, _ := reader.Begin(SessionHistoryCursor{})
			var scratch [1]provider.Message
			if chunk, valid := reader.Chunk(target, 0, scratch[:]); !valid || len(chunk) != 1 {
				t.Fatal("first chunk unavailable before mutation")
			}
			switch mutation {
			case "rewrite", "replace", "truncate", "local_metadata":
				messages := s.Snapshot()
				messages[1].Content = "edited"
				switch mutation {
				case "rewrite":
					s.Rewrite(messages, "edit")
				case "replace":
					s.Replace(messages)
				case "truncate":
					s.Replace(messages[:1])
				case "local_metadata":
					s.ReplaceLocalMetadata(messages)
				}
			case "guard":
				s.updateBatchGuardResults([]provider.ToolCall{{ID: "call", Name: "read"}}, []string{"guarded"})
			case "system":
				s.SetLeadingSystemPrompt("new system")
			case "preview":
				s.UpdateToolCallPreview(provider.ToolCall{ID: "call", Diff: "after"})
			case "id_mint":
				s.ensureMessageIDsForSave(s.Snapshot())
			case "id_rename":
				plan := dagWritePlan{renames: map[string]string{"assistant": "fresh"}}
				plan.applyIDRenames(s)
			case "recovery_expire":
				s.Add(provider.Message{Role: provider.RoleUser, Content: "new authored turn"})
			}
			if chunk, valid := reader.Chunk(target, 1, scratch[:]); valid || len(chunk) != 0 {
				t.Fatal("changed prefix returned a chunk from a mixed version")
			}
			if messages, _, reset := reader.Delta(target); !reset || !reflect.DeepEqual(messages, s.Snapshot()) {
				t.Fatal("atomic-copy fallback did not rebuild the invalidated target")
			}
		})
	}
}

func TestSessionHistoryReaderPinsSourceAndRejectsForeignTarget(t *testing.T) {
	oldSession := NewSession("old session")
	reader := oldSession.HistoryReader()
	target, _, _ := reader.Begin(SessionHistoryCursor{})
	replacement := NewSession("new session")
	replacementReader := replacement.HistoryReader()
	var scratch [2]provider.Message
	if chunk, valid := reader.Chunk(target, 0, scratch[:]); !valid || len(chunk) != 1 || chunk[0].Content != "old session" {
		t.Fatal("request-local reader lost its captured source session")
	}
	if chunk, valid := replacementReader.Chunk(target, 0, scratch[:]); valid || len(chunk) != 0 {
		t.Fatal("replacement accepted another session's target")
	}
	newTarget, start, reset := replacementReader.Begin(target)
	if !reset || start != 0 {
		t.Fatal("replacement reused the previous session's prefix")
	}
	if chunk, valid := replacementReader.Chunk(newTarget, start, scratch[:]); !valid || len(chunk) != 1 || chunk[0].Content != "new session" {
		t.Fatal("replacement target did not describe the new session")
	}
}

func TestSessionHistoryReaderEmptyAndInvalidRanges(t *testing.T) {
	var absent *Session
	target, start, reset := absent.HistoryReader().Begin(SessionHistoryCursor{})
	if !reset || start != 0 || target != (SessionHistoryCursor{}) {
		t.Fatal("absent session did not reset its history")
	}
	if chunk, valid := absent.HistoryReadChunk(target, 0, nil); !valid || len(chunk) != 0 {
		t.Fatal("absent session should provide a coherent empty read")
	}
	s := NewSession("system")
	target, _, _ = s.HistoryReadBegin(SessionHistoryCursor{})
	if chunk, valid := s.HistoryReadChunk(target, 0, nil); valid || len(chunk) != 0 {
		t.Fatal("empty scratch must not report progress for remaining messages")
	}
	var scratch [1]provider.Message
	for _, invalidStart := range []int{-1, 2} {
		if chunk, valid := s.HistoryReadChunk(target, invalidStart, scratch[:]); valid || len(chunk) != 0 {
			t.Fatal("out-of-range chunk start was silently clamped")
		}
	}
	if chunk, valid := s.HistoryReadChunk(target, target.MessageCount(), nil); !valid || len(chunk) != 0 {
		t.Fatal("completed target should report an empty valid read")
	}
	if chunk, valid := absent.HistoryReadChunk(target, 0, scratch[:]); valid || len(chunk) != 0 {
		t.Fatal("absent session accepted a nonempty source target")
	}
}

func TestSessionHistoryReaderUnlocksBeforeProjectionAndPreservesNestedRecords(t *testing.T) {
	s := NewSession("")
	s.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "call", Diff: "before"}}})
	reader := s.HistoryReader()
	target, _, _ := reader.Begin(SessionHistoryCursor{})
	var scratch [1]provider.Message
	chunk, valid := reader.Chunk(target, 0, scratch[:])
	if !valid || len(chunk) != 1 {
		t.Fatal("initial chunk unavailable")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.UpdateToolCallPreview(provider.ToolCall{ID: "call", Diff: "after"})
	}()
	if _, err := json.Marshal(chunk); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("chunk retained the session lock during caller projection")
	}
	if chunk[0].ToolCalls[0].Diff != "before" {
		t.Fatal("writer changed already published nested records")
	}
	if chunk, valid := reader.Chunk(target, target.MessageCount(), scratch[:]); valid || len(chunk) != 0 {
		t.Fatal("end-of-read query accepted an invalidated prefix")
	}
}
