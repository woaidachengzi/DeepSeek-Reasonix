//go:build !windows

package terminalprocess

import (
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

type unixProcess struct {
	cmd       *exec.Cmd
	pty       *os.File
	closeOnce sync.Once
	closeErr  error
	waitDone  chan struct{}
	exitCode  int
	waitErr   error
}

func Available() (bool, string) { return true, "" }

func start(spec Spec) (Process, error) {
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Dir, cmd.Env = spec.Dir, spec.Env
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(spec.Rows), Cols: uint16(spec.Columns)})
	if err != nil {
		return nil, err
	}
	p := &unixProcess{cmd: cmd, pty: file, waitDone: make(chan struct{}), exitCode: -1}
	// A stale create can Close before the host registers its Wait loop. Own the
	// one OS wait immediately so that case cannot leave a zombie child.
	go func() {
		p.waitErr = cmd.Wait()
		if cmd.ProcessState != nil {
			p.exitCode = cmd.ProcessState.ExitCode()
		}
		close(p.waitDone)
	}()
	return p, nil
}

func (p *unixProcess) Read(data []byte) (int, error)  { return p.pty.Read(data) }
func (p *unixProcess) Write(data []byte) (int, error) { return p.pty.Write(data) }

func (p *unixProcess) Resize(columns, rows int) error {
	if !validSize(columns, rows) {
		return ErrInvalidSpec
	}
	return pty.Setsize(p.pty, &pty.Winsize{Rows: uint16(rows), Cols: uint16(columns)})
}

func (p *unixProcess) Wait() (int, error) {
	<-p.waitDone
	return p.exitCode, p.waitErr
}

func (p *unixProcess) Close() error {
	p.closeOnce.Do(func() {
		// StartWithSize creates a new session and controlling terminal. Retain
		// Wails' group cleanup, including children holding the terminal open.
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		p.closeErr = p.pty.Close()
	})
	return p.closeErr
}
