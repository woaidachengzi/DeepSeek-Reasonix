package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
	"unicode/utf8"

	"reasonix/internal/eventwire"
)

type SessionPendingScope struct {
	SessionPath  string `json:"sessionPath"`
	RuntimeEpoch string `json:"runtimeEpoch"`
}
type SessionPendingPrompt struct {
	Scope          SessionPromptScope        `json:"scope"`
	Ask            *eventwire.Ask            `json:"ask,omitempty"`
	Approval       *eventwire.Approval       `json:"approval,omitempty"`
	MCPInteraction *eventwire.MCPInteraction `json:"mcpInteraction,omitempty"`
}
type SessionPendingView struct {
	ProtocolVersion int `json:"protocolVersion"`
	SessionPendingScope
	Revision uint64                 `json:"revision"`
	TurnID   string                 `json:"turnId"`
	Prompts  []SessionPendingPrompt `json:"prompts"`
}

// Bound record allocation before materializing nested question/form structs.
// A byte-bounded body alone could still contain many tiny prompt objects.
func (v *SessionPendingView) UnmarshalJSON(data []byte) error {
	type viewWire SessionPendingView
	var result SessionPendingView
	wire := struct {
		*viewWire
		Prompts json.RawMessage `json:"prompts"`
	}{viewWire: (*viewWire)(&result)}
	if json.Unmarshal(data, &wire) != nil {
		return ErrResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(wire.Prompts))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		return ErrResponse
	}
	result.Prompts = []SessionPendingPrompt{}
	for decoder.More() {
		if len(result.Prompts) >= 32 {
			return ErrResponse
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil || len(raw) > 64<<10 {
			return ErrResponse
		}
		var prompt SessionPendingPrompt
		if json.Unmarshal(raw, &prompt) != nil {
			return ErrResponse
		}
		result.Prompts = append(result.Prompts, prompt)
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim(']') {
		return ErrResponse
	}
	*v = result
	return nil
}

// ReadSessionPending observes exactly one already-owned instance; it does not
// replay prompts, refresh routing, resume, decide, or fall back to ID-only APIs.
func (c *Client) ReadSessionPending(operation context.Context, scope SessionPendingScope) (SessionPendingView, error) {
	if operation == nil {
		return SessionPendingView{}, ErrClient
	}
	if !ValidSessionPendingScope(scope) {
		return SessionPendingView{}, ErrResponse
	}
	bounded, cancel := context.WithTimeout(operation, 15*time.Second)
	defer cancel()
	ctx, finish, err := c.callContext(bounded)
	if err != nil {
		return SessionPendingView{}, err
	}
	defer finish()
	body, _ := json.Marshal(struct {
		ProtocolVersion int `json:"protocolVersion"`
		SessionPendingScope
	}{1, scope})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/desktop/session-pending", bytes.NewReader(body))
	if err != nil {
		return SessionPendingView{}, ErrResponse
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return SessionPendingView{}, c.viewReadError(ctx)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		c.Close()
		return SessionPendingView{}, ErrClosed
	}
	if resp.StatusCode == 404 || resp.StatusCode == 409 {
		return SessionPendingView{}, ErrPromptChanged
	}
	if resp.StatusCode != 200 {
		return SessionPendingView{}, ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil {
		return SessionPendingView{}, c.viewReadError(ctx)
	}
	var result SessionPendingView
	if len(data) > 2<<20 || !utf8.Valid(data) || json.Unmarshal(data, &result) != nil || !ValidSessionPendingView(result, scope) {
		return SessionPendingView{}, ErrResponse
	}
	if ctx.Err() != nil {
		return SessionPendingView{}, c.viewReadError(ctx)
	}
	return result, nil
}

func ValidSessionPendingScope(scope SessionPendingScope) bool {
	return scope.SessionPath != "" && cleanField(scope.SessionPath, 32768) && scope.RuntimeEpoch != "" && cleanField(scope.RuntimeEpoch, 4096)
}

func ValidSessionPendingView(view SessionPendingView, expected SessionPendingScope) bool {
	if view.ProtocolVersion != 1 || view.SessionPendingScope != expected || !ValidSessionPendingScope(expected) || view.Revision == 0 || view.Revision > 9_007_199_254_740_991 || !cleanField(view.TurnID, 4096) || view.Prompts == nil || len(view.Prompts) > 32 {
		return false
	}
	seen := make(map[string]bool)
	for _, prompt := range view.Prompts {
		s := prompt.Scope
		if s.SessionPath != expected.SessionPath || s.RuntimeEpoch != expected.RuntimeEpoch || s.TurnID == "" || s.TurnID != view.TurnID || !cleanField(s.TurnID, 4096) || s.PromptID == "" || !cleanField(s.PromptID, 4096) || !cleanField(s.PromptRuntimeEpoch, 4096) || seen[s.PromptID] {
			return false
		}
		seen[s.PromptID] = true
		raw, err := json.Marshal(prompt)
		if err != nil || len(raw) > 64<<10 {
			return false
		}
		switch s.Kind {
		case "ask":
			if prompt.Ask == nil || prompt.Approval != nil || prompt.MCPInteraction != nil || prompt.Ask.ID != s.PromptID || prompt.Ask.TurnID != s.TurnID {
				return false
			}
		case "approval", "plan", "recovery":
			if prompt.Approval == nil || prompt.Ask != nil || prompt.MCPInteraction != nil || prompt.Approval.ID != s.PromptID || prompt.Approval.TurnID != s.TurnID {
				return false
			}
			kind := "approval"
			if prompt.Approval.Kind == "plan" || prompt.Approval.Kind == "recovery" {
				kind = prompt.Approval.Kind
			}
			if kind != s.Kind {
				return false
			}
		case "mcp":
			if prompt.MCPInteraction == nil || prompt.Ask != nil || prompt.Approval != nil || prompt.MCPInteraction.ID != s.PromptID || prompt.MCPInteraction.TurnID != s.TurnID {
				return false
			}
		default:
			return false
		}
	}
	return true
}
