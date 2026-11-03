package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

// This harness also runs against the fixed 0.1.70 source tree. The runner
// supplies only a legacy constructor there; fixture, HTTP handler, ownership,
// session mutation, JSON serialization and measurement code stay identical.
type historyHTTPFullRuntime struct {
	*bridgeTestRuntime
	core *controllerRuntime
}

func (r *historyHTTPFullRuntime) History() []desktopbridge.HistoryMessage { return r.core.History() }

func historyBenchmarkFixture(count, toolBytes int) []provider.Message {
	messages := make([]provider.Message, 0, count)
	for i := range count {
		m := provider.Message{ID: fmt.Sprintf("message-%d", i), CreatedAt: int64(i + 1)}
		switch i % 4 {
		case 0:
			m.Role, m.Content = provider.RoleUser, fmt.Sprintf("question %d", i)
		case 1:
			m.Role = provider.RoleAssistant
			m.ToolCalls = []provider.ToolCall{{ID: fmt.Sprintf("call-%d", i), Name: "read_file", Arguments: `{"path":"private-fixture"}`}}
			m.RequestUsage = &provider.Usage{PromptTokens: 30, CompletionTokens: 2, TotalTokens: 32, CacheHitTokens: 30}
		case 2:
			m.Role, m.Content = provider.RoleTool, strings.Repeat("private-tool-result", max(1, toolBytes/19))
		default:
			m.Role, m.Content = provider.RoleAssistant, fmt.Sprintf("answer %d", i)
			m.RequestUsage = &provider.Usage{PromptTokens: 40, CompletionTokens: 4, TotalTokens: 44, CacheHitTokens: 40}
		}
		messages = append(messages, m)
	}
	return messages
}

func historyBenchmarkSession(messages []provider.Message) *agent.Session {
	s := agent.NewSession("")
	s.AddBatch(append([]provider.Message(nil), messages...)...)
	return s
}

func historyBenchmarkHost(tb testing.TB, messages []provider.Message) (*agent.Agent, *agent.Session, http.Handler) {
	tb.Helper()
	s := historyBenchmarkSession(messages)
	executor := agent.New(nil, nil, s, agent.Options{}, event.Discard)
	controller := control.New(control.Options{Executor: executor, Sink: event.Discard})
	tb.Cleanup(controller.ReleaseResources)
	runtime := newHistoryBenchmarkRuntime(&controllerRuntime{controller: controller})
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) { return runtime, nil }))
	tb.Cleanup(func() { _ = manager.Shutdown() })
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "history-bench"}); err != nil {
		tb.Fatal(err)
	}
	return executor, s, newBridgeServer(testToken, "history-bench", manager).handler()
}

func historyBenchmarkRead(tb testing.TB, handler http.Handler) *httptest.ResponseRecorder {
	tb.Helper()
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/history-bench/history", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		tb.Fatalf("history status %d: %s", response.Code, response.Body.String())
	}
	return response
}

func BenchmarkHistoryHTTP(b *testing.B) {
	for _, fixture := range []struct {
		name             string
		count, toolBytes int
	}{
		{"1K", 1_000, 0}, {"10K", 10_000, 0}, {"50K", 50_000, 0}, {"10KTool4KiB", 10_000, 4096},
	} {
		for _, mode := range []string{"Warm", "Append", "Rewrite", "Cold"} {
			b.Run(fixture.name+"/"+mode, func(b *testing.B) {
				messages := historyBenchmarkFixture(fixture.count, fixture.toolBytes)
				executor, session, handler := historyBenchmarkHost(b, messages)
				historyBenchmarkRead(b, handler) // warm both complete pipelines
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if mode != "Warm" {
						b.StopTimer()
						session = historyBenchmarkSession(messages)
						executor.SetSession(session)
						if mode != "Cold" {
							historyBenchmarkRead(b, handler)
						}
						b.StartTimer()
						switch mode {
						case "Append":
							session.AddBatch(provider.Message{ID: "append-user", Role: provider.RoleUser, Content: "follow-up"}, provider.Message{ID: "append-answer", Role: provider.RoleAssistant, Content: "new answer"})
						case "Rewrite":
							rewritten := append([]provider.Message(nil), messages...)
							rewritten[0].Content = "edited first question"
							session.Replace(rewritten)
						}
					}
					historyBenchmarkRead(b, handler)
				}
			})
		}
	}
}
