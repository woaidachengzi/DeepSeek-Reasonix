package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
	"unicode/utf8"

	"reasonix/internal/event"
)

// Runtime states are memory-only observations, not ownership/execution grants.
// Mirrored/retiring entries are explicitly tagged. Only serve is presently held
// by this server; mutations still require exact scoped admission at execution.
type RuntimeSessionState struct {
	SessionPath string                     `json:"sessionPath"`
	Current     bool                       `json:"current"`
	Ownership   string                     `json:"ownership"`
	State       event.RuntimeStateSnapshot `json:"state"`
}
type RuntimeStates struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Epoch         string                `json:"epoch"`
	Revision      uint64                `json:"revision"`
	Sessions      []RuntimeSessionState `json:"sessions"`
}

func (c *Client) RuntimeStates(operation context.Context) (RuntimeStates, error) {
	if operation == nil {
		return RuntimeStates{}, ErrClient
	}
	bounded, cancel := context.WithTimeout(operation, 15*time.Second)
	defer cancel()
	ctx, finish, err := c.callContext(bounded)
	if err != nil {
		return RuntimeStates{}, err
	}
	defer finish()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/runtime-states", nil)
	if err != nil {
		return RuntimeStates{}, ErrResponse
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return RuntimeStates{}, c.viewReadError(ctx)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			c.Close()
		}
		return RuntimeStates{}, ErrUnavailable
	}
	const max = 2 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return RuntimeStates{}, c.viewReadError(ctx)
	}
	var result RuntimeStates
	if len(data) > max || !utf8.Valid(data) || json.Unmarshal(data, &result) != nil || result.SchemaVersion != 1 || result.Epoch == "" || !cleanField(result.Epoch, 4096) || result.Revision == 0 || result.Revision > 9_007_199_254_740_991 || result.Sessions == nil || len(result.Sessions) > 128 {
		return RuntimeStates{}, ErrResponse
	}
	seen := make(map[string]bool)
	epochs := make(map[string]bool)
	currentCount := 0
	for _, row := range result.Sessions {
		switch row.Ownership {
		case "serve", "external", "retiring":
		default:
			return RuntimeStates{}, ErrResponse
		}
		if row.Current {
			currentCount++
		}
		if currentCount > 1 {
			return RuntimeStates{}, ErrResponse
		}
		if row.SessionPath == "" || !cleanField(row.SessionPath, 32768) || seen[row.SessionPath] || epochs[row.State.RuntimeEpoch] || row.State.SchemaVersion != 1 || row.State.RuntimeEpoch == "" || row.State.Revision == 0 {
			return RuntimeStates{}, ErrResponse
		}
		seen[row.SessionPath], epochs[row.State.RuntimeEpoch] = true, true
		view := SessionView{ProtocolVersion: 1, SessionPath: row.SessionPath, ReadOnly: true, Ownership: "serve", RuntimeState: &row.State, History: []HistoryMessage{}}
		if !validSessionView(view, row.SessionPath) {
			return RuntimeStates{}, ErrResponse
		}
	}
	if ctx.Err() != nil {
		return RuntimeStates{}, c.viewReadError(ctx)
	}
	return result, nil
}
