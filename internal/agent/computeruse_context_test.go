package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/provider"
)

func desktopHistory(count int) []provider.Message {
	msgs := []provider.Message{{Role: provider.RoleSystem, Content: "Keep user confirmation before sending."}, {Role: provider.RoleUser, Content: "Fill this draft, wait before sending.", Images: []string{"user-attachment"}}}
	for i := range count {
		id := fmt.Sprintf("desktop-%d", i)
		body := fmt.Sprintf(`{"state":{"pid":42,"window_id":11,"focus":{"value":"draft"},"elements":[{"value":%q}]},"observation":%q,"dispatched":false,"state_changed":true,"blocked_reason":"visual_changed","dispatch_unknown":false}`, strings.Repeat("AX desktop detail ", 500), id)
		msgs = append(msgs, provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: id, Name: "use_capability", Arguments: `{"action":"call","capability_id":"tool:computer_use"}`}}}, provider.Message{Role: provider.RoleTool, Name: "use_capability", ToolCallID: id, Content: body, Images: []string{"screenshot-" + id}})
	}
	return msgs
}

func TestComputerUseContextKeepsLatestFramesAndInputOutcomes(t *testing.T) {
	msgs := desktopHistory(60)
	stored, _ := json.Marshal(msgs)
	got := modelInputMessages(msgs)
	currentImages := 0
	for i, m := range got {
		if m.Role != provider.RoleTool {
			if !reflect.DeepEqual(m, msgs[i]) {
				t.Fatal("changed user instructions, attachments or assistant tool pairing")
			}
			continue
		}
		currentImages += len(m.Images)
		body, _ := computerUseResult(m.Content)
		if string(body["dispatched"]) != "false" || string(body["blocked_reason"]) != `"visual_changed"` || m.ToolCallID != msgs[i].ToolCallID {
			t.Fatal("lost the outcome or tool-call pairing")
		}
		if i < len(got)-4 && (len(m.Images) != 0 || body["observation"] != nil || string(body["superseded_observation"]) != "true") {
			t.Fatal("historical desktop observation remained actionable")
		}
	}
	if currentImages != 2 || estimateMessagesTokens(got)*4 >= estimateMessagesTokens(msgs) {
		t.Fatalf("desktop context not bounded: images=%d tokens=%d -> %d", currentImages, estimateMessagesTokens(msgs), estimateMessagesTokens(got))
	}
	after, _ := json.Marshal(msgs)
	if string(after) != string(stored) {
		t.Fatal("request projection mutated durable history")
	}
	if !reflect.DeepEqual(got[len(got)-4:], msgs[len(msgs)-4:]) {
		t.Fatal("rewrote the latest two complete observations")
	}
}

func TestComputerUseContextLeavesUnrelatedAndUnknownResults(t *testing.T) {
	msgs := desktopHistory(5)
	msgs = append(msgs, provider.Message{Role: provider.RoleTool, Name: "other", Content: msgs[3].Content, Images: []string{"unrelated-image"}}, provider.Message{Role: provider.RoleTool, Name: "computer_use", Content: `{"dispatched":null,"state":{"window_id":11}}`, Images: []string{"unknown-image"}})
	got := modelInputMessages(msgs)
	if !reflect.DeepEqual(got[len(got)-2:], msgs[len(msgs)-2:]) {
		t.Fatal("rewrote unrecognized tool outcomes")
	}
	msgs[3].Content = strings.Replace(msgs[3].Content, `"dispatch_unknown":false`, `"dispatch_unknown":true`, 1)
	got = modelInputMessages(msgs)
	body, _ := computerUseResult(got[3].Content)
	if string(body["dispatch_unknown"]) != "true" {
		t.Fatal("historical uncertain dispatch disappeared")
	}
}

func TestComputerUseContextKeepsUnverifiedClickEvidenceWhenImagesAreSuperseded(t *testing.T) {
	msgs := desktopHistory(4)
	body, _ := computerUseResult(msgs[3].Content)
	evidence := json.RawMessage(`{"input_delivery":"posted","target_verification":"window_only","application_effect":"unverified"}`)
	body["click_evidence"] = evidence
	body["cursor"] = json.RawMessage(`{"clicked":true,"screen_x":-1000,"screen_y":200}`)
	body["dispatched"] = json.RawMessage(`true`)
	encoded, _ := json.Marshal(body)
	msgs[3].Content = string(encoded)
	got := computerUseContext(msgs)
	projected, _ := computerUseResult(got[3].Content)
	if len(got[3].Images) != 0 || string(projected["click_evidence"]) != string(evidence) || !strings.Contains(string(projected["cursor"]), `"clicked":true`) {
		t.Fatal("old screenshot removal lost the distinction between cursor feedback and page effect")
	}
}

// Opt-in local replay reports only sizes/counts, never transcript contents.
func TestComputerUseContextLocalReplay(t *testing.T) {
	path := os.Getenv("REASONIX_COMPUTER_USE_REPLAY")
	if path == "" {
		t.Skip("set REASONIX_COMPUTER_USE_REPLAY to a local session JSONL")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var messages []provider.Message
	for {
		var message provider.Message
		if err := decoder.Decode(&message); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, message)
	}
	before := provider.ModelMessages(messages)
	after := modelInputMessages(messages)
	images := func(msgs []provider.Message) int {
		n := 0
		for _, m := range msgs {
			n += len(m.Images)
		}
		return n
	}
	t.Logf("local replay: messages=%d estimated tokens=%d -> %d; images=%d -> %d", len(messages), estimateMessagesTokens(before), estimateMessagesTokens(after), images(before), images(after))
}

func TestComputerUseContextBoundsDOMSnapshotsWithoutLosingLatestTabRefs(t *testing.T) {
	msgs := desktopHistory(5)
	for i, m := range msgs {
		if m.Role != provider.RoleTool {
			continue
		}
		body, _ := computerUseResult(m.Content)
		body["state"] = json.RawMessage(`{"pid":42,"window_id":11,"mode":"snapshot","target_id":"b1","tab_id":"t1","outline":"old DOM outline","refs":[{"ref":"p1:e1","name":"field"}]}`)
		body["summary"] = json.RawMessage(`"Cua window observation; inspect state and image."`)
		encoded, _ := json.Marshal(body)
		msgs[i].Content = string(encoded)
	}
	got := computerUseContext(msgs)
	old, _ := computerUseResult(got[3].Content)
	if strings.Contains(string(old["state"]), "old DOM outline") || strings.Contains(string(old["state"]), "p1:e1") {
		t.Fatal("old DOM refs remained actionable")
	}
	latest, _ := computerUseResult(got[len(got)-1].Content)
	if !strings.Contains(string(latest["state"]), `"tab_id":"t1"`) || !strings.Contains(string(latest["state"]), "p1:e1") {
		t.Fatal("latest browser binding/refs were removed")
	}
	if !strings.Contains(msgs[3].Content, "old DOM outline") {
		t.Fatal("stored DOM evidence was mutated")
	}
}
