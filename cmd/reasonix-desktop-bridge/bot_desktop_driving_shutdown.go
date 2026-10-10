package main

import (
	"context"
	"time"

	"reasonix/internal/bot"
)

// Shutdown is the orderly host-lifecycle gate. Stop ingress before calling it.
// It fences new reservations, cancels/awaits original operations, releases
// original grants once, then closes this component. Unknown remote outcomes
// remain errors; repeated calls share the first outcome, never resend releases.
// Close alone is emergency cancellation, not proof of confirmed revocation.
func (d *previewDesktopDriving) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errPreviewDesktopBinding
	}
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	d.mu.Lock()
	if prior := d.shutdownDone; prior != nil {
		d.mu.Unlock()
		select {
		case <-prior:
		case <-bounded.Done():
			return errPreviewDesktopBinding
		}
		d.mu.Lock()
		err := d.shutdownErr
		d.mu.Unlock()
		return err
	}
	if d.closed || bounded.Err() != nil || d.ctx.Err() != nil {
		d.mu.Unlock()
		return errPreviewDesktopBinding
	}
	d.draining = true
	d.shutdownDone = make(chan struct{})
	var commands []bot.DesktopCommand
	var operations []<-chan struct{}
	for route, binding := range d.routes {
		commands = append(commands, bot.DesktopCommand{Route: route, ActorID: binding.actor, Action: "release"})
		operations = append(operations, binding.done)
		if binding.cancel != nil {
			binding.cancel()
		}
	}
	d.mu.Unlock()
	var result error
	for _, command := range commands {
		if _, err := d.release(bounded, command, true); err != nil {
			result = errPreviewDesktopBinding
		}
	}
	for _, done := range operations {
		select {
		case <-done:
		case <-bounded.Done():
			result = errPreviewDesktopBinding
		}
	}
	if bounded.Err() != nil || d.ctx.Err() != nil {
		result = errPreviewDesktopBinding
	}
	d.Close()
	d.mu.Lock()
	d.shutdownErr = result
	close(d.shutdownDone)
	d.mu.Unlock()
	return result
}
