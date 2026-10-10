package main

import (
	"context"
	"sync/atomic"
	"time"

	"reasonix/internal/desktopbridge"
)

type previewDesktopNotificationConsumer interface {
	Run(context.Context) error
	Close()
}

type previewDesktopNotificationTask struct {
	consumer previewDesktopNotificationConsumer
	done     chan struct{}
}

// One supervisor belongs to one started gateway generation. It discovers all
// published local/remote owners, not saved history, and never retries a failed
// observer under the same owner identity. An incomplete cut fails closed and
// cancels/awaits every consumer rather than presenting partial watch coverage.
type previewDesktopNotificationHost struct {
	catalogue     *previewDesktopCatalogue
	stream        *desktopbridge.OwnedEventStream
	store         *previewDesktopWatchStore
	sender        previewDesktopNotificationSender
	ctx           context.Context
	cancel        context.CancelFunc
	stopCatalogue func() bool
	started       atomic.Bool
	done          chan struct{}
	ready         chan error
	wake          chan struct{}
	refresh       chan chan error
	// These maps have exactly one owner: Run (or a synchronous test reconcile).
	tasks map[previewDesktopCatalogueKey]*previewDesktopNotificationTask
	seen  map[previewDesktopCatalogueKey]bool
}

func newPreviewDesktopNotificationHost(ctx context.Context, catalogue *previewDesktopCatalogue, stream *desktopbridge.OwnedEventStream, store *previewDesktopWatchStore, sender previewDesktopNotificationSender) (*previewDesktopNotificationHost, error) {
	if ctx == nil || ctx.Err() != nil || catalogue == nil || catalogue.ctx.Err() != nil || stream == nil || store == nil || sender == nil {
		return nil, errPreviewDesktopBinding
	}
	owner, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(catalogue.ctx, cancel)
	return &previewDesktopNotificationHost{catalogue: catalogue, stream: stream, store: store, sender: sender, ctx: owner, cancel: cancel, stopCatalogue: stop, done: make(chan struct{}), ready: make(chan error, 1), wake: make(chan struct{}, 1), refresh: make(chan chan error), tasks: make(map[previewDesktopCatalogueKey]*previewDesktopNotificationTask), seen: make(map[previewDesktopCatalogueKey]bool)}, nil
}

func (h *previewDesktopNotificationHost) Close()                { h.stopCatalogue(); h.cancel() }
func (h *previewDesktopNotificationHost) Ready() <-chan error   { return h.ready }
func (h *previewDesktopNotificationHost) Done() <-chan struct{} { return h.done }

func (h *previewDesktopNotificationHost) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errPreviewDesktopBinding
	}
	h.Close()
	if !h.started.Load() {
		return nil
	}
	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
		return errPreviewDesktopBinding
	}
}

// Explicit fresh-cut acknowledgement for host lifecycle transitions. It is
// serialized by Run, never a retry/reconnect to a failed observer generation.
func (h *previewDesktopNotificationHost) Refresh(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || !h.started.Load() {
		return errPreviewDesktopBinding
	}
	reply := make(chan error, 1)
	select {
	case h.refresh <- reply:
	case <-ctx.Done():
		return errPreviewDesktopBinding
	case <-h.ctx.Done():
		return errPreviewDesktopBinding
	}
	select {
	case err := <-reply:
		return err
	case <-ctx.Done():
		return errPreviewDesktopBinding
	case <-h.done:
		return errPreviewDesktopBinding
	}
}

func (h *previewDesktopNotificationHost) drain() {
	// Cancel all first so independent SDK calls can unwind concurrently.
	for _, task := range h.tasks {
		task.consumer.Close()
	}
	for key, task := range h.tasks {
		<-task.done
		delete(h.tasks, key)
	}
}

func (h *previewDesktopNotificationHost) reconcile() error {
	entries, err := h.catalogue.Refresh(h.ctx)
	if err != nil || h.ctx.Err() != nil {
		return errPreviewDesktopBinding
	}
	present := make(map[previewDesktopCatalogueKey]bool, len(entries))
	start := make(chan struct{})
	for _, entry := range entries {
		present[entry.key()] = true
	}
	// Await removed owners before subscribing replacements (including a
	// same-path/new-epoch Controller). No network IO holds catalogue locks.
	for key, task := range h.tasks {
		if !present[key] {
			task.consumer.Close()
		}
	}
	for key, task := range h.tasks {
		if !present[key] {
			<-task.done
			delete(h.tasks, key)
			continue
		}
		select {
		case <-task.done:
			return errPreviewDesktopBinding
		default:
		}
	}
	for _, entry := range entries {
		key := entry.key()
		if h.tasks[key] != nil {
			continue
		}
		// Retain bounded generation tombstones. Disappearing/reappearing with
		// the same identity must not silently reconnect/replay an old observer.
		if h.seen[key] || len(h.seen) >= 1024 || h.ctx.Err() != nil {
			return errPreviewDesktopBinding
		}
		h.seen[key] = true
		var consumer previewDesktopNotificationConsumer
		if entry.local != nil {
			consumer, err = newPreviewDesktopNotifications(h.stream, entry.local.Scope, h.store, h.sender)
		} else {
			consumer, err = newPreviewDesktopRemoteNotifications(h.ctx, h.catalogue, entry, h.store, h.sender)
		}
		if err != nil {
			return errPreviewDesktopBinding
		}
		if h.ctx.Err() != nil {
			consumer.Close()
			return errPreviewDesktopBinding
		}
		task := &previewDesktopNotificationTask{consumer: consumer, done: make(chan struct{})}
		h.tasks[key] = task
		go func() {
			// Do not deliver a partial startup cut while another owner is
			// still opening. Failure cancels this gate and closes the source.
			select {
			case <-start:
				_ = task.consumer.Run(h.ctx)
			case <-h.ctx.Done():
				task.consumer.Close()
			}
			close(task.done)
			select {
			case h.wake <- struct{}{}:
			default:
			}
		}()
	}
	close(start)
	return nil
}

func (h *previewDesktopNotificationHost) Run(ctx context.Context) error {
	if !h.started.CompareAndSwap(false, true) {
		return errPreviewDesktopBinding
	}
	defer close(h.done)
	defer h.drain()
	defer h.Close()
	if ctx == nil || ctx.Err() != nil {
		h.ready <- errPreviewDesktopBinding
		return errPreviewDesktopBinding
	}
	stopRun := context.AfterFunc(ctx, h.cancel)
	defer stopRun()
	err := h.reconcile()
	h.ready <- err
	if err != nil {
		return err
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var reply chan error
		select {
		case <-h.ctx.Done():
			return errPreviewDesktopBinding
		case <-h.wake:
		case <-ticker.C:
		case reply = <-h.refresh:
		}
		err := h.reconcile()
		if reply != nil {
			reply <- err
		}
		if err != nil {
			return err
		}
	}
}
