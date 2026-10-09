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
	ErrTurnCancelChanged    = errors.New("remote turn changed; refresh before stopping")
	ErrCancelOutcomeUnknown = errors.New("remote stop outcome is unknown; refresh the selected session, do not automatically retry")
)

// These identities come from the selected Serve-owned SessionView runtime.
// Catalogue membership alone is not a writable grant; Serve owns final checks.
type SessionCancelScope struct {
	SessionPath  string `json:"sessionPath"`
	RuntimeEpoch string `json:"runtimeEpoch"`
	TurnID       string `json:"turnId"`
}

type SessionCancelReceipt struct {
	ProtocolVersion int    `json:"protocolVersion"`
	SessionPath     string `json:"sessionPath"`
	RuntimeEpoch    string `json:"runtimeEpoch"`
	TurnID          string `json:"turnId"`
	Cancelled       bool   `json:"cancelled"`
}

func (r SessionCancelReceipt) Scope() SessionCancelScope {
	return SessionCancelScope{r.SessionPath, r.RuntimeEpoch, r.TurnID}
}

// CancelSessionTurn issues one explicit stop, never /cancel fallback, resume,
// takeover or retry. A transport failure after dispatch has unknown outcome.
func (c *Client) CancelSessionTurn(operation context.Context, scope SessionCancelScope) (SessionCancelReceipt, error) {
	if operation == nil {
		return SessionCancelReceipt{}, ErrClient
	}
	if scope.SessionPath == "" || !cleanField(scope.SessionPath, 32768) || scope.RuntimeEpoch == "" || !cleanField(scope.RuntimeEpoch, 4096) || scope.TurnID == "" || !cleanField(scope.TurnID, 4096) {
		return SessionCancelReceipt{}, ErrResponse
	}
	bounded, cancel := context.WithTimeout(operation, 15*time.Second)
	defer cancel()
	rows, err := c.Sessions(bounded)
	if err != nil {
		return SessionCancelReceipt{}, err
	}
	listed := false
	for _, row := range rows {
		if row.Path == scope.SessionPath {
			listed = true
			break
		}
	}
	if !listed {
		return SessionCancelReceipt{}, ErrSessionNotListed
	}
	ctx, finish, err := c.callContext(bounded)
	if err != nil {
		return SessionCancelReceipt{}, err
	}
	defer finish()
	data, err := json.Marshal(struct {
		ProtocolVersion int `json:"protocolVersion"`
		SessionCancelScope
	}{1, scope})
	if err != nil || len(data) > 40<<10 {
		return SessionCancelReceipt{}, ErrResponse
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/desktop/session-cancel", bytes.NewReader(data))
	if err != nil {
		return SessionCancelReceipt{}, ErrResponse
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return SessionCancelReceipt{}, errors.Join(ErrCancelOutcomeUnknown, c.viewReadError(ctx))
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		c.Close()
		return SessionCancelReceipt{}, ErrClosed
	}
	if resp.StatusCode == 409 || resp.StatusCode == 404 {
		return SessionCancelReceipt{}, ErrTurnCancelChanged
	}
	if resp.StatusCode != 200 {
		return SessionCancelReceipt{}, ErrCancelOutcomeUnknown
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, (40<<10)+1))
	var receipt SessionCancelReceipt
	if err != nil || len(data) > 40<<10 || !utf8.Valid(data) || json.Unmarshal(data, &receipt) != nil || receipt.ProtocolVersion != 1 || receipt.Scope() != scope || !receipt.Cancelled {
		return SessionCancelReceipt{}, ErrCancelOutcomeUnknown
	}
	if c.Closed() || ctx.Err() != nil {
		return SessionCancelReceipt{}, errors.Join(ErrCancelOutcomeUnknown, c.viewReadError(ctx))
	}
	return receipt, nil
}
