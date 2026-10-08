// Package terminalprocess owns real interactive PTY/ConPTY processes shared by
// the desktop hosts. It is not an IPC command runner: callers must resolve the
// approved shell, workspace, and sanitized environment in their backend owner.
package terminalprocess

import (
	"errors"
	"io"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	MaxColumns = 1000
	MaxRows    = 500
)

var ErrInvalidSpec = errors.New("invalid terminal process specification")

// Spec is backend-owned, never decoded from a renderer request. An empty Env
// means an empty child environment, not implicit process environment inheritance.
type Spec struct {
	Path    string
	Args    []string
	Dir     string
	Env     []string
	Columns int
	Rows    int
}

// Process retains byte-oriented terminal I/O (including escape sequences).
// Close terminates the owned process group/pseudo-console and is idempotent.
// Unix children are reaped even if admission is cancelled before callers Wait.
type Process interface {
	io.ReadWriteCloser
	Resize(columns, rows int) error
	Wait() (exitCode int, err error)
}

func validSize(columns, rows int) bool {
	return columns > 0 && columns <= MaxColumns && rows > 0 && rows <= MaxRows
}

func validateSpec(spec Spec) error {
	if !filepath.IsAbs(spec.Path) || !filepath.IsAbs(spec.Dir) ||
		strings.ContainsRune(spec.Path, 0) || strings.ContainsRune(spec.Dir, 0) || !validSize(spec.Columns, spec.Rows) {
		return ErrInvalidSpec
	}
	for _, arg := range spec.Args {
		if strings.ContainsRune(arg, 0) {
			return ErrInvalidSpec
		}
	}
	for _, entry := range spec.Env {
		// Windows keeps hidden environment keys such as =C: and =ExitCode.
		// Preserve one leading '=' as os/exec does. The spec is backend-owned;
		// this is not permission for renderer-supplied child environments.
		value := entry
		if runtime.GOOS == "windows" && strings.HasPrefix(value, "=") {
			value = value[1:]
		}
		key, _, ok := strings.Cut(value, "=")
		if !ok || key == "" || strings.ContainsRune(entry, 0) {
			return ErrInvalidSpec
		}
	}
	return nil
}

func Start(spec Spec) (Process, error) {
	if err := validateSpec(spec); err != nil {
		return nil, err
	}
	spec.Args = append([]string{}, spec.Args...)
	spec.Env = append([]string{}, spec.Env...)
	return start(spec)
}
