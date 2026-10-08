//go:build windows

package terminalprocess

import (
	"context"
	"sync"

	"github.com/UserExistsError/conpty"
	"golang.org/x/sys/windows"
)

type windowsProcess struct {
	pty       *conpty.ConPty
	closeOnce sync.Once
	closeErr  error
	waitOnce  sync.Once
	exitCode  int
	waitErr   error
}

func Available() (bool, string) {
	if !conpty.IsConPtyAvailable() {
		return false, "integrated terminal requires Windows 10 version 1809 or newer"
	}
	return true, ""
}

func start(spec Spec) (Process, error) {
	if !conpty.IsConPtyAvailable() {
		return nil, conpty.ErrConPtyUnsupported
	}
	commandLine := windows.ComposeCommandLine(append([]string{spec.Path}, spec.Args...))
	p, err := conpty.Start(commandLine, conpty.ConPtyDimensions(spec.Columns, spec.Rows),
		conpty.ConPtyWorkDir(spec.Dir), conpty.ConPtyEnv(spec.Env))
	if err != nil {
		return nil, err
	}
	return &windowsProcess{pty: p}, nil
}

func (p *windowsProcess) Read(data []byte) (int, error)  { return p.pty.Read(data) }
func (p *windowsProcess) Write(data []byte) (int, error) { return p.pty.Write(data) }

func (p *windowsProcess) Resize(columns, rows int) error {
	if !validSize(columns, rows) {
		return ErrInvalidSpec
	}
	return p.pty.Resize(columns, rows)
}

func (p *windowsProcess) Wait() (int, error) {
	p.waitOnce.Do(func() {
		code, err := p.pty.Wait(context.Background())
		p.exitCode, p.waitErr = int(code), err
		if err != nil {
			p.exitCode = -1
		}
	})
	return p.exitCode, p.waitErr
}

func (p *windowsProcess) Close() error {
	p.closeOnce.Do(func() {
		// Close the owned pseudo-console and all its handles, never a host
		// terminal window or another desktop process.
		p.closeErr = p.pty.Close()
	})
	return p.closeErr
}
