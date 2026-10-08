package serve

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/remote/controller"
)

func desktopImageFixture(t *testing.T) (*ownershipFixture, *controller.Client, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REASONIX_HOME", t.TempDir())
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := newOwnershipFixture(t)
	ctrl := control.New(control.Options{SessionDir: f.dir, SessionPath: f.active, WorkspaceRoot: workspace})
	t.Cleanup(ctrl.Close)
	f.server.mu.Lock()
	f.server.ctrl = ctrl
	f.server.mu.Unlock()
	f.server.titleProv = nil
	f.server.auth = newAuthGate(config.ServeConfig{AuthMode: "token", Token: "owned-image-token"})
	f.srv.Close()
	f.srv = httptest.NewServer(f.server.Handler())
	t.Cleanup(f.srv.Close)
	c, err := controller.Connect(context.Background(), context.Background(), f.srv.URL, "owned-image-token")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return f, c, workspace
}
func desktopImagePNG(t *testing.T) []byte {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	pixels.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, pixels); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func saveDesktopImageMeta(t *testing.T, path, workspace string) {
	t.Helper()
	if err := agent.UpdateBranchMeta(path, false, func(meta *agent.BranchMeta) error { meta.WorkspaceRoot = workspace; return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestDesktopSessionImageActualServeAuthenticatedScopeAndRaster(t *testing.T) {
	f, c, workspace := desktopImageFixture(t)
	data := desktopImagePNG(t)
	if err := os.WriteFile(filepath.Join(workspace, "screen shot.png"), data, 0600); err != nil {
		t.Fatal(err)
	}
	path := agent.CanonicalSessionPath(f.active)
	status, _ := f.post(t, "/desktop/session-image", desktopSessionImageRequest{path, workspace, "screen%20shot.png"})
	if status != 401 {
		t.Fatal("image bypassed authentication")
	}
	for _, source := range []string{"screen%20shot.png", filepath.Join(workspace, "screen shot.png"), "file://" + filepath.Join(workspace, "screen shot.png"), "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)} {
		v, err := c.SessionImage(context.Background(), workspace, path, source)
		if err != nil || v.Image.Mime != "image/png" || v.Image.Size != int64(len(data)) || !strings.HasPrefix(v.Image.URL, "data:image/png;base64,") {
			t.Fatalf("image %q: %v", source, err)
		}
	}
	alias := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(workspace, alias); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"screen%20shot.png", filepath.Join(alias, "screen shot.png"), "file://" + filepath.Join(alias, "screen shot.png"), filepath.Join(workspace, "screen shot.png")} {
		if v, err := c.SessionImage(context.Background(), alias, path, source); err != nil || v.Workspace != alias || v.Image.ErrorCode != "" {
			t.Fatal("bootstrap logical alias rejected", err)
		}
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.png"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"../outside.png", filepath.Join(outside, "outside.png"), "escape/outside.png", "file://foreign/image.png", "javascript:private-secret", "data:image/svg+xml;base64,PHN2Zy8+", "http://127.0.0.1/private"} {
		v, err := c.SessionImage(context.Background(), workspace, path, source)
		if err != nil || v.Image.URL != "" || v.Image.ErrorCode == "" {
			t.Fatalf("unsafe source %q: %v", source, err)
		}
	}
	if _, err := c.SessionImage(context.Background(), outside, path, "outside.png"); !errors.Is(err, controller.ErrUnavailable) {
		t.Fatal("another workspace authorized", err)
	}
	if f.server.ctl().SessionPath() != f.active {
		t.Fatal("image read switched session")
	}
}

func TestDesktopSessionImageSavedMetadataAndDetachedIsolation(t *testing.T) {
	f, c, workspace := desktopImageFixture(t)
	if err := os.WriteFile(filepath.Join(workspace, "image.png"), desktopImagePNG(t), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.dir, "saved-image.jsonl")
	saveServeTestSession(t, path)
	canonical := agent.CanonicalSessionPath(path)
	if _, err := c.SessionImage(context.Background(), workspace, canonical, "image.png"); !errors.Is(err, controller.ErrUnavailable) {
		t.Fatal("missing saved workspace fell back", err)
	}
	saveDesktopImageMeta(t, path, workspace)
	before, err := os.ReadFile(agent.BranchMetaPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.SessionImage(context.Background(), workspace, canonical, "image.png"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(agent.BranchMetaPath(path))
	if !bytes.Equal(before, after) {
		t.Fatal("spectator rewrote metadata")
	}
	saveDesktopImageMeta(t, path, t.TempDir())
	if _, err := c.SessionImage(context.Background(), workspace, canonical, "image.png"); !errors.Is(err, controller.ErrUnavailable) {
		t.Fatal("global catalogue implied workspace grant", err)
	}
	detached := control.New(control.Options{SessionDir: f.dir, SessionPath: path, WorkspaceRoot: workspace})
	t.Cleanup(detached.Close)
	d := &detachedSession{path: canonical, ctrl: detached}
	f.server.detachedMu.Lock()
	f.server.detached[canonical] = d
	f.server.detachedMu.Unlock()
	if _, err := c.SessionImage(context.Background(), workspace, canonical, "image.png"); err != nil {
		t.Fatal("owned controller root not used", err)
	}
	f.server.detachedMu.Lock()
	d.retiring = true
	f.server.detachedMu.Unlock()
	if _, err := c.SessionImage(context.Background(), workspace, canonical, "image.png"); !errors.Is(err, controller.ErrUnavailable) {
		t.Fatal("retiring controller fell back", err)
	}
	f.server.markMirrored(mirroredSession{path: path, mirrorID: "owned-image-mirror", phase: mirrorPhaseExternal, lastContact: time.Now()})
	if _, err := c.SessionImage(context.Background(), workspace, canonical, "image.png"); !errors.Is(err, controller.ErrUnavailable) {
		t.Fatal("foreign root fell back to own detached grant", err)
	}
	saveDesktopImageMeta(t, path, workspace)
	if _, err := c.SessionImage(context.Background(), workspace, canonical, "image.png"); err != nil || !f.server.sessionMirrored(path) {
		t.Fatal("spectator reclaimed", err)
	}
}

func TestDesktopSessionImagePublicationFences(t *testing.T) {
	for _, change := range []string{"foreground", "workspace-inode", "workspace-alias", "saved-root", "transcript-inode", "new-detached", "detached-retiring", "detached-replacement", "cancel"} {
		t.Run(change, func(t *testing.T) {
			f, _, workspace := desktopImageFixture(t)
			path := filepath.Join(f.dir, "race-image.jsonl")
			saveServeTestSession(t, path)
			saveDesktopImageMeta(t, path, workspace)
			requestedRoot := workspace
			if change == "workspace-alias" {
				requestedRoot = filepath.Join(t.TempDir(), "alias")
				if err := os.Symlink(workspace, requestedRoot); err != nil {
					t.Fatal(err)
				}
			}
			var ownedDetached *detachedSession
			if strings.HasPrefix(change, "detached-") {
				ctrl := control.New(control.Options{SessionDir: f.dir, SessionPath: path, WorkspaceRoot: workspace})
				t.Cleanup(ctrl.Close)
				ownedDetached = &detachedSession{path: path, ctrl: ctrl}
				f.server.detachedMu.Lock()
				f.server.detached[agent.CanonicalSessionPath(path)] = ownedDetached
				f.server.detachedMu.Unlock()
			}
			data, _ := json.Marshal(desktopSessionImageRequest{agent.CanonicalSessionPath(path), requestedRoot, "image.png"})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := httptest.NewRequest(http.MethodPost, "/desktop/session-image", bytes.NewReader(data)).WithContext(ctx)
			w := httptest.NewRecorder()
			entered := make(chan struct{})
			release := make(chan struct{})
			done := make(chan struct{})
			go func() {
				defer close(done)
				f.server.desktopSessionImageWithReader(w, r, func(context.Context, *os.Root, string, string) desktopbridge.WorkspaceImageView {
					close(entered)
					<-release
					return desktopbridge.WorkspaceImageView{URL: "PRIVATE-STALE-PIXELS", OpenHref: "file:///private"}
				})
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("read not started")
			}
			if !f.server.bindMu.TryLock() {
				t.Fatal("decode retained bind lock")
			}
			switch change {
			case "foreground":
				ctrl := control.New(control.Options{SessionDir: f.dir, SessionPath: f.active, WorkspaceRoot: workspace})
				t.Cleanup(ctrl.Close)
				f.server.mu.Lock()
				f.server.ctrl = ctrl
				f.server.mu.Unlock()
			case "workspace-inode":
				if err := os.Rename(workspace, workspace+"-old"); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Remove(workspace); _ = os.Rename(workspace+"-old", workspace) })
				if err := os.Mkdir(workspace, 0700); err != nil {
					t.Fatal(err)
				}
			case "saved-root":
				saveDesktopImageMeta(t, path, t.TempDir())
			case "workspace-alias":
				if err := os.Remove(requestedRoot); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), requestedRoot); err != nil {
					t.Fatal(err)
				}
			case "detached-retiring":
				f.server.detachedMu.Lock()
				ownedDetached.retiring = true
				f.server.detachedMu.Unlock()
			case "detached-replacement":
				ctrl := control.New(control.Options{SessionDir: f.dir, SessionPath: path, WorkspaceRoot: workspace})
				t.Cleanup(ctrl.Close)
				f.server.detachedMu.Lock()
				ownedDetached.ctrl = ctrl
				f.server.detachedMu.Unlock()
			case "transcript-inode":
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				saveServeTestSession(t, path)
			case "new-detached":
				ctrl := control.New(control.Options{SessionDir: f.dir, SessionPath: path, WorkspaceRoot: workspace})
				t.Cleanup(ctrl.Close)
				f.server.detachedMu.Lock()
				f.server.detached[agent.CanonicalSessionPath(path)] = &detachedSession{path: path, ctrl: ctrl}
				f.server.detachedMu.Unlock()
			case "cancel":
				cancel()
			}
			f.server.bindMu.Unlock()
			close(release)
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("image did not terminate")
			}
			if w.Code != 409 || strings.Contains(w.Body.String(), "PRIVATE") {
				t.Fatalf("stale pixels published: %d", w.Code)
			}
		})
	}
}

func TestDesktopSessionImageRejectsMalformedAndBusyRequests(t *testing.T) {
	f, _, workspace := desktopImageFixture(t)
	input := desktopSessionImageRequest{agent.CanonicalSessionPath(f.active), workspace, "image.png"}
	data, _ := json.Marshal(input)
	for _, body := range []string{string(data) + " {}", strings.TrimSuffix(string(data), "}") + `,"extra":"private"}`, `{"sessionPath":""}`, string(bytes.Repeat([]byte(" "), (24<<20)+1))} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/desktop/session-image", strings.NewReader(body))
		f.server.desktopSessionImage(w, r)
		if w.Code != 400 {
			t.Fatalf("malformed request accepted: %d", w.Code)
		}
	}
	f.server.bindMu.Lock()
	w := httptest.NewRecorder()
	f.server.desktopSessionImage(w, httptest.NewRequest(http.MethodPost, "/desktop/session-image", bytes.NewReader(data)))
	f.server.bindMu.Unlock()
	if w.Code != 409 {
		t.Fatal("binding wait was not bounded")
	}
}

func TestDesktopSavedImageRootRejectsUnboundedOrEscapingMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "saved.jsonl")
	workspace := t.TempDir()
	for _, data := range [][]byte{[]byte(`{"workspace_root":"relative"}`), []byte("invalid private metadata"), bytes.Repeat([]byte(" "), (1<<20)+1)} {
		if err := os.WriteFile(agent.BranchMetaPath(path), data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := desktopSavedImageRoot(path); err == nil {
			t.Fatal("invalid metadata granted a root")
		}
	}
	if err := os.Remove(agent.BranchMetaPath(path)); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.meta")
	data, _ := json.Marshal(map[string]string{"workspace_root": workspace})
	if err := os.WriteFile(outside, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, agent.BranchMetaPath(path)); err != nil {
		t.Fatal(err)
	}
	if _, err := desktopSavedImageRoot(path); err == nil {
		t.Fatal("metadata escaped the transcript directory")
	}
}

func TestDesktopSessionImageAdmissionCancelsWithoutReading(t *testing.T) {
	f, _, workspace := desktopImageFixture(t)
	for i := 0; i < cap(desktopSessionImageSlots); i++ {
		desktopSessionImageSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(desktopSessionImageSlots); i++ {
			<-desktopSessionImageSlots
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data, _ := json.Marshal(desktopSessionImageRequest{agent.CanonicalSessionPath(f.active), workspace, "image.png"})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/desktop/session-image", bytes.NewReader(data)).WithContext(ctx)
	f.server.desktopSessionImageWithReader(w, r, func(context.Context, *os.Root, string, string) desktopbridge.WorkspaceImageView {
		t.Error("cancelled admission decoded pixels")
		return desktopbridge.WorkspaceImageView{}
	})
	if w.Code != 408 {
		t.Fatal("image admission did not cancel")
	}
}
