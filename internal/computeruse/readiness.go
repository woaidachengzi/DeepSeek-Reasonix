package computeruse

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ReadReadiness never requests OS permissions or captures desktop content.
// Keep Cua outside the legacy controller's native-operation lock.
func ReadReadiness(ctx context.Context) (json.RawMessage, error) {
	if runtime.GOOS != "linux" {
		result, err := New().Execute(ctx, Request{Action: "status"})
		return result.State, err
	}
	cua := BundledCuaRuntime()
	if cua == nil {
		return nil, errors.New("Linux Cua runtime is not bundled")
	}
	lifetime, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	reply, err := cua.Call(lifetime, lifetime, CuaRequest{Action: "status"}, nil)
	if err == nil && reply.IsError {
		err = errors.New("unable to read Linux Cua readiness")
	}
	if err == nil && strings.Contains(strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")), "gnome") {
		command := exec.CommandContext(lifetime, "/usr/bin/gnome-shell", "--version")
		command.Env = cuaProcessEnvironment("linux")
		if version, versionErr := command.Output(); versionErr == nil {
			if major := gnomeShellMajor(string(version)); major > 0 {
				var state map[string]any
				if json.Unmarshal(reply.Result, &state) == nil && state != nil {
					state["gnome_shell_major"] = major
					reply.Result, err = json.Marshal(state)
				}
			}
		}
	}
	return reply.Result, err
}

func gnomeShellMajor(version string) int {
	fields := strings.Fields(version)
	if len(fields) != 3 || fields[0] != "GNOME" || fields[1] != "Shell" {
		return 0
	}
	major, _ := strconv.Atoi(strings.Split(fields[2], ".")[0])
	return major
}
