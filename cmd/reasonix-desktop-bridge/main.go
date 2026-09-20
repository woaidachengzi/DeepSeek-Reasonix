// Command reasonix-desktop-bridge exposes the small, local protocol used by a
// desktop host to supervise the Reasonix core. It deliberately starts with no
// session or Agent operations: transport/lifecycle correctness comes first.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"reasonix/internal/desktopbridge"
)

const (
	tokenEnvironment = "REASONIX_DESKTOP_BRIDGE_TOKEN"
	requestIDHeader  = "X-Reasonix-Request-ID"
	maxRequestIDs    = 256
)

type config struct {
	listen    string
	readyFile string
	launchID  string
}

type readyFile struct {
	ProtocolVersion   int    `json:"protocolVersion"`
	Address           string `json:"address"`
	SidecarInstanceID string `json:"sidecarInstanceId"`
	LaunchID          string `json:"launchId"`
}

type healthResponse struct {
	ProtocolVersion   int      `json:"protocolVersion"`
	Status            string   `json:"status"`
	SidecarInstanceID string   `json:"sidecarInstanceId"`
	Capabilities      []string `json:"capabilities"`
}

type shutdownResponse struct {
	ProtocolVersion   int    `json:"protocolVersion"`
	Status            string `json:"status"`
	SidecarInstanceID string `json:"sidecarInstanceId"`
}

type bridgeServer struct {
	token             string
	instanceID        string
	shutdownRequested chan struct{}
	shutdownOnce      sync.Once
	runtimes          *desktopbridge.RuntimeManager
	events            *desktopbridge.EventStream
	requestIDs        *idempotencyLedger
}

func main() {
	var cfg config
	flag.StringVar(&cfg.listen, "listen", "127.0.0.1:0", "loopback address to listen on")
	flag.StringVar(&cfg.readyFile, "ready-file", "", "owner-only readiness file path")
	flag.StringVar(&cfg.launchID, "launch-id", "", "opaque host-generated launch identifier")
	flag.Parse()

	if err := run(context.Background(), cfg, os.Getenv(tokenEnvironment)); err != nil {
		fmt.Fprintln(os.Stderr, "reasonix desktop bridge:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config, token string) error {
	if cfg.readyFile == "" {
		return errors.New("--ready-file is required")
	}
	if cfg.launchID == "" {
		return errors.New("--launch-id is required")
	}
	if len(token) < 32 {
		return fmt.Errorf("%s must be at least 32 bytes", tokenEnvironment)
	}
	if err := requireLoopbackAddress(cfg.listen); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", cfg.listen)
	if err != nil {
		return fmt.Errorf("listen %q: %w", cfg.listen, err)
	}
	defer listener.Close()

	instanceID, err := randomID()
	if err != nil {
		return err
	}
	events := desktopbridge.NewEventStream(1024)
	manager := desktopbridge.NewRuntimeManager(newControllerFactory(events))
	bridge := newBridgeServerWithEvents(token, instanceID, manager, events)
	ready := readyFile{
		ProtocolVersion:   desktopbridge.ProtocolVersion,
		Address:           listener.Addr().String(),
		SidecarInstanceID: instanceID,
		LaunchID:          cfg.launchID,
	}
	if err := writeReadyFile(cfg.readyFile, ready); err != nil {
		return err
	}
	defer removeReadyFile(cfg.readyFile, instanceID)

	server := &http.Server{
		Handler:           bridge.handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	select {
	case <-ctx.Done():
	case <-bridge.shutdownRequested:
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve after shutdown: %w", err)
	}
	return nil
}

func requireLoopbackAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("listen address %q is not an IP loopback address", address)
	}
	return nil
}

func newBridgeServer(token, instanceID string, managers ...*desktopbridge.RuntimeManager) *bridgeServer {
	manager := desktopbridge.NewRuntimeManager(nil)
	if len(managers) > 0 && managers[0] != nil {
		manager = managers[0]
	}
	return newBridgeServerWithEvents(token, instanceID, manager, desktopbridge.NewEventStream(1024))
}

func newBridgeServerWithEvents(token, instanceID string, manager *desktopbridge.RuntimeManager, events *desktopbridge.EventStream) *bridgeServer {
	return &bridgeServer{
		token:             token,
		instanceID:        instanceID,
		shutdownRequested: make(chan struct{}),
		runtimes:          manager,
		events:            events,
		requestIDs:        newIdempotencyLedger(maxRequestIDs),
	}
}

// idempotent remembers completed open/submit responses by an opaque host-generated
// request ID. It stores a hash of the request target and bytes, never the prompt
// itself. A duplicate waits for the original response instead of admitting a
// second Agent turn; reusing an ID for different input is an explicit conflict.
func (b *bridgeServer) idempotent(maxBodyBytes int64, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(requestIDHeader)
		if requestID == "" {
			next(w, r) // Preserve compatibility while older private hosts roll forward.
			return
		}
		if !validRequestID(requestID) {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid desktop bridge request ID")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
		if err != nil || int64(len(body)) > maxBodyBytes {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid desktop bridge request")
			return
		}
		fingerprint := requestFingerprint(r.Method, r.URL.Path, body)
		record, owner, conflict := b.requestIDs.reserve(requestID, fingerprint)
		if conflict {
			writeProtocolError(w, http.StatusConflict, "conflict", "desktop bridge request ID was reused for different input")
			return
		}
		if !owner {
			<-record.done
			writeStoredResponse(w, record.response)
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		captured := newResponseCapture()
		next(captured, r)
		stored := captured.stored()
		b.requestIDs.finish(requestID, stored)
		writeStoredResponse(w, stored)
	}
}

func validRequestID(requestID string) bool {
	if len(requestID) == 0 || len(requestID) > 128 {
		return false
	}
	for _, char := range requestID {
		if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}

func requestFingerprint(method, path string, body []byte) [sha256.Size]byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte(method))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(path))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(body)
	var fingerprint [sha256.Size]byte
	copy(fingerprint[:], hash.Sum(nil))
	return fingerprint
}

type idempotencyRecord struct {
	fingerprint [sha256.Size]byte
	done        chan struct{}
	response    storedResponse
}

type idempotencyLedger struct {
	mu      sync.Mutex
	entries map[string]*idempotencyRecord
	order   []string
	limit   int
}

func newIdempotencyLedger(limit int) *idempotencyLedger {
	return &idempotencyLedger{entries: make(map[string]*idempotencyRecord), limit: limit}
}

func (l *idempotencyLedger) reserve(requestID string, fingerprint [sha256.Size]byte) (record *idempotencyRecord, owner, conflict bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if existing, ok := l.entries[requestID]; ok {
		return existing, false, existing.fingerprint != fingerprint
	}
	record = &idempotencyRecord{fingerprint: fingerprint, done: make(chan struct{})}
	l.entries[requestID] = record
	l.order = append(l.order, requestID)
	return record, true, false
}

func (l *idempotencyLedger) finish(requestID string, response storedResponse) {
	l.mu.Lock()
	defer l.mu.Unlock()
	record, ok := l.entries[requestID]
	if !ok {
		return
	}
	record.response = response
	close(record.done)
	for len(l.entries) > l.limit && len(l.order) > 0 {
		oldestID := l.order[0]
		l.order = l.order[1:]
		oldest, exists := l.entries[oldestID]
		if exists && oldest.response.body != nil {
			delete(l.entries, oldestID)
		}
	}
}

type storedResponse struct {
	status      int
	contentType string
	body        []byte
}

type responseCapture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newResponseCapture() *responseCapture     { return &responseCapture{header: make(http.Header)} }
func (w *responseCapture) Header() http.Header { return w.header }
func (w *responseCapture) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *responseCapture) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(body)
}
func (w *responseCapture) stored() storedResponse {
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	return storedResponse{status: status, contentType: w.header.Get("Content-Type"), body: append([]byte(nil), w.body.Bytes()...)}
}

func writeStoredResponse(w http.ResponseWriter, response storedResponse) {
	if response.contentType != "" {
		w.Header().Set("Content-Type", response.contentType)
	}
	w.WriteHeader(response.status)
	_, _ = w.Write(response.body)
}

func (b *bridgeServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", b.authorized(b.health))
	mux.HandleFunc("POST /v1/sessions:open", b.authorized(b.idempotent(64<<10, b.openSession)))
	mux.HandleFunc("GET /v1/sessions/{id}/snapshot", b.authorized(b.sessionSnapshot))
	mux.HandleFunc("GET /v1/events", b.authorized(b.eventsHandler))
	// ServeMux path wildcards occupy a complete segment, while the public v1
	// routes use the conventional ":submit" and ":cancel" suffixes. Dispatch
	// that narrow route family explicitly to retain the documented URLs.
	mux.HandleFunc("POST /v1/sessions/", b.authorized(b.sessionCommand))
	mux.HandleFunc("POST /v1:shutdown", b.authorized(b.shutdown))
	return mux
}

func (b *bridgeServer) eventsHandler(w http.ResponseWriter, r *http.Request) {
	after, err := parseAfterSequence(r.URL.Query().Get("afterSequence"))
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid afterSequence")
		return
	}
	replay, resyncRequired, live, cancel := b.events.Subscribe(after)
	defer cancel()
	if resyncRequired {
		writeProtocolError(w, http.StatusConflict, "resync_required", "event replay window is no longer available")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	for _, item := range replay {
		if !writeSSE(w, item) {
			return
		}
	}
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case item := <-live:
			if !writeSSE(w, item) {
				return
			}
			flusher.Flush()
		}
	}
}

func parseAfterSequence(raw string) (uint64, error) {
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseUint(raw, 10, 64)
}

func writeSSE(w io.Writer, item desktopbridge.Event) bool {
	_, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", item.Sequence, item.EventKind, item.Payload)
	return err == nil
}

func (b *bridgeServer) sessionCommand(w http.ResponseWriter, r *http.Request) {
	const prefix = "/v1/sessions/"
	path := strings.TrimPrefix(r.URL.Path, prefix)
	switch {
	case strings.HasSuffix(path, ":submit"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":submit"))
		b.idempotent(1<<20, b.submit)(w, r)
	case strings.HasSuffix(path, ":cancel"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":cancel"))
		b.cancel(w, r)
	default:
		writeProtocolError(w, http.StatusNotFound, "not_found", "desktop bridge route not found")
	}
}

func (b *bridgeServer) authorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const bearer = "Bearer "
		authorization := r.Header.Get("Authorization")
		provided := strings.TrimPrefix(authorization, bearer)
		if !strings.HasPrefix(authorization, bearer) ||
			subtle.ConstantTimeCompare([]byte(provided), []byte(b.token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"protocolVersion": desktopbridge.ProtocolVersion,
				"error": map[string]string{
					"code":    "unauthenticated",
					"message": "desktop bridge authentication failed",
				},
			})
			return
		}
		next(w, r)
	}
}

func (b *bridgeServer) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		ProtocolVersion:   desktopbridge.ProtocolVersion,
		Status:            "ok",
		SidecarInstanceID: b.instanceID,
		Capabilities:      []string{"health", "open_session", "session_snapshot", "submit", "cancel", "idempotency", "shutdown"},
	})
}

func (b *bridgeServer) shutdown(w http.ResponseWriter, _ *http.Request) {
	shutdownErr := b.runtimes.Shutdown()
	b.shutdownOnce.Do(func() { close(b.shutdownRequested) })
	if shutdownErr != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to close desktop bridge session")
		return
	}
	writeJSON(w, http.StatusAccepted, shutdownResponse{
		ProtocolVersion:   desktopbridge.ProtocolVersion,
		Status:            "stopping",
		SidecarInstanceID: b.instanceID,
	})
}

type openSessionRequest struct {
	SessionID     string `json:"sessionId"`
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
}

type submitRequest struct {
	Input string `json:"input"`
}

func (b *bridgeServer) openSession(w http.ResponseWriter, r *http.Request) {
	var request openSessionRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid open_session request")
		return
	}
	view, err := b.runtimes.Open(r.Context(), desktopbridge.OpenRequest{SessionID: request.SessionID, WorkspaceRoot: request.WorkspaceRoot})
	if err != nil {
		status, code := http.StatusInternalServerError, "internal"
		switch {
		case errors.Is(err, desktopbridge.ErrInvalidSessionID):
			status, code = http.StatusBadRequest, "invalid_request"
		case errors.Is(err, desktopbridge.ErrSessionConflict), errors.Is(err, desktopbridge.ErrOpenInProgress):
			status, code = http.StatusConflict, "conflict"
		case errors.Is(err, desktopbridge.ErrClosed):
			status, code = http.StatusServiceUnavailable, "shutting_down"
		}
		writeProtocolError(w, status, code, "unable to open desktop bridge session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "session": view})
}

func (b *bridgeServer) sessionSnapshot(w http.ResponseWriter, r *http.Request) {
	view, ok := b.runtimes.Snapshot()
	if !ok || view.ID != r.PathValue("id") {
		writeProtocolError(w, http.StatusNotFound, "not_found", "desktop bridge session not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "sequence": b.events.LatestSequence(), "session": view})
}

func (b *bridgeServer) submit(w http.ResponseWriter, r *http.Request) {
	var request submitRequest
	if err := decodeJSONBody(w, r, 1<<20, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid submit request")
		return
	}
	view, err := b.runtimes.Submit(r.PathValue("id"), request.Input)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to submit desktop bridge input")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "session": view})
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, maxBytes int64, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON value")
		}
		return err
	}
	return nil
}

func (b *bridgeServer) cancel(w http.ResponseWriter, r *http.Request) {
	view, err := b.runtimes.Cancel(r.PathValue("id"))
	if err != nil {
		b.writeRuntimeError(w, err, "unable to cancel desktop bridge input")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "session": view})
}

func (b *bridgeServer) writeRuntimeError(w http.ResponseWriter, err error, message string) {
	status, code := http.StatusInternalServerError, "internal"
	switch {
	case errors.Is(err, desktopbridge.ErrInvalidSessionID), errors.Is(err, desktopbridge.ErrInvalidInput):
		status, code = http.StatusBadRequest, "invalid_request"
	case errors.Is(err, desktopbridge.ErrSessionConflict), errors.Is(err, desktopbridge.ErrOpenInProgress):
		status, code = http.StatusConflict, "conflict"
	case errors.Is(err, desktopbridge.ErrSessionNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, desktopbridge.ErrClosed):
		status, code = http.StatusServiceUnavailable, "shutting_down"
	}
	writeProtocolError(w, status, code, message)
}

func writeProtocolError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"protocolVersion": desktopbridge.ProtocolVersion,
		"error":           map[string]string{"code": code, "message": message},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate sidecar instance id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func writeReadyFile(path string, ready readyFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create ready-file directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".reasonix-bridge-ready-*")
	if err != nil {
		return fmt.Errorf("create ready file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("set ready-file permissions: %w", err)
	}
	if err := json.NewEncoder(temporary).Encode(ready); err != nil {
		temporary.Close()
		return fmt.Errorf("encode ready file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close ready file: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("publish ready file: %w", err)
	}
	return nil
}

func removeReadyFile(path, instanceID string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var ready readyFile
	if json.Unmarshal(content, &ready) == nil && ready.SidecarInstanceID == instanceID {
		_ = os.Remove(path)
	}
}
