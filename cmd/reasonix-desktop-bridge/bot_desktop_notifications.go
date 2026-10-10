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
			previewDesktopDeliverNotification(runCtx, lease, n.sender, summary)
			// Unknown delivery is never retried or redirected to another actor.
		}
	}
}

func previewDesktopDeliverNotification(ctx context.Context, lease previewDesktopWatchLease, sender previewDesktopNotificationSender, summary string) {
	if ctx.Err() != nil || lease.ctx.Err() != nil {
		return
	}
	sendCtx, stop := context.WithCancel(ctx)
	stopLease := context.AfterFunc(lease.ctx, stop)
	defer stopLease()
	defer stop()
	if lease.ctx.Err() == nil && sendCtx.Err() == nil {
		// Gateway owns the final transport/access/admin check. Only a fixed
		// summary reaches IM, with no retry after an unknown delivery.
		_, _ = sender.SendDesktopNotification(sendCtx, lease.watcher.route, lease.watcher.actor, bot.DesktopNotification{Summary: summary})
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
