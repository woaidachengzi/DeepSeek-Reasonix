package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrTurnSubmitChanged    = errors.New("remote session changed; refresh before sending")
	ErrSubmitOutcomeUnknown = errors.New("remote send outcome is unknown; refresh the selected session, do not automatically retry")
)

type SessionSubmitScope struct {
	SessionPath  string `json:"sessionPath"`
	RuntimeEpoch string `json:"runtimeEpoch"`
	Revision     uint64 `json:"revision"`
}

// SessionSubmitRequest is the bridge's flat user-message wire DTO. Endpoint,
// cookie and lifetime remain exclusively owned by the saved remote connection.
type SessionSubmitRequest struct {
	SessionPath  string `json:"sessionPath"`
	RuntimeEpoch string `json:"runtimeEpoch"`
	Revision     uint64 `json:"revision"`
	Text         string `json:"text"`
}

func (r SessionSubmitRequest) Scope() SessionSubmitScope {
	return SessionSubmitScope{r.SessionPath, r.RuntimeEpoch, r.Revision}
}

// Accepted is admission only, not a fabricated completed turn or save receipt.
type SessionSubmitReceipt struct {
	ProtocolVersion int    `json:"protocolVersion"`
	SessionPath     string `json:"sessionPath"`
	RuntimeEpoch    string `json:"runtimeEpoch"`
	Revision        uint64 `json:"revision"`
	Accepted        bool   `json:"accepted"`
}

func (r SessionSubmitReceipt) Scope() SessionSubmitScope {
	return SessionSubmitScope{r.SessionPath, r.RuntimeEpoch, r.Revision}
}

// SubmitSessionTurn dispatches exactly one explicit user message. Catalogue
// membership is only an input whitelist; Serve checks actual runtime authority.
// No /submit fallback, restore, steer, queue, model switch or automatic retry.
func (c *Client) SubmitSessionTurn(operation context.Context, scope SessionSubmitScope, text string) (SessionSubmitReceipt, error) {
	if operation == nil {
		return SessionSubmitReceipt{}, ErrClient
	}
	if scope.SessionPath == "" || !cleanField(scope.SessionPath, 32768) || scope.RuntimeEpoch == "" || !cleanField(scope.RuntimeEpoch, 4096) || scope.Revision == 0 || scope.Revision > 9_007_199_254_740_991 ||
		strings.TrimSpace(text) == "" || len(text) > 512<<10 || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return SessionSubmitReceipt{}, ErrResponse
	}
	bounded, cancel := context.WithTimeout(operation, 20*time.Second)
	defer cancel()
	rows, err := c.Sessions(bounded)
	if err != nil {
		return SessionSubmitReceipt{}, err
	}
	listed := false
	for _, row := range rows {
		if row.Path == scope.SessionPath {
			listed = true
			break
		}
	}
	if !listed {
		return SessionSubmitReceipt{}, ErrSessionNotListed
	}
	ctx, finish, err := c.callContext(bounded)
	if err != nil {
		return SessionSubmitReceipt{}, err
	}
	defer finish()
	data, err := json.Marshal(struct {
		ProtocolVersion int `json:"protocolVersion"`
		SessionSubmitScope
		Text string `json:"text"`
	}{1, scope, text})
	if err != nil || len(data) > 1<<20 {
		return SessionSubmitReceipt{}, ErrResponse
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/desktop/session-submit", bytes.NewReader(data))
	if err != nil {
		return SessionSubmitReceipt{}, ErrResponse
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return SessionSubmitReceipt{}, errors.Join(ErrSubmitOutcomeUnknown, c.viewReadError(ctx))
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		c.Close()
		return SessionSubmitReceipt{}, ErrClosed
	}
	if resp.StatusCode == 409 || resp.StatusCode == 404 {
		return SessionSubmitReceipt{}, ErrTurnSubmitChanged
	}
	if resp.StatusCode != 200 {
		return SessionSubmitReceipt{}, ErrSubmitOutcomeUnknown
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, (40<<10)+1))
	var receipt SessionSubmitReceipt
	if err != nil || len(data) > 40<<10 || !utf8.Valid(data) || json.Unmarshal(data, &receipt) != nil || receipt.ProtocolVersion != 1 || receipt.Scope() != scope || !receipt.Accepted {
		return SessionSubmitReceipt{}, ErrSubmitOutcomeUnknown
	}
	if c.Closed() || ctx.Err() != nil {
		return SessionSubmitReceipt{}, errors.Join(ErrSubmitOutcomeUnknown, c.viewReadError(ctx))
	}
	return receipt, nil
}
