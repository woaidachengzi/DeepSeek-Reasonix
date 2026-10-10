package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"reasonix/internal/bot"
	"reasonix/internal/remote/controller"
)

type previewRemoteDrivingBinding struct {
	entry                                     previewDesktopCatalogueEntry
	route                                     bot.DesktopWatchRoute
	actor, key                                string
	scope                                     controller.SessionDrivingScope
	started                                   time.Time
	ready, uncertain, busy, released, expired bool
	cancel                                    context.CancelFunc
	done                                      chan struct{}
}

// A private remote driving component, not a complete DesktopBridge. Gateway
// generation/admin admission and realtime watch/reclaim remain full-host duties.
// Each reservation binds the original tunnel/epoch, route and actor. Unknown
// mutations fence continuing input; no implicit reacquire or local fallback.
type previewDesktopRemoteDriver struct {
	catalogue *previewDesktopCatalogue
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closed    bool
	bindings  map[bot.DesktopWatchRoute]*previewRemoteDrivingBinding
}

func newPreviewDesktopRemoteDriver(catalogue *previewDesktopCatalogue) *previewDesktopRemoteDriver {
	ctx, cancel := context.WithCancel(catalogue.ctx)
	return &previewDesktopRemoteDriver{catalogue: catalogue, ctx: ctx, cancel: cancel, bindings: make(map[bot.DesktopWatchRoute]*previewRemoteDrivingBinding)}
}

func (d *previewDesktopRemoteDriver) current(b *previewRemoteDrivingBinding) bool {
	return !d.closed && d.ctx.Err() == nil && d.catalogue.remotes != nil && b.entry.remote != nil &&
		d.catalogue.remotes.getController(b.entry.remote.view.ID) == b.entry.remote && !b.entry.remote.client.Closed()
}

func (d *previewDesktopRemoteDriver) pruneLocked() {
	for route, binding := range d.bindings {
		if !d.current(binding) {
			if binding.cancel != nil {
				binding.cancel()
			}
			delete(d.bindings, route)
		} else if !binding.expired && time.Since(binding.started) >= 15*time.Minute {
			// Keep the original key for explicit release: the host's earlier
			// deadline is not evidence that a late remote acquire has expired.
			binding.expired = true
			if binding.cancel != nil {
				binding.cancel()
			}
		}
	}
}

// This synchronous routing probe means "original binding consumes this input",
// not a cached remote permission grant. It does no IO. Drive always verifies
// the original grant on Serve and atomic core admission. Uncertain bindings
// consume/reject input instead of silently routing it to a separate bot task.
func (d *previewDesktopRemoteDriver) DesktopTakeoverActive(route bot.DesktopWatchRoute, actor string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pruneLocked()
	b := d.bindings[route]
	return b != nil && !b.expired && b.actor == actor
}

func (d *previewDesktopRemoteDriver) bindingPresent(route bot.DesktopWatchRoute, actor string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pruneLocked()
	b := d.bindings[route]
	return b != nil && b.actor == actor
}

func (d *previewDesktopRemoteDriver) operation(ctx context.Context) (context.Context, func()) {
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	stop := context.AfterFunc(d.ctx, cancel)
	return bounded, func() { stop(); cancel() }
}

func (d *previewDesktopRemoteDriver) ExecuteDesktopCommand(ctx context.Context, command bot.DesktopCommand) (string, error) {
	if ctx == nil || ctx.Err() != nil || d.ctx.Err() != nil || !validPreviewWatcher(previewDesktopWatcher{command.Route, command.ActorID}) {
		return "", errPreviewDesktopBinding
	}
	bounded, finish := d.operation(ctx)
	defer finish()
	switch command.Action {
	case "takeover":
		if command.AnswerText != "" {
			return "", errPreviewDesktopBinding
		}
		return d.takeover(bounded, command)
	case "drive":
		if command.TargetID != "" || strings.TrimSpace(command.AnswerText) == "" || len(command.AnswerText) > 64<<10 || !utf8.ValidString(command.AnswerText) || strings.ContainsRune(command.AnswerText, 0) {
			return "", errPreviewDesktopBinding
		}
		return d.drive(bounded, command)
	case "release":
		if command.TargetID != "" || command.AnswerText != "" {
			return "", errPreviewDesktopBinding
		}
		return d.release(bounded, command)
	}
	return "", errPreviewDesktopBinding
}

func (d *previewDesktopRemoteDriver) takeover(ctx context.Context, command bot.DesktopCommand) (string, error) {
	entry, err := d.catalogue.Capture(ctx, command.TargetID)
	if err != nil || entry.local != nil || entry.remote == nil {
		return "", errPreviewDesktopBinding
	}
	d.mu.Lock()
	d.pruneLocked()
	if !d.current(&previewRemoteDrivingBinding{entry: entry}) || ctx.Err() != nil {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	if previous := d.bindings[command.Route]; previous != nil {
		// Explicit reacquire is not a retry or a way to replace an uncertain key.
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	for _, b := range d.bindings {
		if b.entry.key() == entry.key() {
			d.mu.Unlock()
			return "", errPreviewDesktopBinding
		}
	}
	if len(d.bindings) >= previewDesktopCatalogueLimit {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	operation, cancel := context.WithCancel(ctx)
	b := &previewRemoteDrivingBinding{entry: entry, route: command.Route, actor: command.ActorID, key: hex.EncodeToString(entropy[:]), started: time.Now(), busy: true, cancel: cancel, done: make(chan struct{})}
	d.bindings[command.Route] = b
	d.mu.Unlock()
	defer cancel()
	defer func() { d.mu.Lock(); b.busy = false; b.cancel = nil; close(b.done); d.mu.Unlock() }()
	request := controller.SessionDrivingRequest{ProtocolVersion: 1, Action: "capture", Scope: controller.SessionDrivingScope{SessionPendingScope: controller.SessionPendingScope{SessionPath: entry.remotePath, RuntimeEpoch: entry.remoteEpoch}}}
	capture, err := entry.remote.client.SessionDriving(operation, request)
	d.mu.Lock()
	if err != nil || operation.Err() != nil || d.bindings[command.Route] != b || !d.current(b) || !d.catalogue.pendingEntryCurrent(entry) {
		if d.bindings[command.Route] == b {
			delete(d.bindings, command.Route)
		}
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	b.scope = capture.Scope
	// Reserve the mutation before IO. Even cancellation/invalid receipt may
	// follow an accepted acquire; retain the original key for explicit release.
	b.uncertain = true
	d.mu.Unlock()
	request.Action = "acquire"
	request.Scope = capture.Scope
	request.Key = b.key
	_, err = entry.remote.client.SessionDriving(operation, request)
	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil || operation.Err() != nil || d.bindings[command.Route] != b || !d.current(b) {
		if errors.Is(err, controller.ErrDrivingChanged) && d.bindings[command.Route] == b {
			delete(d.bindings, command.Route)
		}
		return "", errPreviewDesktopBinding
	}
	b.ready = true
	b.uncertain = false
	return "已接管该远程会话；远程桌面本地输入会收回控制。", nil
}

func (d *previewDesktopRemoteDriver) drive(ctx context.Context, command bot.DesktopCommand) (string, error) {
	d.mu.Lock()
	d.pruneLocked()
	b := d.bindings[command.Route]
	if b == nil || b.actor != command.ActorID || b.busy || !b.ready || b.uncertain || b.expired || ctx.Err() != nil {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	operation, cancel := context.WithCancel(ctx)
	b.busy = true
	b.cancel = cancel
	b.done = make(chan struct{})
	d.mu.Unlock()
	defer cancel()
	defer func() { d.mu.Lock(); b.busy = false; b.cancel = nil; close(b.done); d.mu.Unlock() }()
	request := controller.SessionDrivingRequest{ProtocolVersion: 1, Action: "state", Scope: b.scope, Key: b.key}
	active, err := b.entry.remote.client.SessionDriving(operation, request)
	if err != nil || !active.Active {
		d.mu.Lock()
		if err == nil || errors.Is(err, controller.ErrDrivingChanged) {
			if d.bindings[command.Route] == b {
				delete(d.bindings, command.Route)
			}
		}
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	// Read a fresh idle revision from the same epoch, never replace the grant.
	request.Action = "capture"
	request.Scope.Revision = 0
	request.Scope.ControlVersion = 0
	request.Key = ""
	capture, err := b.entry.remote.client.SessionDriving(operation, request)
	if err != nil || capture.Scope.ControlVersion != b.scope.ControlVersion {
		return "", errPreviewDesktopBinding
	}
	d.mu.Lock()
	if operation.Err() != nil || d.bindings[command.Route] != b || !d.current(b) {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	b.uncertain = true
	d.mu.Unlock()
	request.Action = "input"
	request.Scope = b.scope
	request.Key = b.key
	request.InputRevision = capture.Scope.Revision
	request.Text = command.AnswerText
	_, err = b.entry.remote.client.SessionDriving(operation, request)
	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil || operation.Err() != nil || d.bindings[command.Route] != b || !d.current(b) {
		return "", errPreviewDesktopBinding
	}
	b.uncertain = false
	return "已接纳远程输入；这不表示任务或保存已完成。", nil
}

func (d *previewDesktopRemoteDriver) release(ctx context.Context, command bot.DesktopCommand) (string, error) {
	d.mu.Lock()
	d.pruneLocked()
	if ctx.Err() != nil || d.closed {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	b := d.bindings[command.Route]
	if b == nil {
		d.mu.Unlock()
		return "本聊天的远程接管已解除。", nil
	}
	if b.actor != command.ActorID {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	if b.cancel != nil {
		b.cancel()
	}
	done := b.done
	d.mu.Unlock()
	// Wait for the reserved operation to settle before release; never race a
	// late acquire against revocation or hold the service mutex over HTTP.
	select {
	case <-done:
	case <-ctx.Done():
		return "", errPreviewDesktopBinding
	}
	d.mu.Lock()
	if ctx.Err() != nil || d.closed {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	if d.bindings[command.Route] != b {
		confirmed := b.released || b.scope.Revision == 0
		d.mu.Unlock()
		if confirmed {
			return "本聊天的远程接管已解除。", nil
		}
		return "", errPreviewDesktopBinding
	}
	if !d.current(b) || b.busy {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	if b.scope.Revision == 0 {
		delete(d.bindings, command.Route)
		d.mu.Unlock()
		return "本聊天的远程接管已解除。", nil
	}
	operation, cancel := context.WithCancel(ctx)
	b.busy = true
	b.cancel = cancel
	b.done = make(chan struct{})
	d.mu.Unlock()
	defer cancel()
	defer func() { d.mu.Lock(); b.busy = false; b.cancel = nil; close(b.done); d.mu.Unlock() }()
	_, err := b.entry.remote.client.SessionDriving(operation, controller.SessionDrivingRequest{ProtocolVersion: 1, Action: "release", Scope: b.scope, Key: b.key})
	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil || operation.Err() != nil || !d.current(b) {
		b.uncertain = true
		return "", errPreviewDesktopBinding
	}
	if d.bindings[command.Route] == b {
		b.released = true
		delete(d.bindings, command.Route)
	}
	return "本聊天的远程接管已解除；已接纳任务仍需显式取消。", nil
}

func (d *previewDesktopRemoteDriver) Close() {
	d.cancel()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	for _, b := range d.bindings {
		if b.cancel != nil {
			b.cancel()
		}
	}
	clear(d.bindings)
	// Close fences this host and cancels IO, not a proof of remote revocation.
	// Full host must explicitly release known grants before owner retirement;
	// unknown remote grants retain the core's hard, nonrenewable 15-minute TTL.
}
