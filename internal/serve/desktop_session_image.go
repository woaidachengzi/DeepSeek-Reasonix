package serve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/netclient"
)

type desktopSessionImageRequest struct {
	SessionPath string `json:"sessionPath"`
	Workspace   string `json:"workspace"`
	Source      string `json:"source"`
}
type desktopSessionImageView struct {
	ProtocolVersion int                              `json:"protocolVersion"`
	SessionPath     string                           `json:"sessionPath"`
	Workspace       string                           `json:"workspace"`
	Image           desktopbridge.WorkspaceImageView `json:"image"`
}

// Admission bounds body buffers and raster decode memory, including local and
// inline images. The separate public-image transport still owns its SSRF gate.
var desktopSessionImageSlots = make(chan struct{}, 2)

type desktopImageReader func(context.Context, *os.Root, string, string) desktopbridge.WorkspaceImageView

func (s *Server) desktopSessionImage(w http.ResponseWriter, r *http.Request) {
	s.desktopSessionImageWithReader(w, r, readDesktopSessionImage)
}

// Capture under bindMu, release it for decode/network, revalidate before
// publishing. The reader seam is backend-only, used by deterministic race tests.
func (s *Server) desktopSessionImageWithReader(w http.ResponseWriter, r *http.Request, read desktopImageReader) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	stopBody := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
	defer stopBody()
	select {
	case desktopSessionImageSlots <- struct{}{}:
		defer func() { <-desktopSessionImageSlots }()
	case <-ctx.Done():
		http.Error(w, "remote image request expired", 408)
		return
	}
	var input desktopSessionImageRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 24<<20))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.SessionPath == "" || !desktopImageField(input.SessionPath, 32768) || input.Workspace == "" || !desktopImageField(input.Workspace, 4096) || !filepath.IsAbs(input.Workspace) || strings.TrimSpace(input.Source) == "" || len(input.Source) > desktopbridge.WorkspaceImageSourceLimit || !utf8.ValidString(input.Source) || strings.ContainsRune(input.Source, 0) {
		http.Error(w, "select a remote session image", 400)
		return
	}
	if !s.bindMu.TryLock() {
		http.Error(w, "remote session is changing", 409)
		return
	}
	grant, err := s.captureDesktopImageGrant(input)
	s.bindMu.Unlock()
	if err != nil {
		http.Error(w, "remote image workspace is unavailable", 409)
		return
	}
	defer grant.root.Close()
	if ctx.Err() != nil {
		http.Error(w, "remote image request expired", 408)
		return
	}
	image := read(ctx, grant.root, input.Source, input.Workspace)
	// A remote filesystem path or browser URL is not a local opener grant.
	image.OpenHref = ""
	if !s.bindMu.TryLock() {
		http.Error(w, "remote session is changing", 409)
		return
	}
	defer s.bindMu.Unlock()
	if ctx.Err() != nil || !s.desktopImageGrantValid(grant) {
		http.Error(w, "remote image ownership changed", 409)
		return
	}
	data, err := json.Marshal(desktopSessionImageView{1, input.SessionPath, input.Workspace, image})
	if err != nil || len(data) > 12<<20 {
		http.Error(w, "remote image exceeds display budget", 422)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

type desktopImageGrant struct {
	input                          desktopSessionImageRequest
	foreground, selected           control.SessionAPI
	foregroundPath, base, realRoot string
	detached                       *detachedSession
	detachedController             control.SessionAPI
	external                       bool
	root                           *os.Root
	info                           os.FileInfo
	sessionInfo                    os.FileInfo
}

// Caller holds bindMu. A shared global session directory does not imply that
// every saved session's attachments belong to this Serve's workspace.
func (s *Server) captureDesktopImageGrant(input desktopSessionImageRequest) (*desktopImageGrant, error) {
	path, err := s.resolveSessionPath(input.SessionPath)
	if err != nil || agent.CanonicalSessionPath(path) != input.SessionPath {
		return nil, errors.New("session unavailable")
	}
	foreground := s.ctl()
	if foreground == nil {
		return nil, errors.New("controller unavailable")
	}
	g := &desktopImageGrant{input: input, foreground: foreground, foregroundPath: agent.CanonicalSessionPath(foreground.SessionPath())}
	g.sessionInfo, err = os.Stat(path)
	if err != nil || !g.sessionInfo.Mode().IsRegular() {
		return nil, errors.New("session unavailable")
	}
	g.external = s.sessionMirrored(path) || leaseHeldByForeignRuntime(path)
	if !g.external {
		if input.SessionPath == g.foregroundPath {
			g.selected = foreground
		} else {
			s.detachedMu.Lock()
			g.detached = s.detached[input.SessionPath]
			if g.detached != nil && !g.detached.retiring {
				g.selected = g.detached.ctrl
				g.detachedController = g.detached.ctrl
			}
			s.detachedMu.Unlock()
			if g.detached != nil && g.selected == nil {
				return nil, errors.New("retiring controller")
			}
		}
	}
	// The HTTP connection is scoped to the Serve workspace, not arbitrary
	// workspace metadata. Aliases from SSH bootstrap are allowed only if they
	// resolve to this exact root, and are checked again before publication.
	g.base = foreground.WorkspaceRoot()
	g.realRoot, err = canonicalDesktopImageRoot(g.base)
	requested, requestedErr := canonicalDesktopImageRoot(input.Workspace)
	if err != nil || requestedErr != nil || requested != g.realRoot {
		return nil, errors.New("workspace unavailable")
	}
	if g.selected != nil {
		selectedRoot, err := canonicalDesktopImageRoot(g.selected.WorkspaceRoot())
		if err != nil || selectedRoot != g.realRoot || agent.CanonicalSessionPath(g.selected.SessionPath()) != input.SessionPath {
			return nil, errors.New("selected controller changed")
		}
	} else if savedRoot, err := desktopSavedImageRoot(path); err != nil || savedRoot != g.realRoot {
		return nil, errors.New("saved workspace unavailable")
	}
	g.info, err = os.Stat(g.realRoot)
	if err != nil || !g.info.IsDir() {
		return nil, errors.New("workspace unavailable")
	}
	g.root, err = os.OpenRoot(g.realRoot)
	if err != nil {
		return nil, err
	}
	opened, err := g.root.Stat(".")
	if err != nil || !os.SameFile(g.info, opened) {
		g.root.Close()
		return nil, errors.New("workspace changed")
	}
	return g, nil
}

// Caller holds bindMu; detached ownership has its own lock.
func (s *Server) desktopImageGrantValid(g *desktopImageGrant) bool {
	if s.ctl() != g.foreground || agent.CanonicalSessionPath(g.foreground.SessionPath()) != g.foregroundPath || g.foreground.WorkspaceRoot() != g.base {
		return false
	}
	path, err := s.resolveSessionPath(g.input.SessionPath)
	if err != nil || agent.CanonicalSessionPath(path) != g.input.SessionPath || (s.sessionMirrored(path) || leaseHeldByForeignRuntime(path)) != g.external {
		return false
	}
	root, rootErr := canonicalDesktopImageRoot(g.base)
	sessionInfo, sessionErr := os.Stat(path)
	if sessionErr != nil || !sessionInfo.Mode().IsRegular() || !os.SameFile(g.sessionInfo, sessionInfo) {
		return false
	}
	requested, requestedErr := canonicalDesktopImageRoot(g.input.Workspace)
	info, infoErr := os.Stat(g.realRoot)
	if rootErr != nil || requestedErr != nil || infoErr != nil || root != g.realRoot || requested != root || !os.SameFile(g.info, info) {
		return false
	}
	if g.selected != nil {
		selectedRoot, err := canonicalDesktopImageRoot(g.selected.WorkspaceRoot())
		if err != nil || selectedRoot != root || agent.CanonicalSessionPath(g.selected.SessionPath()) != g.input.SessionPath {
			return false
		}
	} else if savedRoot, err := desktopSavedImageRoot(path); err != nil || savedRoot != root {
		return false
	}
	if g.detached != nil {
		s.detachedMu.Lock()
		valid := s.detached[g.input.SessionPath] == g.detached && !g.detached.retiring && g.detached.ctrl == g.detachedController
		s.detachedMu.Unlock()
		return valid
	}
	if !g.external && g.input.SessionPath != g.foregroundPath {
		s.detachedMu.Lock()
		stillSaved := s.detached[g.input.SessionPath] == nil
		s.detachedMu.Unlock()
		return stillSaved
	}
	return true
}

func canonicalDesktopImageRoot(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) || !desktopImageField(path, 4096) {
		return "", errors.New("invalid workspace")
	}
	return filepath.EvalSymlinks(path)
}

// Read only the bounded persisted workspace grant. Never EnsureBranchMeta:
// a spectator must not heal/migrate/write missing historical metadata. Root
// confinement and file identity checks avoid an unbounded sidecar loader race.
func desktopSavedImageRoot(sessionPath string) (string, error) {
	root, err := os.OpenRoot(filepath.Dir(sessionPath))
	if err != nil {
		return "", err
	}
	defer root.Close()
	path := filepath.Base(agent.BranchMetaPath(sessionPath))
	info, err := root.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return "", errors.New("metadata unavailable")
	}
	f, err := root.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Size() != info.Size() {
		return "", errors.New("metadata changed")
	}
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	var meta struct {
		WorkspaceRoot string `json:"workspace_root"`
	}
	if err != nil || len(data) > 1<<20 || !utf8.Valid(data) || json.Unmarshal(data, &meta) != nil {
		return "", errors.New("metadata unavailable")
	}
	return canonicalDesktopImageRoot(meta.WorkspaceRoot)
}

func readDesktopSessionImage(ctx context.Context, root *os.Root, source, workspaceAlias string) desktopbridge.WorkspaceImageView {
	if !desktopbridge.IsRemoteWorkspaceImage(source) {
		return desktopbridge.ReadWorkspaceImageFromRoot(root, source, workspaceAlias)
	}
	cfg, err := config.LoadUserConfigReadOnly()
	if err != nil {
		return desktopbridge.WorkspaceImageView{ErrorCode: "proxy-config"}
	}
	client, err := netclient.NewPublicImageClient(cfg.NetworkProxySpec(), nil)
	if err != nil {
		return desktopbridge.WorkspaceImageView{ErrorCode: "proxy-config"}
	}
	defer client.CloseIdleConnections()
	return desktopbridge.FetchRemoteWorkspaceImage(ctx, source, client)
}

func desktopImageField(value string, limit int) bool {
	return len(value) <= limit && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}
