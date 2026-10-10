package controller

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"time"
)

type DrivingReclaimObservationRequest struct {
	ProtocolVersion int                 `json:"protocolVersion"`
	Scope           SessionDrivingScope `json:"scope"`
	Key             string              `json:"key"`
}

func ValidDrivingReclaimObservationRequest(r DrivingReclaimObservationRequest) bool {
	return ValidSessionDrivingRequest(SessionDrivingRequest{ProtocolVersion: r.ProtocolVersion, Action: "state", Scope: r.Scope, Key: r.Key})
}

// A single-consumer stream: one reclaimed frame, followed by a blocked read
// until the original grant/source retires. EOF is not a reclaim notification.
// Closing this read-only stream does not release or renew the original grant.
type DrivingReclaimObservationStream struct {
	stream *SessionObservationStream
	seen   bool
}

func (c *Client) ObserveDrivingReclaim(operation context.Context, input DrivingReclaimObservationRequest) (*DrivingReclaimObservationStream, error) {
	if operation == nil {
		return nil, ErrClient
	}
	if !ValidDrivingReclaimObservationRequest(input) {
		return nil, ErrResponse
	}
	ctx, finish, err := c.callContext(operation)
	if err != nil {
		return nil, err
	}
	handshake := time.AfterFunc(20*time.Second, finish)
	defer handshake.Stop()
	data, _ := json.Marshal(input)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/desktop/driving-reclaim-observation", bytes.NewReader(data))
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
		if resp.StatusCode == 404 || resp.StatusCode == 409 {
			return nil, ErrDrivingChanged
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
	return &DrivingReclaimObservationStream{stream: s}, nil
}

func (s *DrivingReclaimObservationStream) Close() {
	if s != nil {
		s.stream.Close()
	}
}
func (s *DrivingReclaimObservationStream) Next() (frame SessionObservationFrame, err error) {
	if s == nil {
		return frame, ErrClient
	}
	defer func() {
		if err != nil {
			s.Close()
		}
	}()
	frame, err = s.stream.read()
	if err != nil {
		return frame, err
	}
	if s.seen || frame.Kind != "reclaimed" {
		return SessionObservationFrame{}, ErrResponse
	}
	s.seen = true
	return frame, nil
}
