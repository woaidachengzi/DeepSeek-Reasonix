package main

import (
	"context"
	"reasonix/internal/bot"
	"reasonix/internal/remote/controller"
)

const previewRemoteReclaimSummary = "远程桌面本地输入已收回本聊天原操作者的接管；原绑定不再接受驾驶输入，请显式解除后检查当前会话。"

type previewRemoteReclaimWorker struct {
	binding *previewRemoteDrivingBinding
	cancel  context.CancelFunc
	done    chan struct{}
}

func (d *previewDesktopRemoteDriver) ConfigureReclaimNotifications(ctx context.Context, sender previewDesktopNotificationSender) error {
	if ctx == nil || ctx.Err() != nil || sender == nil {
		return errPreviewDesktopBinding
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.reclaimSender != nil || len(d.bindings) != 0 {
		return errPreviewDesktopBinding
	}
	d.reclaimCtx, d.reclaimSender = ctx, sender
	d.reclaimWorkers = make(map[*previewRemoteReclaimWorker]struct{})
	return nil
}

func (d *previewDesktopRemoteDriver) startReclaimObservation(operation context.Context, b *previewRemoteDrivingBinding) error {
	d.mu.Lock()
	if d.reclaimSender == nil {
		d.mu.Unlock()
		return nil
	} // Standalone, no gateway.
	if operation.Err() != nil || d.bindings[b.route] != b || !d.current(b) {
		d.mu.Unlock()
		return errPreviewDesktopBinding
	}
	ctx, cancel := context.WithCancel(d.reclaimCtx)
	sender := d.reclaimSender
	d.mu.Unlock()
	stopDriver := context.AfterFunc(d.ctx, cancel)
	stopOpening := context.AfterFunc(operation, cancel)
	stream, err := b.entry.remote.client.ObserveDrivingReclaim(ctx, controller.DrivingReclaimObservationRequest{ProtocolVersion: 1, Scope: b.scope, Key: b.key})
	stopOpening() // Established source belongs to host, not short command.
	if err != nil || ctx.Err() != nil || operation.Err() != nil {
		stopDriver()
		cancel()
		if stream != nil {
			stream.Close()
		}
		return errPreviewDesktopBinding
	}
	d.mu.Lock()
	if d.bindings[b.route] != b || !d.current(b) || operation.Err() != nil {
		d.mu.Unlock()
		stopDriver()
		cancel()
		stream.Close()
		return errPreviewDesktopBinding
	}
	worker := &previewRemoteReclaimWorker{binding: b, cancel: cancel, done: make(chan struct{})}
	d.reclaimWorkers[worker] = struct{}{}
	d.mu.Unlock()
	go func() {
		defer func() {
			stopDriver()
			cancel()
			stream.Close()
			d.mu.Lock()
			delete(d.reclaimWorkers, worker)
			close(worker.done)
			d.mu.Unlock()
		}()
		frame, err := stream.Next()
		d.markReclaimEnded(b)
		if err != nil || frame.Kind != "reclaimed" || ctx.Err() != nil {
			return
		}
		// Read concurrently with SDK IO so EOF/retirement or a malformed extra
		// frame cancels delivery. Never retry, release or cancel an Agent task.
		readerDone := make(chan struct{})
		go func() { defer close(readerDone); _, _ = stream.Next(); cancel() }()
		defer func() { cancel(); stream.Close(); <-readerDone }()
		d.mu.Lock()
		current := d.bindings[b.route] == b && d.current(b)
		d.mu.Unlock()
		if current && ctx.Err() == nil {
			_, _ = sender.SendDesktopNotification(ctx, b.route, b.actor, bot.DesktopNotification{Summary: previewRemoteReclaimSummary})
		}
	}()
	return nil
}

func (d *previewDesktopRemoteDriver) markReclaimEnded(b *previewRemoteDrivingBinding) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.bindings[b.route] == b {
		// Retain original cleanup key. Old input is consumed/rejected instead
		// of becoming an unrelated ordinary bot task.
		b.reclaimEnded = true
		b.ready = false
		b.uncertain = true
	}
}
func (d *previewDesktopRemoteDriver) cancelReclaimWorkersLocked(b *previewRemoteDrivingBinding) {
	for worker := range d.reclaimWorkers {
		if worker.binding == b {
			worker.cancel()
		}
	}
}
func (d *previewDesktopRemoteDriver) AwaitReclaimNotifications(ctx context.Context) error {
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
