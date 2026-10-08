// Package desktopterminal owns interactive terminals for one local session
// workspace, independently of model/controller replacements. It is shared Go
// backend code, not a renderer command executor.
package desktopterminal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/secrets"
	"reasonix/internal/terminalprocess"
)

const (
	SessionLimit = 10
	OutputLimit  = 128 << 10
	closeWait    = 2 * time.Second
	inputQueue   = 4
)

type terminal struct {
	view      desktopbridge.TerminalSessionView
	process   terminalprocess.Process
	input     chan []byte
	stop      chan struct{}
	stopOnce  sync.Once
	readDone  chan struct{}
	done      chan struct{}
	closeDone chan struct{}
	closeOnce sync.Once
	closeErr  error
	output    []byte
	end       uint64
}

func (t *terminal) stopInput() { t.stopOnce.Do(func() { close(t.stop) }) }

type Manager struct {
	root      string
	sessionID string
	events    *desktopbridge.EventStream
	mu        sync.Mutex
	closed    bool
	starting  int
	closing   int
	order     []string
	terminals map[string]*terminal
	start     func(terminalprocess.Spec) (terminalprocess.Process, error)
}

func New(root, sessionID string, events *desktopbridge.EventStream) (*Manager, error) {
	root, err := canonicalDir(root)
	if err != nil || strings.TrimSpace(sessionID) == "" {
		return nil, desktopbridge.ErrTerminalUnavailable
	}
	return &Manager{root: root, sessionID: sessionID, events: events, terminals: make(map[string]*terminal), start: terminalprocess.Start}, nil
}

// MatchesOwner checks actual backend identity before same-session transfer.
// Manager identity is immutable; renderer input cannot rebind roots or events.
func (m *Manager) MatchesOwner(root, sessionID string, events *desktopbridge.EventStream) bool {
	actual, err := canonicalDir(root)
	return err == nil && actual == m.root && sessionID == m.sessionID && events == m.events
}

func canonicalDir(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", desktopbridge.ErrTerminalInput
	}
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", desktopbridge.ErrTerminalUnavailable
	}
	info, err := os.Stat(actual)
	if err != nil || !info.IsDir() {
		return "", desktopbridge.ErrTerminalUnavailable
	}
	return filepath.Clean(actual), nil
}

func (m *Manager) directory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	if filepath.IsAbs(path) || strings.ContainsRune(path, 0) || len(path) > 4096 {
		return "", desktopbridge.ErrTerminalInput
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", desktopbridge.ErrTerminalInput
	}
	root, err := canonicalDir(m.root)
	if err != nil || root != m.root {
		return "", desktopbridge.ErrTerminalUnavailable
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, clean))
	if err != nil {
		return "", desktopbridge.ErrTerminalUnavailable
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", desktopbridge.ErrTerminalInput
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", desktopbridge.ErrTerminalUnavailable
	}
	if info.Mode().IsRegular() {
		target = filepath.Dir(target)
	} else if !info.IsDir() {
		return "", desktopbridge.ErrTerminalInput
	}
	return target, nil
}

func (m *Manager) Workspace() (desktopbridge.TerminalWorkspaceView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return desktopbridge.TerminalWorkspaceView{}, desktopbridge.ErrTerminalUnavailable
	}
	available, reason := terminalprocess.Available()
	view := desktopbridge.TerminalWorkspaceView{Available: available, Reason: reason,
		Sessions: []desktopbridge.TerminalSessionView{}, Shells: shellOptions()}
	for _, id := range m.order {
		view.Sessions = append(view.Sessions, m.terminals[id].view)
	}
	return view, nil
}

func (m *Manager) Create(ctx context.Context, path, shellID string) (desktopbridge.TerminalSessionView, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return desktopbridge.TerminalSessionView{}, desktopbridge.ErrTerminalUnavailable
	}
	if len(m.terminals)+m.starting+m.closing >= SessionLimit {
		m.mu.Unlock()
		return desktopbridge.TerminalSessionView{}, desktopbridge.ErrTerminalBusy
	}
	m.starting++
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.starting--; m.mu.Unlock() }()
	if err := ctx.Err(); err != nil {
		return desktopbridge.TerminalSessionView{}, err
	}
	dir, err := m.directory(path)
	if err != nil {
		return desktopbridge.TerminalSessionView{}, err
	}
	command, err := resolveShell(shellID)
	if err != nil {
		return desktopbridge.TerminalSessionView{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return desktopbridge.TerminalSessionView{}, desktopbridge.ErrTerminalUnavailable
	}
	// Recheck the close gate immediately before starting, then again before
	// registration. Shutdown never waits on the creation/exec syscall.
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed || ctx.Err() != nil {
		return desktopbridge.TerminalSessionView{}, desktopbridge.ErrTerminalUnavailable
	}
	process, err := m.start(terminalprocess.Spec{Path: command.path, Args: command.args, Dir: dir,
		Env: terminalEnvironment(secrets.ProcessEnv()), Columns: 80, Rows: 24})
	if err != nil {
		return desktopbridge.TerminalSessionView{}, desktopbridge.ErrTerminalUnavailable
	}
	t := &terminal{view: desktopbridge.TerminalSessionView{ID: hex.EncodeToString(random[:]), Title: command.label,
		Shell: command.label, Cwd: dir, CreatedAt: time.Now().UnixMilli(), Running: true}, process: process,
		input: make(chan []byte, inputQueue), stop: make(chan struct{}), readDone: make(chan struct{}),
		done: make(chan struct{}), closeDone: make(chan struct{})}
	m.mu.Lock()
	if m.closed || ctx.Err() != nil {
		m.closing++
		m.mu.Unlock()
		// A never-registered terminal emits no output/exit events. Unix Start
		// already owns its reaper; ConPTY Close releases the native handles.
		go func() {
			_ = process.Close()
			m.mu.Lock()
			m.closing--
			m.mu.Unlock()
		}()
		return desktopbridge.TerminalSessionView{}, desktopbridge.ErrTerminalUnavailable
	}
	m.terminals[t.view.ID] = t
	m.order = append(m.order, t.view.ID)
	view := t.view
	m.mu.Unlock()
	go m.read(t)
	go m.write(t)
	go m.wait(t)
	return view, nil
}

// Write acknowledges one copied, bounded input queue entry. It does not wait
// on a potentially blocked OS write while holding the controller owner lock.
func (m *Manager) Write(id string, data []byte) error {
	if len(data) == 0 || len(data) > desktopbridge.TerminalInputLimit {
		return desktopbridge.ErrTerminalInput
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := m.owned(id)
	if err != nil {
		return err
	}
	if !t.view.Running {
		return desktopbridge.ErrTerminalUnavailable
	}
	select {
	case t.input <- append([]byte(nil), data...):
		return nil
	default:
		return desktopbridge.ErrTerminalBusy
	}
}

func (m *Manager) owned(id string) (*terminal, error) {
	if m.closed {
		return nil, desktopbridge.ErrTerminalUnavailable
	}
	t := m.terminals[strings.TrimSpace(id)]
	if t == nil {
		return nil, desktopbridge.ErrTerminalNotFound
	}
	return t, nil
}

func (m *Manager) Resize(id string, columns, rows int) error {
	if columns <= 0 || columns > terminalprocess.MaxColumns || rows <= 0 || rows > terminalprocess.MaxRows {
		return desktopbridge.ErrTerminalInput
	}
	m.mu.Lock()
	t, err := m.owned(id)
	m.mu.Unlock()
	if err != nil {
		return err
	}
	if err := t.process.Resize(columns, rows); err != nil {
		return desktopbridge.ErrTerminalUnavailable
	}
	return nil
}

func (m *Manager) Rename(id, title string) error {
	title = strings.TrimSpace(title)
	if title == "" || !utf8.ValidString(title) || len([]rune(title)) > 80 || strings.ContainsAny(title, "\x00\r\n") {
		return desktopbridge.ErrTerminalInput
	}
	for _, r := range title {
		if unicode.IsControl(r) {
			return desktopbridge.ErrTerminalInput
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := m.owned(id)
	if err == nil {
		t.view.Title = title
	}
	return err
}

func (m *Manager) Output(id string) (desktopbridge.TerminalOutputView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := m.owned(id)
	if err != nil {
		return desktopbridge.TerminalOutputView{}, err
	}
	return desktopbridge.TerminalOutputView{ID: t.view.ID, Data: base64.StdEncoding.EncodeToString(t.output),
		Start: t.end - uint64(len(t.output)), End: t.end}, nil
}

func appendOutput(old, chunk []byte) []byte {
	if len(chunk) >= OutputLimit {
		return append([]byte(nil), chunk[len(chunk)-OutputLimit:]...)
	}
	if overflow := len(old) + len(chunk) - OutputLimit; overflow > 0 {
		old = append([]byte(nil), old[overflow:]...)
	}
	return append(old, chunk...)
}

func (m *Manager) read(t *terminal) {
	defer close(t.readDone)
	buffer := make([]byte, 8<<10)
	for {
		n, err := t.process.Read(buffer)
		if n > 0 {
			m.mu.Lock()
			if m.terminals[t.view.ID] == t && !m.closed {
				start := t.end
				t.end += uint64(n)
				t.output = appendOutput(t.output, buffer[:n])
				m.events.TerminalOutput(m.sessionID, desktopbridge.TerminalOutputView{ID: t.view.ID,
					Data: base64.StdEncoding.EncodeToString(buffer[:n]), Start: start, End: t.end})
			}
			m.mu.Unlock()
		}
		if err != nil {
			// A broken terminal reader must not leave an invisible shell alive
			// accepting input. Preserve final bytes above, then close this PTY.
			m.beginClose(t)
			return
		}
	}
}

func (m *Manager) write(t *terminal) {
	for {
		select {
		case <-t.stop:
			return
		case data := <-t.input:
			for len(data) > 0 {
				select {
				case <-t.stop:
					return
				default:
				}
				n, err := t.process.Write(data)
				if err != nil || n <= 0 || n > len(data) {
					m.beginClose(t)
					return
				}
				data = data[n:]
			}
		}
	}
}

func (m *Manager) wait(t *terminal) {
	code, err := t.process.Wait()
	if err != nil && code == 0 {
		code = -1
	}
	select {
	case <-t.readDone:
	case <-time.After(closeWait):
	}
	m.beginClose(t)
	m.mu.Lock()
	t.view.Running, t.view.ExitCode = false, &code
	removed := m.terminals[t.view.ID] != t || m.closed
	if !m.closed {
		m.events.TerminalExit(m.sessionID, desktopbridge.TerminalExitView{ID: t.view.ID, ExitCode: code, Removed: removed})
	}
	m.mu.Unlock()
	close(t.done)
}

func (m *Manager) beginClose(t *terminal) {
	t.stopInput()
	t.closeOnce.Do(func() {
		go func() {
			t.closeErr = t.process.Close()
			close(t.closeDone)
		}()
	})
}

func (m *Manager) detach(t *terminal) {
	delete(m.terminals, t.view.ID)
	for i, id := range m.order {
		if id == t.view.ID {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.closing++
	go func() {
		<-t.done
		<-t.closeDone
		m.mu.Lock()
		m.closing--
		m.mu.Unlock()
	}()
}

func waitClosed(t *terminal, deadline <-chan time.Time) error {
	for _, done := range []<-chan struct{}{t.closeDone, t.done} {
		select {
		case <-done:
		case <-deadline:
			return desktopbridge.ErrTerminalUnavailable
		}
	}
	if t.closeErr != nil && !errors.Is(t.closeErr, os.ErrClosed) && !errors.Is(t.closeErr, io.ErrClosedPipe) {
		return desktopbridge.ErrTerminalUnavailable
	}
	return nil
}

func (m *Manager) CloseTerminal(id string) error {
	m.mu.Lock()
	t, err := m.owned(id)
	if err == nil {
		m.detach(t)
	}
	m.mu.Unlock()
	if err != nil {
		return err
	}
	m.beginClose(t)
	deadline := time.NewTimer(closeWait)
	defer deadline.Stop()
	return waitClosed(t, deadline.C)
}

func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		pending := m.closing != 0 || m.starting != 0
		m.mu.Unlock()
		if pending {
			return desktopbridge.ErrTerminalUnavailable
		}
		return nil
	}
	m.closed = true
	all := make([]*terminal, 0, len(m.terminals))
	for _, t := range m.terminals {
		all = append(all, t)
	}
	for _, t := range all {
		m.detach(t)
	}
	m.mu.Unlock()
	for _, t := range all {
		m.beginClose(t)
	}
	deadline := time.NewTimer(closeWait)
	defer deadline.Stop()
	for _, t := range all {
		if err := waitClosed(t, deadline.C); err != nil {
			return err
		}
	}
	m.mu.Lock()
	pendingStart := m.starting != 0
	m.mu.Unlock()
	if pendingStart {
		return desktopbridge.ErrTerminalUnavailable
	}
	return nil
}
