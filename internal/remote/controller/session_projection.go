package controller

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"weak"

	"reasonix/internal/event"
	"reasonix/internal/eventwire"
)

var ErrProjectionReconcile = errors.New("remote projection changed or expired; refresh the selected session snapshot")

type ProjectionReplay struct {
	Events            []eventwire.Event `json:"events"`
	FloorSequence     uint64            `json:"floorSeq"`
	LatestSequence    uint64            `json:"latestSeq"`
	NextAfterSequence uint64            `json:"nextAfterSeq"`
	HasMore           bool              `json:"hasMore"`
	RuntimeEpoch      string            `json:"runtimeEpoch,omitempty"`
}

// Bound replay object allocation before materializing typed event structs.
// The whole response is already byte-bounded; this additionally prevents a
// small JSON array of millions of empty objects from growing a huge slice.
func (r *ProjectionReplay) UnmarshalJSON(data []byte) error {
	type replayWire ProjectionReplay
	var result ProjectionReplay
	wire := struct {
		*replayWire
		Events json.RawMessage `json:"events"`
	}{replayWire: (*replayWire)(&result)}
	if json.Unmarshal(data, &wire) != nil {
		return ErrResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(wire.Events))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('[') {
		return ErrResponse
	}
	result.Events = []eventwire.Event{}
	for decoder.More() {
		if len(result.Events) >= 512 {
			return ErrResponse
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil || len(raw) > sessionEventFrameMax {
			return ErrResponse
		}
		var frame eventwire.Event
		if json.Unmarshal(raw, &frame) != nil {
			return ErrResponse
		}
		result.Events = append(result.Events, frame)
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim(']') {
		return ErrResponse
	}
	*r = result
	return nil
}

// SessionProjection is a display cut, not provider history, ownership or a
// prompt/approval grant. Live events must be buffered before reading it.
type SessionProjection struct {
	ProtocolVersion     int              `json:"protocolVersion"`
	SessionPath         string           `json:"sessionPath"`
	ReadOnly            bool             `json:"readOnly"`
	Initial             bool             `json:"initial"`
	History             []HistoryMessage `json:"history"`
	UserSuffix          []HistoryMessage `json:"userSuffix"`
	ActiveTurnID        string           `json:"activeTurnId,omitempty"`
	TurnStatus          string           `json:"turnStatus,omitempty"`
	ReplayAfterSequence uint64           `json:"replayAfterSeq"`
	Replay              ProjectionReplay `json:"replay"`
	PageToken           string           `json:"pageToken,omitempty"`
}

func (c *Client) SessionProjection(operation context.Context, path string) (SessionProjection, error) {
	return c.readSessionProjection(operation, path, nil)
}

// Only the server-issued continuation and exact next cursor leave the host.
// No automatic retry, unbounded page accumulation or legacy route fallback.
func (c *Client) SessionProjectionPage(operation context.Context, path string, previous SessionProjection) (SessionProjection, error) {
	if operation == nil {
		return SessionProjection{}, ErrClient
	}
	if !previous.Replay.HasMore || len(previous.Replay.Events) == 0 {
		return SessionProjection{}, ErrResponse
	}
	after := previous.Replay.Events[0].Sequence
	if after == 0 || !validSessionProjection(previous, path, after-1, previous.Initial) {
		return SessionProjection{}, ErrResponse
	}
	return c.readSessionProjection(operation, path, &previous)
}

// ProjectionCursor is host-only compact metadata. It retains neither history
// nor events, and cannot be reconstructed from JSON or reused by another owner.
type ProjectionCursor struct {
	owner    weak.Pointer[Client]
	previous SessionProjection
}

func (c *Client) ProjectionCursor(previous SessionProjection) (ProjectionCursor, error) {
	if c == nil || c.Closed() {
		return ProjectionCursor{}, ErrClosed
	}
	if !previous.Replay.HasMore || len(previous.Replay.Events) == 0 || previous.Replay.Events[0].Sequence == 0 || !validSessionProjection(previous, previous.SessionPath, previous.Replay.Events[0].Sequence-1, previous.Initial) {
		return ProjectionCursor{}, ErrResponse
	}
	previous.History = nil
	previous.UserSuffix = nil
	previous.Replay.Events = nil
	return ProjectionCursor{owner: weak.Make(c), previous: previous}, nil
}

func (c *Client) SessionProjectionNext(operation context.Context, path string, cursor ProjectionCursor) (SessionProjection, error) {
	if c == nil || cursor.owner.Value() != c || cursor.previous.SessionPath != path || !cursor.previous.Replay.HasMore {
		return SessionProjection{}, ErrProjectionReconcile
	}
	return c.readSessionProjection(operation, path, &cursor.previous)
}

func (c *Client) readSessionProjection(operation context.Context, path string, previous *SessionProjection) (SessionProjection, error) {
	if operation == nil {
		return SessionProjection{}, ErrClient
	}
	if path == "" || !cleanField(path, 32768) {
		return SessionProjection{}, ErrResponse
	}
	bounded, cancel := context.WithTimeout(operation, 20*time.Second)
	defer cancel()
	rows, err := c.Sessions(bounded)
	if err != nil {
		return SessionProjection{}, err
	}
	listed := false
	for _, row := range rows {
		listed = listed || row.Path == path
	}
	if !listed {
		return SessionProjection{}, ErrSessionNotListed
	}
	ctx, finish, err := c.callContext(bounded)
	if err != nil {
		return SessionProjection{}, err
	}
	defer finish()
	query := url.Values{"session": {path}}
	var after uint64
	if previous != nil {
		after = previous.Replay.NextAfterSequence
		query.Set("page", previous.PageToken)
		query.Set("after", strconv.FormatUint(after, 10))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/desktop/session-projection?"+query.Encode(), nil)
	if err != nil {
		return SessionProjection{}, ErrResponse
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return SessionProjection{}, c.viewReadError(ctx)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			c.Close()
		}
		if resp.StatusCode == 409 {
			return SessionProjection{}, ErrProjectionReconcile
		}
		return SessionProjection{}, ErrUnavailable
	}
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return SessionProjection{}, ErrResponse
	}
	const max = 30 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return SessionProjection{}, c.viewReadError(ctx)
	}
	var view SessionProjection
	if len(data) > max || !utf8.Valid(data) || json.Unmarshal(data, &view) != nil {
		return SessionProjection{}, ErrResponse
	}
	if previous == nil {
		after = view.ReplayAfterSequence
	}
	if !validSessionProjection(view, path, after, previous == nil) {
		return SessionProjection{}, ErrResponse
	}
	for i, frame := range view.Replay.Events {
		view.Replay.Events[i] = projectSessionAdmission(frame)
	}
	if previous != nil && (view.ActiveTurnID != previous.ActiveTurnID || view.TurnStatus != previous.TurnStatus || view.ReplayAfterSequence != previous.ReplayAfterSequence || view.Replay.LatestSequence != previous.Replay.LatestSequence || view.Replay.RuntimeEpoch != previous.Replay.RuntimeEpoch || (view.PageToken != "" && view.PageToken != previous.PageToken)) {
		return SessionProjection{}, ErrProjectionReconcile
	}
	if c.Closed() || ctx.Err() != nil {
		return SessionProjection{}, c.viewReadError(ctx)
	}
	return view, nil
}

func validProjectionToken(value string) bool {
	if len(value) != 32 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validSessionProjection(v SessionProjection, path string, after uint64, initial bool) bool {
	const maxJS uint64 = 9_007_199_254_740_991
	r := v.Replay
	if v.ProtocolVersion != 1 || v.SessionPath != path || !v.ReadOnly || v.Initial != initial || !validHistoryMessages(v.History) || !validHistoryMessages(v.UserSuffix) || len(v.History)+len(v.UserSuffix) > 100000 || !cleanField(v.ActiveTurnID, 4096) || !cleanField(r.RuntimeEpoch, 4096) || r.Events == nil || len(r.Events) > 512 || r.LatestSequence > maxJS || r.FloorSequence == 0 || r.FloorSequence > r.LatestSequence+1 || v.ReplayAfterSequence > after || after > r.LatestSequence || v.ReplayAfterSequence < r.FloorSequence-1 || r.NextAfterSequence < after || r.NextAfterSequence > r.LatestSequence {
		return false
	}
	if !initial && (len(v.History) != 0 || len(v.UserSuffix) != 0) {
		return false
	}
	if initial && v.ReplayAfterSequence != after {
		return false
	}
	if r.HasMore != (r.NextAfterSequence < r.LatestSequence) || (r.HasMore && !validProjectionToken(v.PageToken)) || (!r.HasMore && v.PageToken != "") {
		return false
	}
	seen := make(map[string]bool, len(v.History)+len(v.UserSuffix))
	for _, rows := range [][]HistoryMessage{v.History, v.UserSuffix} {
		for _, row := range rows {
			if seen[row.ID] {
				return false
			}
			seen[row.ID] = true
		}
	}
	for _, row := range v.UserSuffix {
		if row.Role != "user" {
			return false
		}
	}
	switch event.TurnStatus(v.TurnStatus) {
	case "", event.TurnCompleted, event.TurnInterrupted, event.TurnFailed, event.TurnProtocolFailed:
		if v.ActiveTurnID != "" || len(v.UserSuffix) != 0 || len(r.Events) != 0 || r.HasMore || v.ReplayAfterSequence != r.LatestSequence {
			return false
		}
	case event.TurnQueued, event.TurnInProgress, event.TurnWaitingUser, event.TurnCancelling:
		if v.ActiveTurnID == "" {
			return false
		}
	default:
		return false
	}
	expected := after
	for _, frame := range r.Events {
		if frame.Sequence != expected+1 || frame.SessionPath != path || frame.TurnID != v.ActiveTurnID || !sessionEventKinds[frame.Kind] || !cleanField(frame.ItemID, 4096) || !cleanField(frame.PromptID, 4096) || !validSessionMessageIdentity(frame) {
			return false
		}
		switch event.TurnStatus(frame.Status) {
		case event.TurnQueued, event.TurnInProgress, event.TurnWaitingUser, event.TurnCancelling:
		default:
			return false
		}
		encoded, err := json.Marshal(frame)
		if err != nil || len(encoded) > sessionEventFrameMax {
			return false
		}
		expected = frame.Sequence
	}
	return expected == r.NextAfterSequence && (!r.HasMore || len(r.Events) > 0)
}
