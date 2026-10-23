package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"reasonix/internal/shellparse"
	"reasonix/internal/tool"
)

const desktopAutomationRefusal = "Desktop automation through Bash is blocked in this interactive task. Use computer_use with app-scoped approval and verified window/focus receipts. Do not replace a refusal with osascript/JXA, System Events, CGEvent, or clipboard probes. If the native tool has no approval host, stop and report the host configuration failure."

func parseBashParams(ctx context.Context, args json.RawMessage) (bashParams, error) {
	var p bashParams
	if err := json.Unmarshal(args, &p); err != nil {
		return p, fmt.Errorf("invalid args: %w", err)
	}
	if err := validateBashParams(p); err != nil {
		return p, err
	}
	if tool.ComputerUseApproverFrom(ctx) != nil && desktopAutomationCommand(p.Command) {
		return p, tool.Blocked(desktopAutomationRefusal)
	}
	return p, nil
}

// Catch direct and inline versions of the fallback observed in desktop tasks.
// This is a tool-routing guard, not a sandbox for arbitrary programs or scripts.
func desktopAutomationCommand(command string) bool { return desktopAutomationShell(command, 0) }

func desktopAutomationShell(command string, depth int) bool {
	if depth > 4 {
		return true
	}
	file, err := shellparse.ParseBash(command)
	if err != nil {
		return false
	}
	blocked := false
	syntax.Walk(file, func(node syntax.Node) bool {
		if blocked {
			return false
		}
		if stmt, ok := node.(*syntax.Stmt); ok {
			if call, ok := stmt.Cmd.(*syntax.CallExpr); ok {
				blocked = desktopAutomationCall(call, stmt, depth)
			}
		}
		return !blocked
	})
	return blocked
}

func desktopAutomationCall(call *syntax.CallExpr, statement *syntax.Stmt, depth int) bool {
	name, rest := desktopCommandWords(call)
	switch name {
	case "osascript", "cliclick", "screencapture":
		return true
	case "sh", "bash", "zsh", "dash", "eval":
		for _, word := range rest {
			if !strings.HasPrefix(word, "-") && desktopAutomationShell(word, depth+1) {
				return true
			}
		}
	case "python", "python3", "node", "ruby", "perl", "swift":
		var text bytes.Buffer
		_ = syntax.NewPrinter().Print(&text, statement)
		source := strings.ToLower(text.String())
		for _, api := range []string{"cgevent", "axuielement", "pyautogui", "pynput", "system events", "nsworkspace", "applicationservices"} {
			if strings.Contains(source, api) {
				return true
			}
		}
	}
	return false
}

func desktopCommandWords(call *syntax.CallExpr) (string, []string) {
	var name string
	var rest []string
	for _, arg := range call.Args {
		word, ok := shellparse.StaticWord(arg)
		if !ok {
			continue
		}
		if name != "" {
			rest = append(rest, word)
			continue
		}
		base := strings.ToLower(shellparse.WordBase(word))
		if shellparse.IsAssignment(word) || strings.HasPrefix(word, "-") || base == "env" || base == "command" || base == "exec" || base == "sudo" {
			continue
		}
		name = base
	}
	return name, rest
}
