package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"

	"reasonix/internal/provider"
)

type fingerprintCall struct {
	ID               string `json:"id,omitempty"`
	Name             string `json:"name,omitempty"`
	Arguments        string `json:"args,omitempty"`
	ThoughtSignature string `json:"ts,omitempty"`
}
type fingerprintMessage struct {
	Role               string                      `json:"r"`
	Content            string                      `json:"c,omitempty"`
	Images             []string                    `json:"img,omitempty"`
	ReasoningContent   string                      `json:"rc,omitempty"`
	ReasoningID        string                      `json:"rid,omitempty"`
	ReasoningStatus    string                      `json:"rst,omitempty"`
	ReasoningSignature string                      `json:"rsig,omitempty"`
	ToolCallID         string                      `json:"tid,omitempty"`
	Name               string                      `json:"n,omitempty"`
	ToolCalls          []fingerprintCall           `json:"tc,omitempty"`
	ThinkingBlocks     []provider.ThinkingBlock    `json:"tb,omitempty"`
	ResponsesItems     []json.RawMessage           `json:"ri,omitempty"`
	ServerSearch       []provider.ServerSearchCall `json:"ss,omitempty"`
}

// fingerprintJSONWriter omits only the final newline added by Encoder.Encode.
// Holding one byte also handles an encoder writing a value in multiple chunks.
type fingerprintJSONWriter struct {
	hash    hash.Hash
	last    [1]byte
	pending bool
}

func (w *fingerprintJSONWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.pending {
		_, _ = w.hash.Write(w.last[:])
	}
	_, _ = w.hash.Write(p[:len(p)-1])
	w.last[0] = p[len(p)-1]
	w.pending = true
	return len(p), nil
}

// providerVisibleFingerprint retains the legacy JSON-array bytes without a
// transcript-sized wire copy or JSON buffer. Borrowed fields are read only.
func providerVisibleFingerprint(msgs []provider.Message) string {
	sum := sha256.New()
	writer := fingerprintJSONWriter{hash: sum}
	encoder := json.NewEncoder(&writer)
	_, _ = sum.Write([]byte{'['})
	var wm fingerprintMessage
	var calls []fingerprintCall
	for i := range msgs {
		m := &msgs[i]
		calls = calls[:0]
		for _, tc := range m.ToolCalls {
			calls = append(calls, fingerprintCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments, ThoughtSignature: tc.ThoughtSignature})
		}
		wm = fingerprintMessage{
			Role: string(m.Role), Content: m.Content, Images: m.Images,
			ReasoningContent: m.ReasoningContent, ReasoningID: m.ReasoningID,
			ReasoningStatus: m.ReasoningStatus, ReasoningSignature: m.ReasoningSignature,
			ToolCallID: m.ToolCallID, Name: m.Name, ToolCalls: calls,
			ThinkingBlocks: m.ThinkingBlocks, ResponsesItems: m.ResponsesItems, ServerSearch: m.ServerSearch,
		}
		if i > 0 {
			_, _ = sum.Write([]byte{','})
		}
		if err := encoder.Encode(&wm); err != nil {
			return ""
		}
		if !writer.pending || writer.last[0] != '\n' {
			return ""
		}
		writer.pending = false
	}
	_, _ = sum.Write([]byte{']'})
	return hex.EncodeToString(sum.Sum(nil)[:16])
}
