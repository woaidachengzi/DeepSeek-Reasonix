package main

import (
	"context"
	"errors"
	"os"
	"runtime"
	"time"
)

// --host-pid is supplied only by the macOS native launcher. Compare the
// kernel-reported parent, not PID existence: after parent loss/reparenting,
// PID reuse cannot make an unrelated process own this sidecar. No process is
// signalled here; cancellation shuts down only this bridge's own runtime.
func hostLifetimeContext(ctx context.Context, expected int) (context.Context, context.CancelFunc, error) {
	if expected == 0 {
		return ctx, func() {}, nil // Existing standalone bridge clients.
	}
	if runtime.GOOS != "darwin" || expected <= 1 || os.Getppid() != expected {
		return nil, nil, errors.New("--host-pid must identify the actual macOS parent process")
	}
	owned, cancel := context.WithCancel(ctx)
	ticker := time.NewTicker(100 * time.Millisecond)
	go func() {
		defer ticker.Stop()
		watchHostLifetime(owned, cancel, expected, os.Getppid, ticker.C)
	}()
	return owned, cancel, nil
}

func watchHostLifetime(ctx context.Context, cancel context.CancelFunc, expected int, parent func() int, ticks <-chan time.Time) {
	for {
		if parent() != expected {
			cancel()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}
