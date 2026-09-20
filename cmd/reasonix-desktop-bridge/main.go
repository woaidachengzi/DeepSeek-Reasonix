// Command reasonix-desktop-bridge exposes the small, local protocol used by a
// desktop host to supervise the Reasonix core. It deliberately starts with no
// session or Agent operations: transport/lifecycle correctness comes first.
package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	protocolVersion  = 1
	tokenEnvironment = "REASONIX_DESKTOP_BRIDGE_TOKEN"
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
	bridge := newBridgeServer(token, instanceID)
	ready := readyFile{
		ProtocolVersion:   protocolVersion,
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

func newBridgeServer(token, instanceID string) *bridgeServer {
	return &bridgeServer{
		token:             token,
		instanceID:        instanceID,
		shutdownRequested: make(chan struct{}),
	}
}

func (b *bridgeServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", b.authorized(b.health))
	mux.HandleFunc("POST /v1:shutdown", b.authorized(b.shutdown))
	return mux
}

func (b *bridgeServer) authorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const bearer = "Bearer "
		authorization := r.Header.Get("Authorization")
		provided := strings.TrimPrefix(authorization, bearer)
		if !strings.HasPrefix(authorization, bearer) ||
			subtle.ConstantTimeCompare([]byte(provided), []byte(b.token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"protocolVersion": protocolVersion,
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
		ProtocolVersion:   protocolVersion,
		Status:            "ok",
		SidecarInstanceID: b.instanceID,
		Capabilities:      []string{"health", "shutdown"},
	})
}

func (b *bridgeServer) shutdown(w http.ResponseWriter, _ *http.Request) {
	b.shutdownOnce.Do(func() { close(b.shutdownRequested) })
	writeJSON(w, http.StatusAccepted, shutdownResponse{
		ProtocolVersion:   protocolVersion,
		Status:            "stopping",
		SidecarInstanceID: b.instanceID,
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
