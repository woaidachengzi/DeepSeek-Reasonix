package main

import (
	"context"
	"sync/atomic"

	"reasonix/internal/bot"
	"reasonix/internal/desktopbridge"
)

type previewDesktopNotificationSender interface {
	SendDesktopNotification(context.Context, bot.DesktopWatchRoute, string, bot.DesktopNotification) (bot.SendResult, error)
}

// One consumer binds one already-published owner and one gateway generation.
// The full host must Close/await it on gateway replacement, and explicitly open
// a fresh observer on owner changes. No automatic replay/rebind/retry is allowed.
type previewDesktopNotifications struct {
	sub     *desktopbridge.OwnedEventSubscription
	store   *previewDesktopWatchStore
	sender  previewDesktopNotificationSender
	started atomic.Bool
}

func newPreviewDesktopNotifications(stream *desktopbridge.OwnedEventStream, scope desktopbridge.OwnedCommandScope, store *previewDesktopWatchStore, sender previewDesktopNotificationSender) (*previewDesktopNotifications, error) {
	if stream == nil || store == nil || sender == nil {
		return nil, desktopbridge.ErrOwnedObservationClosed
	}
	sub, err := stream.Subscribe(scope)
	if err != nil {
		return nil, err
	}
	return &previewDesktopNotifications{sub: sub, store: store, sender: sender}, nil
}

func (n *previewDesktopNotifications) Close() { n.sub.Close() }

func (n *previewDesktopNotifications) Run(ctx context.Context) error {
	if !n.started.CompareAndSwap(false, true) {
		return desktopbridge.ErrOwnedObservationClosed
	}
	if ctx == nil {
		n.Close()
		return desktopbridge.ErrOwnedObservationClosed
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer n.Close()
	// A dedicated bounded observer-lifetime worker also interrupts SDK IO when
	// the source retires/overflows while Run is not blocked in Read.
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-n.sub.Done():
			cancel()
		case <-runCtx.Done():
		}
	}()
	defer func() { cancel(); <-done }()
	for {
		frame, err := n.sub.Read(runCtx)
		if err != nil {
			return err
		}
		summary := previewDesktopEventSummary(frame.Kind)
		if summary == "" {
			continue
		}
		for _, lease := range n.store.deliveryLeases() {
			select {
			case <-n.sub.Done():
				return desktopbridge.ErrOwnedObservationClosed
			default:
			}
			if runCtx.Err() != nil {
				return runCtx.Err()
			}
			if lease.ctx.Err() != nil {
				continue
			}
			sendCtx, stop := context.WithCancel(runCtx)
			stopLease := context.AfterFunc(lease.ctx, stop)
			if lease.ctx.Err() == nil && sendCtx.Err() == nil {
				// Gateway owns the final current transport/access/admin check.
				// No raw event bytes, owner path/name/ID or prompt ID reach IM.
				_, _ = n.sender.SendDesktopNotification(sendCtx, lease.watcher.route, lease.watcher.actor, bot.DesktopNotification{Summary: summary})
			}
			stopLease()
			stop()
			// Unknown delivery is never retried or redirected to another actor.
		}
	}
}

func previewDesktopEventSummary(kind string) string {
	switch kind {
	case "turn_started":
		return "Preview 会话开始处理。"
	case "turn_done":
		return "Preview 会话本轮处理已结束；请在桌面查看结果。"
	case "ask_request", "approval_request", "mcp_interaction":
		return "Preview 会话有待处理提示；请在桌面查看并处理。"
	}
	return ""
}
