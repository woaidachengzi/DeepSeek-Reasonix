package controller

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"
)

type SessionObservationRequest struct {
	ProtocolVersion int                 `json:"protocolVersion"`
	Scope           SessionPendingScope `json:"scope"`
}

// Neither scope nor transcript/prompt data is carried in observed frames.
type SessionObservationFrame struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Kind            string `json:"kind"`
}

func ValidSessionObservationRequest(r SessionObservationRequest) bool {
	return r.ProtocolVersion == 1 && ValidSessionPendingScope(r.Scope)
}

type SessionObservationStream struct {
	client  *Client
	ctx     context.Context
	body    io.ReadCloser
	scanner *bufio.Scanner
	finish  func()
	once    sync.Once
}

// ObserveSession binds the exact published Controller epoch. No catalogue
// fallback, saved history, replay, automatic reconnect or implicit driving.
func (c *Client) ObserveSession(operation context.Context, input SessionObservationRequest) (*SessionObservationStream, error) {
	if operation == nil {
		return nil, ErrClient
	}
	if !ValidSessionObservationRequest(input) {
		return nil, ErrResponse
	}
	ctx, finish, err := c.callContext(operation)
	if err != nil {
		return nil, err
	}
	// Bound response headers and the ready handshake, without placing a
	// deadline on a successfully established long-lived observation.
	handshake := time.AfterFunc(20*time.Second, finish)
	defer handshake.Stop()
	data, _ := json.Marshal(input)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/desktop/session-observation", bytes.NewReader(data))
	if err != nil {
		finish()
		return nil, ErrResponse
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")
	resp, err := c.http.Do(req)
	if err != nil {
		failure := c.viewReadError(ctx)
		finish()
		return nil, failure
	}
	media, _, mediaErr := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != 200 || mediaErr != nil || media != "application/x-ndjson" {
		_ = resp.Body.Close()
		finish()
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			c.Close()
			return nil, ErrClosed
		}
		return nil, ErrUnavailable
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256), 1025)
	s := &SessionObservationStream{client: c, ctx: ctx, body: resp.Body, scanner: scanner, finish: finish}
	if frame, err := s.read(); err != nil || frame.Kind != "ready" {
		s.Close()
		return nil, ErrResponse
	}
	if !handshake.Stop() || ctx.Err() != nil || c.Closed() {
		s.Close()
		return nil, ErrUnavailable
	}
	return s, nil
}

func (s *SessionObservationStream) Close() {
	if s != nil {
		s.once.Do(func() { s.finish(); _ = s.body.Close() })
	}
}

func (s *SessionObservationStream) read() (SessionObservationFrame, error) {
	if s.ctx.Err() != nil || s.client.Closed() {
		return SessionObservationFrame{}, s.client.viewReadError(s.ctx)
	}
	if !s.scanner.Scan() {
		return SessionObservationFrame{}, ErrStreamInterrupted
	}
	raw := s.scanner.Bytes()
	var frame SessionObservationFrame
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 1024 || !utf8.Valid(raw) || d.Decode(&frame) != nil || d.Decode(new(any)) != io.EOF || frame.ProtocolVersion != 1 {
		return frame, ErrResponse
	}
	if s.ctx.Err() != nil || s.client.Closed() {
		return SessionObservationFrame{}, s.client.viewReadError(s.ctx)
	}
	return frame, nil
}

func (s *SessionObservationStream) Next() (frame SessionObservationFrame, err error) {
	if s == nil {
		return frame, ErrClient
	}
	defer func() {
		if err != nil {
			s.Close()
		}
	}()
	frame, err = s.read()
	if err != nil {
		return frame, err
	}
	switch frame.Kind {
	case "turn_started", "turn_done", "ask_request", "approval_request", "mcp_interaction":
		return frame, nil
	default:
		return SessionObservationFrame{}, ErrResponse
	}
}
