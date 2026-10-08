package desktopbridge

import (
	"context"
	"encoding/base64"
	"errors"
	"sync/atomic"
	"testing"
)

type terminalRuntimeFixture struct {
	*fakeRuntime
	block   chan struct{}
	entered chan struct{}
	calls   atomic.Int32
	closes  atomic.Int32
}

func TestTerminalEventsRejectInvalidOrOverBudgetFrames(t *testing.T) {
	s := NewEventStream(4)
	for _, frame := range []TerminalOutputView{
		{ID: "id", Data: "not-base64", End: 1},
		{ID: "id", Data: "YQ==", Start: 2, End: 1},
		{ID: "id", Data: "YQ==", End: 2},
		{ID: "id", Data: base64.StdEncoding.EncodeToString(make([]byte, (8<<10)+1)), End: (8 << 10) + 1},
	} {
		s.TerminalOutput("owned", frame)
	}
	if s.LatestSequence() != 0 {
		t.Fatal("invalid terminal frame entered event ledger")
	}
	s.TerminalOutput("owned", TerminalOutputView{ID: "id", Data: "YQ==", End: 1})
	if s.LatestSequence() != 1 {
		t.Fatal("valid terminal frame was rejected")
	}
}

func (r *terminalRuntimeFixture) TerminalWorkspace() (TerminalWorkspaceView, error) {
	r.calls.Add(1)
	if r.block != nil {
		close(r.entered)
		<-r.block
	}
	return TerminalWorkspaceView{Available: true, Sessions: []TerminalSessionView{{ID: "owned-terminal"}}, Shells: []TerminalShellView{}}, nil
}
func (r *terminalRuntimeFixture) CreateTerminal(context.Context, string, string) (TerminalSessionView, error) {
	_, err := r.TerminalWorkspace()
	return TerminalSessionView{ID: "late-owned-terminal"}, err
}
func (r *terminalRuntimeFixture) WriteTerminal(string, []byte) error    { r.calls.Add(1); return nil }
func (r *terminalRuntimeFixture) ResizeTerminal(string, int, int) error { return nil }
func (r *terminalRuntimeFixture) RenameTerminal(string, string) error   { return nil }
func (r *terminalRuntimeFixture) CloseTerminal(string) error            { r.closes.Add(1); return nil }
func (r *terminalRuntimeFixture) TerminalOutput(string) (TerminalOutputView, error) {
	_, err := r.TerminalWorkspace()
	return TerminalOutputView{ID: "owned-terminal", Data: "b2xk"}, err
}

func TestTerminalRuntimeOwnershipRejectsUnopenedForeignClosedAndUnsupported(t *testing.T) {
	r := &terminalRuntimeFixture{fakeRuntime: &fakeRuntime{path: "/owned", state: "idle"}}
	m := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return r, nil }))
	if _, err := m.TerminalWorkspace("owned"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatal("unopened session exposed terminals")
	}
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "owned"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.TerminalWorkspace("foreign"); !errors.Is(err, ErrSessionNotFound) || r.calls.Load() != 0 {
		t.Fatal("foreign session reached terminal provider")
	}
	if err := m.WriteTerminal("owned", "id", make([]byte, TerminalInputLimit+1)); !errors.Is(err, ErrTerminalInput) || r.calls.Load() != 0 {
		t.Fatal("oversized terminal input reached provider")
	}
	if _, err := m.TerminalWorkspace("owned"); err != nil {
		t.Fatal(err)
	}
	if err := m.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.TerminalWorkspace("owned"); !errors.Is(err, ErrClosed) {
		t.Fatal("closed owner exposed terminals")
	}
	unsupported := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
		return &fakeRuntime{path: "/owned", state: "idle"}, nil
	}))
	if _, err := unsupported.Open(context.Background(), OpenRequest{SessionID: "owned"}); err != nil {
		t.Fatal(err)
	}
	defer unsupported.Shutdown()
	if _, err := unsupported.TerminalWorkspace("owned"); !errors.Is(err, ErrTerminalUnavailable) {
		t.Fatal("unsupported runtime exposed terminal fallback")
	}
}

func TestTerminalResponsesFenceSwitchAwayAndBackWithoutBlockingOwnership(t *testing.T) {
	for _, action := range []string{"workspace", "create", "output"} {
		t.Run(action, func(t *testing.T) {
			old := &terminalRuntimeFixture{fakeRuntime: &fakeRuntime{path: "/old", state: "idle"},
				block: make(chan struct{}), entered: make(chan struct{})}
			calls := 0
			m := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) {
				calls++
				if calls == 1 {
					return old, nil
				}
				return &terminalRuntimeFixture{fakeRuntime: &fakeRuntime{path: "/new", state: "idle"}}, nil
			}))
			if _, err := m.Open(context.Background(), OpenRequest{SessionID: "same"}); err != nil {
				t.Fatal(err)
			}
			defer m.Shutdown()
			result := make(chan error, 1)
			go func() {
				var err error
				switch action {
				case "workspace":
					_, err = m.TerminalWorkspace("same")
				case "create":
					_, err = m.CreateTerminal(context.Background(), "same", ".", "sh")
				case "output":
					_, err = m.TerminalOutput("same", "owned-terminal")
				}
				result <- err
			}()
			<-old.entered
			if _, err := m.Switch(context.Background(), OpenRequest{SessionID: "other"}); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Switch(context.Background(), OpenRequest{SessionID: "same"}); err != nil {
				t.Fatal(err)
			}
			close(old.block)
			if err := <-result; !errors.Is(err, ErrSessionNotFound) {
				t.Fatal("stale terminal response crossed controller generation")
			}
			if action == "create" && old.closes.Load() != 1 {
				t.Fatal("late terminal creation was not cleaned up")
			}
		})
	}
}
