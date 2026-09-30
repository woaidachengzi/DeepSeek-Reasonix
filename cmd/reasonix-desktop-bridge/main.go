// Command reasonix-desktop-bridge exposes the small, local protocol used by a
// desktop host to inspect a redacted provider summary and supervise the
// Reasonix core's session and Agent lifecycle.
package main

import (
	"bufio"
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

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/mcpregistry"
	"reasonix/internal/profilegate"
	"reasonix/internal/sessionidentity"
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
	token              string
	instanceID         string
	shutdownRequested  chan struct{}
	shutdownOnce       sync.Once
	runtimes           *desktopbridge.RuntimeManager
	events             *desktopbridge.EventStream
	requestIDs         *idempotencyLedger
	remoteSessions     *previewRemoteSessions
	packageOpsMu       sync.Mutex
	subagentTryMu      sync.Mutex
	subagentTryRun     *subagentTryRun
	cacheIdentityReads bool
	identityReadMu     sync.Mutex
	identityReadStore  *sessionidentity.Store
	mcpRegistry        *mcpregistry.Client
	botRuntime         *previewBotRuntime
}

func main() {
	var cfg config
	flag.StringVar(&cfg.listen, "listen", "127.0.0.1:0", "loopback address to listen on")
	flag.StringVar(&cfg.readyFile, "ready-file", "", "owner-only readiness file path")
	flag.StringVar(&cfg.launchID, "launch-id", "", "opaque host-generated launch identifier")
	flag.Parse()

	token, err := consumeBridgeToken()
	if err == nil {
		err = run(context.Background(), cfg, token)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "reasonix desktop bridge:", err)
		os.Exit(1)
	}
}

// The host writes one bounded token line to the child's private stdin pipe.
// Clear any inherited legacy environment value before starting the core, so
// it cannot leak to subprocesses or act as an alternate authentication source.
func consumeBridgeToken() (string, error) {
	if err := os.Unsetenv(tokenEnvironment); err != nil {
		return "", fmt.Errorf("clear desktop bridge token from process environment: %w", err)
	}
	line, err := bufio.NewReader(io.LimitReader(os.Stdin, 129)).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read desktop bridge token from stdin: %w", err)
	}
	token := strings.TrimSuffix(line, "\n")
	if len(token) < 32 || !validRequestID(token) {
		return "", errors.New("desktop bridge token from stdin is invalid")
	}
	// The bundled Tauri child handle keeps its pipe writer open for the process
	// lifetime. Close our reader and replace stdin before the core can start a
	// tool that reads from or inherits standard input.
	if err := os.Stdin.Close(); err != nil {
		return "", fmt.Errorf("close desktop bridge token pipe: %w", err)
	}
	input, err := os.Open(os.DevNull)
	if err != nil {
		return "", fmt.Errorf("replace desktop bridge stdin: %w", err)
	}
	os.Stdin = input
	return token, nil
}

func run(ctx context.Context, cfg config, token string) (runErr error) {
	if cfg.readyFile == "" {
		return errors.New("--ready-file is required")
	}
	if cfg.launchID == "" {
		return errors.New("--launch-id is required")
	}
	if len(token) < 32 {
		return errors.New("desktop bridge token must be at least 32 bytes")
	}
	if err := requireLoopbackAddress(cfg.listen); err != nil {
		return err
	}
	sessionDir := appconfig.SessionDir()
	if sessionDir == "" {
		return errors.New("session profile root is unavailable")
	}
	releaseProfile, err := profilegate.TryAcquireDesktop(appconfig.ReasonixHomeDir(), filepath.Dir(sessionDir))
	if err != nil {
		return fmt.Errorf("session profile ownership: %w", err)
	}
	defer releaseProfile()
	if identityPath := appconfig.DesktopSessionIdentityPath(); identityPath != "" {
		exists, err := sessionidentity.IdentityDatabaseExists(identityPath, appconfig.SessionProfileRoot())
		if err != nil {
			return fmt.Errorf("inspect session identity store: %w", err)
		}
		if exists {
			identities, err := sessionidentity.Open(ctx, identityPath, appconfig.SessionProfileRoot())
			if err != nil {
				return fmt.Errorf("migrate session identity store: %w", err)
			}
			_, discardErr := identities.DiscardTerminalManualTitleRenames(ctx)
			closeErr := identities.Close()
			if discardErr != nil {
				return fmt.Errorf("reconcile terminal session title intents before startup: %w", discardErr)
			}
			if err := closeErr; err != nil {
				return fmt.Errorf("close migrated session identity store: %w", err)
			}
		}
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
	defer func() {
		if err := manager.Shutdown(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("close session runtime: %w", err))
		}
	}()
	bridge := newBridgeServerWithEvents(token, instanceID, manager, events)
	bridge.botRuntime.refreshAsync(ctx)
	defer bridge.botRuntime.stop()
	defer bridge.remoteSessions.closeAll()
	bridge.cacheIdentityReads = true
	defer bridge.closeIdentityReadStore()
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
		botRuntime:        newPreviewBotRuntime(),
		remoteSessions:    newPreviewRemoteSessions(),
	}
}

// readIdentityStore reuses one integrity-checked read-only SQLite connection
// for the lifetime of the production sidecar. SQLite starts a fresh read
// snapshot for each query, so writer commits remain visible while pagination
// avoids running quick_check again for every page. Test-created servers retain
// the previous request-scoped open/close behavior.
func (b *bridgeServer) readIdentityStore(ctx context.Context, path, profileRoot string) (*sessionidentity.Store, bool, func(), error) {
	if !b.cacheIdentityReads {
		store, exists, err := sessionidentity.OpenReadOnlyIfExists(ctx, path, profileRoot)
		if err != nil || !exists {
			return store, exists, func() {}, err
		}
		return store, true, func() { _ = store.Close() }, nil
	}
	b.identityReadMu.Lock()
	defer b.identityReadMu.Unlock()
	if b.identityReadStore != nil {
		if err := b.identityReadStore.ValidateReadOnlyPath(path, profileRoot); err != nil {
			_ = b.identityReadStore.Close()
			b.identityReadStore = nil
			return nil, false, func() {}, err
		}
		return b.identityReadStore, true, func() {}, nil
	}
	store, exists, err := sessionidentity.OpenReadOnlyIfExists(ctx, path, profileRoot)
	if err != nil || !exists {
		return store, exists, func() {}, err
	}
	b.identityReadStore = store
	return store, true, func() {}, nil
}

func (b *bridgeServer) closeIdentityReadStore() {
	b.identityReadMu.Lock()
	store := b.identityReadStore
	b.identityReadStore = nil
	b.identityReadMu.Unlock()
	if store != nil {
		_ = store.Close()
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
	mux.HandleFunc("GET /v1/providers", b.authorized(b.providerSummary))
	mux.HandleFunc("GET /v1/settings/provider-configs", b.authorized(b.providerConfigs))
	mux.HandleFunc("POST /v1/settings/provider-configs", b.authorized(b.idempotent(64<<10, b.saveProviderConfig)))
	mux.HandleFunc("POST /v1/settings/provider-configs/delete", b.authorized(b.idempotent(64<<10, b.deleteProviderConfig)))
	mux.HandleFunc("POST /v1/settings/provider-configs/discover-models", b.authorized(b.discoverProviderModels))
	mux.HandleFunc("POST /v1/settings/provider-model-probe", b.authorized(b.testProviderModel))
	mux.HandleFunc("POST /v1/settings/usage-stats", b.authorized(b.usageStats))
	mux.HandleFunc("GET /v1/settings/permissions", b.authorized(b.permissionSettings))
	mux.HandleFunc("POST /v1/settings/permissions", b.authorized(b.idempotent(64<<10, b.changePermissionSettings)))
	mux.HandleFunc("GET /v1/settings/secrets", b.authorized(b.secretsSettings))
	mux.HandleFunc("POST /v1/settings/secrets", b.authorized(b.idempotent(64<<10, b.changeSecretsSettings)))
	mux.HandleFunc("GET /v1/settings/sandbox", b.authorized(b.sandboxSettings))
	mux.HandleFunc("POST /v1/settings/sandbox", b.authorized(b.idempotent(64<<10, b.changeSandboxSettings)))
	mux.HandleFunc("GET /v1/settings/network", b.authorized(b.networkSettings))
	mux.HandleFunc("POST /v1/settings/network", b.authorized(b.idempotent(64<<10, b.changeNetworkSettings)))
	mux.HandleFunc("GET /v1/settings/skills", b.authorized(b.skillsSettings))
	mux.HandleFunc("POST /v1/settings/skills", b.authorized(b.idempotent(64<<10, b.changeSkillsSettings)))
	mux.HandleFunc("POST /v1/settings/skills/plan", b.authorized(b.planSkillInstall))
	mux.HandleFunc("POST /v1/settings/skills/install", b.authorized(b.idempotent(64<<10, b.installSkill)))
	mux.HandleFunc("POST /v1/settings/skills/archive", b.authorized(b.idempotent(64<<10, b.archiveSkill)))
	mux.HandleFunc("POST /v1/settings/skills/restore", b.authorized(b.idempotent(64<<10, b.restoreSkill)))
	mux.HandleFunc("GET /v1/settings/plugins", b.authorized(b.pluginSettings))
	mux.HandleFunc("POST /v1/settings/plugins", b.authorized(b.idempotent(64<<10, b.changePluginSettings)))
	mux.HandleFunc("POST /v1/settings/plugins/doctor", b.authorized(b.pluginDoctor))
	mux.HandleFunc("POST /v1/settings/plugins/plan", b.authorized(b.planPluginInstall))
	mux.HandleFunc("POST /v1/settings/plugins/install", b.authorized(b.idempotent(64<<10, b.installPlugin)))
	mux.HandleFunc("POST /v1/settings/plugins/remove", b.authorized(b.idempotent(64<<10, b.removePlugin)))
	mux.HandleFunc("GET /v1/settings/subagents", b.authorized(b.subagentSettings))
	mux.HandleFunc("POST /v1/settings/subagents", b.authorized(b.idempotent(256<<10, b.changeSubagentSettings)))
	mux.HandleFunc("POST /v1/settings/subagents/try", b.authorized(b.trySubagentProfile))
	mux.HandleFunc("GET /v1/settings/subagents/try/status", b.authorized(b.subagentProfileTryStatus))
	mux.HandleFunc("POST /v1/settings/subagents/try/cancel", b.authorized(b.cancelSubagentProfileTry))
	mux.HandleFunc("GET /v1/settings/hooks", b.authorized(b.hooksSettings))
	mux.HandleFunc("POST /v1/settings/hooks", b.authorized(b.idempotent(64<<10, b.changeHooksSettings)))
	mux.HandleFunc("GET /v1/settings/memory", b.authorized(b.memorySettings))
	mux.HandleFunc("POST /v1/settings/memory", b.authorized(b.idempotent(3<<20, b.changeMemorySettings)))
	mux.HandleFunc("GET /v1/settings/memory/suggestions", b.authorized(b.memorySuggestions))
	mux.HandleFunc("POST /v1/settings/memory/suggestions/accept", b.authorized(b.idempotent(16<<10, b.acceptMemorySuggestion)))
	mux.HandleFunc("POST /v1/settings/default-model", b.authorized(b.idempotent(64<<10, b.setDefaultModel)))
	mux.HandleFunc("POST /v1/sessions/{id}/model", b.authorized(b.idempotent(64<<10, b.setSessionModel)))
	mux.HandleFunc("POST /v1/settings/model-role", b.authorized(b.idempotent(64<<10, b.setModelRole)))
	mux.HandleFunc("POST /v1/settings/agent-preferences", b.authorized(b.idempotent(64<<10, b.setAgentPreferences)))
	mux.HandleFunc("GET /v1/settings/desktop", b.authorized(b.desktopPreferences))
	mux.HandleFunc("GET /v1/settings/storage", b.authorized(b.storageSettings))
	mux.HandleFunc("GET /v1/settings/diagnostics/capabilities", b.authorized(b.capabilityDiagnostics))
	mux.HandleFunc("GET /v1/settings/diagnostics/runtime", b.authorized(b.runtimeDoctor))
	mux.HandleFunc("GET /v1/settings/remote", b.authorized(b.remoteSettings))
	mux.HandleFunc("POST /v1/settings/remote/scan", b.authorized(b.scanRemoteSSHConfig))
	mux.HandleFunc("POST /v1/settings/remote/connect", b.authorized(b.connectRemoteHost))
	mux.HandleFunc("POST /v1/settings/remote/disconnect", b.authorized(b.disconnectRemoteHost))
	mux.HandleFunc("POST /v1/settings/remote/browse", b.authorized(b.browseRemoteHost))
	mux.HandleFunc("POST /v1/settings/remote/preview", b.authorized(b.previewRemoteFile))
	mux.HandleFunc("POST /v1/settings/remote/save", b.authorized(b.saveRemoteFile))
	mux.HandleFunc("POST /v1/settings/remote/hosts", b.authorized(b.idempotent(64<<10, b.changeRemoteSettings)))
	mux.HandleFunc("GET /v1/settings/bots", b.authorized(b.botSettings))
	mux.HandleFunc("POST /v1/settings/bots", b.authorized(b.idempotent(64<<10, b.changeBotSettings)))
	mux.HandleFunc("GET /v1/settings/bots/runtime", b.authorized(b.botRuntimeStatus))
	mux.HandleFunc("POST /v1/settings/desktop/approval", b.authorized(b.idempotent(64<<10, b.setDesktopApproval)))
	mux.HandleFunc("POST /v1/settings/desktop/terminal-theme", b.authorized(b.idempotent(64<<10, b.setDesktopTerminalTheme)))
	mux.HandleFunc("POST /v1/settings/desktop/appearance", b.authorized(b.idempotent(64<<10, b.setDesktopAppearance)))
	mux.HandleFunc("POST /v1/settings/desktop/language", b.authorized(b.idempotent(64<<10, b.setDesktopLanguage)))
	mux.HandleFunc("POST /v1/settings/desktop/currency", b.authorized(b.idempotent(64<<10, b.setDesktopCurrency)))
	mux.HandleFunc("POST /v1/settings/provider-key", b.authorized(b.idempotent(64<<10, b.setProviderKey)))
	mux.HandleFunc("POST /v1/sessions:open", b.authorized(b.idempotent(64<<10, b.openSession)))
	mux.HandleFunc("POST /v1/sessions:switch", b.authorized(b.idempotent(64<<10, b.switchSession)))
	mux.HandleFunc("POST /v1/sessions:previews", b.authorized(b.sessionPreviews))
	mux.HandleFunc("POST /v1/sessions/sync-catalog", b.authorized(b.syncSessionCatalog))
	mux.HandleFunc("POST /v1/sessions/titles/first-message", b.authorized(b.backfillSessionTitles))
	mux.HandleFunc("PATCH /v1/sessions/{id}/title", b.authorized(b.idempotent(64<<10, b.renameSession)))
	mux.HandleFunc("DELETE /v1/sessions/{id}", b.authorized(b.idempotent(64<<10, b.deleteSession)))
	mux.HandleFunc("GET /v1/sessions/deletion-recovery", b.authorized(b.pendingSessionDeletes))
	mux.HandleFunc("GET /v1/sessions/deletion-recovery/page", b.authorized(b.pendingSessionDeletesPage))
	mux.HandleFunc("GET /v1/sessions/title-recovery", b.authorized(b.pendingSessionTitleRecoveries))
	mux.HandleFunc("GET /v1/sessions/{id}/snapshot", b.authorized(b.sessionSnapshot))
	mux.HandleFunc("GET /v1/sessions/{id}/balance", b.authorized(b.sessionBalance))
	mux.HandleFunc("POST /v1/sessions/{id}/mcp/runtime", b.authorized(b.idempotent(64<<10, b.sessionMCPRuntimeAction)))
	mux.HandleFunc("POST /v1/sessions/{id}/mcp/auth/clear", b.authorized(b.idempotent(64<<10, b.sessionMCPClearAuthentication)))
	mux.HandleFunc("POST /v1/sessions/{id}/mcp/oauth", b.authorized(b.idempotent(64<<10, b.sessionMCPOAuthStart)))
	mux.HandleFunc("GET /v1/sessions/{id}/mcp/oauth/{flow}", b.authorized(b.sessionMCPOAuthStatus))
	mux.HandleFunc("DELETE /v1/sessions/{id}/mcp/oauth/{flow}", b.authorized(b.sessionMCPOAuthCancel))
	mux.HandleFunc("GET /v1/sessions/{id}/history", b.authorized(b.sessionHistory))
	mux.HandleFunc("GET /v1/sessions/snapshot", b.authorized(b.sessionDirectorySnapshot))
	mux.HandleFunc("GET /v1/sessions/shadow-snapshot", b.authorized(b.sessionShadowSnapshot))
	mux.HandleFunc("POST /v1/sessions/shadow-audit-snapshot", b.authorized(b.sessionShadowAuditSnapshot))
	mux.HandleFunc("GET /v1/sessions", b.authorized(b.sessionList))
	mux.HandleFunc("GET /v1/projects", b.authorized(b.projectFolders))
	mux.HandleFunc("POST /v1/sessions/import-catalog", b.authorized(b.importLegacyCatalog))
	mux.HandleFunc("POST /v1/sessions/scan-import-candidates", b.authorized(b.scanImportCandidates))
	mux.HandleFunc("POST /v1/sessions/import-scan", b.authorized(b.idempotent(2<<20, b.applyScanImport)))
	mux.HandleFunc("GET /v1/sessions/inventory", b.authorized(b.sessionInventory))
	mux.HandleFunc("GET /v1/mcp/servers", b.authorized(b.listMCPServers))
	mux.HandleFunc("GET /v1/mcp/marketplace", b.authorized(b.searchMCPMarketplace))
	mux.HandleFunc("POST /v1/mcp/marketplace/resolve", b.authorized(b.resolveMCPMarketplace))
	mux.HandleFunc("POST /v1/mcp/servers/activation", b.authorized(b.idempotent(64<<10, b.setMCPServerEnabled)))
	mux.HandleFunc("POST /v1/mcp/servers", b.authorized(b.idempotent(256<<10, b.upsertMCPServer)))
	mux.HandleFunc("DELETE /v1/mcp/servers", b.authorized(b.deleteMCPServer))
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
	// The Tauri host consumes the versioned bridge envelope from each data
	// frame. Keeping only Payload here loses session, kind, sequence, and
	// protocol metadata, so the host cannot deserialize or deliver the event.
	data, err := json.Marshal(item)
	if err != nil {
		return false
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", item.Sequence, item.EventKind, data)
	return err == nil
}

func (b *bridgeServer) sessionCommand(w http.ResponseWriter, r *http.Request) {
	const prefix = "/v1/sessions/"
	path := strings.TrimPrefix(r.URL.Path, prefix)
	switch {
	case strings.HasSuffix(path, ":submit"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":submit"))
		b.idempotent(1<<20, b.submit)(w, r)
	case strings.HasSuffix(path, ":attach"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":attach"))
		b.idempotent(64<<10, b.attachFile)(w, r)
	case strings.HasSuffix(path, ":workspace"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":workspace"))
		b.workspaceList(w, r)
	case strings.HasSuffix(path, ":workspace-file"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":workspace-file"))
		b.workspaceFile(w, r)
	case strings.HasSuffix(path, ":workspace-changes"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":workspace-changes"))
		b.workspaceChanges(w, r)
	case strings.HasSuffix(path, ":workspace-change-detail"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":workspace-change-detail"))
		b.workspaceChangeDetail(w, r)
	case strings.HasSuffix(path, ":workspace-file-revert-preview"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":workspace-file-revert-preview"))
		b.workspaceFileRevertPreview(w, r)
	case strings.HasSuffix(path, ":workspace-file-revert-commit"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":workspace-file-revert-commit"))
		b.idempotent(16<<10, b.workspaceFileRevertCommit)(w, r)
	case strings.HasSuffix(path, ":workspace-file-revert-undo"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":workspace-file-revert-undo"))
		b.idempotent(16<<10, b.workspaceFileRevertUndo)(w, r)
	case strings.HasSuffix(path, ":checkpoints"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":checkpoints"))
		b.workspaceCheckpoints(w, r)
	case strings.HasSuffix(path, ":code-rewind-preview"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":code-rewind-preview"))
		b.codeRewindPreview(w, r)
	case strings.HasSuffix(path, ":code-rewind-commit"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":code-rewind-commit"))
		b.idempotent(16<<10, b.codeRewindCommit)(w, r)
	case strings.HasSuffix(path, ":conversation-rewind-preview"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":conversation-rewind-preview"))
		b.conversationRewindPreview(w, r)
	case strings.HasSuffix(path, ":conversation-rewind-commit"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":conversation-rewind-commit"))
		b.idempotent(16<<10, b.conversationRewindCommit)(w, r)
	case strings.HasSuffix(path, ":conversation-rewind-undo"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":conversation-rewind-undo"))
		b.idempotent(16<<10, b.conversationRewindUndo)(w, r)
	case strings.HasSuffix(path, ":session-heads"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":session-heads"))
		b.sessionHeads(w, r)
	case strings.HasSuffix(path, ":session-head-switch"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":session-head-switch"))
		b.idempotent(16<<10, b.sessionHeadSwitch)(w, r)
	case strings.HasSuffix(path, ":combined-rewind-preview"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":combined-rewind-preview"))
		b.combinedRewindPreview(w, r)
	case strings.HasSuffix(path, ":combined-rewind-commit"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":combined-rewind-commit"))
		b.idempotent(16<<10, b.combinedRewindCommit)(w, r)
	case strings.HasSuffix(path, ":legacy-fork-preview"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":legacy-fork-preview"))
		b.legacyForkPreview(w, r)
	case strings.HasSuffix(path, ":legacy-fork-commit"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":legacy-fork-commit"))
		b.idempotent(16<<10, b.legacyForkCommit)(w, r)
	case strings.HasSuffix(path, ":cancel"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":cancel"))
		b.cancel(w, r)
	case strings.HasSuffix(path, ":approve"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":approve"))
		b.approve(w, r)
	case strings.HasSuffix(path, ":answer"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":answer"))
		b.answerQuestion(w, r)
	case strings.HasSuffix(path, ":mcp"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":mcp"))
		b.answerMCPInteraction(w, r)
	case strings.HasSuffix(path, ":replay-prompts"):
		r.SetPathValue("id", strings.TrimSuffix(path, ":replay-prompts"))
		b.replayPendingPrompts(w, r)
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
		Capabilities:      []string{"health", "provider_summary", "provider_configs", "save_provider_config", "delete_provider_config", "discover_provider_models", "provider_model_probe", "usage_stats", "storage_settings", "permission_settings", "set_permission_settings", "secrets_settings", "set_secrets_settings", "sandbox_settings", "set_sandbox_settings", "network_settings", "set_network_settings", "skills_settings", "set_skills_settings", "plan_skill_install", "install_skill", "archive_skill", "restore_skill", "plugin_settings", "set_plugin_settings", "plugin_doctor", "plan_plugin_install", "install_plugin", "remove_plugin", "subagent_settings", "set_subagent_settings", "try_subagent", "subagent_try_status", "cancel_subagent_try", "hooks_settings", "set_hooks_settings", "memory_settings", "set_memory_settings", "memory_suggestions", "accept_memory_suggestion", "set_default_model", "set_session_model", "set_model_role", "set_agent_preferences", "desktop_preferences", "set_desktop_approval", "set_provider_key", "bot_settings", "set_bot_settings", "bot_runtime_status", "remote_ssh_probe", "remote_ssh_connect", "remote_ssh_disconnect", "remote_workspace_browse", "remote_file_preview", "remote_file_save", "open_session", "switch_session", "session_snapshot", "session_balance", "session_history", "rename_session", "delete_session", "attach_file", "workspace_list", "workspace_file_preview", "workspace_changes", "workspace_change_detail", "submit", "cancel", "approve", "answer_question", "answer_mcp_interaction", "mcp_servers", "mcp_server_activation", "mcp_runtime_action", "mcp_marketplace", "session_catalog_sync", "session_directory_snapshot_v1", "session_directory_snapshot_full_v1", "session_shadow_snapshot_v1", "session_shadow_audit_snapshot_v1", "session_delete_recovery_list_v1", "session_title_intent_v1", "session_title_recovery_list_v1", "session_scan_import_review_v1", "project_folders_read", "replay_pending_prompts", "idempotency", "shutdown"},
	})
}

func (b *bridgeServer) providerSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := loadProviderSummaryForScope(r.URL.Query().Get("scope"), r.URL.Query().Get("workspaceRoot"))
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read provider summary")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (b *bridgeServer) providerConfigs(w http.ResponseWriter, _ *http.Request) {
	view, err := loadProviderConfigs(b.token)
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read provider settings")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) discoverProviderModels(w http.ResponseWriter, r *http.Request) {
	var input discoverProviderModelsRequest
	if err := decodeJSONBody(w, r, 8<<10, &input); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid model discovery request")
		return
	}
	result, err := discoverSavedProviderModels(input, b.token)
	if err != nil {
		if errors.Is(err, errPreviewProviderDiscoveryChanged) {
			writeProtocolError(w, http.StatusConflict, "conflict", "provider settings changed; reload before discovering models")
		} else {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (b *bridgeServer) saveProviderConfig(w http.ResponseWriter, r *http.Request) {
	var input saveProviderConfigRequest
	if err := decodeJSONBody(w, r, 64<<10, &input); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid provider settings")
		return
	}
	var err error
	if input.PresetID != "" {
		err = persistProviderPreset(input, b.token)
	} else {
		err = persistProviderConfig(input)
	}
	if err != nil {
		message := "provider settings could not be saved"
		if input.PresetID != "" {
			message = err.Error()
		}
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", message)
		return
	}
	b.providerConfigs(w, r)
}

func (b *bridgeServer) deleteProviderConfig(w http.ResponseWriter, r *http.Request) {
	var input deleteProviderConfigRequest
	if err := decodeJSONBody(w, r, 64<<10, &input); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid provider deletion request")
		return
	}
	if err := removeProviderConfig(input, b.token); err != nil {
		if errors.Is(err, errPreviewProviderChanged) {
			writeProtocolError(w, http.StatusConflict, "conflict", err.Error())
		} else {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}
	b.providerConfigs(w, r)
}

func (b *bridgeServer) usageStats(w http.ResponseWriter, r *http.Request) {
	var request previewUsageStatsRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid usage statistics request")
		return
	}
	result, err := queryPreviewUsageStats(request)
	if err != nil {
		if errors.Is(err, errInvalidPreviewUsageRequest) {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid usage statistics range or source")
		} else {
			writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read Preview usage statistics")
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (b *bridgeServer) permissionSettings(w http.ResponseWriter, r *http.Request) {
	view, err := loadPermissionSettings(r.URL.Query().Get("workspaceRoot"), r.URL.Query().Get("scope"))
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read Preview permission settings")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) changePermissionSettings(w http.ResponseWriter, r *http.Request) {
	var change permissionSettingsChange
	if err := decodeJSONBody(w, r, 64<<10, &change); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid permission settings request")
		return
	}
	view, err := persistPermissionChange(change)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "permission settings could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) secretsSettings(w http.ResponseWriter, _ *http.Request) {
	view, err := loadSecretsSettings()
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read Preview secrets settings")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) changeSecretsSettings(w http.ResponseWriter, r *http.Request) {
	var change secretsSettingsChange
	if err := decodeJSONBody(w, r, 8<<10, &change); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid Preview secrets settings request")
		return
	}
	view, err := persistSecretsSettings(change)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "Preview secrets settings could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) sandboxSettings(w http.ResponseWriter, r *http.Request) {
	view, err := loadSandboxSettings(r.URL.Query().Get("workspaceRoot"))
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read Preview sandbox settings")
		return
	}
	if sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId")); sessionID != "" {
		if shell, active := b.runtimes.BoundShell(sessionID); active {
			view.EffectiveShell = shell
			view.ShellReloadRequired = shell != view.ResolvedShell
		}
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) changeSandboxSettings(w http.ResponseWriter, r *http.Request) {
	var change sandboxSettingsChange
	if err := decodeJSONBody(w, r, 64<<10, &change); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid sandbox settings request")
		return
	}
	view, err := persistSandboxSettings(change)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "sandbox settings could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) networkSettings(w http.ResponseWriter, _ *http.Request) {
	view, err := loadNetworkSettings()
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read Preview network settings")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) changeNetworkSettings(w http.ResponseWriter, r *http.Request) {
	var change networkSettingsChange
	if err := decodeJSONBody(w, r, 64<<10, &change); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid network settings request")
		return
	}
	view, err := persistNetworkSettings(change)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "network settings could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) skillsSettings(w http.ResponseWriter, r *http.Request) {
	view, err := loadSkillsSettings(r.URL.Query().Get("workspaceRoot"))
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "unable to read Preview skill settings")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) changeSkillsSettings(w http.ResponseWriter, r *http.Request) {
	var change skillsSettingsChange
	if err := decodeJSONBody(w, r, 64<<10, &change); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid skill settings request")
		return
	}
	view, err := persistSkillsSettings(change)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "skill settings could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) subagentSettings(w http.ResponseWriter, r *http.Request) {
	view, err := loadSubagentSettings(r.URL.Query().Get("workspaceRoot"))
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "unable to read Preview subagent settings")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) changeSubagentSettings(w http.ResponseWriter, r *http.Request) {
	var change subagentSettingsChange
	if err := decodeJSONBody(w, r, 256<<10, &change); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid subagent settings request")
		return
	}
	view, err := persistSubagentSettings(change)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) hooksSettings(w http.ResponseWriter, r *http.Request) {
	view, err := loadHooksSettings(r.URL.Query().Get("scope"), r.URL.Query().Get("workspaceRoot"))
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "unable to read Preview hooks settings")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) changeHooksSettings(w http.ResponseWriter, r *http.Request) {
	var change previewHooksSettingsChange
	if err := decodeJSONBody(w, r, 64<<10, &change); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid hooks settings request")
		return
	}
	view, err := persistHooksSettings(change)
	if err != nil {
		if errors.Is(err, errHooksSettingsConflict) {
			writeProtocolError(w, http.StatusConflict, "conflict", err.Error())
		} else {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) memorySettings(w http.ResponseWriter, r *http.Request) {
	workspaceRoot := r.URL.Query().Get("workspaceRoot")
	view, err := loadPreviewMemorySettings(workspaceRoot)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "unable to read Preview memory settings")
		return
	}
	b.attachActiveMemoryRecall(&view)
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) attachActiveMemoryRecall(view *previewMemorySettingsView) {
	if view == nil || b.runtimes == nil {
		return
	}
	if session, ok := b.runtimes.Snapshot(); ok {
		if recall, active := b.runtimes.MemoryRecall(session.ID, view.WorkspaceRoot); active {
			view.LastRecall = recall
		}
	}
}

func (b *bridgeServer) changeMemorySettings(w http.ResponseWriter, r *http.Request) {
	var change previewMemorySettingsChange
	if err := decodeJSONBody(w, r, 3<<20, &change); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid memory settings request")
		return
	}
	view, err := persistPreviewMemorySettings(change)
	if err != nil {
		if errors.Is(err, errPreviewMemoryChanged) {
			writeProtocolError(w, http.StatusConflict, "conflict", err.Error())
		} else {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}
	b.attachActiveMemoryRecall(&view)
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) desktopPreferences(w http.ResponseWriter, _ *http.Request) {
	view, err := loadDesktopPreferences()
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read desktop preferences")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) setDesktopApproval(w http.ResponseWriter, r *http.Request) {
	var request setDesktopApprovalRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil || (request.Mode != "ask" && request.Mode != "auto" && request.Mode != "yolo") {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid desktop approval mode")
		return
	}
	if err := persistDesktopApprovalMode(request.Mode); err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to save desktop approval mode")
		return
	}
	b.desktopPreferences(w, r)
}

func (b *bridgeServer) setDesktopTerminalTheme(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Theme string `json:"theme"`
	}
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil || (request.Theme != "auto" && request.Theme != "dark" && request.Theme != "light") {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid desktop terminal theme")
		return
	}
	if err := persistDesktopTerminalTheme(request.Theme); err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to save desktop terminal theme")
		return
	}
	b.desktopPreferences(w, r)
}

func (b *bridgeServer) setDesktopAppearance(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Theme string `json:"theme"`
		Style string `json:"style"`
	}
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid desktop appearance")
		return
	}
	if (request.Theme != "auto" && request.Theme != "dark" && request.Theme != "light") || (request.Style != "" && request.Style != "graphite" && request.Style != "aurora" && request.Style != "slate" && request.Style != "carbon" && request.Style != "nocturne" && request.Style != "amber") {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid desktop appearance")
		return
	}
	if err := persistDesktopAppearance(request.Theme, request.Style); err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to save desktop appearance")
		return
	}
	b.desktopPreferences(w, r)
}

func (b *bridgeServer) setDesktopLanguage(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Language string `json:"language"`
	}
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil || (request.Language != "" && request.Language != "en" && request.Language != "zh") {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid desktop language")
		return
	}
	if err := persistDesktopLanguage(request.Language); err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to save desktop language")
		return
	}
	b.desktopPreferences(w, r)
}

func (b *bridgeServer) setDesktopCurrency(w http.ResponseWriter, r *http.Request) {
	var request setDesktopCurrencyRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid desktop display currency")
		return
	}
	if err := persistDesktopCurrency(request.Currency); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "unable to save desktop display currency")
		return
	}
	b.desktopPreferences(w, r)
}

func (b *bridgeServer) setDefaultModel(w http.ResponseWriter, r *http.Request) {
	var request setDefaultModelRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid default model request")
		return
	}
	if err := persistDefaultModelForScope(request.Model, request.Scope, request.WorkspaceRoot); err != nil {
		if errors.Is(err, errDefaultModelUnavailable) {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "selected model is not configured and ready")
			return
		}
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to save the Preview default model")
		return
	}
	summary, err := loadProviderSummaryForScope(request.Scope, request.WorkspaceRoot)
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read provider summary")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (b *bridgeServer) setModelRole(w http.ResponseWriter, r *http.Request) {
	var request setModelRoleRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid model role request")
		return
	}
	if err := persistModelRole(request); err != nil {
		if errors.Is(err, errDefaultModelUnavailable) {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "selected role model is not available")
			return
		}
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to save the Preview model role")
		return
	}
	summary, err := loadProviderSummaryForScope(request.Scope, request.WorkspaceRoot)
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read provider summary")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (b *bridgeServer) setAgentPreferences(w http.ResponseWriter, r *http.Request) {
	var request setAgentPreferenceRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid agent preference request")
		return
	}
	if err := persistAgentPreferences(request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "agent preference could not be saved")
		return
	}
	if request.ReasoningLanguage != "" && request.Scope != "project" && b.runtimes != nil {
		if session, active := b.runtimes.Snapshot(); active && session.WorkspaceRoot != "" {
			if effective, err := appconfig.LoadForRootWithoutCredentialsReadOnly(session.WorkspaceRoot); err == nil {
				b.runtimes.SetReasoningLanguage(session.ID, effective.ReasoningLanguage())
			}
		}
	}
	summary, err := loadProviderSummaryForScope(request.Scope, request.WorkspaceRoot)
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read agent preferences")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (b *bridgeServer) setProviderKey(w http.ResponseWriter, r *http.Request) {
	var request setProviderKeyRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid provider key request")
		return
	}
	if err := updateProviderKey(request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "provider key could not be updated")
		return
	}
	summary, err := loadProviderSummary()
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read provider summary")
		return
	}
	writeJSON(w, http.StatusOK, summary)
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

type approveRequest struct {
	ID    string `json:"id"`
	Allow bool   `json:"allow"`
}

type answerQuestionRequest struct {
	ID      string                    `json:"id"`
	Answers []desktopbridge.AskAnswer `json:"answers"`
}

type answerMCPInteractionRequest struct {
	ID      string         `json:"id"`
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

type renameSessionRequest struct {
	Title string `json:"title"`
}

// deleteSessionResponse is the wire body for a completed delete. The host uses
// it to tell "removed" apart from "the service answered something else".
type deleteSessionResponse struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Deleted         bool   `json:"deleted"`
	SessionID       string `json:"sessionId"`
}

type attachFileRequest struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
}

type workspaceRequest struct {
	Path string `json:"path"`
}

type workspaceFileRequest struct {
	Path string `json:"path"`
}

type workspaceChangeDetailRequest struct {
	Path string `json:"path"`
}

type workspaceFileRevertCommitRequest struct {
	PlanID     string `json:"planId"`
	Resolution string `json:"resolution"`
}

type workspaceFileRevertUndoRequest struct {
	TransactionID string `json:"transactionId"`
}

type codeRewindPreviewRequest struct {
	Turn int `json:"turn"`
}

type codeRewindCommitRequest struct {
	PlanID                 string `json:"planId"`
	ConfirmPartialCoverage bool   `json:"confirmPartialCoverage"`
}

type workspaceListResponse struct {
	ProtocolVersion int                            `json:"protocolVersion"`
	Path            string                         `json:"path"`
	Entries         []desktopbridge.WorkspaceEntry `json:"entries"`
	Truncated       bool                           `json:"truncated"`
}

type workspaceFileResponse struct {
	ProtocolVersion int                                `json:"protocolVersion"`
	Preview         desktopbridge.WorkspaceFilePreview `json:"preview"`
}

type workspaceChangesResponse struct {
	ProtocolVersion int                            `json:"protocolVersion"`
	Changes         desktopbridge.WorkspaceChanges `json:"changes"`
}

type workspaceChangeDetailResponse struct {
	ProtocolVersion int                                 `json:"protocolVersion"`
	Detail          desktopbridge.WorkspaceChangeDetail `json:"detail"`
}

type workspaceFileRevertPlanResponse struct {
	ProtocolVersion int                                   `json:"protocolVersion"`
	Plan            desktopbridge.WorkspaceFileRevertPlan `json:"plan"`
}

type workspaceFileRevertResultResponse struct {
	ProtocolVersion int                                     `json:"protocolVersion"`
	Result          desktopbridge.WorkspaceFileRevertResult `json:"result"`
}

type workspaceCheckpointsResponse struct {
	ProtocolVersion int                                     `json:"protocolVersion"`
	Checkpoints     []desktopbridge.WorkspaceCheckpointView `json:"checkpoints"`
}

type codeRewindPlanResponse struct {
	ProtocolVersion int                                   `json:"protocolVersion"`
	Plan            desktopbridge.WorkspaceCodeRewindPlan `json:"plan"`
}

type attachmentResponse struct {
	ProtocolVersion int                          `json:"protocolVersion"`
	Attachment      desktopbridge.AttachmentView `json:"attachment"`
}

type historyResponse struct {
	ProtocolVersion int                            `json:"protocolVersion"`
	Sequence        uint64                         `json:"sequence"`
	Session         desktopbridge.SessionView      `json:"session"`
	Messages        []desktopbridge.HistoryMessage `json:"messages"`
	StartIndex      int                            `json:"startIndex"`
	TotalMessages   int                            `json:"totalMessages"`
}

func (b *bridgeServer) openSession(w http.ResponseWriter, r *http.Request) {
	var request openSessionRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid open_session request")
		return
	}
	view, err := b.runtimes.Open(r.Context(), desktopbridge.OpenRequest{SessionID: request.SessionID, WorkspaceRoot: request.WorkspaceRoot})
	if err != nil {
		b.writeRuntimeError(w, err, "unable to open desktop bridge session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "session": view})
}

func (b *bridgeServer) switchSession(w http.ResponseWriter, r *http.Request) {
	var request openSessionRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid switch_session request")
		return
	}
	view, err := b.runtimes.Switch(r.Context(), desktopbridge.OpenRequest{SessionID: request.SessionID, WorkspaceRoot: request.WorkspaceRoot})
	if err != nil {
		b.writeRuntimeError(w, err, "unable to switch desktop bridge session")
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
	// Capture the event boundary before reading metrics. Later events must stay
	// replayable instead of being skipped by a newer cursor with older values.
	sequence := b.events.LatestSequence()
	response := map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "sequence": sequence, "session": view}
	if metrics, ok := b.runtimes.SessionMetrics(view.ID); ok {
		response["metrics"] = metrics
	}
	writeJSON(w, http.StatusOK, response)
}

func (b *bridgeServer) sessionBalance(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	balance, found, err := b.runtimes.SessionBalance(r.Context(), sessionID)
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "balance_unavailable", "wallet balance is unavailable")
		return
	}
	if !found {
		writeProtocolError(w, http.StatusNotFound, "not_found", "desktop bridge session not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "balance": balance})
}

type sessionMCPRuntimeActionRequest struct {
	Name   string `json:"name"`
	Action string `json:"action"`
}

func (b *bridgeServer) sessionMCPRuntimeAction(w http.ResponseWriter, r *http.Request) {
	var request sessionMCPRuntimeActionRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid MCP runtime action")
		return
	}
	count, err := b.runtimes.MCPRuntimeAction(r.PathValue("id"), request.Name, request.Action)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to change the current session MCP connection")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"protocolVersion": desktopbridge.ProtocolVersion,
		"name":            strings.TrimSpace(request.Name),
		"action":          request.Action,
		"toolCount":       count,
	})
}

type sessionMCPClearAuthenticationRequest struct {
	Name string `json:"name"`
}

func (b *bridgeServer) sessionMCPClearAuthentication(w http.ResponseWriter, r *http.Request) {
	var request sessionMCPClearAuthenticationRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid MCP credential request")
		return
	}
	changed, err := b.runtimes.ClearMCPAuthentication(r.PathValue("id"), request.Name)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to clear MCP authentication")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"protocolVersion": desktopbridge.ProtocolVersion,
		"name":            strings.TrimSpace(request.Name),
		"changed":         changed,
	})
}

type sessionMCPOAuthStartRequest struct {
	Name string `json:"name"`
}

func (b *bridgeServer) sessionMCPOAuthStart(w http.ResponseWriter, r *http.Request) {
	var request sessionMCPOAuthStartRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid MCP authorization request")
		return
	}
	flowID, err := b.runtimes.StartMCPOAuth(r.PathValue("id"), request.Name)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to start MCP authorization")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"protocolVersion": desktopbridge.ProtocolVersion,
		"flowId":          flowID,
		"name":            strings.TrimSpace(request.Name),
		"status":          "pending",
	})
}

func (b *bridgeServer) sessionMCPOAuthStatus(w http.ResponseWriter, r *http.Request) {
	flow, err := b.runtimes.MCPOAuthStatus(r.PathValue("id"), r.PathValue("flow"))
	if err != nil {
		b.writeRuntimeError(w, err, "unable to read MCP authorization status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"protocolVersion": desktopbridge.ProtocolVersion,
		"flowId":          flow.ID,
		"name":            flow.Name,
		"status":          flow.Status,
	})
}

func (b *bridgeServer) sessionMCPOAuthCancel(w http.ResponseWriter, r *http.Request) {
	if err := b.runtimes.CancelMCPOAuth(r.PathValue("id"), r.PathValue("flow")); err != nil {
		b.writeRuntimeError(w, err, "unable to cancel MCP authorization")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"protocolVersion": desktopbridge.ProtocolVersion,
		"flowId":          r.PathValue("flow"),
		"status":          "canceled",
	})
}

func (b *bridgeServer) setSessionModel(w http.ResponseWriter, r *http.Request) {
	var request setSessionModelRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid session model request")
		return
	}
	active, ok := b.runtimes.Snapshot()
	if !ok || active.ID != r.PathValue("id") {
		writeProtocolError(w, http.StatusNotFound, "not_found", "desktop bridge session not found")
		return
	}
	model, err := resolveConfiguredSessionModel(active.WorkspaceRoot, request.Model)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "model_unavailable", "model is not configured for this workspace")
		return
	}
	view, err := b.runtimes.SetSessionModel(r.Context(), active.ID, model)
	if err != nil {
		if errors.Is(err, desktopbridge.ErrSessionModelSwitch) {
			writeProtocolError(w, http.StatusBadGateway, "model_switch_failed", "model could not be started; the previous session model was restored")
			return
		}
		if errors.Is(err, desktopbridge.ErrSessionModelRecover) {
			writeProtocolError(w, http.StatusInternalServerError, "session_model_recovery_failed", "model switch failed and the previous session could not be restored")
			return
		}
		b.writeRuntimeError(w, err, "unable to change the active session model")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "session": view})
}

func (b *bridgeServer) sessionHistory(w http.ResponseWriter, r *http.Request) {
	history, err := b.runtimes.History(r.PathValue("id"))
	if err != nil {
		b.writeRuntimeError(w, err, "unable to read desktop bridge session history")
		return
	}
	writeJSON(w, http.StatusOK, historyResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Sequence:        b.events.LatestSequence(),
		Session:         history.Session,
		Messages:        history.Messages,
		StartIndex:      history.StartIndex,
		TotalMessages:   history.TotalMessages,
	})
}

func (b *bridgeServer) renameSession(w http.ResponseWriter, r *http.Request) {
	var request renameSessionRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid rename_session request")
		return
	}
	view, err := b.runtimes.RenameSession(r.PathValue("id"), request.Title)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to rename desktop bridge session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "session": view})
}

// deleteSession removes the owned session's durable artifacts. The host drops
// its catalog entry only after this succeeds, so a failure never leaves a
// listed conversation the user cannot reopen.
func (b *bridgeServer) deleteSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("id"))
	if err := b.deleteOwnedOrInterruptedSession(r.Context(), sessionID); err != nil {
		b.writeRuntimeError(w, err, "unable to delete desktop bridge session")
		return
	}
	writeJSON(w, http.StatusOK, deleteSessionResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Deleted:         true,
		SessionID:       sessionID,
	})
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

func (b *bridgeServer) attachFile(w http.ResponseWriter, r *http.Request) {
	var request attachFileRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid attach_file request")
		return
	}
	if request.SessionID != r.PathValue("id") {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "attachment session does not match route")
		return
	}
	attachment, err := b.runtimes.AttachFile(request.SessionID, request.Path)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to attach file to desktop bridge session")
		return
	}
	writeJSON(w, http.StatusCreated, attachmentResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Attachment:      attachment,
	})
}

func (b *bridgeServer) workspaceList(w http.ResponseWriter, r *http.Request) {
	var request workspaceRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid workspace request")
		return
	}
	workspace, err := b.runtimes.Workspace(r.PathValue("id"), request.Path)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to list desktop bridge workspace")
		return
	}
	writeJSON(w, http.StatusOK, workspaceListResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Path:            workspace.Path,
		Entries:         workspace.Entries,
		Truncated:       workspace.Truncated,
	})
}

func (b *bridgeServer) workspaceFile(w http.ResponseWriter, r *http.Request) {
	var request workspaceFileRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid workspace file request")
		return
	}
	preview, err := b.runtimes.WorkspaceFilePreview(r.PathValue("id"), request.Path)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to preview desktop bridge workspace file")
		return
	}
	writeJSON(w, http.StatusOK, workspaceFileResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Preview:         preview,
	})
}

func (b *bridgeServer) workspaceChanges(w http.ResponseWriter, r *http.Request) {
	changes, err := b.runtimes.WorkspaceChanges(r.PathValue("id"))
	if err != nil {
		b.writeRuntimeError(w, err, "unable to list desktop bridge workspace changes")
		return
	}
	writeJSON(w, http.StatusOK, workspaceChangesResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Changes: changes})
}

func (b *bridgeServer) workspaceChangeDetail(w http.ResponseWriter, r *http.Request) {
	var request workspaceChangeDetailRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid workspace change detail request")
		return
	}
	detail, err := b.runtimes.WorkspaceChangeDetail(r.PathValue("id"), request.Path)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to read desktop bridge workspace change detail")
		return
	}
	writeJSON(w, http.StatusOK, workspaceChangeDetailResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Detail: detail})
}

func (b *bridgeServer) workspaceFileRevertPreview(w http.ResponseWriter, r *http.Request) {
	var request workspaceChangeDetailRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid workspace file revert preview request")
		return
	}
	plan, err := b.runtimes.PrepareWorkspaceFileRevert(r.PathValue("id"), request.Path)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to prepare workspace file revert")
		return
	}
	writeJSON(w, http.StatusOK, workspaceFileRevertPlanResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Plan: plan})
}

func (b *bridgeServer) workspaceFileRevertCommit(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(requestIDHeader) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "workspace file revert requires a request ID")
		return
	}
	var request workspaceFileRevertCommitRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid workspace file revert commit request")
		return
	}
	result, err := b.runtimes.CommitWorkspaceFileRevert(r.PathValue("id"), request.PlanID, request.Resolution)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to commit workspace file revert")
		return
	}
	writeJSON(w, http.StatusOK, workspaceFileRevertResultResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Result: result})
}

func (b *bridgeServer) workspaceFileRevertUndo(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(requestIDHeader) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "workspace file revert undo requires a request ID")
		return
	}
	var request workspaceFileRevertUndoRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid workspace file revert undo request")
		return
	}
	result, err := b.runtimes.UndoWorkspaceFileRevert(r.PathValue("id"), request.TransactionID)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to undo workspace file revert")
		return
	}
	writeJSON(w, http.StatusOK, workspaceFileRevertResultResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Result: result})
}

func (b *bridgeServer) workspaceCheckpoints(w http.ResponseWriter, r *http.Request) {
	checkpoints, err := b.runtimes.WorkspaceCheckpoints(r.PathValue("id"))
	if err != nil {
		b.writeRuntimeError(w, err, "unable to list checkpoints")
		return
	}
	writeJSON(w, http.StatusOK, workspaceCheckpointsResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Checkpoints: checkpoints})
}

func (b *bridgeServer) codeRewindPreview(w http.ResponseWriter, r *http.Request) {
	var request codeRewindPreviewRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid code rewind preview request")
		return
	}
	plan, err := b.runtimes.PrepareCodeRewind(r.PathValue("id"), request.Turn)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to prepare code rewind")
		return
	}
	writeJSON(w, http.StatusOK, codeRewindPlanResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Plan: plan})
}

func (b *bridgeServer) codeRewindCommit(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(requestIDHeader) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "code rewind requires a request ID")
		return
	}
	var request codeRewindCommitRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid code rewind commit request")
		return
	}
	result, err := b.runtimes.CommitCodeRewind(r.PathValue("id"), request.PlanID, request.ConfirmPartialCoverage)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to commit code rewind")
		return
	}
	writeJSON(w, http.StatusOK, workspaceFileRevertResultResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Result: result})
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

func (b *bridgeServer) approve(w http.ResponseWriter, r *http.Request) {
	var request approveRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil || strings.TrimSpace(request.ID) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid approval request")
		return
	}
	view, err := b.runtimes.Approve(r.PathValue("id"), request.ID, request.Allow)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to answer desktop bridge approval")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "session": view})
}

func (b *bridgeServer) answerQuestion(w http.ResponseWriter, r *http.Request) {
	var request answerQuestionRequest
	if err := decodeJSONBody(w, r, 256<<10, &request); err != nil || strings.TrimSpace(request.ID) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid question answer request")
		return
	}
	view, err := b.runtimes.AnswerQuestion(r.PathValue("id"), request.ID, request.Answers)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to answer desktop bridge question")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "session": view})
}

func (b *bridgeServer) answerMCPInteraction(w http.ResponseWriter, r *http.Request) {
	var request answerMCPInteractionRequest
	if err := decodeJSONBody(w, r, 256<<10, &request); err != nil || strings.TrimSpace(request.ID) == "" || strings.TrimSpace(request.Action) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid MCP interaction answer request")
		return
	}
	view, err := b.runtimes.AnswerMCPInteraction(r.PathValue("id"), request.ID, request.Action, request.Content)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to answer desktop bridge MCP interaction")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "session": view})
}

func (b *bridgeServer) replayPendingPrompts(w http.ResponseWriter, r *http.Request) {
	view, err := b.runtimes.ReplayPendingPrompts(r.PathValue("id"))
	if err != nil {
		b.writeRuntimeError(w, err, "unable to replay desktop bridge prompts")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"protocolVersion": desktopbridge.ProtocolVersion, "session": view})
}

func (b *bridgeServer) writeRuntimeError(w http.ResponseWriter, err error, message string) {
	status, code := http.StatusInternalServerError, "internal"
	switch {
	case errors.Is(err, ErrKnownSessionMissing):
		status, code = http.StatusConflict, "session_missing"
	case errors.Is(err, ErrKnownSessionDeleting):
		status, code = http.StatusConflict, "session_deleting"
	case errors.Is(err, ErrKnownSessionDeleted):
		status, code = http.StatusConflict, "session_deleted"
	case errors.Is(err, desktopbridge.ErrInvalidSessionID), errors.Is(err, desktopbridge.ErrInvalidInput), errors.Is(err, desktopbridge.ErrInvalidAttachment), errors.Is(err, desktopbridge.ErrInvalidWorkspacePath), errors.Is(err, desktopbridge.ErrInvalidTitle):
		status, code = http.StatusBadRequest, "invalid_request"
	case errors.Is(err, desktopbridge.ErrSessionConflict), errors.Is(err, desktopbridge.ErrOpenInProgress):
		status, code = http.StatusConflict, "conflict"
	case errors.Is(err, desktopbridge.ErrSessionNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, desktopbridge.ErrClosed):
		status, code = http.StatusServiceUnavailable, "shutting_down"
	case errors.Is(err, desktopbridge.ErrSessionModelRecover):
		status, code = http.StatusInternalServerError, "session_model_recovery_failed"
	case errors.Is(err, desktopbridge.ErrSessionModelSwitch):
		status, code = http.StatusBadGateway, "model_switch_failed"
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
