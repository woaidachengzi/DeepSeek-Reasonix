package agent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func textTurn(text string) []provider.Chunk {
	return []provider.Chunk{{Type: provider.ChunkText, Text: text}, {Type: provider.ChunkDone}}
}

func TestRunRetriesReasoningOnlyFinalAnswer(t *testing.T) {
	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{{Type: provider.ChunkReasoning, Text: "I should answer the user."}, {Type: provider.ChunkDone}},
		textTurn("visible answer"),
	}}
	a := New(prov, tool.NewRegistry(), NewSession(""), Options{}, event.Discard)
	if err := a.Run(t.Context(), "answer me"); err != nil {
		t.Fatal(err)
	}
	if prov.call != 2 || lastAssistantContent(a.sess.conversation) != "visible answer" {
		t.Fatalf("calls=%d history=%+v", prov.call, a.Session().Snapshot())
	}
	if !sessionHasUserMessageContaining(a.sess.conversation, "visible answer") {
		t.Fatal("missing bounded continuation")
	}
}

func TestRunPrefixesReasoningLanguageOnReasoningOnlyCompletion(t *testing.T) {
	prov := &mockProvider{name: "p", streams: [][]provider.Chunk{
		{{Type: provider.ChunkReasoning, Text: "I should answer the user."}, {Type: provider.ChunkDone}},
		textTurn("visible reply"),
	}}
	a := New(prov, tool.NewRegistry(), NewSession(""), Options{ReasoningLanguage: "zh"}, event.Discard)
	if err := a.Run(t.Context(), "answer me"); err != nil {
		t.Fatal(err)
	}
	if len(prov.requests) != 2 {
		t.Fatalf("requests=%d", len(prov.requests))
	}
	for _, req := range prov.requests {
		got := lastUser(req)
		if !strings.HasPrefix(got, "<reasoning-language>") || !strings.Contains(got, "简体中文") {
			t.Fatalf("missing language preference: %q", got)
		}
	}
	if !strings.Contains(lastUser(prov.requests[1]), "visible answer") {
		t.Fatal("missing continuation")
	}
}

func TestRunRequiresVisibleFinalForInternalCallers(t *testing.T) {
	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{{Type: provider.ChunkReasoning, Text: "thinking 1"}, {Type: provider.ChunkDone}},
		{{Type: provider.ChunkReasoning, Text: "thinking 2"}, {Type: provider.ChunkDone}},
		{{Type: provider.ChunkReasoning, Text: "thinking 3"}, {Type: provider.ChunkDone}},
	}}
	a := New(prov, tool.NewRegistry(), NewSession(""), Options{RequireVisibleFinal: true}, event.Discard)

	err := a.Run(context.Background(), "answer me")
	if err == nil {
		t.Fatal("expected repeated empty final answers to stop the run")
	}
	if !strings.Contains(err.Error(), "visible final answer") {
		t.Fatalf("error = %v, want visible final answer", err)
	}
	if prov.call != maxEmptyFinalBlocks {
		t.Fatalf("provider calls = %d, want %d empty-answer attempts", prov.call, maxEmptyFinalBlocks)
	}
}

func TestRunRetriesZeroContentWithTheSameFrozenRequest(t *testing.T) {
	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{{Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "visible reply"}, {Type: provider.ChunkDone}},
	}}
	a := New(prov, tool.NewRegistry(), NewSession(""), Options{}, event.Discard)

	if err := a.Run(context.Background(), "answer me"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if prov.call != 2 {
		t.Fatalf("provider calls = %d, want one empty-response retry", prov.call)
	}
	if len(prov.requests) != 2 || !reflect.DeepEqual(prov.requests[0], prov.requests[1]) {
		t.Fatalf("retry requests differ; want the same frozen request:\nfirst=%#v\nsecond=%#v", prov.requests[0], prov.requests[1])
	}
	if sessionHasUserMessageContaining(a.sess.conversation, "visible answer") {
		t.Fatal("empty-response retry must not inject a synthetic user prompt")
	}
	if got := lastAssistantContent(a.sess.conversation); got != "visible reply" {
		t.Fatalf("last assistant content = %q, want successful retry answer", got)
	}
}

func TestRunStopsAfterExhaustedZeroContentRetriesWithoutCommittingEmptyMessages(t *testing.T) {
	turns := make([][]provider.Chunk, maxSamplingAttempts)
	for i := range turns {
		turns[i] = []provider.Chunk{{Type: provider.ChunkDone}}
	}
	prov := &scriptedProvider{name: "p", turns: turns}
	sink := &recordSink{}
	a := New(prov, tool.NewRegistry(), NewSession(""), Options{}, sink)

	err := a.Run(context.Background(), "answer me")
	if !errors.Is(err, provider.ErrEmptyResponse) {
		t.Fatalf("Run error = %v, want ErrEmptyResponse", err)
	}
	if prov.call != maxSamplingAttempts {
		t.Fatalf("provider calls = %d, want %d bounded attempts", prov.call, maxSamplingAttempts)
	}
	for _, message := range a.sess.conversation.Messages {
		if message.Role == provider.RoleAssistant {
			t.Fatalf("empty attempt committed assistant message: %+v", message)
		}
		if message.Role == provider.RoleUser && strings.Contains(message.Content, "visible answer") {
			t.Fatalf("empty attempt injected synthetic user prompt: %q", message.Content)
		}
	}
	retries := sink.kinds(event.Retrying)
	if len(retries) != maxStreamRecoveries {
		t.Fatalf("retry events = %d, want %d", len(retries), maxStreamRecoveries)
	}
}

func lastAssistantContent(s *Session) string {
	var out string
	for _, m := range s.Messages {
		if m.Role == provider.RoleAssistant {
			out = m.Content
		}
	}
	return out
}

// deepseekThinkingProvider exercises the same no-answer contract for
// providers that require round-tripping reasoning alongside tool calls.
type deepseekThinkingProvider struct{ *scriptedProvider }

func (deepseekThinkingProvider) RequiresToolCallReasoning() bool { return true }

func TestRunRetriesReasoningOnlyStopAcrossProviderPolicies(t *testing.T) {
	for _, thinking := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "thinking"}[thinking], func(t *testing.T) {
			p := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
				{{Type: provider.ChunkReasoning, Text: "reasoning only"}, {Type: provider.ChunkUsage, Usage: &provider.Usage{FinishReason: "stop", TotalTokens: 10}}, {Type: provider.ChunkDone}},
				textTurn("answer"),
			}}
			var prov provider.Provider = p
			if thinking {
				prov = deepseekThinkingProvider{p}
			}
			a := New(prov, tool.NewRegistry(), NewSession(""), Options{}, event.Discard)
			if err := a.Run(t.Context(), "answer me"); err != nil {
				t.Fatal(err)
			}
			if p.call != 2 || lastAssistantContent(a.sess.conversation) != "answer" {
				t.Fatalf("calls=%d history=%+v", p.call, a.Session().Snapshot())
			}
		})
	}
}

func BenchmarkHasVisibleFinalAnswer(b *testing.B) {
	cases := []struct {
		name string
		text string
	}{
		{"normal", "visible reply"},
		{"leading-space", strings.Repeat(" ", 256) + "visible reply"},
		{"all-space", strings.Repeat(" \n\t", 256)},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			var got bool
			for range b.N {
				got = hasVisibleFinalAnswer(tc.text)
			}
			_ = got
		})
	}
}
