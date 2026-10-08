package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
	"unicode/utf8"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// Display-only wire types. Raw provider replay bodies, configuration, endpoint
// and credentials deliberately have no fields, including inside search cards.
type HistoryToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type HistorySearch struct {
	SourcesStatus string                     `json:"sources_status,omitempty"`
	ID            string                     `json:"id"`
	Query         string                     `json:"query,omitempty"`
	Results       []provider.ServerSearchHit `json:"results,omitempty"`
}
type HistoryMessage struct {
	Role             string                           `json:"role"`
	Content          string                           `json:"content"`
	Reasoning        string                           `json:"reasoning,omitempty"`
	Missing          []string                         `json:"missing,omitempty"`
	ToolCalls        []HistoryToolCall                `json:"toolCalls,omitempty"`
	ToolCallID       string                           `json:"toolCallId,omitempty"`
	ToolName         string                           `json:"toolName,omitempty"`
	ServerSearch     []HistorySearch                  `json:"serverSearch,omitempty"`
	ProtocolRecovery *provider.ProtocolRecoveryAction `json:"protocolRecovery,omitempty"`
}
type SessionView struct {
	ProtocolVersion int                         `json:"protocolVersion"`
	SessionPath     string                      `json:"sessionPath"`
	ReadOnly        bool                        `json:"readOnly"`
	Ownership       string                      `json:"ownership"`
	Current         bool                        `json:"current"`
	ModelRef        string                      `json:"modelRef"`
	Label           string                      `json:"label"`
	RuntimeState    *event.RuntimeStateSnapshot `json:"runtimeState,omitempty"`
	History         []HistoryMessage            `json:"history"`
}

var ErrSessionNotListed = errors.New("remote session is no longer listed; refresh the remote sessions")

// SessionView reads one explicit catalogue member, never a foreground fallback
// or a resume/reclaim request. Its server route is versioned and side-effect
// free; an older Serve fails rather than falling back to legacy /status.
func (c *Client) SessionView(operation context.Context, path string) (SessionView, error) {
	if operation == nil {
		return SessionView{}, ErrClient
	}
	if path == "" || !cleanField(path, 32768) {
		return SessionView{}, ErrResponse
	}
	bounded, cancel := context.WithTimeout(operation, 20*time.Second)
	defer cancel()
	rows, err := c.Sessions(bounded)
	if err != nil {
		return SessionView{}, err
	}
	listed := false
	for _, row := range rows {
		if row.Path == path {
			listed = true
			break
		}
	}
	if !listed {
		return SessionView{}, ErrSessionNotListed
	}
	ctx, finish, err := c.callContext(bounded)
	if err != nil {
		return SessionView{}, err
	}
	defer finish()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/desktop/session-view?"+url.Values{"session": {path}}.Encode(), nil)
	if err != nil {
		return SessionView{}, ErrResponse
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return SessionView{}, c.viewReadError(ctx)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			c.Close()
		}
		return SessionView{}, ErrUnavailable
	}
	// Reserve bridge envelope room below the native host's 32 MiB body gate.
	const max = 30 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return SessionView{}, c.viewReadError(ctx)
	}
	var view SessionView
	if len(data) > max || !utf8.Valid(data) || json.Unmarshal(data, &view) != nil || !validSessionView(view, path) {
		return SessionView{}, ErrResponse
	}
	if c.Closed() || ctx.Err() != nil {
		return SessionView{}, c.viewReadError(ctx)
	}
	return view, nil
}
func (c *Client) viewReadError(ctx context.Context) error {
	if c.Closed() {
		return ErrClosed
	}
	if ctx.Err() != nil {
		return errors.Join(ErrUnavailable, ctx.Err())
	}
	return ErrUnavailable
}
func validSessionView(v SessionView, path string) bool {
	if v.ProtocolVersion != 1 || v.SessionPath != path || !v.ReadOnly || v.History == nil || len(v.History) > 100000 || !cleanField(v.ModelRef, 4096) || !cleanField(v.Label, 4096) {
		return false
	}
	switch v.Ownership {
	case "serve":
		if v.RuntimeState == nil {
			return false
		}
	case "saved", "external":
		if v.RuntimeState != nil || v.ModelRef != "" || v.Label != "" {
			return false
		}
	default:
		return false
	}
	if s := v.RuntimeState; s != nil {
		const maxJS = 9_007_199_254_740_991
		if s.SchemaVersion < 0 || s.SchemaVersion > 1 || s.Revision > maxJS || s.TurnEventSeq > maxJS || s.BackgroundJobs < 0 || s.BackgroundJobs > maxJS || !cleanField(s.RuntimeEpoch, 4096) || !cleanField(s.TurnID, 4096) || !cleanField(s.Activity, 8192) {
			return false
		}
		switch s.Phase {
		case "idle", "executing", "finishing", "closed":
		default:
			return false
		}
		switch s.TurnStatus {
		case "", event.TurnQueued, event.TurnInProgress, event.TurnWaitingUser, event.TurnCancelling, event.TurnCompleted, event.TurnInterrupted, event.TurnFailed, event.TurnProtocolFailed:
		default:
			return false
		}
	}
	for _, m := range v.History {
		switch m.Role {
		case "system", "user", "assistant", "tool", "notice", "protocol_recovery", "final_readiness":
		default:
			return false
		}
		if len(m.ToolCalls) > 10000 || len(m.ServerSearch) > 10000 || len(m.Missing) > 10000 || !cleanField(m.ToolCallID, 4096) || !cleanField(m.ToolName, 4096) {
			return false
		}
		for _, tc := range m.ToolCalls {
			if tc.ID == "" || tc.Name == "" || !cleanField(tc.ID, 4096) || !cleanField(tc.Name, 4096) {
				return false
			}
		}
		if m.ProtocolRecovery != nil && (m.ProtocolRecovery.ID == "" || !cleanField(m.ProtocolRecovery.ID, 4096)) {
			return false
		}
		for _, sc := range m.ServerSearch {
			if !cleanField(sc.ID, 4096) || !cleanField(sc.SourcesStatus, 4096) || len(sc.Results) > 10000 {
				return false
			}
			for _, hit := range sc.Results {
				if !cleanField(hit.URL, 32768) {
					return false
				}
			}
		}
	}
	return true
}
