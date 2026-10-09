package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/remote/controller"
	"reasonix/internal/stats"
)

func closeDesktopReadTestUsage(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := stats.CloseUsageCatalogs(ctx); err != nil {
		t.Fatal("close usage projection before temporary profile cleanup", err)
	}
}

func TestDesktopSessionPendingStrictReadAndBindingGate(t *testing.T) {
	closeDesktopReadTestUsage(t)
	f := newOwnershipFixture(t)
	t.Cleanup(func() { closeDesktopReadTestUsage(t) })
	scope := controller.SessionPendingScope{SessionPath: agent.CanonicalSessionPath(f.active), RuntimeEpoch: runtimeStateOf(f.server.ctl()).RuntimeEpoch}
	body, _ := json.Marshal(struct {
		ProtocolVersion int `json:"protocolVersion"`
		controller.SessionPendingScope
	}{1, scope})
	for _, input := range []string{"{}", string(body) + " {}", strings.TrimSuffix(string(body), "}") + ",\"answer\":true}", strings.Repeat("x", (256<<10)+1)} {
		response, err := http.Post(f.srv.URL+"/desktop/session-pending", "application/json", strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 400 {
			t.Fatal("bad read body accepted", response.StatusCode)
		}
	}
	f.server.bindMu.Lock()
	response, err := http.Post(f.srv.URL+"/desktop/session-pending", "application/json", strings.NewReader(string(body)))
	f.server.bindMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 409 {
		t.Fatal("read waited on/rebound owner")
	}
}

func TestDesktopSessionPendingIdleAndForeignAreReadOnly(t *testing.T) {
	closeDesktopReadTestUsage(t)
	f, client := desktopViewFixture(t)
	t.Cleanup(func() { closeDesktopReadTestUsage(t) })
	scope := controller.SessionPendingScope{SessionPath: agent.CanonicalSessionPath(f.active), RuntimeEpoch: runtimeStateOf(f.server.ctl()).RuntimeEpoch}
	result, err := client.ReadSessionPending(context.Background(), scope)
	if err != nil || result.Prompts == nil || len(result.Prompts) != 0 {
		t.Fatal(result, err)
	}
	f.server.bindMu.Lock()
	f.server.runtimeLeaseProbe = func(string) bool { return true }
	f.server.bindMu.Unlock()
	if _, err := client.ReadSessionPending(context.Background(), scope); err == nil {
		t.Fatal("foreign owner read admitted")
	}
}
