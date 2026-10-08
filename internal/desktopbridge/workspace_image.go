package desktopbridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const workspaceImageMaxBytes = 16 << 20
const WorkspaceImageSourceLimit = workspaceImageMaxBytes*4/3 + 1024
const workspaceImageMaxPixels = 40_000_000
const workspaceImagePreviewSide = 1200

// WorkspaceImageView contains only bounded, re-encoded raster pixels, never a
// filesystem URL or SVG. Size describes the original file, not the preview.
type WorkspaceImageView struct {
	URL       string `json:"url"`
	Filename  string `json:"filename,omitempty"`
	Mime      string `json:"mime,omitempty"`
	Size      int64  `json:"size,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
	OpenHref  string `json:"openHref,omitempty"`
}

// WorkspaceImage holds the ownership lock through resolution and decoding, so
// a concurrent session switch cannot substitute another session's workspace.
func (m *RuntimeManager) WorkspaceImage(sessionID, source string) (WorkspaceImageView, error) {
	return m.WorkspaceImageWithRemote(context.Background(), sessionID, source, nil)
}

// Remote work releases the controller lock and fences its result against the
// owner's epoch, including switch-away-and-back and same-session model rebuild.
func (m *RuntimeManager) WorkspaceImageWithRemote(ctx context.Context, sessionID, source string, remote func(context.Context, string) WorkspaceImageView) (WorkspaceImageView, error) {
	m.mu.Lock()
	locked := true
	defer func() {
		if locked {
			m.mu.Unlock()
		}
	}()
	if m.closed {
		return WorkspaceImageView{}, ErrClosed
	}
	if m.runtime == nil || strings.TrimSpace(sessionID) == "" || m.view.ID != strings.TrimSpace(sessionID) {
		return WorkspaceImageView{}, ErrSessionNotFound
	}
	if IsRemoteWorkspaceImage(source) {
		if remote == nil {
			return WorkspaceImageView{ErrorCode: "blocked-remote"}, nil
		}
		epoch := m.ownerEpoch
		m.mu.Unlock()
		locked = false
		view := remote(ctx, source)
		m.mu.Lock()
		locked = true
		if m.closed {
			return WorkspaceImageView{}, ErrClosed
		}
		if m.runtime == nil || m.view.ID != strings.TrimSpace(sessionID) || m.ownerEpoch != epoch {
			return WorkspaceImageView{}, ErrSessionNotFound
		}
		return view, nil
	}
	provider, ok := m.runtime.(RuntimeWorkspaceProvider)
	if !ok {
		return WorkspaceImageView{}, ErrInvalidWorkspacePath
	}
	root, err := provider.LocalWorkspace()
	if err != nil || strings.TrimSpace(root) == "" {
		return WorkspaceImageView{}, ErrInvalidWorkspacePath
	}
	return readWorkspaceImage(root, source), nil
}

func IsRemoteWorkspaceImage(source string) bool {
	source = strings.TrimSpace(source)
	return strings.HasPrefix(source, "//") || (len(source) >= 7 && strings.EqualFold(source[:7], "http://")) ||
		(len(source) >= 8 && strings.EqualFold(source[:8], "https://"))
}

func readWorkspaceImage(base, source string) WorkspaceImageView {
	if len(source) >= 5 && strings.EqualFold(source[:5], "data:") {
		return ReadWorkspaceImageFromRoot(nil, source)
	}
	// Retain validation order for the existing local API.
	if _, err := workspaceImagePath(base, source); err != nil {
		return WorkspaceImageView{ErrorCode: "forbidden"}
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return WorkspaceImageView{ErrorCode: "not-found"}
	}
	defer root.Close()
	return ReadWorkspaceImageFromRoot(root, source)
}

// ReadWorkspaceImageFromRoot uses a directory handle whose session ownership
// the caller established. The caller retains the handle and fences publication.
// A nil handle permits inline raster data only, never filesystem access.
func ReadWorkspaceImageFromRoot(root *os.Root, source string, aliases ...string) WorkspaceImageView {
	fail := func(code string) WorkspaceImageView { return WorkspaceImageView{ErrorCode: code} }
	if len(source) >= 5 && strings.EqualFold(source[:5], "data:") {
		if len(source) > WorkspaceImageSourceLimit {
			return fail("too-large")
		}
		header, payload, ok := strings.Cut(source, ",")
		formats := map[string]string{"data:image/png;base64": "png", "data:image/jpeg;base64": "jpeg", "data:image/gif;base64": "gif", "data:image/webp;base64": "webp"}
		format := formats[strings.ToLower(header)]
		if !ok || format == "" {
			return fail("unsupported-type")
		}
		data, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return fail("invalid-image")
		}
		return workspaceImagePixels(data, "image", int64(len(data)), format)
	}
	if root == nil {
		return fail("forbidden")
	}
	path, err := workspaceImagePath(root.Name(), source)
	// A backend workspace alias may spell an absolute attachment differently
	// from the canonical root name. Accept it only if it still names this held
	// directory inode, never by simply trusting an alternate path prefix.
	if err != nil && len(aliases) > 0 {
		owned, ownedErr := root.Stat(".")
		for _, alias := range aliases {
			if ownedErr != nil || !filepath.IsAbs(alias) {
				continue
			}
			candidate, aliasErr := os.Stat(alias)
			if aliasErr != nil || !os.SameFile(owned, candidate) {
				continue
			}
			if relative, pathErr := workspaceImagePath(alias, source); pathErr == nil {
				path, err = relative, nil
				break
			}
		}
	}
	if err != nil {
		return fail("forbidden")
	}
	// os.Root confines both path traversal and symlink resolution during open,
	// including filesystem races. EvalSymlinks followed by os.Open is not enough.
	info, err := root.Stat(path)
	if err != nil {
		return fail("not-found")
	}
	if !info.Mode().IsRegular() {
		return fail("not-a-file")
	}
	if info.Size() <= 0 || info.Size() > workspaceImageMaxBytes {
		return fail("too-large")
	}
	f, err := root.Open(path)
	if err != nil {
		return fail("not-found")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) || opened.Size() != info.Size() {
		return fail("changed-file")
	}
	data, err := io.ReadAll(io.LimitReader(f, workspaceImageMaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > workspaceImageMaxBytes {
		return fail("too-large")
	}
	return workspaceImagePixels(data, filepath.Base(path), opened.Size(), "")
}

func workspaceImagePixels(data []byte, filename string, size int64, declaredFormat string) WorkspaceImageView {
	fail := func(code string) WorkspaceImageView { return WorkspaceImageView{ErrorCode: code} }
	if len(data) == 0 || len(data) > workspaceImageMaxBytes {
		return fail("too-large")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg" && format != "gif" && format != "webp") {
		return fail("unsupported-type")
	}
	if declaredFormat != "" && format != declaredFormat {
		return fail("invalid-image")
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width) > workspaceImageMaxPixels/int64(config.Height) {
		return fail("too-large")
	}
	pixels, _, err := image.Decode(bytes.NewReader(data))
	if err != nil || pixels.Bounds().Dx() != config.Width || pixels.Bounds().Dy() != config.Height {
		return fail("invalid-image")
	}
	w, h := config.Width, config.Height
	if w > workspaceImagePreviewSide || h > workspaceImagePreviewSide {
		if w >= h {
			h = max(1, h*workspaceImagePreviewSide/w)
			w = workspaceImagePreviewSide
		} else {
			w = max(1, w*workspaceImagePreviewSide/h)
			h = workspaceImagePreviewSide
		}
	}
	preview := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(preview, preview.Bounds(), pixels, pixels.Bounds(), draw.Src, nil)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, preview); err != nil || encoded.Len() > 8<<20 {
		return fail("invalid-image")
	}
	return WorkspaceImageView{
		URL:      "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()),
		Filename: filename, Mime: "image/png", Size: size,
	}
}

func workspaceImagePath(base, source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" || len(source) > 4096 || strings.ContainsRune(source, 0) {
		return "", os.ErrInvalid
	}
	if !filepath.IsAbs(source) {
		u, err := url.Parse(source)
		if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "" && !strings.EqualFold(u.Scheme, "file")) {
			return "", os.ErrInvalid
		}
		if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
			return "", os.ErrPermission
		}
		source = filepath.FromSlash(u.Path)
		if strings.EqualFold(u.Scheme, "file") && runtime.GOOS == "windows" && len(source) >= 3 && source[0] == '\\' && source[2] == ':' {
			source = source[1:]
		}
	}
	if filepath.IsAbs(source) {
		absolute, err := filepath.Abs(base)
		if err != nil {
			return "", err
		}
		var relErr error
		source, relErr = filepath.Rel(absolute, source)
		if relErr != nil {
			return "", relErr
		}
	}
	path := filepath.Clean(source)
	if !filepath.IsLocal(path) || path == "." || strings.ContainsRune(path, 0) || strings.HasPrefix(path, "\\\\") {
		return "", errors.New("image path escapes owned workspace")
	}
	return path, nil
}
