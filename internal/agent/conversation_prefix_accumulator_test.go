package agent

import (
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"testing"

	"reasonix/internal/provider"
	"reasonix/internal/sessioncontext"
)

func TestConversationPrefixAccumulatorZeroAndBatchedParity(t *testing.T) {
	context := sessioncontext.Build(sessioncontext.Sections{Environment: "darwin/arm64", Workspace: "Current workspace: /tmp/prefix-parity"})
	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: "system", ReadResult: json.RawMessage(`{`)},
		{Role: provider.RoleUser, Content: "question 界😀"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "call", Name: "echo", Arguments: `{"text":"value"}`}}},
		{Role: provider.RoleTool, Content: "private result", ToolCallID: "call"},
		{Role: provider.RoleUser, Origin: provider.MessageOriginHost, Content: "<pinned_context_revision>host revision</pinned_context_revision>", ReadResult: json.RawMessage(`{`)},
		{Role: provider.RoleUser, Content: context.Content, ReadResult: json.RawMessage(`{`)},
		{Role: provider.RoleAssistant, Content: "answer"},
		{Role: provider.RoleUser, Origin: provider.MessageOriginHost, Content: "ordinary hidden host input still participates"},
		{Role: provider.RoleUser, Origin: provider.MessageOriginUser, Content: "<pinned_context_revision>user quotation participates</pinned_context_revision>"},
		{Role: provider.RoleAssistant, Content: "next answer", RequestUsage: &provider.Usage{TotalTokens: 7}},
	}
	want, err := ConversationPrefixTokens(messages)
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range []int{1, 2, len(messages)} {
		t.Run("chunk-"+strconv.Itoa(chunk), func(t *testing.T) {
			var prefix ConversationPrefixAccumulator
			if prefix.Token() != "" {
				t.Fatal("zero accumulator fabricated an empty-transcript token")
			}
			last := ""
			for start := 0; start < len(messages); start += chunk {
				for index := start; index < min(start+chunk, len(messages)); index++ {
					included, err := prefix.Append(messages[index])
					if err != nil {
						t.Fatalf("append %d: %v", index, err)
					}
					if included != (want[index] != "") {
						t.Fatalf("append %d inclusion = %v, original token = %q", index, included, want[index])
					}
					if included {
						last = want[index]
						if decoded, err := hex.DecodeString(last); err != nil || len(decoded) != 32 {
							t.Fatalf("invalid opaque SHA-256 token %q", last)
						}
					}
					if got := prefix.Token(); got != last {
						t.Fatalf("append %d token = %q, want exact original prefix %q", index, got, last)
					}
				}
			}
		})
	}
}

func TestConversationPrefixAccumulatorHashesEveryMessageField(t *testing.T) {
	// Canonical local metadata intentionally participates in fork identity too.
	// The reflection check makes a new Message field require an explicit fixture
	// decision rather than silently narrowing this compatibility test.
	const encoded = `{
		"role":"assistant","id":"message-id","origin":"user","content":"answer",
		"raw_content":"raw question","provider_content":"legacy provider text","images":["data:image/png;base64,eA=="],
		"reasoning_content":"private reasoning","reasoning_id":"reasoning-id","reasoning_status":"completed",
		"reasoning_signature":"proof","reasoning_state":"complete","thinking_blocks":[{"type":"thinking","thinking":"private","signature":"proof"}],
		"tool_calls":[{"id":"call","name":"echo","arguments":"{}","write_intents":[{"version":1}]}],
		"responses_items":[{"type":"reasoning","id":"response-id"}],
		"server_search":[{"id":"search-id","query":"query","raw":{"encrypted_content":"private"}}],
		"tool_call_id":"call","name":"echo","memoryCitations":[{"source":"memory","note":"note"}],
		"workDurationMs":123,"createdAt":456,"requestUsage":{"PromptTokens":5,"CompletionTokens":2,"TotalTokens":7},
		"edited":true,"original":"original prompt","local_only":true,
		"decision_receipt":{"id":"decision","kind":"approval","outcome":"allowed"},
		"decision_receipts":[{"id":"decision-list","kind":"approval","outcome":"denied"}],
		"interrupted_turn":{"terminalStatus":"interrupted","pending":true},
		"final_readiness_recovery":{"pending":true,"checkpoint":{"version":1}},
		"protocol_recovery":{"version":1},"read_pause":{"id":"pause","reads":[{"readId":"read","path":"/tmp/file","reason":"paused"}]},
		"tool_execution":{"kind":"shell","state":"completed"},"tool_run_state":"completed",
		"read_result":{"protocol_version":1},"tool_diagnostic":{"private":"diagnostic"},
		"mcp_app":{"server":"server","tool":"tool","generation":1,"rawResult":{"private":"raw"},"structured":{"value":1}},
		"vision_summary":{"version":1,"summary":"image summary"}
	}`
	var message provider.Message
	if err := json.Unmarshal([]byte(encoded), &message); err != nil {
		t.Fatal(err)
	}
	value := reflect.ValueOf(message)
	typeOf := value.Type()
	base := accumulatorTokenForTest(t, []provider.Message{message})
	for i := range value.NumField() {
		field := typeOf.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			if value.Field(i).IsZero() {
				t.Fatalf("fixture does not populate provider.Message.%s", field.Name)
			}
			changed := message
			reflect.ValueOf(&changed).Elem().Field(i).SetZero()
			if got := accumulatorTokenForTest(t, []provider.Message{changed}); got == base {
				t.Fatalf("provider.Message.%s did not affect canonical identity", field.Name)
			}
		})
	}
}

func accumulatorTokenForTest(t *testing.T, messages []provider.Message) string {
	t.Helper()
	want, err := ConversationPrefixTokens(messages)
	if err != nil {
		t.Fatal(err)
	}
	var prefix ConversationPrefixAccumulator
	last := ""
	for index, message := range messages {
		included, err := prefix.Append(message)
		if err != nil {
			t.Fatal(err)
		}
		if included {
			last = want[index]
		}
		if prefix.Token() != last {
			t.Fatalf("token %d = %q, original = %q", index, prefix.Token(), last)
		}
	}
	return last
}

func TestConversationPrefixAccumulatorSerializationErrorStickyAndReset(t *testing.T) {
	badJSON := json.RawMessage(`{`)
	cases := map[string]provider.Message{
		"responses item":       {ResponsesItems: []json.RawMessage{badJSON}},
		"server search":        {ServerSearch: []provider.ServerSearchCall{{Raw: badJSON}}},
		"write intent":         {ToolCalls: []provider.ToolCall{{WriteIntents: []json.RawMessage{badJSON}}}},
		"readiness checkpoint": {FinalReadinessRecovery: &provider.FinalReadinessRecovery{Checkpoint: badJSON}},
		"protocol recovery":    {ProtocolRecovery: badJSON},
		"read result":          {ReadResult: badJSON},
		"tool diagnostic":      {ToolDiagnostic: badJSON},
		"mcp raw result":       {MCPApp: &provider.MCPAppPresentation{RawResult: badJSON}},
		"mcp structured":       {MCPApp: &provider.MCPAppPresentation{Structured: badJSON}},
		"non-finite usage":     {RequestUsage: &provider.Usage{CacheWriteBilledTokens: math.Inf(1)}},
	}
	first := provider.Message{Role: provider.RoleUser, Content: "question"}
	last := provider.Message{Role: provider.RoleAssistant, Content: "answer"}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			bad.Role = provider.RoleTool
			if _, err := ConversationPrefixTokens([]provider.Message{first, bad, last}); err == nil {
				t.Fatal("malformed fixture did not fail original hashing")
			}
			var prefix ConversationPrefixAccumulator
			if _, err := prefix.Append(first); err != nil {
				t.Fatal(err)
			}
			if included, err := prefix.Append(bad); included || err == nil || prefix.Token() != "" {
				t.Fatalf("serialization failure did not invalidate prefix: included=%v, error=%v, token=%q", included, err, prefix.Token())
			}
			for _, following := range []provider.Message{last, {Role: provider.RoleSystem, Content: "excluded"}} {
				if included, err := prefix.Append(following); included || err == nil || prefix.Token() != "" {
					t.Fatal("a later append recovered an invalid prefix without reset")
				}
			}
			prefix = ConversationPrefixAccumulator{}
			for _, message := range []provider.Message{first, last} {
				if _, err := prefix.Append(message); err != nil {
					t.Fatal(err)
				}
			}
			if got, want := prefix.Token(), accumulatorTokenForTest(t, []provider.Message{first, last}); got != want || got == "" {
				t.Fatalf("zero reset token = %q, want %q", got, want)
			}
		})
	}
}
