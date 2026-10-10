package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/bot"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/provider"
)

func awaitDesktopDriverIdle(t *testing.T, manager *desktopbridge.RuntimeManager) desktopbridge.OwnedCommandView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if view, ok := manager.CommandSnapshot(); ok && view.State.Phase == "idle" && !view.State.Running {
			return view
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("actual Controller did not become idle")
	return desktopbridge.OwnedCommandView{}
}

func desktopDriverCallContains(call ownedCommandCall, text string) bool {
	for _, message := range call.request.Messages {
		if message.Role == provider.RoleUser && strings.Contains(message.Content, text) {
			return true
		}
	}
	return false
}

func TestPreviewDesktopDriverActualControllerTakeoverAndLocalReclaim(t *testing.T) {
	profile := t.TempDir()
	t.Setenv("REASONIX_HOME", profile)
	t.Setenv("REASONIX_STATE_HOME", profile)
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	p, endpoint := newOwnedCommandFixtureProvider(t)
	config := fmt.Sprintf(`default_model="local/alpha"
[desktop]
provider_access=["local"]
[[providers]]
name="local"
kind="preview-owned-command-test"
base_url=%q
models=["alpha","beta"]
default="alpha"
`, endpoint)
	if err := os.WriteFile(filepath.Join(profile, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	manager := desktopbridge.NewRuntimeManager(newControllerFactory(nil))
	t.Cleanup(func() { _ = manager.Shutdown() })
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "driver-owned", WorkspaceRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	driver := newPreviewDesktopDriver(manager)
	defer driver.Close()
	ctx := context.Background()
	route := bot.DesktopWatchRoute{Platform: bot.PlatformFeishu, ConnectionID: "private-fixture", Domain: "feishu", ChatType: bot.ChatDM, ChatID: "owner-chat"}
	command := bot.DesktopCommand{Route: route, ActorID: "owner", Action: "takeover", TargetID: "driver-owned"}
	wrong := command
	wrong.TargetID = "saved-or-remote"
	if _, err := driver.ExecuteDesktopCommand(ctx, wrong); !errors.Is(err, errPreviewDesktopBinding) {
		t.Fatal(err)
	}
	if _, err := driver.ExecuteDesktopCommand(ctx, command); err != nil {
		t.Fatal(err)
	}
	if !driver.DesktopTakeoverActive(route, "owner") {
		t.Fatal("binding not active")
	}
	if _, err := driver.ExecuteDesktopCommand(ctx, command); err != nil {
		t.Fatal("same capture must be idempotent", err)
	}
	for _, mutate := range []func(*bot.DesktopWatchRoute){
		func(r *bot.DesktopWatchRoute) { r.ConnectionID += "-other" },
		func(r *bot.DesktopWatchRoute) { r.Domain += "-other" },
		func(r *bot.DesktopWatchRoute) { r.Platform = bot.PlatformQQ },
		func(r *bot.DesktopWatchRoute) { r.ChatType = bot.ChatGroup },
		func(r *bot.DesktopWatchRoute) { r.ChatID += "-other" },
	} {
		foreign := command
		mutate(&foreign.Route)
		if driver.DesktopTakeoverActive(foreign.Route, "owner") {
			t.Fatal("foreign route inherited takeover")
		}
		if _, err := driver.ExecuteDesktopCommand(ctx, foreign); !errors.Is(err, errPreviewDesktopBinding) {
			t.Fatal("silently stole owner", err)
		}
		foreign.Action = "release"
		if _, err := driver.ExecuteDesktopCommand(ctx, foreign); err != nil {
			t.Fatal(err)
		}
		if !driver.DesktopTakeoverActive(route, "owner") {
			t.Fatal("foreign release removed owner")
		}
	}
	wrong = command
	wrong.ActorID = "another-admin-in-same-chat"
	if driver.DesktopTakeoverActive(route, wrong.ActorID) {
		t.Fatal("another group member inherited actor-bound takeover")
	}
	if _, err := driver.ExecuteDesktopCommand(ctx, wrong); !errors.Is(err, errPreviewDesktopBinding) {
		t.Fatal("actor changed without release", err)
	}
	wrong.Action, wrong.AnswerText = "drive", "wrong actor"
	if _, err := driver.ExecuteDesktopCommand(ctx, wrong); !errors.Is(err, errPreviewDesktopBinding) {
		t.Fatal(err)
	}
	wrong.Action, wrong.AnswerText = "release", ""
	if _, err := driver.ExecuteDesktopCommand(ctx, wrong); !errors.Is(err, errPreviewDesktopBinding) || !driver.DesktopTakeoverActive(route, "owner") {
		t.Fatal("another actor released local holder", err)
	}
	command.Action, command.AnswerText = "drive", "/clear"
	if _, err := driver.ExecuteDesktopCommand(ctx, command); err != nil {
		t.Fatal(err)
	}
	call := awaitOwnedCommand(t, p.calls)
	if !desktopDriverCallContains(call, "/clear") {
		t.Fatal("remote input was interpreted instead of submitted as plain text")
	}
	if _, err := driver.ExecuteDesktopCommand(ctx, command); !errors.Is(err, control.ErrTurnSubmitScope) {
		t.Fatal("busy drive queued or retried", err)
	}
	close(p.finish)
	old := awaitDesktopDriverIdle(t, manager)
	if !driver.DesktopTakeoverActive(route, "owner") {
		t.Fatal("successful remote turn reclaimed itself")
	}
	if _, err := manager.Submit(ctx, "driver-owned", "local owner reclaims"); err != nil {
		t.Fatal(err)
	}
	localCall := awaitOwnedCommand(t, p.calls)
	if !desktopDriverCallContains(localCall, "local owner reclaims") {
		t.Fatal("local input missing")
	}
	current := awaitDesktopDriverIdle(t, manager)
	if current.LocalInputVersion <= old.LocalInputVersion {
		t.Fatal("local reclaim fence did not advance")
	}
	staleTakeover := bot.DesktopCommand{Route: route, ActorID: "owner", Action: "takeover", TargetID: "driver-owned"}
	if _, err := driver.executeCaptured(ctx, staleTakeover, &old); !errors.Is(err, errPreviewDesktopBinding) {
		t.Fatal("stale directory capture adopted new local control version", err)
	}
	// Even if an old remote caller refreshes only the idle revision, the local
	// reclaim version still blocks it under manager's atomic admission lock.
	old.State = current.State
	if err := manager.SubmitOwned(ctx, old, "stale post-local drive"); !errors.Is(err, desktopbridge.ErrOwnedRuntimeChanged) {
		t.Fatal(err)
	}
	if driver.DesktopTakeoverActive(route, "owner") {
		t.Fatal("local send failed to reclaim binding")
	}
	if _, err := driver.ExecuteDesktopCommand(ctx, command); !errors.Is(err, errPreviewDesktopBinding) {
		t.Fatal("drive rebound implicitly", err)
	}
	command.Action = "takeover"
	if _, err := driver.ExecuteDesktopCommand(ctx, command); err != nil {
		t.Fatal(err)
	}
	beforeModel := current.Scope
	if _, err := manager.SetSessionModel(ctx, "driver-owned", "local/beta"); err != nil {
		t.Fatal(err)
	}
	current = awaitDesktopDriverIdle(t, manager)
	if current.Scope == beforeModel || driver.DesktopTakeoverActive(route, "owner") {
		t.Fatal("same-ID model replacement adopted old binding")
	}
	command.Action = "drive"
	if _, err := driver.ExecuteDesktopCommand(ctx, command); !errors.Is(err, errPreviewDesktopBinding) {
		t.Fatal(err)
	}
	command.Action = "takeover"
	if _, err := driver.ExecuteDesktopCommand(ctx, command); err != nil {
		t.Fatal(err)
	}
	// Losing admin does not change the authenticated actor's identity. The
	// driver's release requires that original actor, not a different account.
	command.Action, command.ActorID = "release", "owner"
	if _, err := driver.ExecuteDesktopCommand(ctx, command); err != nil {
		t.Fatal("relinquish must work after admin loss", err)
	}
	if driver.DesktopTakeoverActive(route, "owner") {
		t.Fatal("release kept binding")
	}
	command.Action, command.ActorID = "takeover", "owner"
	if _, err := driver.ExecuteDesktopCommand(ctx, command); err != nil {
		t.Fatal(err)
	}
	driver.Close()
	driver.Close()
	if driver.DesktopTakeoverActive(route, "owner") {
		t.Fatal("closed driver retained binding")
	}
	if _, err := driver.ExecuteDesktopCommand(ctx, command); !errors.Is(err, errPreviewDesktopBinding) {
		t.Fatal(err)
	}
	if _, ok := manager.CommandSnapshot(); !ok {
		t.Fatal("driver cleanup shut down local Controller")
	}
	if len(p.calls) != 0 {
		t.Fatal("invalid command dispatched a model request")
	}
}
