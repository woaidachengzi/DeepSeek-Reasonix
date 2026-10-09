package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
	"unicode/utf8"
)

var (
	ErrPromptChanged        = errors.New("remote prompt changed; refresh before answering")
	ErrPromptOutcomeUnknown = errors.New("remote decision outcome is unknown; refresh the selected session and do not automatically retry")
)

// ResolveSessionPrompt performs one explicit scoped decision, never a legacy
// /approve fallback, restore, foreground switch, or retry on uncertain outcome.
func (c *Client) ResolveSessionPrompt(operation context.Context, input SessionPromptRequest) (SessionPromptReceipt, error) {
	if operation == nil {
		return SessionPromptReceipt{}, ErrClient
	}
	input.Answer = append(json.RawMessage(nil), input.Answer...)
	if _, err := DecodeSessionPromptAnswer(input); err != nil {
		return SessionPromptReceipt{}, err
	}
	bounded, cancel := context.WithTimeout(operation, 20*time.Second)
	defer cancel()
	rows, err := c.Sessions(bounded)
	if err != nil {
		return SessionPromptReceipt{}, err
	}
	listed := false
	for _, row := range rows {
		if row.Path == input.SessionPath {
			listed = true
			break
		}
	}
	if !listed {
		return SessionPromptReceipt{}, ErrSessionNotListed
	}
	ctx, finish, err := c.callContext(bounded)
	if err != nil {
		return SessionPromptReceipt{}, err
	}
	defer finish()
	data, err := json.Marshal(struct {
		ProtocolVersion int `json:"protocolVersion"`
		SessionPromptRequest
	}{1, input})
	if err != nil || len(data) > 256<<10 {
		return SessionPromptReceipt{}, ErrResponse
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/desktop/session-prompt", bytes.NewReader(data))
	if err != nil {
		return SessionPromptReceipt{}, ErrResponse
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return SessionPromptReceipt{}, errors.Join(ErrPromptOutcomeUnknown, c.viewReadError(ctx))
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		c.Close()
		return SessionPromptReceipt{}, ErrClosed
	}
	if resp.StatusCode == 404 || resp.StatusCode == 409 {
		return SessionPromptReceipt{}, ErrPromptChanged
	}
	if resp.StatusCode != 200 {
		return SessionPromptReceipt{}, ErrPromptOutcomeUnknown
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	var receipt SessionPromptReceipt
	if err != nil || len(data) > 64<<10 || !utf8.Valid(data) || json.Unmarshal(data, &receipt) != nil || receipt.ProtocolVersion != 1 || receipt.SessionPromptScope != input.SessionPromptScope || !receipt.Resolved {
		return SessionPromptReceipt{}, ErrPromptOutcomeUnknown
	}
	if c.Closed() || ctx.Err() != nil {
		return SessionPromptReceipt{}, errors.Join(ErrPromptOutcomeUnknown, c.viewReadError(ctx))
	}
	return receipt, nil
}
