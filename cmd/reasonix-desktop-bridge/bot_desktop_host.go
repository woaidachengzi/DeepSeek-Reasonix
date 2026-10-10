package main

import (
	"context"
	"sync"
	"time"

	"reasonix/internal/bot"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
)

// One scoped host belongs to one gateway generation. Its catalogue remains
// alive until original driving releases finish, even if ingress is cancelled.
// The persisted watch store and actual runtimes/tunnels belong to the sidecar.
type previewDesktopHost struct {
	lifecycle                 chan struct{}
	mu                        sync.Mutex
	ctx                       context.Context
	cancel                    context.CancelFunc
	catalogue                 *previewDesktopCatalogue
	commands                  *previewDesktopCommands
	stream                    *desktopbridge.OwnedEventStream
	store                     *previewDesktopWatchStore
	notifications             *previewDesktopNotificationHost
	started, active, draining bool
	shutdownDone              chan struct{}
	shutdownErr               error
	fenced                    map[bot.DesktopWatchRoute]string
}

var _ bot.DesktopBridge = (*previewDesktopHost)(nil)
var _ bot.DesktopScopedBridge = (*previewDesktopHost)(nil)

func newPreviewDesktopHost(ctx context.Context, manager *desktopbridge.RuntimeManager, remotes *previewRemoteSessions, stream *desktopbridge.OwnedEventStream, store *previewDesktopWatchStore) (*previewDesktopHost, error) {
	if ctx == nil || ctx.Err() != nil || stream == nil || store == nil || manager == nil && remotes == nil {
		return nil, errPreviewDesktopBinding
	}
	owner, cancel := context.WithCancel(ctx)
	catalogue := newPreviewDesktopCatalogue(manager, remotes)
	return &previewDesktopHost{lifecycle: make(chan struct{}, 1), ctx: owner, cancel: cancel, catalogue: catalogue, commands: newPreviewDesktopCommandsWithWatch(catalogue, store), stream: stream, store: store, fenced: make(map[bot.DesktopWatchRoute]string)}, nil
}

// Start binds only the original, already-started gateway sender. Context bounds
// startup waiting; the established observation belongs to the host lifetime.
func (h *previewDesktopHost) Start(ctx context.Context, sender previewDesktopNotificationSender) error {
	if ctx == nil || ctx.Err() != nil || sender == nil {
		return errPreviewDesktopBinding
	}
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	select {
	case h.lifecycle <- struct{}{}:
	case <-bounded.Done():
		return errPreviewDesktopBinding
	}
	defer func() { <-h.lifecycle }()
	h.mu.Lock()
	if h.started || h.draining || h.ctx.Err() != nil {
		h.mu.Unlock()
		return errPreviewDesktopBinding
	}
	h.started = true
	h.mu.Unlock()
	if err := h.commands.driving.local.ConfigureReclaimNotifications(h.ctx, sender); err != nil {
		h.StopIngress()
		return errPreviewDesktopBinding
	}
	n, err := newPreviewDesktopNotificationHost(h.ctx, h.catalogue, h.stream, h.store, sender)
	if err != nil {
		h.StopIngress()
		return errPreviewDesktopBinding
	}
	h.notifications = n
	go func() {
		_ = n.Run(h.ctx)
		h.StopIngress() // A failed observer cannot leave commands advertised live.
		// Cleanup authority outlives cancelled ingress; release original grants.
		_ = h.Shutdown(context.Background())
	}()
	select {
	case err = <-n.Ready():
	case <-bounded.Done():
		err = errPreviewDesktopBinding
	case <-h.ctx.Done():
		err = errPreviewDesktopBinding
	}
	if err != nil {
		n.Close()
		h.StopIngress()
		return errPreviewDesktopBinding
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.draining || h.ctx.Err() != nil {
		return errPreviewDesktopBinding
	}
	h.active = true
	return nil
}

func (h *previewDesktopHost) StopIngress() {
	h.mu.Lock()
	h.draining = true
	h.active = false
	h.mu.Unlock()
	h.cancel()
	h.commands.cancel()
	h.rememberDrivingRoutes()
}

func (h *previewDesktopHost) rememberDrivingRoutes() {
	h.commands.driving.mu.Lock()
	defer h.commands.driving.mu.Unlock()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.draining {
		for route, binding := range h.commands.driving.routes {
			h.fenced[route] = binding.actor
		}
	}
}

func (h *previewDesktopHost) Available() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.active && !h.draining && h.ctx.Err() == nil
}

func (h *previewDesktopHost) ExecuteDesktopCommand(ctx context.Context, command bot.DesktopCommand) (string, error) {
	if ctx == nil || !h.Available() {
		return "", errPreviewDesktopBinding
	}
	operation, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(h.ctx, cancel)
	defer stop()
	defer cancel()
	defer h.rememberDrivingRoutes()
	return h.commands.ExecuteDesktopCommand(operation, command)
}
func (h *previewDesktopHost) DesktopTakeoverActive(route bot.DesktopWatchRoute, actor string) bool {
	h.mu.Lock()
	fenced := actor != "" && h.fenced[route] == actor
	h.mu.Unlock()
	// A retired/unknown holder still consumes continuing input in the old
	// gateway; it cannot silently become an unrelated ordinary bot task.
	return fenced || h.Available() && h.commands.DesktopTakeoverActive(route, actor)
}

// Call after stopping gateway ingress. Cancel/await notification workers first,
// then release original driving bindings once, then retire the catalogue.
// Concurrent/repeated shutdown shares the first outcome; unknown is not retry.
func (h *previewDesktopHost) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errPreviewDesktopBinding
	}
	h.StopIngress()
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	select {
	case h.lifecycle <- struct{}{}:
	case <-bounded.Done():
		return errPreviewDesktopBinding
	}
	defer func() { <-h.lifecycle }()
	if h.shutdownDone != nil {
		return h.shutdownErr
	}
	h.shutdownDone = make(chan struct{})
	var result error
	if h.notifications != nil {
		if err := h.notifications.Shutdown(bounded); err != nil {
			result = errPreviewDesktopBinding
		}
	}
	if err := h.commands.Shutdown(bounded); err != nil {
		result = errPreviewDesktopBinding
	}
	h.catalogue.Close()
	h.shutdownErr = result
	close(h.shutdownDone)
	return result
}

// Compatibility methods deny unscoped/actor-less access explicitly. The real
// gateway must use DesktopScopedBridge, including after errors or cancellation.
func (*previewDesktopHost) Sessions() []bot.DesktopSessionInfo { return nil }
func (*previewDesktopHost) SetWatch(bot.DesktopWatchRoute, bool) error {
	return errPreviewDesktopBinding
}
func (*previewDesktopHost) Watching(bot.DesktopWatchRoute) bool             { return false }
func (*previewDesktopHost) Approve(string, bool) (string, error)            { return "", errPreviewDesktopBinding }
func (*previewDesktopHost) AskQuestions(string) ([]event.AskQuestion, bool) { return nil, false }
func (*previewDesktopHost) Answer(string, []event.AskAnswer) (string, error) {
	return "", errPreviewDesktopBinding
}
func (*previewDesktopHost) Takeover(bot.DesktopWatchRoute, string) (string, error) {
	return "", errPreviewDesktopBinding
}
func (*previewDesktopHost) Release(bot.DesktopWatchRoute) (string, error) {
	return "", errPreviewDesktopBinding
}
func (*previewDesktopHost) TakeoverTab(bot.DesktopWatchRoute) string { return "" }
func (*previewDesktopHost) DriveInput(bot.DesktopWatchRoute, string) (string, error) {
	return "", errPreviewDesktopBinding
}
