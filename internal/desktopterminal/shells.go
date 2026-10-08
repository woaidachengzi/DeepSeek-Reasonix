package desktopterminal

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sandbox"
)

type shellCommand struct {
	path  string
	args  []string
	label string
}

func shellOptions() []desktopbridge.TerminalShellView {
	options := []desktopbridge.TerminalShellView{{ID: "default", Label: "Default shell"}}
	ids := []string{"zsh", "bash", "fish", "sh"}
	if runtime.GOOS == "windows" {
		ids = []string{"powershell", "windows-powershell", "cmd", "bash"}
	}
	for _, id := range ids {
		if command, err := namedShell(id); err == nil {
			options = append(options, desktopbridge.TerminalShellView{ID: id, Label: command.label})
		}
	}
	return options
}

// Matches the existing Wails default-shell preference and discovery contract.
// The explicit path comes only from the backend's read-only user settings.
func resolveShell(id string) (shellCommand, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" || id == "auto" {
		id = "default"
	}
	if id != "default" {
		return namedShell(id)
	}
	if cfg, err := config.LoadUserConfigReadOnly(); err == nil {
		if command, ok := shellFromConfig(cfg.Tools.Shell.Prefer, cfg.Tools.Shell.Path); ok {
			return command, nil
		}
	}
	if runtime.GOOS != "windows" {
		if path := strings.TrimSpace(os.Getenv("SHELL")); path != "" {
			if actual, err := exec.LookPath(path); err == nil && filepath.IsAbs(actual) {
				return commandForPath(actual, filepath.Base(actual)), nil
			}
		}
	}
	for _, option := range shellOptions()[1:] {
		if command, err := namedShell(option.ID); err == nil {
			return command, nil
		}
	}
	return shellCommand{}, desktopbridge.ErrTerminalUnavailable
}

func shellFromConfig(prefer, path string) (shellCommand, bool) {
	prefer = strings.ToLower(strings.TrimSpace(prefer))
	if prefer != "bash" && prefer != "powershell" && prefer != "pwsh" {
		return shellCommand{}, false
	}
	path = sandbox.ConfiguredShellPathForPreference(prefer, path)
	if path != "" {
		if actual, err := exec.LookPath(path); err == nil && filepath.IsAbs(actual) {
			return commandForPath(actual, strings.TrimSuffix(filepath.Base(actual), filepath.Ext(actual))), true
		}
	}
	shell := sandbox.ResolveShell(prefer, "", nil)
	actual, err := exec.LookPath(shell.Path)
	if err != nil || !filepath.IsAbs(actual) {
		return shellCommand{}, false
	}
	return commandForPath(actual, strings.TrimSuffix(filepath.Base(actual), filepath.Ext(actual))), true
}

func namedShell(id string) (shellCommand, error) {
	var binary, label string
	switch id {
	case "bash", "zsh", "fish", "sh":
		binary, label = id, id
	case "powershell", "pwsh":
		binary, label = "pwsh", "PowerShell"
		if runtime.GOOS == "windows" {
			binary = "pwsh.exe"
		}
	case "windows-powershell":
		binary, label = "powershell.exe", "Windows PowerShell"
	case "cmd":
		binary, label = "cmd.exe", "Command Prompt"
	default:
		return shellCommand{}, desktopbridge.ErrTerminalInput
	}
	path, err := exec.LookPath(binary)
	if err != nil || !filepath.IsAbs(path) {
		return shellCommand{}, desktopbridge.ErrTerminalUnavailable
	}
	return commandForPath(path, label), nil
}

func commandForPath(path, label string) shellCommand {
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	args := []string{}
	switch base {
	case "bash", "zsh", "sh", "ksh", "fish":
		args = []string{"-l"}
	case "pwsh", "powershell":
		args = []string{"-NoLogo"}
	case "cmd":
		args = []string{"/Q"}
	}
	return shellCommand{path, args, label}
}

func terminalEnvironment(base []string) []string {
	env := make([]string, 0, len(base)+2)
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if ok && (strings.EqualFold(key, "TERM") || strings.EqualFold(key, "COLORTERM")) {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "TERM=xterm-256color", "COLORTERM=truecolor")
}
