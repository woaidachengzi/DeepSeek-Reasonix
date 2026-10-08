package desktopterminal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/terminalprocess"
)

type fixtureProcess struct {
	reader       *io.PipeReader
	output       *io.PipeWriter
	closed       chan struct{}
	closeOnce    sync.Once
	writes       chan []byte
	writeEntered chan struct{}
	enterOnce    sync.Once
	writeGate    chan struct{}
}

func newFixtureProcess() *fixtureProcess {
	r, w := io.Pipe()
	return &fixtureProcess{reader: r, output: w, closed: make(chan struct{}), writes: make(chan []byte, 16), writeEntered: make(chan struct{})}
}

func (p *fixtureProcess) Read(data []byte) (int, error) { return p.reader.Read(data) }
func (p *fixtureProcess) Write(data []byte) (int, error) {
	p.enterOnce.Do(func() { close(p.writeEntered) })
	if p.writeGate != nil {
		select {
		case <-p.writeGate:
		case <-p.closed:
			return 0, io.ErrClosedPipe
		}
	}
	select {
	case <-p.closed:
		return 0, io.ErrClosedPipe
	case p.writes <- append([]byte(nil), data...):
		return len(data), nil
	}
}
func (p *fixtureProcess) Resize(int, int) error { return nil }
func (p *fixtureProcess) Wait() (int, error)    { <-p.closed; return 0, nil }
func (p *fixtureProcess) Close() error {
	p.closeOnce.Do(func() { close(p.closed); _ = p.reader.Close(); _ = p.output.Close() })
	return nil
}

func fixtureManager(t *testing.T) *Manager {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REASONIX_HOME", t.TempDir())
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	m, err := New(t.TempDir(), "owned-session", desktopbridge.NewEventStream(128))
	if err != nil {
		t.Fatal(err)
	}
	m.start = func(terminalprocess.Spec) (terminalprocess.Process, error) { return newFixtureProcess(), nil }
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func fixtureShell() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "sh"
}

func mustCreate(t *testing.T, m *Manager) desktopbridge.TerminalSessionView {
	t.Helper()
	view, err := m.Create(context.Background(), ".", fixtureShell())
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func eventually(t *testing.T, check func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal(description)
}

func TestDirectoryAndShellAdmissionNeverStartsUnownedTargets(t *testing.T) {
	m := fixtureManager(t)
	calls := 0
	m.start = func(spec terminalprocess.Spec) (terminalprocess.Process, error) {
		calls++
		if !filepath.IsAbs(spec.Path) || spec.Dir != m.root {
			t.Error("backend start spec did not use owned paths")
		}
		return newFixtureProcess(), nil
	}
	for _, path := range []string{"../", "sub/../../outside", t.TempDir(), ".\x00", strings.Repeat("a", 4097)} {
		if _, err := m.Create(context.Background(), path, fixtureShell()); !errors.Is(err, desktopbridge.ErrTerminalInput) {
			t.Fatal("unsafe terminal directory accepted")
		}
	}
	for _, shell := range []string{"/bin/sh", "sh -c whoami", "../../sh", "shell-owned-sentinel"} {
		if _, err := m.Create(context.Background(), ".", shell); !errors.Is(err, desktopbridge.ErrTerminalInput) {
			t.Fatal("renderer executable/arguments accepted as shell ID")
		}
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(m.root, "external")); err == nil {
		if _, err := m.Create(context.Background(), "external", fixtureShell()); !errors.Is(err, desktopbridge.ErrTerminalInput) {
			t.Fatal("external symlink directory accepted")
		}
	}
	if calls != 0 {
		t.Fatal("unsafe request reached process start")
	}
	if err := os.WriteFile(filepath.Join(m.root, "owned.txt"), []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	view, err := m.Create(context.Background(), "owned.txt", fixtureShell())
	if err != nil || view.Cwd != m.root || calls != 1 {
		t.Fatal("owned file did not resolve to its parent directory")
	}
}

func TestOutputSnapshotAndEventsPreserveBytesWithBoundedOffsets(t *testing.T) {
	m := fixtureManager(t)
	p := newFixtureProcess()
	m.start = func(terminalprocess.Spec) (terminalprocess.Process, error) { return p, nil }
	view := mustCreate(t, m)
	raw := append(bytes.Repeat([]byte{0xff, 0, 0x1b, '[', 0xe4}, OutputLimit/5+3), []byte("owned-end")...)
	if _, err := p.output.Write(raw); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { output, _ := m.Output(view.ID); return output.End == uint64(len(raw)) }, "terminal output did not settle")
	output, err := m.Output(view.ID)
	if err != nil || output.Start != uint64(len(raw)-OutputLimit) {
		t.Fatal("snapshot byte offsets are wrong")
	}
	decoded, err := base64.StdEncoding.DecodeString(output.Data)
	if err != nil || !bytes.Equal(decoded, raw[len(raw)-OutputLimit:]) {
		t.Fatal("snapshot corrupted terminal bytes")
	}
	events, resync, _, cancel := m.events.Subscribe(0)
	cancel()
	if resync || len(events) == 0 {
		t.Fatal("bounded fixture events missing")
	}
	var combined []byte
	for _, frame := range events {
		if frame.EventKind != "terminal_output" || frame.SessionID != "owned-session" {
			t.Fatal("terminal bytes became Agent events or crossed session ownership")
		}
		var chunk desktopbridge.TerminalOutputView
		if err := json.Unmarshal(frame.Payload, &chunk); err != nil || chunk.Start != uint64(len(combined)) {
			t.Fatal("terminal frame offsets are wrong")
		}
		data, err := base64.StdEncoding.DecodeString(chunk.Data)
		if err != nil || len(data) > 8<<10 || chunk.End-chunk.Start != uint64(len(data)) {
			t.Fatal("terminal frame byte budget is wrong")
		}
		combined = append(combined, data...)
	}
	if !bytes.Equal(combined, raw) {
		t.Fatal("terminal live frames lost byte identity")
	}
}

func TestInputQueueCopiesOrdersAndBoundsBlockedWrites(t *testing.T) {
	m := fixtureManager(t)
	p := newFixtureProcess()
	p.writeGate = make(chan struct{})
	m.start = func(terminalprocess.Spec) (terminalprocess.Process, error) { return p, nil }
	view := mustCreate(t, m)
	if err := m.Write(view.ID, []byte("first")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.writeEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("terminal writer did not start")
	}
	data := []byte("owned")
	for i := 0; i < inputQueue; i++ {
		if err := m.Write(view.ID, data); err != nil {
			t.Fatal(err)
		}
	}
	copy(data, "other")
	if !errors.Is(m.Write(view.ID, []byte("overflow")), desktopbridge.ErrTerminalBusy) {
		t.Fatal("blocked write queue was not bounded")
	}
	if !errors.Is(m.Write(view.ID, nil), desktopbridge.ErrTerminalInput) ||
		!errors.Is(m.Write(view.ID, make([]byte, desktopbridge.TerminalInputLimit+1)), desktopbridge.ErrTerminalInput) {
		t.Fatal("terminal input byte budget not enforced")
	}
	close(p.writeGate)
	for i := 0; i < inputQueue+1; i++ {
		select {
		case actual := <-p.writes:
			want := "owned"
			if i == 0 {
				want = "first"
			}
			if string(actual) != want {
				t.Fatal("queued terminal input changed or reordered")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("queued terminal input did not drain")
		}
	}
	if err := m.CloseTerminal(view.ID); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(m.Write(view.ID, []byte("late")), desktopbridge.ErrTerminalNotFound) {
		t.Fatal("closed terminal admitted late input")
	}
}

func TestLimitRenameResizeAndForeignIDs(t *testing.T) {
	m := fixtureManager(t)
	for i := 0; i < SessionLimit; i++ {
		mustCreate(t, m)
	}
	if _, err := m.Create(context.Background(), ".", fixtureShell()); !errors.Is(err, desktopbridge.ErrTerminalBusy) {
		t.Fatal("terminal session budget not enforced")
	}
	view, err := m.Workspace()
	if err != nil || len(view.Sessions) != SessionLimit || len(view.Shells) == 0 {
		t.Fatal("terminal workspace inventory incomplete")
	}
	id := view.Sessions[0].ID
	if err := m.Rename(id, "自有终端"); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"", "\x00", "\x1b", "line\nnext", strings.Repeat("中", 81)} {
		if !errors.Is(m.Rename(id, title), desktopbridge.ErrTerminalInput) {
			t.Fatal("unsafe terminal title accepted")
		}
	}
	if !errors.Is(m.Resize(id, 1001, 24), desktopbridge.ErrTerminalInput) ||
		!errors.Is(m.Resize(id, 80, 0), desktopbridge.ErrTerminalInput) {
		t.Fatal("resize budget not enforced")
	}
	if !errors.Is(m.Write("foreign", []byte("x")), desktopbridge.ErrTerminalNotFound) {
		t.Fatal("foreign terminal accepted input")
	}
	if _, err := m.Output("foreign"); !errors.Is(err, desktopbridge.ErrTerminalNotFound) {
		t.Fatal("foreign terminal exposed output")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Workspace(); !errors.Is(err, desktopbridge.ErrTerminalUnavailable) {
		t.Fatal("closed owner exposed terminal workspace")
	}
	if _, err := m.Create(context.Background(), ".", fixtureShell()); !errors.Is(err, desktopbridge.ErrTerminalUnavailable) {
		t.Fatal("closed owner created terminal")
	}
}

func TestCloseUnblocksPendingWriteAndDropsQueuedInput(t *testing.T) {
	m := fixtureManager(t)
	p := newFixtureProcess()
	p.writeGate = make(chan struct{})
	m.start = func(terminalprocess.Spec) (terminalprocess.Process, error) { return p, nil }
	view := mustCreate(t, m)
	if err := m.Write(view.ID, []byte("blocked")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.writeEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("blocked writer did not start")
	}
	if err := m.Write(view.ID, []byte("must-not-run")); err != nil {
		t.Fatal(err)
	}
	if err := m.CloseTerminal(view.ID); err != nil {
		t.Fatal("close did not cancel blocked terminal I/O")
	}
	select {
	case <-p.writes:
		t.Fatal("closed terminal executed queued input")
	default:
	}
}

func TestTerminalEventReplayGapHasAuthoritativeBoundedSnapshot(t *testing.T) {
	m := fixtureManager(t)
	m.events = desktopbridge.NewEventStream(1)
	p := newFixtureProcess()
	m.start = func(terminalprocess.Spec) (terminalprocess.Process, error) { return p, nil }
	view := mustCreate(t, m)
	for _, data := range []string{"first", "second"} {
		if _, err := p.output.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	eventually(t, func() bool { output, _ := m.Output(view.ID); return output.End == 11 }, "terminal snapshot did not catch up")
	_, resync, live, cancel := m.events.Subscribe(0)
	cancel()
	if !resync || live != nil {
		t.Fatal("stale event cursor did not require resync")
	}
	output, err := m.Output(view.ID)
	data, decodeErr := base64.StdEncoding.DecodeString(output.Data)
	if err != nil || decodeErr != nil || string(data) != "firstsecond" || output.Start != 0 {
		t.Fatal("resync snapshot lost terminal byte identity")
	}
}

func TestBrokenReaderClosesOwnProcessAndPublishesTerminalExit(t *testing.T) {
	m := fixtureManager(t)
	p := newFixtureProcess()
	m.start = func(terminalprocess.Spec) (terminalprocess.Process, error) { return p, nil }
	view := mustCreate(t, m)
	if err := p.output.Close(); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		workspace, err := m.Workspace()
		return err == nil && len(workspace.Sessions) == 1 && !workspace.Sessions[0].Running && workspace.Sessions[0].ExitCode != nil
	}, "broken reader left terminal running")
	select {
	case <-p.closed:
	default:
		t.Fatal("broken reader left own process alive")
	}
	if !errors.Is(m.Write(view.ID, []byte("late")), desktopbridge.ErrTerminalUnavailable) {
		t.Fatal("exited terminal accepted input")
	}
	frames, _, _, cancel := m.events.Subscribe(0)
	cancel()
	if len(frames) != 1 || frames[0].EventKind != "terminal_exit" || frames[0].SessionID != "owned-session" {
		t.Fatal("broken reader exit was not session-scoped")
	}
}

func TestSlowCreateCannotRegisterAfterShutdownOrCancellation(t *testing.T) {
	for _, cancelOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "shutdown", true: "cancel"}[cancelOnly], func(t *testing.T) {
			m := fixtureManager(t)
			p := newFixtureProcess()
			entered, release := make(chan struct{}), make(chan struct{})
			m.start = func(terminalprocess.Spec) (terminalprocess.Process, error) { close(entered); <-release; return p, nil }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := m.Create(ctx, ".", fixtureShell()); result <- err }()
			<-entered
			if cancelOnly {
				cancel()
			} else if !errors.Is(m.Close(), desktopbridge.ErrTerminalUnavailable) {
				t.Fatal("pending start incorrectly reported clean shutdown")
			}
			close(release)
			if err := <-result; !errors.Is(err, desktopbridge.ErrTerminalUnavailable) {
				t.Fatal("stale create was admitted")
			}
			select {
			case <-p.closed:
			case <-time.After(5 * time.Second):
				t.Fatal("stale created process survived cancellation")
			}
			m.mu.Lock()
			count := len(m.terminals)
			m.mu.Unlock()
			if count != 0 {
				t.Fatal("stale create registered a terminal")
			}
		})
	}
}
