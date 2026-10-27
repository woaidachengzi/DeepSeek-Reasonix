package provider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/provider"
	"reasonix/internal/provider/anthropic"
	"reasonix/internal/provider/openai"
	"reasonix/internal/provider/responses"
)

// Capture actual outgoing adapter bytes; no paid API call or user credential.
func TestCurrentDeepSeekImagesAndLowEffortAllAdapters(t *testing.T) {
	for _, kind := range []string{"openai", "anthropic", "responses"} {
		for _, model := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp", "deepseek-v4-pro"} {
			for _, off := range []bool{false, true} {
				label := kind + "/" + model
				if off {
					label += "/explicit-off"
				}
				t.Run(label, func(t *testing.T) {
					var body []byte
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, _ = io.ReadAll(r.Body)
						http.Error(w, "fixture authentication rejection", http.StatusUnauthorized)
					}))
					defer server.Close()
					base := "https://api.deepseek.com"
					if kind == "anthropic" {
						base += "/anthropic"
					}
					info, ok := provider.OfficialDeepSeekModelInfo(kind, base, model)
					if !ok {
						t.Fatal("missing direct model metadata")
					}
					if info.ContextWindow != 1_000_000 || info.MaxOutputTokens != 384_000 || !info.Reasoning {
						t.Fatalf("direct model limits lost: %+v", info)
					}
					if off {
						info.InputModalities = []provider.ModelModality{provider.ModalityText}
					}
					extra := map[string]any{"request_url": server.URL, "effort": "low", "vision": true}
					cfg := provider.Config{Name: "disposable-adapter-test", BaseURL: base, Model: model, APIKey: "fixture", Extra: extra, ModelInfo: &info}
					var p provider.Provider
					var err error
					switch kind {
					case "openai":
						p, err = openai.New(cfg)
					case "anthropic":
						p, err = anthropic.New(cfg)
					case "responses":
						p = responses.New(responses.Config{Name: cfg.Name, BaseURL: base, Model: model, APIKey: "fixture", RequestURL: server.URL, Effort: "low", Mode: "stateless", ModelInfo: &info, Extra: extra})
					}
					if err != nil {
						t.Fatal(err)
					}
					defer p.(interface{ CloseIdleConnections() }).CloseIdleConnections()
					_, err = p.Stream(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "describe", Images: []string{"data:image/png;base64,AAAA"}}}})
					if err == nil {
						t.Fatal("expected fixture rejection")
					}
					var wire map[string]any
					if err := json.Unmarshal(body, &wire); err != nil {
						t.Fatalf("missing wire body: %v", err)
					}
					if wire["model"] != model {
						t.Fatalf("model selection rewritten: %v", wire["model"])
					}
					wantImage := model != "deepseek-v4-pro" && !off
					if strings.Contains(string(body), "AAAA") != wantImage {
						t.Fatalf("image inclusion wrong (want %v): %s", wantImage, body)
					}
					switch kind {
					case "openai":
						if wire["reasoning_effort"] != "low" {
							t.Fatalf("Chat effort: %s", body)
						}
					case "anthropic":
						if v, ok := wire["output_config"].(map[string]any); !ok || v["effort"] != "low" {
							t.Fatalf("Messages effort: %s", body)
						}
					case "responses":
						if v, ok := wire["reasoning"].(map[string]any); !ok || v["effort"] != "low" {
							t.Fatalf("Responses effort: %s", body)
						}
					}
				})
			}
		}
	}
}

func TestOpenCodeGoInferenceRoutesAndIsolation(t *testing.T) {
	for _, tc := range []struct {
		kind, base, model string
		want              bool
	}{
		{"openai", "https://opencode.ai/inference/go/openai/v1", "deepseek-v4.1-flash", true},
		{"responses", "https://opencode.ai/inference/go/openai/v1", "grok-4.5", true},
		{"anthropic", "https://opencode.ai/inference/go/anthropic", "qwen3.8-max", true},
		{"anthropic", "https://opencode.ai/inference/go/anthropic", "deepseek-v4.1-flash", false},
		{"responses", "https://opencode.ai/inference/go/openai/v1", "deepseek-v4.1-flash", false},
		{"anthropic", "https://opencode.ai/zen/go", "deepseek-v4.1-flash", false},
		{"responses", "https://opencode.ai/zen/go/v1", "deepseek-v4.1-flash", false},
		{"openai", "https://opencode.ai.attacker.example/inference/go/openai/v1", "deepseek-v4.1-flash", false},
		{"openai", "https://opencode.ai/inference/go/openai/v1?key=x", "deepseek-v4.1-flash", false},
	} {
		if _, ok := provider.LookupOfficialOpenCodeGo(tc.kind, tc.base, tc.model); ok != tc.want {
			t.Fatalf("%+v: supported=%v", tc, ok)
		}
	}
	info, ok := provider.BuiltinModelInfo("openai", "https://opencode.ai/inference/go/openai/v1", "deepseek-v4.1-flash")
	if !ok || !info.SupportsInput(provider.ModalityImage) || info.ContextWindow != 1_000_000 {
		t.Fatalf("Go Flash metadata: %+v %v", info, ok)
	}
}

type goRouteCaptureTransport func(*http.Request) (*http.Response, error)

func (f goRouteCaptureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenCodeGoDerivedRequestURLs(t *testing.T) {
	for _, tc := range []struct{ kind, base, model, endpoint string }{
		{"openai", "https://opencode.ai/zen/go/v1", "deepseek-v4-flash", "/chat/completions"},
		{"anthropic", "https://opencode.ai/zen/go", "qwen3.8-max", "/v1/messages"},
		{"responses", "https://opencode.ai/zen/go/v1", "grok-4.5", "/responses"},
		{"openai", "https://opencode.ai/inference/go/openai/v1", "deepseek-v4.1-flash", "/chat/completions"},
		{"anthropic", "https://opencode.ai/inference/go/anthropic", "qwen3.8-max", "/v1/messages"},
		{"responses", "https://opencode.ai/inference/go/openai/v1", "grok-4.5", "/responses"},
	} {
		t.Run(tc.base+"/"+tc.kind, func(t *testing.T) {
			observed := ""
			client := &http.Client{Transport: goRouteCaptureTransport(func(r *http.Request) (*http.Response, error) {
				observed = r.URL.String()
				if r.Header.Get("X-Opencode-Session") == "" || r.Header.Get("User-Agent") != "Reasonix" {
					t.Fatalf("missing client/session headers: %v", r.Header)
				}
				return &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("fixture rejection")), Request: r}, nil
			})}
			cfg := provider.Config{Name: "disposable-route-test", BaseURL: tc.base, Model: tc.model, APIKey: "fixture", HTTPClient: client}
			var p provider.Provider
			var err error
			switch tc.kind {
			case "openai":
				p, err = openai.New(cfg)
			case "anthropic":
				p, err = anthropic.New(cfg)
			case "responses":
				p = responses.New(responses.Config{Name: cfg.Name, BaseURL: tc.base, Model: tc.model, APIKey: "fixture", HTTPClient: client, Mode: "stateless"})
			}
			if err != nil {
				t.Fatal(err)
			}
			defer p.(interface{ CloseIdleConnections() }).CloseIdleConnections()
			_, err = p.Stream(provider.WithCacheSession(context.Background(), "disposable-url-test"), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "fixture"}}})
			if err == nil || observed != tc.base+tc.endpoint {
				t.Fatalf("derived URL=%q want=%q err=%v", observed, tc.base+tc.endpoint, err)
			}
		})
	}
}
