package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const SessionImageSourceLimit = (16<<20)*4/3 + 1024

// Display pixels only. No browser URL, file path opener grant or replay/config
// field is represented, even if a malicious/newer Serve sends one.
type SessionImage struct {
	URL       string `json:"url"`
	Filename  string `json:"filename,omitempty"`
	Mime      string `json:"mime,omitempty"`
	Size      int64  `json:"size,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}
type SessionImageView struct {
	ProtocolVersion int          `json:"protocolVersion"`
	SessionPath     string       `json:"sessionPath"`
	Workspace       string       `json:"workspace"`
	Image           SessionImage `json:"image"`
}

// workspace must come from the backend-owned SSH bootstrap result, not a
// renderer grant. The bridge/native layers keep it out of the inbound request.
func (c *Client) SessionImage(operation context.Context, workspace, path, source string) (SessionImageView, error) {
	if operation == nil {
		return SessionImageView{}, ErrClient
	}
	if workspace == "" || !strings.HasPrefix(workspace, "/") || !cleanField(workspace, 4096) || path == "" || !cleanField(path, 32768) || strings.TrimSpace(source) == "" || len(source) > SessionImageSourceLimit || !utf8.ValidString(source) || strings.ContainsRune(source, 0) {
		return SessionImageView{}, ErrResponse
	}
	bounded, cancel := context.WithTimeout(operation, 30*time.Second)
	defer cancel()
	rows, err := c.Sessions(bounded)
	if err != nil {
		return SessionImageView{}, err
	}
	listed := false
	for _, row := range rows {
		if row.Path == path {
			listed = true
			break
		}
	}
	if !listed {
		return SessionImageView{}, ErrSessionNotListed
	}
	ctx, finish, err := c.callContext(bounded)
	if err != nil {
		return SessionImageView{}, err
	}
	defer finish()
	data, err := json.Marshal(struct {
		SessionPath string `json:"sessionPath"`
		Workspace   string `json:"workspace"`
		Source      string `json:"source"`
	}{path, workspace, source})
	if err != nil || len(data) > 24<<20 {
		return SessionImageView{}, ErrResponse
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/desktop/session-image", bytes.NewReader(data))
	if err != nil {
		return SessionImageView{}, ErrResponse
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return SessionImageView{}, c.viewReadError(ctx)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			c.Close()
		}
		return SessionImageView{}, ErrUnavailable
	}
	const max = 12 << 20
	data, err = io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return SessionImageView{}, c.viewReadError(ctx)
	}
	var view SessionImageView
	if len(data) > max || !utf8.Valid(data) || json.Unmarshal(data, &view) != nil || view.ProtocolVersion != 1 || view.SessionPath != path || view.Workspace != workspace || !validSessionImage(view.Image) {
		return SessionImageView{}, ErrResponse
	}
	if c.Closed() || ctx.Err() != nil {
		return SessionImageView{}, c.viewReadError(ctx)
	}
	return view, nil
}

func validSessionImage(image SessionImage) bool {
	if !cleanField(image.Filename, 4096) || image.Size < 0 || image.Size > 16<<20 {
		return false
	}
	if image.ErrorCode != "" {
		if image.URL != "" || image.Mime != "" || image.Filename != "" || image.Size != 0 {
			return false
		}
		switch image.ErrorCode {
		case "blocked-remote", "proxy-config", "fetch-failed", "not-found", "forbidden", "not-a-file", "too-large", "changed-file", "invalid-image", "unsupported-type":
			return true
		default:
			return false
		}
	}
	const prefix = "data:image/png;base64,"
	if image.Mime != "image/png" || !strings.HasPrefix(image.URL, prefix) || len(image.URL) > (8<<20)*4/3+len(prefix)+4 {
		return false
	}
	payload := strings.TrimPrefix(image.URL, prefix)
	data, err := base64.StdEncoding.Strict().DecodeString(payload)
	if err != nil || len(data) > 8<<20 || base64.StdEncoding.EncodeToString(data) != payload {
		return false
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 1200 || cfg.Height > 1200 {
		return false
	}
	pixels, err := png.Decode(bytes.NewReader(data))
	return err == nil && pixels.Bounds().Dx() == cfg.Width && pixels.Bounds().Dy() == cfg.Height
}
