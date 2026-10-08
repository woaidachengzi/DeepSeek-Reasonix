package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrClosed      = errors.New("remote controller connection changed; reopen the remote workspace")
	ErrResponse    = errors.New("remote controller response is invalid; check the remote Serve version")
	ErrUnavailable = errors.New("remote controller request failed; reconnect the saved SSH host")
)

// Session entries are display/catalogue data only, never local paths or grants.
type Session struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Title      string `json:"title"`
	Turns      int64  `json:"turns"`
	Current    bool   `json:"current"`
	Running    bool   `json:"running"`
	TakenOver  bool   `json:"takenOver"`
	MtimeMilli int64  `json:"mtimeMilli"`
}

type Client struct {
	http        *http.Client
	base        string
	owner       context.Context
	cancel      context.CancelFunc
	stopCleanup func() bool
	closeOnce   sync.Once
}

// Connect separates the SSH/host owner lifetime from the attach HTTP request.
// Finishing an attach must not cancel the connection; revoking its SSH owner
// must cancel both in-flight authentication and all later reads/streams.
func Connect(owner, operation context.Context, base, token string) (*Client, error) {
	if owner == nil || operation == nil {
		return nil, ErrClient
	}
	httpClient, err := NewHTTPClient(base)
	if err != nil {
		return nil, err
	}
	u, _, _ := parseBase(base)
	ctx, cancel := context.WithCancel(owner)
	c := &Client{http: httpClient, base: u.String(), owner: ctx, cancel: cancel}
	c.stopCleanup = context.AfterFunc(ctx, httpClient.CloseIdleConnections)
	call, finish, err := c.callContext(operation)
	if err != nil {
		c.Close()
		return nil, err
	}
	defer finish()
	if err := Handshake(call, httpClient, c.base, token); err != nil {
		c.Close()
		return nil, err
	}
	if c.Closed() || operation.Err() != nil {
		c.Close()
		return nil, ErrClosed
	}
	return c, nil
}

func (c *Client) Closed() bool { return c == nil || c.owner.Err() != nil }
func (c *Client) Close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() { c.cancel(); c.stopCleanup(); c.http.CloseIdleConnections() })
}

func (c *Client) callContext(operation context.Context) (context.Context, func(), error) {
	if c.Closed() {
		return nil, nil, ErrClosed
	}
	if operation == nil {
		return nil, nil, ErrClient
	}
	ctx, cancel := context.WithCancel(operation)
	stop := context.AfterFunc(c.owner, cancel)
	return ctx, func() { stop(); cancel() }, nil
}

func cleanField(value string, limit int) bool {
	return len(value) <= limit && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func cleanTitle(value string) bool {
	// Serve's fallback title is the first prompt preview and may be multiline.
	// Preserve whitespace as display data, but reject terminal/control escapes.
	return len(value) <= 8192 && utf8.ValidString(value) && strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) < 0
}

func (c *Client) Sessions(operation context.Context) ([]Session, error) {
	if operation == nil {
		return nil, ErrClient
	}
	bounded, cancel := context.WithTimeout(operation, 15*time.Second)
	defer cancel()
	ctx, finish, err := c.callContext(bounded)
	if err != nil {
		return nil, err
	}
	defer finish()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/sessions", nil)
	if err != nil {
		return nil, ErrResponse
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if c.Closed() {
			return nil, ErrClosed
		}
		if ctx.Err() != nil {
			return nil, errors.Join(ErrUnavailable, ctx.Err())
		}
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// A restarted Serve can invalidate its old cookie without dropping
		// SSH. Reopen must authenticate afresh, not reuse this dead handle.
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			c.Close()
		}
		return nil, ErrUnavailable
	}
	const max = 8 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		if c.Closed() {
			return nil, ErrClosed
		}
		if ctx.Err() != nil {
			return nil, errors.Join(ErrUnavailable, ctx.Err())
		}
		return nil, ErrUnavailable
	}
	if len(data) > max {
		return nil, ErrResponse
	}
	var entries []Session
	// Unknown Serve fields are not forwarded. Additional credentials, URLs or
	// settings in a newer response can never become renderer fields by accident.
	if !utf8.Valid(data) || json.Unmarshal(data, &entries) != nil || entries == nil || len(entries) > 10000 {
		return nil, ErrResponse
	}
	seen := make(map[string]bool, len(entries))
	current := 0
	for _, entry := range entries {
		if !cleanField(entry.Name, 4096) || !cleanField(entry.Path, 32768) || entry.Path == "" || !cleanTitle(entry.Title) || entry.Turns < 0 || entry.Turns > 9_007_199_254_740_991 || entry.MtimeMilli < 0 || entry.MtimeMilli > 9_007_199_254_740_991 || seen[entry.Path] {
			return nil, ErrResponse
		}
		seen[entry.Path] = true
		if entry.Current {
			current++
		}
	}
	if current > 1 {
		return nil, ErrResponse
	}
	if c.Closed() {
		return nil, ErrClosed
	}
	if ctx.Err() != nil {
		return nil, errors.Join(ErrUnavailable, ctx.Err())
	}
	return entries, nil
}
