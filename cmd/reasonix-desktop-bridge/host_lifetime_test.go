package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHostLifetimeCancelsOnReparentingAndStopsAfterNormalExit(t *testing.T) {
	for _, reparent := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "parent-gone"}[reparent], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			parents := make(chan int, 2)
			parents <- 1234
			observed := make(chan struct{}, 2)
			ticks := make(chan time.Time, 1)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				watchHostLifetime(ctx, cancel, 1234, func() int {
					value := <-parents
					observed <- struct{}{}
					return value
				}, ticks)
			}()
			select {
			case <-observed:
			case <-time.After(time.Second):
				t.Fatal("parent was not checked")
			}
			select {
			case <-ctx.Done():
				t.Fatal("live parent was cancelled")
			default:
			}
			if reparent {
				parents <- 1
				ticks <- time.Time{}
			} else {
				cancel()
			}
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("host lifetime was not cancelled")
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("host watcher did not stop")
			}
		})
	}
}

func TestRunRejectsForeignHostBeforeProfileOrReadinessWrites(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "missing-profile")
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	ready := filepath.Join(root, "ready.json")
	for _, pid := range []int{-1, 1, os.Getpid()} {
		if err := run(context.Background(), config{listen: "127.0.0.1:0", readyFile: ready, launchID: "foreign-host", hostPID: pid}, testToken); err == nil {
			t.Fatal("invalid parent identity was accepted")
		}
		for _, path := range []string{home, ready} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("foreign parent startup wrote profile/readiness")
			}
		}
	}
}
