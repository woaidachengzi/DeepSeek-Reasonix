//go:build !windows

package desktopterminal

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"reasonix/internal/terminalprocess"
)

func TestManagerRealPTYInputOutputAndShutdown(t *testing.T) {
	m := fixtureManager(t)
	privateHome := t.TempDir()
	m.start = func(spec terminalprocess.Spec) (terminalprocess.Process, error) {
		// Real manager/process path, but no login rc or inherited user env.
		spec.Args = []string{"-i"}
		spec.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + privateHome, "ENV=", "PS1=", "TERM=xterm-256color"}
		return terminalprocess.Start(spec)
	}
	view, err := m.Create(context.Background(), ".", "sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Resize(view.ID, 100, 30); err != nil {
		t.Fatal(err)
	}
	if err := m.Write(view.ID, []byte("stty -echo\nprintf '%s\\n' 'manager-''pty-ready'\nstty size\n")); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		output, err := m.Output(view.ID)
		data, decodeErr := base64.StdEncoding.DecodeString(output.Data)
		return err == nil && decodeErr == nil && strings.Contains(string(data), "manager-pty-ready") && strings.Contains(string(data), "30 100")
	}, "real terminal manager did not receive executed output/geometry")
	if err := m.Close(); err != nil {
		t.Fatal("real terminal manager shutdown failed")
	}
	if m.events.LatestSequence() == 0 {
		t.Fatal("real terminal emitted no session-scoped frames")
	}
}
