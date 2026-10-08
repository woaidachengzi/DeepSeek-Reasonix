// Package controller is the shared host-only Serve transport. Its input is a
// backend-owned SSH tunnel address, never a renderer URL or proxy configuration.
package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrOrigin       = errors.New("remote controller requires its original loopback HTTP tunnel; reconnect the saved SSH host")
	ErrClient       = errors.New("remote controller transport is unavailable; reconnect the saved SSH host")
	ErrHandshake    = errors.New("remote controller authentication failed; reconnect the saved SSH host")
	ErrAuthResponse = errors.New("remote controller authentication response exceeds its budget; check the remote Serve version")
)

const authResponseMax = 4 << 10

type origin struct {
	ip   net.IP
	port int
}

func parseOrigin(u *url.URL) (origin, error) {
	if u == nil || u.Scheme != "http" || u.User != nil || u.Opaque != "" || u.Fragment != "" || strings.ContainsAny(u.Host, "\\\r\n") {
		return origin{}, ErrOrigin
	}
	ip := net.ParseIP(u.Hostname())
	port, err := strconv.Atoi(u.Port())
	if ip == nil || !ip.IsLoopback() || err != nil || port < 1 || port > 65535 {
		return origin{}, ErrOrigin
	}
	return origin{ip: ip, port: port}, nil
}

func parseBase(base string) (*url.URL, origin, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return nil, origin{}, ErrOrigin // Never echo an untrusted URL.
	}
	o, err := parseOrigin(u)
	if err != nil || (u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery {
		return nil, origin{}, ErrOrigin
	}
	u.Path = ""
	return u, o, nil
}

func (o origin) matches(u *url.URL) bool {
	other, err := parseOrigin(u)
	return err == nil && o.ip.Equal(other.ip) && o.port == other.port
}

// Cookies are scoped by host, not port. Pin the origin before every request so
// even a direct caller cannot send this jar's token cookie to another tunnel.
// This is separate from CheckRedirect: both protections must remain enabled.
type tunnelTransport struct {
	origin origin
	inner  *http.Transport
}

func (t *tunnelTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || !t.origin.matches(req.URL) || (req.Host != "" && req.Host != req.URL.Host) {
		return nil, ErrOrigin
	}
	return t.inner.RoundTrip(req)
}

func (t *tunnelTransport) CloseIdleConnections() { t.inner.CloseIdleConnections() }

// NewHTTPClient owns a dedicated transport and jar. It ignores ambient HTTP
// proxies, numeric addresses need no DNS, and it never follows redirects. Do
// not set a whole-client Timeout: the same client carries long-lived SSE;
// request contexts own body/stream cancellation instead.
func NewHTTPClient(base string) (*http.Client, error) {
	_, o, err := parseBase(base)
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, ErrClient
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            dialer.DialContext,
		MaxIdleConns:           32,
		MaxIdleConnsPerHost:    8,
		IdleConnTimeout:        90 * time.Second,
		ResponseHeaderTimeout:  15 * time.Second,
		MaxResponseHeaderBytes: 1 << 20,
		ExpectContinueTimeout:  time.Second,
	}
	return &http.Client{
		Jar: jar, Transport: &tunnelTransport{origin: o, inner: transport},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}

// Handshake sends the token once, only in a bounded JSON body. Subsequent API
// and SSE calls use the host-only jar, not query/fragment/header credentials.
// No failure body or token is returned. The response drain has both a byte
// limit and deadline; an unhealthy Serve cannot hang authentication forever.
func Handshake(ctx context.Context, client *http.Client, base, token string) error {
	u, _, err := parseBase(base)
	if err != nil {
		return err
	}
	if ctx == nil || client == nil || client.Jar == nil {
		return ErrClient
	}
	transport, ok := client.Transport.(*tunnelTransport)
	if !ok || !transport.origin.matches(u) {
		return ErrClient
	}
	if token == "" || len(token) > 4096 || strings.IndexFunc(token, func(r rune) bool { return r < 0x21 || r > 0x7e }) >= 0 {
		return ErrHandshake
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"token": token})
	if len(body) > 8<<10 {
		return ErrHandshake // Match Serve's JSON body budget after escaping.
	}
	u.Path = "/auth/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return ErrHandshake
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		// Preserve context classification without leaking request/body/network
		// details. The caller decides whether to reconnect, never blind-retries.
		if ctx.Err() != nil {
			return errors.Join(ErrHandshake, ctx.Err())
		}
		return ErrHandshake
	}
	defer resp.Body.Close()
	size, err := io.Copy(io.Discard, io.LimitReader(resp.Body, authResponseMax+1))
	if size > authResponseMax {
		return ErrAuthResponse
	}
	if err != nil {
		if ctx.Err() != nil {
			return errors.Join(ErrHandshake, ctx.Err())
		}
		return ErrHandshake
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%w (status %d)", ErrHandshake, resp.StatusCode)
	}
	return nil
}
