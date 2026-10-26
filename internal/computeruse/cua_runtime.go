package computeruse

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

const cuaProtocol = 1
const maxCuaReply = 16 << 20
const cuaSourceRevision = "e754cfec69c41400eb2aabde716c27d15607b2f0"

type CuaImage struct {
	MIME string `json:"mime_type"`
	Data string `json:"data_base64"`
}

type CuaReply struct {
	Protocol       int             `json:"protocol"`
	ID             uint64          `json:"id"`
	Driver         string          `json:"driver"`
	Version        string          `json:"version"`
	SourceRevision string          `json:"source_revision"`
	Result         json.RawMessage `json:"result"`
	Text           string          `json:"text"`
	Images         []CuaImage      `json:"images,omitempty"`
	Action         json.RawMessage `json:"action,omitempty"`
	IsError        bool            `json:"is_error"`
	ErrorCode      string          `json:"error_code,omitempty"`
	Error          string          `json:"error,omitempty"`
	Completion     string          `json:"completion,omitempty"`
}

// Transport uncertainty must never be turned into a retryable refusal.
type CuaTransportError struct{ Cause error }

func (e *CuaTransportError) Error() string { return e.Cause.Error() }
func (e *CuaTransportError) Unwrap() error { return e.Cause }

type cuaProcess struct {
	cmd   *exec.Cmd
	input io.WriteCloser
	lines *bufio.Scanner
	done  chan struct{}
}

// CuaRuntime has no network endpoint. One private child belongs to exactly one
// host Run; cancellation kills it and invalidates all SDK snapshots/tokens.
type CuaRuntime struct {
	binary   string
	args     []string
	mu       sync.Mutex
	lifetime context.Context
	process  *cuaProcess
	sequence uint64
	tasks    map[context.Context]*CuaRuntime
}

func BundledCuaRuntime() *CuaRuntime {
	if os.Getenv("REASONIX_COMPUTER_USE_ENGINE") == "native" {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return nil
	}
	// Installed bundles place both signed executables in Contents/MacOS.
	binary := filepath.Join(filepath.Dir(executable), "reasonix-cua-runtime")
	if info, err := os.Lstat(binary); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
		return &CuaRuntime{binary: binary, tasks: make(map[context.Context]*CuaRuntime)}
	}
	return nil
}

func (c *CuaRuntime) stop() {
	if c.process == nil {
		return
	}
	p := c.process
	c.process = nil
	_ = p.input.Close()
	_ = p.cmd.Process.Kill()
	<-p.done
}

func (c *CuaRuntime) start(lifetime context.Context) error {
	cmd := exec.CommandContext(lifetime, c.binary, c.args...)
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return err
	}
	// Do not inherit model keys, user config, shell init, or driver overrides.
	cmd.Env = cuaProcessEnvironment(runtime.GOOS)
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = input.Close()
		_ = output.Close()
		return err
	}
	p := &cuaProcess{cmd: cmd, input: input, lines: bufio.NewScanner(output), done: make(chan struct{})}
	p.lines.Buffer(make([]byte, 64<<10), maxCuaReply)
	c.process, c.lifetime = p, lifetime
	go func() {
		_ = cmd.Wait()
		_ = input.Close()
		close(p.done)
	}()
	return nil
}

func (c *CuaRuntime) Call(ctx, lifetime context.Context, request CuaRequest, target *Target) (CuaReply, error) {
	return c.call(ctx, lifetime, request, target, false)
}

// Only the interactive host may supply this approval; it is never a model field.
func (c *CuaRuntime) CallWithBrowserApproval(ctx, lifetime context.Context, request CuaRequest, target *Target) (CuaReply, error) {
	if request.Action != "browser_prepare" || target == nil {
		return CuaReply{}, errors.New("browser approval applies only to an exact app attachment")
	}
	return c.call(ctx, lifetime, request, target, true)
}

func (c *CuaRuntime) call(ctx, lifetime context.Context, request CuaRequest, target *Target, browserApproved bool) (CuaReply, error) {
	if lifetime == nil || lifetime.Done() == nil {
		return CuaReply{}, errors.New("Cua requires a host-owned task lifetime")
	}
	if err := request.Validate(); err != nil {
		return CuaReply{}, err
	}
	if request.Action == "browser_prepare" && !browserApproved {
		return CuaReply{}, errors.New("browser attachment requires separate profile approval")
	}
	if lifetime.Err() != nil || ctx.Err() != nil {
		return CuaReply{}, context.Canceled
	}
	if c.tasks != nil {
		return c.task(lifetime).call(ctx, lifetime, request, target, browserApproved)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	desktopMu.Lock()
	defer desktopMu.Unlock()
	if ctx.Err() != nil || lifetime.Err() != nil {
		return CuaReply{}, context.Canceled
	}
	if c.lifetime != lifetime {
		c.stop()
		if err := c.start(lifetime); err != nil {
			return CuaReply{}, err
		}
	}
	if c.process == nil {
		return CuaReply{}, errors.New("Cua channel ended; start a new task, do not replay uncertain input")
	}
	p := c.process
	c.sequence++
	id := c.sequence
	body, _ := json.Marshal(struct {
		Protocol        int        `json:"protocol"`
		ID              uint64     `json:"id"`
		Request         CuaRequest `json:"request"`
		Target          *Target    `json:"approved_target"`
		BrowserApproved bool       `json:"browser_profile_approved"`
	}{cuaProtocol, id, request, target, browserApproved})
	type outcome struct {
		value CuaReply
		err   error
	}
	completed := make(chan outcome, 1)
	go func() {
		if _, err := p.input.Write(append(body, '\n')); err != nil {
			completed <- outcome{err: err}
			return
		}
		if !p.lines.Scan() {
			err := p.lines.Err()
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			completed <- outcome{err: err}
			return
		}
		var value CuaReply
		err := json.Unmarshal(p.lines.Bytes(), &value)
		if err == nil && (value.Protocol != cuaProtocol || value.ID != id || value.Driver != "cua" || value.SourceRevision != cuaSourceRevision) {
			err = errors.New("Cua response identity is invalid")
		}
		completed <- outcome{value, err}
	}()
	// A transport failure after the write cannot prove that input was unsent.
	timeout := 45 * time.Second
	if request.Action == "browser_prepare" {
		// The browser can require its own visible consent after host approval.
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case result := <-completed:
		if result.err == nil {
			return result.value, nil
		}
		c.stopAfterRead(p)
		return CuaReply{}, &CuaTransportError{Cause: fmt.Errorf("Cua channel failed; input effect unknown, inspect before retrying: %w", result.err)}
	case <-ctx.Done():
		_ = p.cmd.Process.Kill()
		<-completed
		c.stopAfterRead(p)
		return CuaReply{}, &CuaTransportError{Cause: errors.New("Cua action interrupted; input effect unknown, inspect before retrying")}
	}
}

func (c *CuaRuntime) task(lifetime context.Context) *CuaRuntime {
	c.mu.Lock()
	defer c.mu.Unlock()
	if task := c.tasks[lifetime]; task != nil {
		return task
	}
	task := &CuaRuntime{binary: c.binary, args: c.args}
	c.tasks[lifetime] = task
	go func() {
		<-lifetime.Done()
		task.mu.Lock()
		task.stop()
		task.mu.Unlock()
		c.mu.Lock()
		delete(c.tasks, lifetime)
		c.mu.Unlock()
	}()
	return task
}

func (c *CuaRuntime) stopAfterRead(p *cuaProcess) {
	_ = p.input.Close()
	_ = p.cmd.Process.Kill()
	<-p.done
	c.process = nil
}
