package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/netclient"
	"reasonix/internal/remote"
	"reasonix/internal/remote/sftpfs"
)

type remoteConnectRequest struct {
	Name             string `json:"name"`
	TrustFingerprint string `json:"trustFingerprint,omitempty"`
	Password         string `json:"password,omitempty"`
	Passphrase       string `json:"passphrase,omitempty"`
}

type remoteConnectResponse struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Status          string `json:"status"`
	Host            string `json:"host,omitempty"`
	Address         string `json:"address,omitempty"`
	KeyType         string `json:"keyType,omitempty"`
	Fingerprint     string `json:"fingerprint,omitempty"`
	Message         string `json:"message,omitempty"`
}

type remoteDisconnectRequest struct {
	Name string `json:"name"`
}

type remoteDisconnectResponse struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Disconnected    bool   `json:"disconnected"`
	Name            string `json:"name"`
}

type remoteBrowseRequest struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

type remoteBrowseEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Symlink bool   `json:"symlink"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
}

type remoteBrowseResponse struct {
	ProtocolVersion int                 `json:"protocolVersion"`
	Path            string              `json:"path"`
	ParentPath      string              `json:"parentPath"`
	Entries         []remoteBrowseEntry `json:"entries"`
	Truncated       bool                `json:"truncated"`
}

type remotePreviewRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type remotePreviewResponse struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Path            string `json:"path"`
	Kind            string `json:"kind"`
	Content         string `json:"content"`
	Revision        string `json:"revision"`
	Truncated       bool   `json:"truncated"`
}

type remoteSaveRequest struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Content  string `json:"content"`
}

type remoteSaveResponse struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Path            string `json:"path"`
	Revision        string `json:"revision"`
}

type previewRemoteSessions struct {
	mu      sync.Mutex
	saveMu  sync.Mutex
	clients map[string]*remote.Client
	peers   map[string]remote.HostKeyQuestion
}

func newPreviewRemoteSessions() *previewRemoteSessions {
	return &previewRemoteSessions{clients: map[string]*remote.Client{}, peers: map[string]remote.HostKeyQuestion{}}
}

func (m *previewRemoteSessions) get(name string) (*remote.Client, remote.HostKeyQuestion) {
	if m == nil {
		return nil, remote.HostKeyQuestion{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.clients[name], m.peers[name]
}

func (m *previewRemoteSessions) put(name string, client *remote.Client, peer remote.HostKeyQuestion) {
	m.mu.Lock()
	previous := m.clients[name]
	m.clients[name] = client
	m.peers[name] = peer
	m.mu.Unlock()
	if previous != nil && previous != client {
		_ = previous.Close()
	}
}

func (m *previewRemoteSessions) disconnect(name string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	client := m.clients[name]
	delete(m.clients, name)
	delete(m.peers, name)
	m.mu.Unlock()
	if client != nil {
		_ = client.Close()
	}
}

func (m *previewRemoteSessions) closeAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	clients := m.clients
	m.clients = map[string]*remote.Client{}
	m.peers = map[string]remote.HostKeyQuestion{}
	m.mu.Unlock()
	for _, client := range clients {
		if client != nil {
			_ = client.Close()
		}
	}
}

func (b *bridgeServer) connectRemoteHost(w http.ResponseWriter, r *http.Request) {
	var input remoteConnectRequest
	if err := decodeJSONBody(w, r, 16<<10, &input); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid SSH connection request")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.TrustFingerprint = strings.TrimSpace(input.TrustFingerprint)
	if !previewProviderName.MatchString(input.Name) || len(input.TrustFingerprint) > 256 || len(input.Password) > 4096 || len(input.Passphrase) > 4096 || strings.ContainsAny(input.Password, "\r\n") || strings.ContainsAny(input.Passphrase, "\r\n") {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid SSH connection request")
		return
	}
	if current, peer := b.remoteSessions.get(input.Name); current != nil && current.Status().Status == remote.StatusConnected {
		writeJSON(w, http.StatusOK, remoteConnectResponse{
			ProtocolVersion: desktopbridge.ProtocolVersion, Status: "connected", Host: peer.Host,
			Address: peer.Address, KeyType: peer.KeyType, Fingerprint: peer.Fingerprint,
		})
		return
	}
	b.remoteSessions.disconnect(input.Name)

	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		writeJSON(w, http.StatusOK, remoteConnectResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Status: "failed", Message: "config_unavailable"})
		return
	}
	if _, ok := cfg.RemoteHost(input.Name); !ok {
		writeProtocolError(w, http.StatusNotFound, "not_found", "saved SSH host not found")
		return
	}
	sshConfig, err := remote.LoadUserSSHConfig()
	if err != nil {
		writeJSON(w, http.StatusOK, remoteConnectResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Status: "failed", Message: "ssh_config_unavailable"})
		return
	}
	host, err := remote.ResolveHost(cfg, input.Name, sshConfig)
	if err != nil {
		writeJSON(w, http.StatusOK, remoteConnectResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Status: "failed", Message: "ssh_config_invalid"})
		return
	}
	jumps, err := remote.ResolveJumpHosts(cfg, host.ProxyJump, sshConfig)
	if err != nil {
		writeJSON(w, http.StatusOK, remoteConnectResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Status: "failed", Message: "ssh_jump_invalid"})
		return
	}
	dialer, err := netclient.NewStreamDialer(cfg.NetworkProxySpec())
	if err != nil {
		writeJSON(w, http.StatusOK, remoteConnectResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Status: "failed", Message: "network_proxy_invalid"})
		return
	}

	var pending remote.HostKeyQuestion
	var verified remote.HostKeyQuestion
	policy := &remote.HostKeyPolicy{
		Prompt: func(_ context.Context, question remote.HostKeyQuestion) (bool, error) {
			if input.TrustFingerprint == question.Fingerprint {
				return true, nil
			}
			pending = question
			return false, nil
		},
		Verified: func(question remote.HostKeyQuestion) {
			if question.Host == host.Label() {
				verified = question
			}
		},
	}
	client, err := remote.New(remote.Options{
		Host: host, Auth: previewRemoteAuth(host, input.Password, input.Passphrase),
		JumpHosts: previewRemoteJumpOptions(jumps), HostKeys: policy, Dialer: dialer,
	})
	if err != nil {
		writeJSON(w, http.StatusOK, remoteConnectResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Status: "failed", Message: "ssh_client_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		_ = client.Close()
		if pending.Fingerprint != "" {
			writeJSON(w, http.StatusOK, remoteConnectResponse{
				ProtocolVersion: desktopbridge.ProtocolVersion, Status: "host_key_confirmation",
				Host: pending.Host, Address: pending.Address, KeyType: pending.KeyType, Fingerprint: pending.Fingerprint,
			})
			return
		}
		message := "connection_failed"
		switch {
		case errors.Is(err, remote.ErrAuthFailed):
			message = "authentication_failed"
		case errors.Is(err, remote.ErrHostKeyMismatch):
			message = "host_key_mismatch"
		case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
			message = "connection_timeout"
		}
		writeJSON(w, http.StatusOK, remoteConnectResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Status: "failed", Message: message})
		return
	}
	if verified.Fingerprint == "" {
		_ = client.Close()
		writeJSON(w, http.StatusOK, remoteConnectResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Status: "failed", Message: "host_key_not_verified"})
		return
	}
	b.remoteSessions.put(input.Name, client, verified)
	writeJSON(w, http.StatusOK, remoteConnectResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion, Status: "connected", Host: host.Label(),
		Address: verified.Address, KeyType: verified.KeyType, Fingerprint: verified.Fingerprint,
	})
}

func (b *bridgeServer) disconnectRemoteHost(w http.ResponseWriter, r *http.Request) {
	var input remoteDisconnectRequest
	if err := decodeJSONBody(w, r, 8<<10, &input); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid SSH disconnect request")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if !previewProviderName.MatchString(input.Name) {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid SSH disconnect request")
		return
	}
	b.remoteSessions.disconnect(input.Name)
	writeJSON(w, http.StatusOK, remoteDisconnectResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Disconnected: true, Name: input.Name})
}

func (b *bridgeServer) browseRemoteHost(w http.ResponseWriter, r *http.Request) {
	var input remoteBrowseRequest
	if err := decodeJSONBody(w, r, 12<<10, &input); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid remote directory request")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Path = strings.TrimSpace(input.Path)
	if !previewProviderName.MatchString(input.Name) || len(input.Path) > 4096 || strings.ContainsRune(input.Path, '\x00') {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid remote directory request")
		return
	}
	client, _ := b.remoteSessions.get(input.Name)
	if client == nil || client.Status().Status != remote.StatusConnected {
		writeProtocolError(w, http.StatusConflict, "conflict", "remote host is not connected")
		return
	}
	if input.Path == "" {
		cfg, err := configpkg.LoadUserConfigReadOnly()
		if err == nil {
			if host, ok := cfg.RemoteHost(input.Name); ok {
				input.Path = host.Workspace
			}
		}
		if input.Path == "" {
			input.Path = "~"
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	fsys, err := client.SFTP()
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_browse_failed", "could not open remote directory service")
		return
	}
	resolved, err := fsys.ResolvePath(ctx, input.Path)
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_browse_failed", "could not resolve remote directory")
		return
	}
	entries, err := fsys.List(ctx, resolved)
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_browse_failed", "could not list remote directory")
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	const maxEntries = 500
	truncated := len(entries) > maxEntries
	if truncated {
		entries = entries[:maxEntries]
	}
	out := remoteBrowseResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Path:            resolved,
		ParentPath:      path.Dir(resolved),
		Entries:         make([]remoteBrowseEntry, 0, len(entries)),
		Truncated:       truncated,
	}
	for _, entry := range entries {
		out.Entries = append(out.Entries, remoteBrowseEntry{
			Name: entry.Name, Path: entry.Path, IsDir: entry.IsDir, Symlink: entry.Symlink,
			Size: entry.Size, ModTime: entry.ModTime,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (b *bridgeServer) previewRemoteFile(w http.ResponseWriter, r *http.Request) {
	var input remotePreviewRequest
	if err := decodeJSONBody(w, r, 12<<10, &input); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid remote file request")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Path = strings.TrimSpace(input.Path)
	if !previewProviderName.MatchString(input.Name) || input.Path == "" || len(input.Path) > 4096 || strings.ContainsRune(input.Path, '\x00') {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid remote file request")
		return
	}
	client, _ := b.remoteSessions.get(input.Name)
	if client == nil || client.Status().Status != remote.StatusConnected {
		writeProtocolError(w, http.StatusConflict, "conflict", "remote host is not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	fsys, err := client.SFTP()
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_preview_failed", "could not open remote file service")
		return
	}
	resolved, err := fsys.RealPath(ctx, input.Path)
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_preview_failed", "could not resolve remote file")
		return
	}
	entry, err := fsys.Stat(ctx, resolved)
	if err != nil || entry.IsDir || !entry.Mode.IsRegular() {
		writeProtocolError(w, http.StatusBadRequest, "remote_preview_failed", "remote path is not a readable file")
		return
	}
	const maxPreviewBytes = int64(1 << 20)
	data, truncated, kind, err := fsys.ReadFile(ctx, resolved, maxPreviewBytes)
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_preview_failed", "could not read remote file")
		return
	}
	kindName := "text"
	content := string(data)
	revision := ""
	if kind != sftpfs.KindText {
		kindName = "binary"
		content = ""
	} else {
		revision = remoteContentRevision(data)
	}
	writeJSON(w, http.StatusOK, remotePreviewResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Path:            resolved,
		Kind:            kindName,
		Content:         content,
		Revision:        revision,
		Truncated:       truncated,
	})
}

func (b *bridgeServer) saveRemoteFile(w http.ResponseWriter, r *http.Request) {
	var input remoteSaveRequest
	if err := decodeJSONBody(w, r, (6<<20)+16<<10, &input); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid remote file save request")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Path = strings.TrimSpace(input.Path)
	if !previewProviderName.MatchString(input.Name) || input.Path == "" || len(input.Path) > 4096 || strings.ContainsRune(input.Path, '\x00') || len(input.Content) > 1<<20 || !utf8.ValidString(input.Content) || strings.ContainsRune(input.Content, '\x00') {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid remote file save request")
		return
	}
	if len(input.Revision) != sha256.Size*2 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid remote file revision")
		return
	}
	if _, err := hex.DecodeString(input.Revision); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid remote file revision")
		return
	}
	if b.remoteSessions == nil {
		writeProtocolError(w, http.StatusConflict, "conflict", "remote host is not connected")
		return
	}
	// Keep the revision check and replacement in one bridge-local critical
	// section so two Preview saves cannot both accept the same old revision.
	b.remoteSessions.saveMu.Lock()
	defer b.remoteSessions.saveMu.Unlock()
	client, _ := b.remoteSessions.get(input.Name)
	if client == nil || client.Status().Status != remote.StatusConnected {
		writeProtocolError(w, http.StatusConflict, "conflict", "remote host is not connected")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	fsys, err := client.SFTP()
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_save_failed", "could not open remote file service")
		return
	}
	resolved, err := fsys.RealPath(ctx, input.Path)
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_save_failed", "could not resolve remote file")
		return
	}
	entry, err := fsys.Stat(ctx, resolved)
	if err != nil || entry.IsDir || !entry.Mode.IsRegular() {
		writeProtocolError(w, http.StatusBadRequest, "remote_save_failed", "remote path is not a regular file")
		return
	}
	current, truncated, kind, err := fsys.ReadFile(ctx, resolved, 1<<20)
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_save_failed", "could not read remote file")
		return
	}
	if truncated || kind != sftpfs.KindText {
		writeProtocolError(w, http.StatusConflict, "remote_file_changed", "remote file is no longer an editable text file")
		return
	}
	if remoteContentRevision(current) != strings.ToLower(input.Revision) {
		writeProtocolError(w, http.StatusConflict, "remote_file_changed", "remote file changed since it was loaded")
		return
	}
	if err := fsys.WriteFileAtomic(ctx, resolved, []byte(input.Content), entry.Mode.Perm()); err != nil {
		writeProtocolError(w, http.StatusBadGateway, "remote_save_failed", "could not save remote file")
		return
	}
	writeJSON(w, http.StatusOK, remoteSaveResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Path:            resolved,
		Revision:        remoteContentRevision([]byte(input.Content)),
	})
}

func remoteContentRevision(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func previewRemoteAuth(host remote.ResolvedHost, requestPassword, requestPassphrase string) remote.AuthOptions {
	auth := remote.AuthOptions{}
	if requestPassword != "" {
		// A credential prompt is strictly per connection attempt; it may
		// override a stale stored credential without editing Preview config.
		auth.Password = func() (string, error) { return requestPassword, nil }
	} else if host.PasswordEnv != "" {
		key := host.PasswordEnv
		auth.Password = func() (string, error) { return configpkg.ResolveCredential(key).Value, nil }
	}
	if requestPassphrase != "" {
		auth.Passphrase = func() (string, error) { return requestPassphrase, nil }
	} else if host.PassphraseEnv != "" {
		key := host.PassphraseEnv
		auth.Passphrase = func() (string, error) { return configpkg.ResolveCredential(key).Value, nil }
	}
	return auth
}

func previewRemoteJumpOptions(hosts []remote.ResolvedHost) []remote.JumpHostOptions {
	options := make([]remote.JumpHostOptions, 0, len(hosts))
	for _, host := range hosts {
		options = append(options, remote.JumpHostOptions{Host: host, Auth: previewRemoteAuth(host, "", "")})
	}
	return options
}
