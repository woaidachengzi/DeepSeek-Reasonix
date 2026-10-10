package main

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"reasonix/internal/bot"
)

type previewDrivingRoute struct {
	actor  string
	entry  previewDesktopCatalogueEntry
	busy   bool
	cancel context.CancelFunc
	done   chan struct{}
}

// One route cannot own both local and remote driving. Reservations survive
// unknown remote outcomes and never reroute a drive/release by a fresh handle.
// IO never holds mu; release cancels/awaits the original in-flight reservation.
type previewDesktopDriving struct {
	catalogue    *previewDesktopCatalogue
	local        *previewDesktopDriver
	remote       *previewDesktopRemoteDriver
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	closed       bool
	draining     bool
	shutdownDone chan struct{}
	shutdownErr  error
	routes       map[bot.DesktopWatchRoute]*previewDrivingRoute
}

func newPreviewDesktopDriving(catalogue *previewDesktopCatalogue) *previewDesktopDriving {
	ctx, cancel := context.WithCancel(catalogue.ctx)
	return &previewDesktopDriving{catalogue: catalogue, local: newPreviewDesktopDriver(catalogue.manager), remote: newPreviewDesktopRemoteDriver(catalogue), ctx: ctx, cancel: cancel, routes: make(map[bot.DesktopWatchRoute]*previewDrivingRoute)}
}

func (d *previewDesktopDriving) present(route bot.DesktopWatchRoute, b *previewDrivingRoute) bool {
	if b.entry.local != nil {
		return d.local.DesktopTakeoverActive(route, b.actor)
	}
	return d.remote.bindingPresent(route, b.actor)
}

func (d *previewDesktopDriving) pruneLocked() {
	for route, b := range d.routes {
		if !b.busy && !d.present(route, b) {
			delete(d.routes, route)
		}
	}
}

func (d *previewDesktopDriving) DesktopTakeoverActive(route bot.DesktopWatchRoute, actor string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.draining || d.ctx.Err() != nil {
		return false
	}
	d.pruneLocked()
	b := d.routes[route]
	if b == nil || b.actor != actor {
		return false
	}
	if b.busy {
		return true
	}
	if b.entry.local != nil {
		return d.local.DesktopTakeoverActive(route, actor)
	}
	return d.remote.DesktopTakeoverActive(route, actor)
}

func (d *previewDesktopDriving) ExecuteDesktopCommand(ctx context.Context, command bot.DesktopCommand) (string, error) {
	if ctx == nil || ctx.Err() != nil || d.ctx.Err() != nil || !validPreviewWatcher(previewDesktopWatcher{command.Route, command.ActorID}) {
		return "", errPreviewDesktopBinding
	}
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	stop := context.AfterFunc(d.ctx, cancel)
	defer func() { stop(); cancel() }()
	if command.Action == "release" {
		if command.TargetID != "" || command.AnswerText != "" {
			return "", errPreviewDesktopBinding
		}
		return d.release(bounded, command, false)
	}
	var entry previewDesktopCatalogueEntry
	if command.Action == "takeover" {
		if command.AnswerText != "" {
			return "", errPreviewDesktopBinding
		}
		var err error
		entry, err = d.catalogue.Capture(bounded, command.TargetID)
		if err != nil {
			return "", errPreviewDesktopBinding
		}
	} else if command.Action != "drive" || command.TargetID != "" || strings.TrimSpace(command.AnswerText) == "" || len(command.AnswerText) > 64<<10 || !utf8.ValidString(command.AnswerText) || strings.ContainsRune(command.AnswerText, 0) {
		return "", errPreviewDesktopBinding
	}
	d.mu.Lock()
	d.pruneLocked()
	if d.closed || d.draining || bounded.Err() != nil {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	b := d.routes[command.Route]
	if command.Action == "takeover" {
		if b != nil || len(d.routes) >= previewDesktopCatalogueLimit {
			d.mu.Unlock()
			return "", errPreviewDesktopBinding
		}
		b = &previewDrivingRoute{actor: command.ActorID, entry: entry}
		d.routes[command.Route] = b
	} else if b == nil || b.actor != command.ActorID || b.busy {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	operation, cancelOp := context.WithCancel(bounded)
	b.busy = true
	b.cancel = cancelOp
	b.done = make(chan struct{})
	done := b.done
	d.mu.Unlock()
	defer cancelOp()
	defer func() {
		d.mu.Lock()
		b.busy = false
		b.cancel = nil
		close(done)
		if d.routes[command.Route] == b && !d.present(command.Route, b) {
			delete(d.routes, command.Route)
		}
		d.mu.Unlock()
	}()
	var result string
	var err error
	if b.entry.local != nil {
		if command.Action == "takeover" {
			command.TargetID = b.entry.local.Scope.SessionID
		}
		result, err = d.local.executeCaptured(operation, command, b.entry.local)
	} else {
		result, err = d.remote.ExecuteDesktopCommand(operation, command)
	}
	if err != nil || operation.Err() != nil || d.ctx.Err() != nil {
		return "", errPreviewDesktopBinding
	}
	return result, nil
}

func (d *previewDesktopDriving) release(ctx context.Context, command bot.DesktopCommand, shutdown bool) (string, error) {
	d.mu.Lock()
	d.pruneLocked()
	if d.closed || !shutdown && d.draining || ctx.Err() != nil {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	b := d.routes[command.Route]
	if b == nil {
		d.mu.Unlock()
		return "本聊天没有可用的 Preview 接管绑定。", nil
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
	select {
	case <-done:
	case <-ctx.Done():
		return "", errPreviewDesktopBinding
	}
	d.mu.Lock()
	if d.closed || ctx.Err() != nil || b.busy {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	// A cancelled initial capture may have removed its reservation. Still use
	// that original backend to revoke; never release a newly-created route.
	if current := d.routes[command.Route]; current != nil && current != b {
		d.mu.Unlock()
		return "", errPreviewDesktopBinding
	}
	operation, cancel := context.WithCancel(ctx)
	b.busy = true
	b.cancel = cancel
	b.done = make(chan struct{})
	done = b.done
	if d.routes[command.Route] == nil {
		d.routes[command.Route] = b
	}
	d.mu.Unlock()
	defer cancel()
	defer func() {
		d.mu.Lock()
		b.busy = false
		b.cancel = nil
		close(done)
		if d.routes[command.Route] == b && !d.present(command.Route, b) {
			delete(d.routes, command.Route)
		}
		d.mu.Unlock()
	}()
	var result string
	var err error
	if b.entry.local != nil {
		result, err = d.local.ExecuteDesktopCommand(operation, command)
	} else {
		result, err = d.remote.ExecuteDesktopCommand(operation, command)
	}
	if err != nil || operation.Err() != nil || d.ctx.Err() != nil {
		return "", errPreviewDesktopBinding
	}
	return result, nil
}

func (d *previewDesktopDriving) Close() {
	d.cancel()
	d.mu.Lock()
	d.closed = true
	for _, b := range d.routes {
		if b.cancel != nil {
			b.cancel()
		}
	}
	clear(d.routes)
	d.mu.Unlock()
	d.local.Close()
	d.remote.Close()
}
