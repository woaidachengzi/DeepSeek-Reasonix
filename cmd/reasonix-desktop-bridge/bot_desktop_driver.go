package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"reasonix/internal/bot"
	"reasonix/internal/desktopbridge"
)

var errPreviewDesktopBinding = errors.New("Preview desktop driving binding is unavailable")

type previewDesktopBinding struct {
	route        bot.DesktopWatchRoute
	actor        string
	scope        desktopbridge.OwnedCommandScope
	localVersion uint64
}

// previewDesktopDriver owns only the manager's current local Controller.
// It is a component of the future full host, not an all-session DesktopBridge.
// It never opens/restores sessions or adopts remote/Wails owners. Holding mu
// across bounded manager admission serializes release/rebind with dispatch;
// no external network send or manager callback is permitted inside this lock.
type previewDesktopDriver struct {
	manager  *desktopbridge.RuntimeManager
	mu       sync.Mutex
	bindings map[string]previewDesktopBinding
	closed   bool
}

func newPreviewDesktopDriver(manager *desktopbridge.RuntimeManager) *previewDesktopDriver {
	return &previewDesktopDriver{manager: manager, bindings: make(map[string]previewDesktopBinding)}
}

func (d *previewDesktopDriver) pruneLocked() (desktopbridge.OwnedCommandView, bool) {
	if d.closed || d.manager == nil {
		clear(d.bindings)
		return desktopbridge.OwnedCommandView{}, false
	}
	view, ok := d.manager.CommandSnapshot()
	for key, binding := range d.bindings {
		if !ok || binding.scope != view.Scope || binding.localVersion != view.LocalInputVersion {
			delete(d.bindings, key)
		}
	}
	return view, ok
}

func (d *previewDesktopDriver) DesktopTakeoverActive(route bot.DesktopWatchRoute, actor string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pruneLocked()
	binding, ok := d.bindings[route.Key()]
	return ok && binding.actor == actor
}

func (d *previewDesktopDriver) ExecuteDesktopCommand(ctx context.Context, command bot.DesktopCommand) (string, error) {
	return d.executeCaptured(ctx, command, nil)
}

func (d *previewDesktopDriver) executeCaptured(ctx context.Context, command bot.DesktopCommand, expected *desktopbridge.OwnedCommandView) (string, error) {
	if ctx == nil || ctx.Err() != nil || strings.TrimSpace(command.ActorID) == "" || !validPreviewDesktopRoute(command.Route) {
		return "", errPreviewDesktopBinding
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	view, current := d.pruneLocked()
	if d.closed || ctx.Err() != nil {
		return "", errPreviewDesktopBinding
	}
	key := command.Route.Key()
	switch command.Action {
	case "takeover":
		if !current || view.Scope.SessionID != command.TargetID || expected != nil && (expected.Scope != view.Scope || expected.LocalInputVersion != view.LocalInputVersion) {
			return "", errPreviewDesktopBinding
		}
		if previous, exists := d.bindings[key]; exists {
			if previous.actor != command.ActorID || previous.scope != view.Scope {
				return "", errPreviewDesktopBinding
			}
			return "本聊天已接管当前 Preview 本地会话。", nil
		}
		// Another chat cannot silently steal the live owner, including an
		// administrator in the same group or another connection/domain.
		if len(d.bindings) != 0 {
			return "", errPreviewDesktopBinding
		}
		d.bindings[key] = previewDesktopBinding{command.Route, command.ActorID, view.Scope, view.LocalInputVersion}
		return "已接管当前 Preview 本地会话；桌面本地发送会收回控制。", nil
	case "release":
		// Release is route-scoped revocation, including after actor admin loss.
		// It cannot dispatch a turn or alter any other route's binding.
		if binding, exists := d.bindings[key]; exists && binding.actor != command.ActorID {
			return "", errPreviewDesktopBinding
		}
		delete(d.bindings, key)
		return "本聊天的 Preview 本地接管已解除。", nil
	case "drive":
		binding, exists := d.bindings[key]
		if !current || !exists || binding.actor != command.ActorID || binding.scope != view.Scope || binding.localVersion != view.LocalInputVersion || len(command.AnswerText) > 64<<10 || strings.TrimSpace(command.AnswerText) == "" {
			return "", errPreviewDesktopBinding
		}
		// Do not hold an idle revision across turns, but do preserve it from
		// this capture through atomic admission. Local reclaim is independent:
		// SubmitOwned checks the capture's local version again under manager.mu.
		if err := d.manager.SubmitOwned(ctx, view, command.AnswerText); err != nil {
			return "", err
		}
		return "已接纳输入；这不表示任务或保存已完成。", nil
	case "status":
		if !current {
			return "Preview 当前没有本地 live 会话。", nil
		}
		// Do not expose workspace paths, titles, model details or pending text
		// to shared chats. Full local+remote catalogue is a separate host duty.
		return fmt.Sprintf("Preview 当前本地 live 会话：%s [%s]", view.Scope.SessionID, view.State.Phase), nil
	}
	return "", errPreviewDesktopBinding
}

func validPreviewDesktopRoute(route bot.DesktopWatchRoute) bool {
	if strings.TrimSpace(route.ConnectionID) == "" || strings.TrimSpace(route.ChatID) == "" || len(route.ConnectionID) > 512 || len(route.Domain) > 512 || len(route.ChatID) > 512 {
		return false
	}
	switch route.Platform {
	case bot.PlatformQQ, bot.PlatformFeishu, bot.PlatformWeixin, bot.PlatformDingtalk:
	default:
		return false
	}
	switch route.ChatType {
	case bot.ChatDM, bot.ChatDirect, bot.ChatGroup, bot.ChatGuild, bot.ChatThread:
		return true
	}
	return false
}

func (d *previewDesktopDriver) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	clear(d.bindings)
}
