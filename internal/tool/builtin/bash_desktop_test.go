package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/tool"
)

func TestDesktopFallbackRouting(t *testing.T) {
	for _, command := range []string{
		`/usr/bin/osascript -l JavaScript -e 'ObjC.import("Cocoa")'`,
		`env -i command osascript -e 'tell application "System Events" to keystroke "test"'`,
		`bash -c "osascript -e 'test'"`,
		`printf ok && /usr/bin/osascript script.scpt`,
		`echo "$(osascript -e 'test')"`,
		`python3 -c 'import Quartz; Quartz.CGEventPost(0, event)'`,
		"python3 <<'PY'\nimport Quartz\nQuartz.CGEventPost(0, event)\nPY",
		`swift -e 'import ApplicationServices'`,
		`cliclick t:test`,
		`screencapture /tmp/fallback.png`,
	} {
		if !desktopAutomationCommand(command) {
			t.Errorf("desktop fallback was not recognized: %s", command)
		}
	}
	for _, command := range []string{`rg 'CGEvent|osascript' src`, `cat script.scpt`, `printf 'osascript CGEvent'`, `go test ./...`, `node --check app.js`, `python3 -c 'print(1+1)'`} {
		if desktopAutomationCommand(command) {
			t.Errorf("development command was mistaken for desktop input: %s", command)
		}
	}
}

func TestDesktopFallbackRefusesBeforeAnyShellLaunch(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "launched")
	// A harmless stand-in proves a refusal happens before process startup.
	program := filepath.Join(root, "osascript")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nprintf launched > '"+marker+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"command": "'" + program + "'", "run_in_background": true})
	ctx := tool.WithComputerUseApprover(t.Context(), desktopConsentFunc(func(context.Context, tool.ComputerUseTarget) (bool, error) {
		t.Fatal("Bash must not manufacture an app consent request")
		return false, nil
	}))
	result, err := (bash{workDir: root}).ExecuteDetailed(ctx, args)
	if _, blocked := tool.BlockedMessage(err); !blocked || result.Execution.State != tool.ShellStateNotRun || result.Execution.MutationRisk != tool.ShellMutationNotStarted {
		t.Fatalf("fallback did not fail before launch: %+v %v", result.Execution, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("desktop fallback launched a process")
	}
}
