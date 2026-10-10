package main

import (
	"context"

	"reasonix/internal/bot"
	"reasonix/internal/desktopbridge"
)

const previewLocalReclaimSummary = "桌面本地输入已收回本聊天原操作者的接管；后续输入不会继续驱动该原绑定。"

type previewLocalReclaimWorker struct {
	route  bot.DesktopWatchRoute
	actor  string
	cancel context.CancelFunc
	done   chan struct{}
}

func (d *previewDesktopDriver) ConfigureReclaimNotifications(ctx context.Context, sender previewDesktopNotificationSender) error {
	if ctx == nil || ctx.Err() != nil || sender == nil {
		return errPreviewDesktopBinding
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.reclaimSender != nil || len(d.bindings) != 0 {
		return errPreviewDesktopBinding
	}
	d.reclaimCtx, d.reclaimSender = ctx, sender
	d.reclaimWorkers = make(map[*previewLocalReclaimWorker]struct{})
	return nil
}

// Caller holds driver.mu, while registration validates actual manager identity
// and the input fence under manager.mu. No polling and no source/body replay.
func (d *previewDesktopDriver) observeReclaimLocked(view desktopbridge.OwnedCommandView, route bot.DesktopWatchRoute, actor string) error {
	if d.reclaimSender == nil {
		return nil
	} // Standalone component, no configured gateway.
	ctx, cancel := context.WithCancel(d.reclaimCtx)
	observation, err := d.manager.ObserveLocalReclaim(ctx, view)
	if err != nil {
		cancel()
		return errPreviewDesktopBinding
	}
	worker := &previewLocalReclaimWorker{route: route, actor: actor, cancel: cancel, done: make(chan struct{})}
	d.reclaimWorkers[worker] = struct{}{}
	sender := d.reclaimSender
	go func() {
		defer func() {
			cancel()
			observation.Close()
			d.mu.Lock()
			delete(d.reclaimWorkers, worker)
			close(worker.done)
			d.mu.Unlock()
		}()
		select {
		case <-ctx.Done():
			return
		case <-observation.Done():
		}
		if ctx.Err() != nil || !observation.Reclaimed() {
			return
		}
		// Source lifetime remains observable while SDK IO is in flight.
		fenceDone := make(chan struct{})
		go func() {
			defer close(fenceDone)
			select {
			case <-observation.Retired():
				cancel()
			case <-ctx.Done():
			}
		}()
		defer func() { cancel(); <-fenceDone }()
		select {
		case <-observation.Retired():
			return
		default:
		}
		if ctx.Err() == nil {
			// Direct control notice to the original holder, independent of the
			// optional global watch subscription. Gateway rechecks admin/access
			// and original started transport; unknown delivery is not retried.
			_, _ = sender.SendDesktopNotification(ctx, route, actor, bot.DesktopNotification{Summary: previewLocalReclaimSummary})
		}
	}()
	return nil
}

func (d *previewDesktopDriver) cancelReclaimWorkersLocked(route bot.DesktopWatchRoute, actor string) {
	for worker := range d.reclaimWorkers {
		if worker.route == route && (actor == "" || worker.actor == actor) {
			worker.cancel()
		}
	}
}

func (d *previewDesktopDriver) AwaitReclaimNotifications(ctx context.Context) error {
	if ctx == nil {
		return errPreviewDesktopBinding
	}
	d.mu.Lock()
	var waits []<-chan struct{}
	for worker := range d.reclaimWorkers {
		waits = append(waits, worker.done)
	}
	d.mu.Unlock()
	for _, done := range waits {
		select {
		case <-done:
		case <-ctx.Done():
			return errPreviewDesktopBinding
		}
	}
	return nil
}
