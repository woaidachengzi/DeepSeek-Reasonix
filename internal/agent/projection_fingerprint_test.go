package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"reasonix/internal/provider"
)

func legacyProviderFingerprint(msgs []provider.Message) string {
	type wireCall struct {
		ID               string `json:"id,omitempty"`
		Name             string `json:"name,omitempty"`
		Arguments        string `json:"args,omitempty"`
		ThoughtSignature string `json:"ts,omitempty"`
	}
	type wireMsg struct {
		Role               string                      `json:"r"`
		Content            string                      `json:"c,omitempty"`
		Images             []string                    `json:"img,omitempty"`
		ReasoningContent   string                      `json:"rc,omitempty"`
		ReasoningID        string                      `json:"rid,omitempty"`
		ReasoningStatus    string                      `json:"rst,omitempty"`
		ReasoningSignature string                      `json:"rsig,omitempty"`
		ToolCallID         string                      `json:"tid,omitempty"`
		Name               string                      `json:"n,omitempty"`
		ToolCalls          []wireCall                  `json:"tc,omitempty"`
		ThinkingBlocks     []provider.ThinkingBlock    `json:"tb,omitempty"`
		ResponsesItems     []json.RawMessage           `json:"ri,omitempty"`
		ServerSearch       []provider.ServerSearchCall `json:"ss,omitempty"`
	}
	wire := make([]wireMsg, 0, len(msgs))
	for _, m := range msgs {
		wm := wireMsg{
			Role:               string(m.Role),
			Content:            m.Content,
			Images:             append([]string(nil), m.Images...),
			ReasoningContent:   m.ReasoningContent,
			ReasoningID:        m.ReasoningID,
			ReasoningStatus:    m.ReasoningStatus,
			ReasoningSignature: m.ReasoningSignature,
			ThinkingBlocks:     m.ThinkingBlocks,
			ToolCallID:         m.ToolCallID,
			Name:               m.Name,
		}
		for _, tc := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireCall{
				ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments, ThoughtSignature: tc.ThoughtSignature,
			})
		}
		if len(m.ResponsesItems) > 0 {
			wm.ResponsesItems = make([]json.RawMessage, len(m.ResponsesItems))
			for i, item := range m.ResponsesItems {
				wm.ResponsesItems[i] = append(json.RawMessage(nil), item...)
			}
		}
		if len(m.ServerSearch) > 0 {
			wm.ServerSearch = append([]provider.ServerSearchCall(nil), m.ServerSearch...)
		}
		wire = append(wire, wm)
	}
	b, err := json.Marshal(wire)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:16])
}

func projectionFingerprintFixture(n int) []provider.Message {
	out := make([]provider.Message, 0, n)
	for i := 0; i < n; i++ {
		msg := provider.Message{Role: provider.RoleAssistant, Content: strings.Repeat("synthetic 中文<&> ", 32)}
		if i%4 == 0 {
			msg.ToolCalls = []provider.ToolCall{{ID: fmt.Sprint(i), Name: "read_file", Arguments: `{"path":"synthetic"}`}}
		}
		if i%8 == 0 {
			msg.ResponsesItems = []json.RawMessage{json.RawMessage(`{"type":"message", "text":"<fixture>"}`)}
		}
		out = append(out, msg)
	}
	return out
}

func TestProviderFingerprintLegacyParity(t *testing.T) {
	cases := [][]provider.Message{nil, {}, projectionFingerprintFixture(1), projectionFingerprintFixture(256)}
	cases = append(cases, []provider.Message{{Role: provider.RoleUser, Content: "\xff\xfe\n<&>\u2028", Images: []string{"data:image/png;base64,synthetic"}, ReasoningContent: "reasoning", ReasoningID: "r", ReasoningStatus: "done", ReasoningSignature: "signed"}})
	cases = append(cases, []provider.Message{{Role: provider.RoleAssistant, ResponsesItems: []json.RawMessage{json.RawMessage(`{broken`)}}})
	for i, msgs := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if got, want := providerVisibleFingerprint(msgs), legacyProviderFingerprint(msgs); got != want {
				t.Fatalf("fingerprint mismatch: %q != %q", got, want)
			}
		})
	}
}

func BenchmarkProjectionFingerprint(b *testing.B) {
	for _, n := range []int{200, 10000, 50000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			msgs := projectionFingerprintFixture(n)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if providerVisibleFingerprint(msgs) == "" {
					b.Fatal("empty fingerprint")
				}
			}
		})
	}
}

func BenchmarkCoveredProjectionHash(b *testing.B) {
	msgs := projectionFingerprintFixture(10000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if coveredPrefixHash(msgs, len(msgs)) == "" {
			b.Fatal("empty fingerprint")
		}
	}
}

func TestProviderFingerprintRichFieldsAndReadOnly(t *testing.T) {
	msg := provider.Message{
		Role: provider.RoleAssistant, Content: "body\n<&>\u2028", ReasoningContent: "private proof", ReasoningID: "rid", ReasoningStatus: "done", ReasoningSignature: "signed",
		ToolCallID: "tid", Name: "reader", Images: []string{"synthetic image"},
		ToolCalls:      []provider.ToolCall{{ID: "a", Name: "read_file", Arguments: `{"path":"a"}`, ThoughtSignature: "proof", Diff: "local diff"}},
		ThinkingBlocks: []provider.ThinkingBlock{{Type: "thinking", Thinking: "r", Signature: "s"}, {Type: "redacted_thinking", Data: "proof"}},
		ResponsesItems: []json.RawMessage{nil, json.RawMessage(` { "type": "message", "text": "<x>" } `)},
		ServerSearch:   []provider.ServerSearchCall{{ID: "search", Query: "中文", Results: []provider.ServerSearchHit{{Title: "fixture", URL: "https://example.invalid"}}, Raw: json.RawMessage(`{"encrypted_content":"synthetic"}`)}},
	}
	msgs := []provider.Message{msg, {Role: provider.RoleUser, Content: "next"}}
	before, _ := json.Marshal(msgs)
	want := legacyProviderFingerprint(msgs)
	if got := providerVisibleFingerprint(msgs); got != want || got == "" {
		t.Fatalf("rich field parity: %q != %q", got, want)
	}
	after, _ := json.Marshal(msgs)
	if string(before) != string(after) {
		t.Fatal("fingerprint mutated source records")
	}
	msgs[0].ResponsesItems[1] = json.RawMessage(`{broken`)
	if providerVisibleFingerprint(msgs) != "" {
		t.Fatal("malformed replay JSON did not fail closed")
	}
}

func FuzzProviderFingerprintLegacyParity(f *testing.F) {
	f.Add(`[ {"role":"assistant","content":"<x>中文","responses_items":[{"type":"x"}]} ]`)
	f.Add(`[]`)
	f.Fuzz(func(t *testing.T, raw string) {
		var msgs []provider.Message
		if json.Unmarshal([]byte(raw), &msgs) != nil {
			return
		}
		if got, want := providerVisibleFingerprint(msgs), legacyProviderFingerprint(msgs); got != want {
			t.Fatalf("fingerprint parity: %q != %q", got, want)
		}
	})
}
