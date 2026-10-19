package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"reasonix/internal/netclient"
)

func TestSpeechFailuresCarryStageAndSafeRecoveryCodes(t *testing.T) {
	for _, stage := range []string{"tts", "asr"} {
		for _, fixture := range []struct {
			status int
			kind   string
		}{{401, "auth"}, {403, "auth"}, {404, "unsupported"}, {429, "rate_limit"}, {502, "upstream"}, {503, "upstream"}, {200, "result"}} {
			t.Run(fmt.Sprintf("%s-%d", stage, fixture.status), func(t *testing.T) {
				calls := 0
				server := newProviderProbeTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.WriteHeader(fixture.status)
					fmt.Fprint(w, `{"error":"PRIVATE_AUDIO PRIVATE_KEY"}`)
				}))
				defer server.Close()
				target := speechTarget{Endpoint: server.URL, Protocol: "mimo", Model: speechModel, Key: "PRIVATE_KEY"}
				var err error
				if stage == "tts" {
					_, err = synthesizeSpeech(context.Background(), target, netclient.ProxySpec{Mode: netclient.ModeOff}, "hello", "mimo_default")
				} else {
					_, err = recognizeSpeechService(context.Background(), target, netclient.ProxySpec{Mode: netclient.ModeOff}, speechTestWav())
				}
				var failure *speechServiceError
				if !errors.As(err, &failure) || failure.kind != fixture.kind {
					t.Fatalf("failure category=%v", err)
				}
				w := httptest.NewRecorder()
				writeSpeechFailure(w, stage, err)
				var body struct{ Error struct{ Code string } }
				if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Error.Code != "speech_"+stage+"_"+fixture.kind {
					t.Fatal("stage/code lost")
				}
				if calls != 1 || strings.Contains(w.Body.String(), "PRIVATE") {
					t.Fatal("private service details leaked or automatic retry spent quota")
				}
			})
		}
	}
	for _, err := range []error{context.DeadlineExceeded, os.ErrDeadlineExceeded} {
		var failure *speechServiceError
		if !errors.As(speechConnectionFailure(err, "safe message"), &failure) || failure.kind != "timeout" {
			t.Fatal("timeout lost")
		}
	}
	var failure *speechServiceError
	if !errors.As(speechConnectionFailure(errors.New("private hostname"), "safe message"), &failure) || failure.kind != "network" || strings.Contains(failure.Error(), "hostname") {
		t.Fatal("network error leaked")
	}
}
