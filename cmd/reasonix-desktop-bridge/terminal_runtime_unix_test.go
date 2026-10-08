//go:build !windows

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"reasonix/internal/desktopbridge"
)

func TestActualControllerTerminalHTTPPTYSwitchAndShutdown(t *testing.T) {
	profile, home := t.TempDir(), t.TempDir()
	for key, value := range map[string]string{"HOME": home, "REASONIX_HOME": profile, "REASONIX_STATE_HOME": profile,
		"REASONIX_CACHE_HOME": t.TempDir(), "REASONIX_CREDENTIALS_STORE": "file", "ENV": "", "BASH_ENV": "", "ZDOTDIR": home, "SHELL": "/bin/sh"} {
		t.Setenv(key, value)
	}
	events := desktopbridge.NewEventStream(128)
	manager := desktopbridge.NewRuntimeManager(newControllerFactory(events))
	t.Cleanup(func() { _ = manager.Shutdown() })
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "terminal-global"}); err != nil {
		t.Fatal(err)
	}
	bridge := newBridgeServerWithEvents(testToken, "terminal-real-controller-fixture", manager, events)
	request := func(method, route, body, requestID string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, route, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+testToken)
		if requestID != "" {
			r.Header.Set(requestIDHeader, requestID)
		}
		w := httptest.NewRecorder()
		bridge.handler().ServeHTTP(w, r)
		return w
	}
	root := "/v1/sessions/terminal-global/terminal"
	response := request("POST", root, `{"path":".","shellId":"sh"}`, "real-owned-create")
	if response.Code != http.StatusCreated {
		t.Fatalf("actual terminal create status %d", response.Code)
	}
	var created terminalSessionResponse
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	target, err := manager.WorkspaceTarget("terminal-global")
	if err != nil || created.Terminal.Cwd != target.WorkspaceRoot {
		t.Fatal("terminal cwd did not use actual Global controller root")
	}
	term := root + "/" + created.Terminal.ID
	if request("POST", term+"/resize", `{"cols":100,"rows":30}`, "resize-owned").Code != http.StatusOK {
		t.Fatal("actual PTY resize failed")
	}
	input, _ := json.Marshal(terminalInputRequest{Data: base64.StdEncoding.EncodeToString([]byte("stty -echo\nprintf '%s\\n' 'bridge-''pty-accepted'\nprintf 'bridge-owned-pid:%s\\n' \"$$\"\nstty size\n"))})
	for i := 0; i < 2; i++ {
		if request("POST", term+"/input", string(input), "input-owned").Code != http.StatusAccepted {
			t.Fatal("actual terminal input admission failed")
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	var output string
	for time.Now().Before(deadline) {
		response = request("GET", term+"/output", "", "")
		var snapshot terminalOutputResponse
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &snapshot) != nil {
			t.Fatal("actual terminal snapshot failed")
		}
		decoded, err := base64.StdEncoding.DecodeString(snapshot.Output.Data)
		if err != nil {
			t.Fatal(err)
		}
		output = string(decoded)
		if strings.Contains(output, "bridge-pty-accepted") && strings.Contains(output, "30 100") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if strings.Count(output, "bridge-pty-accepted") != 1 || !strings.Contains(output, "30 100") {
		t.Fatal("real PTY execution/geometry/idempotence assertion failed")
	}
	pidMatch := regexp.MustCompile(`bridge-owned-pid:([0-9]+)`).FindStringSubmatch(output)
	if len(pidMatch) != 2 {
		t.Fatal("owned PTY process identity was not observed")
	}
	oldPID, err := strconv.Atoi(pidMatch[1])
	if err != nil || oldPID <= 1 {
		t.Fatal("owned PTY process identity invalid")
	}
	history, err := manager.History("terminal-global")
	if err != nil || len(history.Messages) != 0 {
		t.Fatal("terminal output entered model-visible transcript")
	}
	frames, _, _, cancel := events.Subscribe(0)
	cancel()
	found := false
	for _, frame := range frames {
		if frame.EventKind == "terminal_output" && frame.SessionID == "terminal-global" {
			found = true
		}
	}
	if !found {
		t.Fatal("actual controller emitted no owned terminal frames")
	}
	if _, err := manager.Switch(context.Background(), desktopbridge.OpenRequest{SessionID: "terminal-next"}); err != nil {
		t.Fatal(err)
	}
	assertOwnedTerminalPIDGone(t, oldPID)
	if _, err := manager.TerminalOutput("terminal-global", created.Terminal.ID); !errors.Is(err, desktopbridge.ErrSessionNotFound) {
		t.Fatal("old terminal crossed session switch")
	}
	if _, err := manager.TerminalOutput("terminal-next", created.Terminal.ID); !errors.Is(err, desktopbridge.ErrTerminalNotFound) {
		t.Fatal("old terminal survived into new owner")
	}
	view, err := manager.TerminalWorkspace("terminal-next")
	if err != nil || len(view.Sessions) != 0 {
		t.Fatal("new owner inherited old terminals")
	}
	newTerminal, err := manager.CreateTerminal(context.Background(), "terminal-next", ".", "sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.WriteTerminal("terminal-next", newTerminal.ID, []byte("stty -echo\nprintf 'shutdown-owned-pid:%s\\n' \"$$\"\n")); err != nil {
		t.Fatal(err)
	}
	var newPID int
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := manager.TerminalOutput("terminal-next", newTerminal.ID)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := base64.StdEncoding.DecodeString(snapshot.Data)
		match := regexp.MustCompile(`shutdown-owned-pid:([0-9]+)`).FindSubmatch(data)
		if len(match) == 2 {
			newPID, _ = strconv.Atoi(string(match[1]))
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if newPID <= 1 {
		t.Fatal("second owned PTY process identity missing")
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal("actual terminal/controller shutdown failed")
	}
	assertOwnedTerminalPIDGone(t, newPID)
}

// Probe only the PID printed by our own PTY fixture. Signal 0 is read-only;
// never kill by process name, a user PID, or a recycled/unknown identity.
func assertOwnedTerminalPIDGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("owned terminal process survived controller switch/shutdown")
}

type terminalRebuildGateFactory struct {
	*controllerFactory
	entered, release chan struct{}
}

func (f *terminalRebuildGateFactory) Open(ctx context.Context, req desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
	if f.entered != nil && req.ModelRef == "terminal/deepseek-v4-flash" {
		close(f.entered)
		<-f.release
	}
	return f.controllerFactory.Open(ctx, req)
}

func TestActualTerminalKeepsPTYAcrossModelEffortSettingsAndRecovery(t *testing.T) {
	profile, home := t.TempDir(), t.TempDir()
	for key, value := range map[string]string{"HOME": home, "REASONIX_HOME": profile, "REASONIX_STATE_HOME": profile,
		"REASONIX_CACHE_HOME": t.TempDir(), "REASONIX_CREDENTIALS_STORE": "file", "ENV": "", "BASH_ENV": "", "ZDOTDIR": home, "SHELL": "/bin/sh",
		"REASONIX_TERMINAL_REBUILD_FIXTURE_KEY": "fixture-only"} {
		t.Setenv(key, value)
	}
	// No provider request is needed for this test; the loopback server ensures
	// even an accidental request could not contact a real account or model.
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("terminal rebuild made an unexpected provider request")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer mock.Close()
	config := fmt.Sprintf(`default_model = "terminal/deepseek-v4-flash"
[[providers]]
name = "terminal"
kind = "openai"
base_url = "%s/v1"
api_key_env = "REASONIX_TERMINAL_REBUILD_FIXTURE_KEY"
models = ["deepseek-v4-flash", "ordinary"]
default = "deepseek-v4-flash"
no_proxy = true
`, mock.URL)
	if err := os.WriteFile(filepath.Join(profile, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	credentialPath := filepath.Join(profile, ".env")
	if err := os.WriteFile(credentialPath, []byte("REASONIX_TERMINAL_REBUILD_FIXTURE_KEY=fixture-only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	events := desktopbridge.NewEventStream(512)
	factory := &terminalRebuildGateFactory{controllerFactory: newControllerFactory(events)}
	manager := desktopbridge.NewRuntimeManager(factory)
	t.Cleanup(func() { _ = manager.Shutdown() })
	ctx := context.Background()
	if _, err := manager.Open(ctx, desktopbridge.OpenRequest{SessionID: "terminal-rebuild", WorkspaceRoot: t.TempDir(), ModelRef: "terminal/deepseek-v4-flash"}); err != nil {
		t.Fatal(err)
	}
	terminal, err := manager.CreateTerminal(ctx, "terminal-rebuild", ".", "sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.WriteTerminal("terminal-rebuild", terminal.ID, []byte("stty -echo\n")); err != nil {
		t.Fatal(err)
	}
	var originalPID int
	assertSamePTY := func(stage string) {
		t.Helper()
		if err := manager.WriteTerminal("terminal-rebuild", terminal.ID, []byte(fmt.Sprintf("printf 'terminal-stage-%s:%%s\\n' \"$$\"\n", stage))); err != nil {
			t.Fatal(err)
		}
		pattern := regexp.MustCompile("terminal-stage-" + stage + `:([0-9]+)`)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			snapshot, err := manager.TerminalOutput("terminal-rebuild", terminal.ID)
			if err != nil {
				t.Fatal(err)
			}
			bytes, err := base64.StdEncoding.DecodeString(snapshot.Data)
			if err != nil {
				t.Fatal(err)
			}
			match := pattern.FindSubmatch(bytes)
			if len(match) == 2 {
				pid, _ := strconv.Atoi(string(match[1]))
				if pid <= 1 {
					t.Fatal("owned PTY PID invalid")
				}
				if originalPID == 0 {
					originalPID = pid
				} else if pid != originalPID {
					t.Fatal("replacement restarted the shell instead of preserving the PTY")
				}
				workspace, err := manager.TerminalWorkspace("terminal-rebuild")
				if err != nil || len(workspace.Sessions) != 1 || workspace.Sessions[0].ID != terminal.ID || !workspace.Sessions[0].Running {
					t.Fatal("terminal identity was lost or duplicated")
				}
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("retained PTY stopped accepting input")
	}
	assertSamePTY("initial")
	if _, err := manager.SetSessionEffort(ctx, "terminal-rebuild", "terminal/deepseek-v4-flash", "high"); err != nil {
		t.Fatal(err)
	}
	assertSamePTY("effort")
	if _, err := manager.SetSessionModel(ctx, "terminal-rebuild", "terminal/ordinary"); err != nil {
		t.Fatal(err)
	}
	assertSamePTY("model")
	if err := os.WriteFile(credentialPath, []byte("REASONIX_TERMINAL_REBUILD_FIXTURE_KEY=second-fixture-only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RebuildSettings(ctx, "terminal-rebuild"); err != nil {
		t.Fatal(err)
	}
	assertSamePTY("settings")
	if _, err := manager.SetSessionModel(ctx, "terminal-rebuild", "terminal/missing-model"); !errors.Is(err, desktopbridge.ErrSessionModelSwitch) {
		t.Fatal("failed model switch did not restore the prior controller", err)
	}
	assertSamePTY("recovery")
	history, err := manager.History("terminal-rebuild")
	if err != nil || len(history.Messages) != 0 {
		t.Fatal("terminal output entered transcript during rebuild")
	}
	// Block the actual factory after outgoing snapshot, when the terminals are
	// owned by a retention lease rather than either controller. Shutdown must
	// reclaim the known shell NOW, without waiting for this build to finish.
	factory.entered, factory.release = make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := manager.SetSessionModel(ctx, "terminal-rebuild", "terminal/deepseek-v4-flash")
		finished <- err
	}()
	<-factory.entered
	if err := manager.Shutdown(); err != nil {
		close(factory.release)
		<-finished
		t.Fatal(err)
	}
	// Ensure a failed PID assertion still releases and joins the test goroutine.
	func() {
		defer func() {
			close(factory.release)
			if err := <-finished; !errors.Is(err, desktopbridge.ErrClosed) {
				t.Error("late controller was published after shutdown", err)
			}
		}()
		assertOwnedTerminalPIDGone(t, originalPID)
	}()
}
