package main

import (
	"context"
	"sync/atomic"

	"reasonix/internal/remote/controller"
)

// Each consumer belongs to an original published tunnel, Controller epoch and
// gateway generation. The host owns Run/await, and opens new owners explicitly;
// this consumer never refreshes the catalogue or reconnects after retirement.
type previewDesktopRemoteNotifications struct {
	remotes       *previewRemoteSessions
	entry         previewDesktopCatalogueEntry
	stream        *controller.SessionObservationStream
	store         *previewDesktopWatchStore
	sender        previewDesktopNotificationSender
	ctx           context.Context
	cancel        context.CancelFunc
	stopCatalogue func() bool
	started       atomic.Bool
}

func newPreviewDesktopRemoteNotifications(ctx context.Context, catalogue *previewDesktopCatalogue, entry previewDesktopCatalogueEntry, store *previewDesktopWatchStore, sender previewDesktopNotificationSender) (*previewDesktopRemoteNotifications, error) {
	if ctx == nil || ctx.Err() != nil || catalogue == nil || catalogue.remotes == nil || entry.local != nil || entry.remote == nil || store == nil || sender == nil {
		return nil, errPreviewDesktopBinding
	}
	if catalogue.ctx.Err() != nil || catalogue.remotes.getController(entry.remote.view.ID) != entry.remote {
		return nil, errPreviewDesktopBinding
	}
	owner, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(catalogue.ctx, cancel)
	stream, err := entry.remote.client.ObserveSession(owner, controller.SessionObservationRequest{ProtocolVersion: 1, Scope: controller.SessionPendingScope{SessionPath: entry.remotePath, RuntimeEpoch: entry.remoteEpoch}})
	if err != nil || owner.Err() != nil || catalogue.remotes.getController(entry.remote.view.ID) != entry.remote {
		stop()
		cancel()
		if stream != nil {
			stream.Close()
		}
		return nil, errPreviewDesktopBinding
	}
	return &previewDesktopRemoteNotifications{remotes: catalogue.remotes, entry: entry, stream: stream, store: store, sender: sender, ctx: owner, cancel: cancel, stopCatalogue: stop}, nil
}

func (n *previewDesktopRemoteNotifications) Close() {
	n.stopCatalogue()
	n.cancel()
	n.stream.Close()
}

func (n *previewDesktopRemoteNotifications) current() bool {
	return n.ctx.Err() == nil && !n.entry.remote.client.Closed() && n.remotes.getController(n.entry.remote.view.ID) == n.entry.remote
}

func (n *previewDesktopRemoteNotifications) Run(ctx context.Context) error {
	if !n.started.CompareAndSwap(false, true) {
		return errPreviewDesktopBinding
	}
	defer n.Close()
	if ctx == nil || ctx.Err() != nil || !n.current() {
		return errPreviewDesktopBinding
	}
	stopRun := context.AfterFunc(ctx, n.cancel)
	stopConnection := context.AfterFunc(n.entry.remote.owner, n.cancel)
	defer stopRun()
	defer stopConnection()
	// Keep reading while SDK delivery is in flight, so remote EOF/retirement
	// cancels that delivery rather than being discovered only after it finishes.
	// At most 32 queued kinds and one reader-held frame. Overflow rejects the
	// queued prefix and cancels delivery; no replay or automatic retry follows.
	frames := make(chan string, 32)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer n.cancel()
		for {
			frame, err := n.stream.Next()
			if err != nil || !n.current() {
				return
			}
			select {
			case frames <- frame.Kind:
			case <-n.ctx.Done():
				return
			default:
				return
			}
		}
	}()
	defer func() { n.Close(); <-readerDone }()
	for {
		select {
		case <-n.ctx.Done():
			return errPreviewDesktopBinding
		case kind := <-frames:
			if !n.current() {
				return errPreviewDesktopBinding
			}
			summary := previewDesktopEventSummary(kind)
			if summary == "" {
				return errPreviewDesktopBinding
			}
			for _, lease := range n.store.deliveryLeases() {
				if !n.current() {
					return errPreviewDesktopBinding
				}
				previewDesktopDeliverNotification(n.ctx, lease, n.sender, summary)
			}
		}
	}
}
