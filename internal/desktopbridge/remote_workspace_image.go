package desktopbridge

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"reasonix/internal/netclient"
)

const remoteWorkspaceImageMaxBytes = 10 << 20
const remoteWorkspaceImageTimeout = 20 * time.Second

var remoteWorkspaceImageSlots = make(chan struct{}, 2)

// Production uses PublicImageTransport. The injectable client supports only
// isolated transport tests and is never supplied by a renderer.
func FetchRemoteWorkspaceImage(parent context.Context, source string, client *http.Client) WorkspaceImageView {
	fail := func(code string) WorkspaceImageView { return WorkspaceImageView{ErrorCode: code} }
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "//") {
		source = "https:" + source
	}
	validated, err := netclient.ValidatePublicImageURL(source)
	if err != nil {
		return fail("blocked-remote")
	}
	ctx, cancel := context.WithTimeout(parent, remoteWorkspaceImageTimeout)
	defer cancel()
	select {
	case remoteWorkspaceImageSlots <- struct{}{}:
		defer func() { <-remoteWorkspaceImageSlots }()
	case <-ctx.Done():
		return fail("fetch-failed")
	}
	if client == nil || client.Transport == nil {
		return fail("fetch-failed")
	}
	copy := *client
	copy.Timeout = remoteWorkspaceImageTimeout
	copy.Jar = nil
	copy.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		request.Header.Del("Authorization")
		request.Header.Del("Cookie")
		request.Header.Del("Referer")
		if len(via) >= 5 {
			return http.ErrUseLastResponse
		}
		validated, err := netclient.ValidatePublicImageURL(request.URL.String())
		if err != nil {
			return err
		}
		request.URL, err = url.Parse(validated)
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, validated, nil)
	if err != nil {
		return fail("blocked-remote")
	}
	request.Header.Set("Accept", "image/png,image/jpeg,image/gif,image/webp")
	request.Header.Set("User-Agent", "Reasonix-Desktop/1.0")
	response, err := copy.Do(request)
	if err != nil {
		return fail("fetch-failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fail("fetch-failed")
	}
	if response.ContentLength > remoteWorkspaceImageMaxBytes {
		return fail("too-large")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, remoteWorkspaceImageMaxBytes+1))
	if err != nil {
		return fail("fetch-failed")
	}
	if len(body) > remoteWorkspaceImageMaxBytes {
		return fail("too-large")
	}
	u, _ := url.Parse(validated)
	name := path.Base(u.Path)
	if name == "." || name == "/" || name == "" {
		name = "image"
	}
	view := workspaceImagePixels(body, name, int64(len(body)), "")
	view.OpenHref = validated
	return view
}
