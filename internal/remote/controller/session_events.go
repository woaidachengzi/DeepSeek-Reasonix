package controller

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"

	"reasonix/internal/eventwire"
)

var ErrStreamInterrupted = errors.New("remote session event stream ended; reopen and reconcile the remote session")

const sessionEventFrameMax = 8 << 20

var sessionEventKinds = func() map[string]bool {
	kinds := make(map[string]bool)
	for _, kind := range eventwire.KindNames() {
		kinds[kind] = true
	}
	return kinds
}()

// SessionEventStream has one Next consumer. Its lifetime belongs to both the
// selected operation and the authenticated SSH/controller owner. It never
// retries, adopts another foreground, or turns an SSE id into a replay grant.
type SessionEventStream struct {
	client    *Client
	ctx       context.Context
	path      string
	body      io.ReadCloser
	scanner   *bufio.Scanner
	finish    func()
	closeOnce sync.Once
}

// SessionEvents opens the all-session backend stream, but exposes only frames
// explicitly tagged with this catalogue member. This includes detached turns;
// an untagged/foreign event is never attributed to the selected foreground.
// Callers must open before resume/submit, then reconcile a snapshot and pending
// prompts. A successful open alone is not a writable session capability.
func (c *Client) SessionEvents(operation context.Context, path string) (*SessionEventStream, error) {
	if operation == nil {
		return nil, ErrClient
	}
	if path == "" || !cleanField(path, 32768) {
		return nil, ErrResponse
	}
	rows, err := c.Sessions(operation)
	if err != nil {
		return nil, err
	}
	listed := false
	for _, row := range rows {
		listed = listed || row.Path == path
	}
	if !listed {
		return nil, ErrSessionNotListed
	}
	ctx, finish, err := c.callContext(operation)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/events?all=1", nil)
	if err != nil {
		finish()
		return nil, ErrResponse
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		failure := c.viewReadError(ctx)
		finish()
		return nil, failure
	}
	media, _, mediaErr := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || mediaErr != nil || media != "text/event-stream" {
		resp.Body.Close()
		finish()
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			c.Close()
		}
		return nil, ErrUnavailable
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), sessionEventFrameMax+1)
	stream := &SessionEventStream{client: c, ctx: ctx, path: path, body: resp.Body, scanner: scanner, finish: finish}
	if ctx.Err() != nil || c.Closed() {
		stream.Close()
		return nil, c.viewReadError(ctx)
	}
	return stream, nil
}

func (s *SessionEventStream) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() { s.finish(); s.body.Close() })
}

// Next strips unknown fields through the shared typed event projection. The
// transport boundary and correlation budget are checked before publication;
// bridge/native consumers still own capability-specific payload validation.
func (s *SessionEventStream) Next() (frame eventwire.Event, err error) {
	if s == nil {
		return frame, ErrClient
	}
	defer func() {
		if err != nil {
			s.Close()
		}
	}()
	var data strings.Builder
	hasData := false
	bytes := 0
	for {
		if s.ctx.Err() != nil || s.client.Closed() {
			return frame, s.client.viewReadError(s.ctx)
		}
		if !s.scanner.Scan() {
			if s.ctx.Err() != nil || s.client.Closed() {
				return frame, s.client.viewReadError(s.ctx)
			}
			if s.scanner.Err() != nil || hasData {
				return frame, ErrResponse
			}
			return frame, ErrStreamInterrupted
		}
		line := s.scanner.Text()
		bytes += len(line) + 1
		if bytes > sessionEventFrameMax {
			return frame, ErrResponse
		}
		if line != "" {
			field, value, _ := strings.Cut(line, ":")
			if field == "data" {
				if hasData {
					data.WriteByte('\n')
				}
				data.WriteString(strings.TrimPrefix(value, " "))
				hasData = true
			}
			continue
		}
		bytes = 0
		if !hasData {
			continue
		}
		body := data.String()
		data.Reset()
		hasData = false
		if !utf8.ValidString(body) || !strings.HasPrefix(strings.TrimSpace(body), "{") || json.Unmarshal([]byte(body), &frame) != nil {
			return eventwire.Event{}, ErrResponse
		}
		if frame.SessionPath != s.path {
			frame = eventwire.Event{}
			continue
		}
		if !sessionEventKinds[frame.Kind] || frame.Sequence > 9_007_199_254_740_991 || !cleanField(frame.TurnID, 4096) || !cleanField(frame.ItemID, 4096) || !cleanField(frame.PromptID, 4096) || !cleanField(frame.MessageID, 4096) || (frame.MessageID != "" && frame.Kind != "steer") {
			return eventwire.Event{}, ErrResponse
		}
		if s.ctx.Err() != nil || s.client.Closed() {
			return eventwire.Event{}, s.client.viewReadError(s.ctx)
		}
		return frame, nil
	}
}
