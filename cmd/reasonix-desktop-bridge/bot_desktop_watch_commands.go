package main

import (
	"context"

	"reasonix/internal/bot"
)

// The full host owns the persisted store across gateway generations. Command
// Close must not delete subscriptions or close that shared store. Consumers
// independently validate current gateway/actor permissions before every send.
func newPreviewDesktopCommandsWithWatch(catalogue *previewDesktopCatalogue, watch *previewDesktopWatchStore) *previewDesktopCommands {
	h := newPreviewDesktopCommands(catalogue)
	h.watch = watch
	return h
}

func (h *previewDesktopCommands) watchCommand(ctx context.Context, command bot.DesktopCommand) (string, error) {
	if h.watch == nil || command.AnswerText != "" || ctx.Err() != nil || h.ctx.Err() != nil {
		return "", errPreviewDesktopBinding
	}
	switch command.TargetID {
	case "status":
		if h.watch.Watching(command.Route) {
			return "本聊天已开启桌面事件订阅；实际送达仍需当前网关权限与事件监听。", nil
		}
		return "本聊天未开启桌面事件订阅。", nil
	case "on", "off":
		if err := h.watch.SetWatch(ctx, command.Route, command.ActorID, command.TargetID == "on"); err != nil {
			return "", errPreviewDesktopBinding
		}
		if command.TargetID == "on" {
			return "已开启本聊天的桌面事件订阅；实际送达仍需当前网关权限与事件监听。", nil
		}
		return "已关闭本聊天的桌面事件订阅。", nil
	}
	return "", errPreviewDesktopBinding
}
