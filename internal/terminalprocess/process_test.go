package terminalprocess

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSpecAdmissionDoesNotLaunchInvalidRequests(t *testing.T) {
	root := t.TempDir()
	valid := Spec{Path: filepath.Join(root, "owned-shell"), Dir: root, Columns: 80, Rows: 24}
	if err := validateSpec(valid); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Spec){
		"relative program": func(s *Spec) { s.Path = "sh" },
		"relative cwd":     func(s *Spec) { s.Dir = "." },
		"nul program":      func(s *Spec) { s.Path += "\x00" },
		"nul cwd":          func(s *Spec) { s.Dir += "\x00" },
		"nul argument":     func(s *Spec) { s.Args = []string{"a\x00b"} },
		"nul env":          func(s *Spec) { s.Env = []string{"SAFE=a\x00b"} },
		"missing env key":  func(s *Spec) { s.Env = []string{"=value"} },
		"missing env =":    func(s *Spec) { s.Env = []string{"SAFE"} },
		"zero columns":     func(s *Spec) { s.Columns = 0 },
		"negative rows":    func(s *Spec) { s.Rows = -1 },
		"column budget":    func(s *Spec) { s.Columns = MaxColumns + 1 },
		"row budget":       func(s *Spec) { s.Rows = MaxRows + 1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := valid
			mutate(&spec)
			proc, err := Start(spec)
			if proc != nil || !errors.Is(err, ErrInvalidSpec) {
				t.Fatal("invalid spec reached platform launch")
			}
		})
	}
}

func TestSpecEnvironmentPreservesWindowsDriveEntriesOnlyOnWindows(t *testing.T) {
	root := t.TempDir()
	spec := Spec{Path: filepath.Join(root, "owned-shell"), Dir: root, Columns: 80, Rows: 24,
		Env: []string{"=C:=C:\\owned", "=ExitCode=00000000", "SAFE=value"}}
	if err := validateSpec(spec); (err == nil) != (runtime.GOOS == "windows") {
		t.Fatal("drive environment admission does not match platform")
	}
}
