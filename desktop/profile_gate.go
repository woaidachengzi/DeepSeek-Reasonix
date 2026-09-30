package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"reasonix/internal/config"
	"reasonix/internal/profilegate"
)

func acquireDesktopProfiles() (func(), error) {
	return profilegate.TryAcquireDesktop(config.ReasonixHomeDir(), config.SessionProfileRoot())
}

// Retain Wails' own single-instance handoff: an ordinary second Wails launch
// still notifies the first window. A conflicting bridge (or dev instance)
// receives no bound methods, startup writers or shutdown snapshots. It exits
// once the WebView is ready, without presenting the hidden main window.
func refuseDesktopProfileStartup(appOptions *options.App) {
	appOptions.Bind = nil
	appOptions.OnStartup = nil
	appOptions.OnBeforeClose = nil
	appOptions.OnShutdown = nil
	appOptions.OnDomReady = func(ctx context.Context) { runtime.Quit(ctx) }
}
