package controller

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrDrivingChanged = errors.New("remote driving authority changed; select the current owner")
	ErrDrivingUnknown = errors.New("remote driving outcome is unknown; inspect the original owner, do not automatically retry")
)

// Scope retains the original capture across turns. InputRevision is separate:
// a new idle turn must not silently adopt a new control generation or holder.
type SessionDrivingScope struct {
	SessionPendingScope
	Revision       uint64 `json:"revision"`
	ControlVersion uint64 `json:"controlVersion"`
}

func (s *SessionDrivingScope) UnmarshalJSON(data []byte) error {
	var wire struct {
		SessionPath    string  `json:"sessionPath"`
		RuntimeEpoch   string  `json:"runtimeEpoch"`
		Revision       *uint64 `json:"revision"`
		ControlVersion *uint64 `json:"controlVersion"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&wire) != nil || d.Decode(new(any)) != io.EOF || wire.Revision == nil || wire.ControlVersion == nil {
		return ErrResponse
	}
	*s = SessionDrivingScope{SessionPendingScope{wire.SessionPath, wire.RuntimeEpoch}, *wire.Revision, *wire.ControlVersion}
	return nil
}

type SessionDrivingRequest struct {
	ProtocolVersion int                 `json:"protocolVersion"`
	Action          string              `json:"action"`
	Scope           SessionDrivingScope `json:"scope"`
	// Private host-generated 128-bit key. Never echo into receipts or events.
	Key           string `json:"key,omitempty"`
	InputRevision uint64 `json:"inputRevision,omitempty"`
	Text          string `json:"text,omitempty"`
}

type SessionDrivingReceipt struct {
	ProtocolVersion int                 `json:"protocolVersion"`
	Action          string              `json:"action"`
	Scope           SessionDrivingScope `json:"scope"`
	Active          bool                `json:"active"`
	Accepted        bool                `json:"accepted"`
	Released        bool                `json:"released"`
}

func ValidSessionDrivingRequest(r SessionDrivingRequest) bool {
	if r.ProtocolVersion != 1 || !ValidSessionPendingScope(r.Scope.SessionPendingScope) || r.Scope.Revision > 9_007_199_254_740_991 || r.Scope.ControlVersion >= 9_007_199_254_740_991 {
		return false
	}
	if r.Action == "capture" {
		return r.Scope.Revision == 0 && r.Scope.ControlVersion == 0 && r.Key == "" && r.InputRevision == 0 && r.Text == ""
	}
	if r.Scope.Revision == 0 || len(r.Key) != 32 || r.Key != strings.ToLower(r.Key) {
		return false
	}
	if _, err := hex.DecodeString(r.Key); err != nil {
		return false
	}
	switch r.Action {
	case "acquire", "state", "release":
		return r.InputRevision == 0 && r.Text == ""
	case "input":
		return r.InputRevision != 0 && r.InputRevision <= 9_007_199_254_740_991 && strings.TrimSpace(r.Text) != "" && len(r.Text) <= 64<<10 && utf8.ValidString(r.Text) && !strings.ContainsRune(r.Text, 0)
	default:
		return false
	}
}

func validDrivingReceipt(v SessionDrivingReceipt, r SessionDrivingRequest) bool {
	if v.ProtocolVersion != 1 || v.Action != r.Action || v.Scope.SessionPendingScope != r.Scope.SessionPendingScope || v.Scope.Revision == 0 || v.Scope.Revision > 9_007_199_254_740_991 || v.Scope.ControlVersion >= 9_007_199_254_740_991 || (r.Action != "capture" && v.Scope != r.Scope) {
		return false
	}
	switch r.Action {
	case "capture":
		return !v.Active && !v.Accepted && !v.Released
	case "acquire":
		return v.Active && !v.Accepted && !v.Released
	case "state":
		return !v.Accepted && !v.Released
	case "release":
		return !v.Active && !v.Accepted && v.Released
	case "input":
		return !v.Active && v.Accepted && !v.Released
	}
	return false
}

// SessionDriving makes one authenticated, lifetime-owned fixed-endpoint call.
// No saved-history whitelist, restore, foreground switch, fallback or retry.
// Accepted denotes admission only; cancellation must not imply turn failure.
func (c *Client) SessionDriving(operation context.Context, input SessionDrivingRequest) (SessionDrivingReceipt, error) {
	if operation == nil {
		return SessionDrivingReceipt{}, ErrClient
	}
	if !ValidSessionDrivingRequest(input) {
		return SessionDrivingReceipt{}, ErrResponse
	}
	bounded, cancel := context.WithTimeout(operation, 20*time.Second)
	defer cancel()
	ctx, finish, err := c.callContext(bounded)
	if err != nil {
		return SessionDrivingReceipt{}, err
	}
	defer finish()
	data, err := json.Marshal(input)
	if err != nil || len(data) > 1<<20 {
		return SessionDrivingReceipt{}, ErrResponse
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/desktop/session-driving", bytes.NewReader(data))
	if err != nil {
		return SessionDrivingReceipt{}, ErrResponse
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return SessionDrivingReceipt{}, errors.Join(ErrDrivingUnknown, c.viewReadError(ctx))
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		c.Close()
		return SessionDrivingReceipt{}, ErrClosed
	}
	if resp.StatusCode == 404 || resp.StatusCode == 409 {
		return SessionDrivingReceipt{}, ErrDrivingChanged
	}
	if resp.StatusCode != 200 {
		return SessionDrivingReceipt{}, ErrDrivingUnknown
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, (256<<10)+1))
	var receipt SessionDrivingReceipt
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err != nil || len(data) > 256<<10 || !utf8.Valid(data) || d.Decode(&receipt) != nil || d.Decode(new(any)) != io.EOF || !validDrivingReceipt(receipt, input) {
		return SessionDrivingReceipt{}, ErrDrivingUnknown
	}
	if c.Closed() || ctx.Err() != nil {
		return SessionDrivingReceipt{}, errors.Join(ErrDrivingUnknown, c.viewReadError(ctx))
	}
	return receipt, nil
}
