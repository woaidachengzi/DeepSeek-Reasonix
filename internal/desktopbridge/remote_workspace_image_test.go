package desktopbridge

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type imageRoundTripFunc func(*http.Request) (*http.Response, error)

func (f imageRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRemoteWorkspaceImageBoundsReencodesAndValidatesEveryRedirect(t *testing.T) {
	png := imageFixture(t, 2, 2)
	for _, tc := range []struct {
		name, source, location, code string
		body                         []byte
		status                       int
	}{
		{"valid", "https://images.example.com/safe.png#secret", "", "", png, 200},
		{"private URL", "http://127.0.0.1/private", "", "blocked-remote", png, 200},
		{"private redirect", "https://images.example.com/redirect", "http://169.254.169.254/secret", "fetch-failed", nil, 302},
		{"redirect loop", "https://images.example.com/loop", "https://images.example.com/loop", "fetch-failed", nil, 302},
		{"wrong bytes", "https://images.example.com/image.png", "", "unsupported-type", []byte("secret-not-image"), 200},
		{"SVG", "https://images.example.com/image.svg", "", "unsupported-type", []byte("<svg onload='alert(1)'/>"), 200},
		{"oversized", "https://images.example.com/image", "", "too-large", bytes.Repeat([]byte("x"), remoteWorkspaceImageMaxBytes+1), 200},
		{"bad status", "https://images.example.com/image", "", "fetch-failed", png, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: imageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Referer") != "" || r.URL.Fragment != "" {
					t.Fatal("request forwarded credentials or fragment")
				}
				header := make(http.Header)
				if tc.location != "" {
					header.Set("Location", tc.location)
				}
				return &http.Response{StatusCode: tc.status, Header: header, Body: io.NopCloser(bytes.NewReader(tc.body)), ContentLength: -1, Request: r}, nil
			})}
			view := FetchRemoteWorkspaceImage(context.Background(), tc.source, client)
			if view.ErrorCode != tc.code {
				t.Fatalf("error: %q want %q", view.ErrorCode, tc.code)
			}
			if tc.code == "" && (!strings.HasPrefix(view.URL, "data:image/png;base64,") || view.OpenHref != "https://images.example.com/safe.png") {
				t.Fatal("remote pixels or safe open URL missing")
			}
			if tc.code != "" && view.URL != "" {
				t.Fatal("error returned pixels")
			}
			if tc.name == "private URL" && calls != 0 {
				t.Fatal("private target reached transport")
			}
			if tc.name == "private redirect" && calls != 1 {
				t.Fatal("private redirect reached transport")
			}
			if tc.name == "redirect loop" && calls != 5 {
				t.Fatal("redirect budget differs")
			}
		})
	}
}

func TestRemoteWorkspaceImageCancellationReleasesItsSlot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &http.Client{Transport: imageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		cancel()
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	if view := FetchRemoteWorkspaceImage(ctx, "https://images.example.com/image", client); view.ErrorCode != "fetch-failed" {
		t.Fatal(view.ErrorCode)
	}
	if len(remoteWorkspaceImageSlots) != 0 {
		t.Fatal("cancelled image retained fetch slot")
	}
}

func TestRemoteWorkspaceImageDoesNotLockControllerAndRejectsReplacedOwner(t *testing.T) {
	root := t.TempDir()
	manager := NewRuntimeManager(RuntimeFactoryFunc(func(_ context.Context, request OpenRequest) (Runtime, error) {
		return &workspaceRuntime{fakeRuntime: &fakeRuntime{path: "/" + request.SessionID, state: "idle"}, root: root}, nil
	}))
	t.Cleanup(func() { _ = manager.Shutdown() })
	if _, err := manager.Open(context.Background(), OpenRequest{SessionID: "first"}); err != nil {
		t.Fatal(err)
	}
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		_, err := manager.WorkspaceImageWithRemote(ctx, "first", "https://images.example.com/image", func(ctx context.Context, source string) WorkspaceImageView {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
			}
			return WorkspaceImageView{URL: "data:image/png;base64,old"}
		})
		finished <- err
	}()
	<-started
	switched := make(chan error, 1)
	go func() {
		_, err := manager.Switch(ctx, OpenRequest{SessionID: "second"})
		if err == nil {
			_, err = manager.Switch(ctx, OpenRequest{SessionID: "first"})
		}
		switched <- err
	}()
	select {
	case err := <-switched:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("remote image prevented controller switch")
	}
	close(release)
	if err := <-finished; !errors.Is(err, ErrSessionNotFound) {
		t.Fatal("switch-away-and-back accepted old pixels", err)
	}
	called := false
	if _, err := manager.WorkspaceImageWithRemote(ctx, "other", "https://images.example.com/image", func(context.Context, string) WorkspaceImageView { called = true; return WorkspaceImageView{} }); !errors.Is(err, ErrSessionNotFound) || called {
		t.Fatal("unowned image reached network")
	}
}
