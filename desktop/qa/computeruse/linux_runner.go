//go:build linux

// Local acceptance host: the real builtin route, real process identity and
// private worker. It accepts only one explicitly selected disposable fixture.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reasonix/internal/computeruse"
	"reasonix/internal/sandbox"
	"reasonix/internal/tool"
	"reasonix/internal/tool/builtin"
)

type fixtureApproval struct {
	target  tool.ComputerUseTarget
	browser bool
}

func (a fixtureApproval) ApproveComputerUse(_ context.Context, target tool.ComputerUseTarget) (bool, error) {
	return target == a.target, nil
}
func (a fixtureApproval) ApproveBrowserProfile(_ context.Context, target tool.ComputerUseTarget, window int) (bool, error) {
	return a.browser && target == a.target && window > 0, nil
}

func main() {
	pid := flag.Int("pid", 0, "PID of this run's disposable fixture")
	browser := flag.Bool("browser-profile-approved", false, "explicitly approve attaching this disposable browser profile")
	flag.Parse()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	target, err := computeruse.New().Target(ctx, *pid)
	if err != nil || !computeruse.Available() {
		fmt.Fprintln(os.Stderr, "fixture identity/runtime unavailable", err)
		os.Exit(1)
	}
	ctx = tool.WithComputerUseLifetime(ctx, ctx)
	ctx = tool.WithComputerUseApprover(ctx, fixtureApproval{target: tool.ComputerUseTarget(target), browser: *browser})
	control := builtin.ConfineComputerUse(sandbox.Spec{})
	input := bufio.NewScanner(os.Stdin)
	input.Buffer(make([]byte, 4096), 128<<10)
	output := json.NewEncoder(os.Stdout)
	for input.Scan() {
		var request computeruse.CuaRequest
		if json.Unmarshal(input.Bytes(), &request) != nil || (request.Action != "status" && request.PID != *pid) || request.Action == "apps" {
			_ = output.Encode(map[string]any{"error": "fixture scope refused"})
			continue
		}
		body, images, err := control.(interface {
			ExecuteWithImages(context.Context, json.RawMessage) (string, []string, error)
		}).ExecuteWithImages(ctx, input.Bytes())
		result := map[string]any{"body": json.RawMessage(body), "image_count": len(images)}
		if body == "" {
			result["body"] = nil
		}
		if err != nil {
			result["error"] = err.Error()
		}
		_ = output.Encode(result)
	}
	if err := input.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
