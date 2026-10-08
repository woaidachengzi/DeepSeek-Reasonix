package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func clientImageFixture(t *testing.T, listed bool, handler http.HandlerFunc) (*Client, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			_, _ = io.Copy(io.Discard, r.Body)
			http.SetCookie(w, &http.Cookie{Name: "owned_image", Value: "backend", Path: "/", HttpOnly: true})
			w.WriteHeader(204)
			return
		}
		if cookie, err := r.Cookie("owned_image"); err != nil || cookie.Value != "backend" || r.Header.Get("Authorization") != "" {
			t.Error("backend cookie missing/auth header exposed")
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/sessions" {
			if r.Method != http.MethodGet {
				t.Error("catalogue mutated")
			}
			rows := []Session{}
			if listed {
				rows = append(rows, Session{Path: ownedViewPath})
			}
			_ = json.NewEncoder(w).Encode(rows)
			return
		}
		calls.Add(1)
		if r.URL.Path != "/desktop/session-image" || r.Method != http.MethodPost || r.URL.RawQuery != "" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("wrong image route")
		}
		var input map[string]string
		if json.NewDecoder(r.Body).Decode(&input) != nil || len(input) != 3 || input["workspace"] != "/remote/workspace-alias" || input["sessionPath"] != ownedViewPath || input["source"] != "screen%20shot.png" {
			t.Error("wrong scoped image request")
		}
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	c, err := Connect(context.Background(), context.Background(), s.URL, "owned-image-token")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c, &calls
}
func clientImageResponse(t *testing.T) map[string]any {
	t.Helper()
	var pixels bytes.Buffer
	if err := png.Encode(&pixels, image.NewNRGBA(image.Rect(0, 0, 2, 1))); err != nil {
		t.Fatal(err)
	}
	return map[string]any{"protocolVersion": 1, "sessionPath": ownedViewPath, "workspace": "/remote/workspace-alias", "image": map[string]any{"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(pixels.Bytes()), "mime": "image/png", "filename": "screen shot.png", "size": pixels.Len(), "openHref": "file:///private", "credentials": "private-config"}, "token": "private-token"}
}
func TestClientSessionImagePixelsScopeAndPrivacy(t *testing.T) {
	response := clientImageResponse(t)
	c, calls := clientImageFixture(t, true, func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(response) })
	v, err := c.SessionImage(context.Background(), "/remote/workspace-alias", ownedViewPath, "screen%20shot.png")
	if err != nil || v.Image.Mime != "image/png" || calls.Load() != 1 {
		t.Fatal("valid scoped image rejected", err)
	}
	data, _ := json.Marshal(v)
	if strings.Contains(string(data), "private") || strings.Contains(string(data), "openHref") {
		t.Fatal("remote opener or private fields leaked")
	}
	c, calls = clientImageFixture(t, false, func(w http.ResponseWriter, r *http.Request) { t.Error("unlisted image requested") })
	if _, err := c.SessionImage(context.Background(), "/remote/workspace-alias", ownedViewPath, "screen%20shot.png"); !errors.Is(err, ErrSessionNotListed) || calls.Load() != 0 {
		t.Fatal("unlisted path accepted", err)
	}
	if _, err := c.SessionImage(nil, "/remote/workspace-alias", ownedViewPath, "screen%20shot.png"); !errors.Is(err, ErrClient) {
		t.Fatal("nil context accepted")
	}
}
func TestClientSessionImageRejectsScopePixelBudgetAndErrors(t *testing.T) {
	for _, mutate := range []func(map[string]any){
		func(v map[string]any) { v["protocolVersion"] = 2 },
		func(v map[string]any) { v["sessionPath"] = "/other.jsonl" },
		func(v map[string]any) { v["workspace"] = "/other-workspace" },
		func(v map[string]any) { v["image"].(map[string]any)["url"] = "file:///private" },
		func(v map[string]any) { v["image"].(map[string]any)["url"] = "https://example.invalid/private" },
		func(v map[string]any) { v["image"].(map[string]any)["mime"] = "image/svg+xml" },
		func(v map[string]any) { v["image"].(map[string]any)["filename"] = "private\nname" },
		func(v map[string]any) { v["image"].(map[string]any)["size"] = (16 << 20) + 1 },
		func(v map[string]any) {
			v["image"] = map[string]any{"url": "data:image/png;base64,broken", "mime": "image/png"}
		},
		func(v map[string]any) { v["image"] = map[string]any{"url": "", "errorCode": "PRIVATE DIAGNOSTIC"} },
		func(v map[string]any) { v["image"].(map[string]any)["errorCode"] = "forbidden" },
	} {
		response := clientImageResponse(t)
		mutate(response)
		c, _ := clientImageFixture(t, true, func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(response) })
		if _, err := c.SessionImage(context.Background(), "/remote/workspace-alias", ownedViewPath, "screen%20shot.png"); !errors.Is(err, ErrResponse) {
			t.Fatal("invalid image accepted", err)
		}
	}
	for _, status := range []int{401, 403, 404, 409, 502} {
		c, _ := clientImageFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, "PRIVATE DIAGNOSTIC")
		})
		_, err := c.SessionImage(context.Background(), "/remote/workspace-alias", ownedViewPath, "screen%20shot.png")
		if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "PRIVATE") || c.Closed() != (status == 401 || status == 403) {
			t.Fatal("error leaked or wrong owner revocation", err)
		}
	}
	c, _ := clientImageFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat(" ", (12<<20)+1))
	})
	if _, err := c.SessionImage(context.Background(), "/remote/workspace-alias", ownedViewPath, "screen%20shot.png"); !errors.Is(err, ErrResponse) {
		t.Fatal("response budget not enforced", err)
	}
	for _, code := range []string{"forbidden", "blocked-remote", "too-large"} {
		c, _ := clientImageFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
			v := clientImageResponse(t)
			v["image"] = map[string]string{"url": "", "errorCode": code}
			_ = json.NewEncoder(w).Encode(v)
		})
		if v, err := c.SessionImage(context.Background(), "/remote/workspace-alias", ownedViewPath, "screen%20shot.png"); err != nil || v.Image.ErrorCode != code {
			t.Fatal("fixed image refusal rejected", err)
		}
	}
}
func TestClientSessionImageOwnerCancelsBody(t *testing.T) {
	entered := make(chan struct{})
	finished := make(chan struct{})
	abort := make(chan struct{})
	c, _ := clientImageFixture(t, true, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(entered)
		select {
		case <-r.Context().Done():
		case <-abort:
		}
		close(finished)
	})
	defer close(abort)
	done := make(chan error, 1)
	go func() {
		_, err := c.SessionImage(context.Background(), "/remote/workspace-alias", ownedViewPath, "screen%20shot.png")
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("image read not started")
	}
	c.Close()
	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read survived owner close")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("remote body not canceled")
	}
}

func TestValidSessionImageRejectsOversizedDimensionsAndNoncanonicalPNG(t *testing.T) {
	var pixels bytes.Buffer
	if err := png.Encode(&pixels, image.NewNRGBA(image.Rect(0, 0, 1201, 1))); err != nil {
		t.Fatal(err)
	}
	v := SessionImage{URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(pixels.Bytes()), Mime: "image/png"}
	if validSessionImage(v) {
		t.Fatal("oversized raster accepted")
	}
	v.URL = strings.Replace(v.URL, ",", ",\n", 1)
	if validSessionImage(v) {
		t.Fatal("noncanonical base64 accepted")
	}
	if validSessionImage(SessionImage{URL: "data:image/svg+xml;base64,PHN2Zy8+", Mime: "image/svg+xml"}) {
		t.Fatal("SVG accepted")
	}
}
