package computeruse

import (
	"context"
	"encoding/json"
	"errors"
	"math"
)

type Target struct {
	PID        int     `json:"pid"`
	Name       string  `json:"name"`
	BundleID   string  `json:"bundle_id"`
	LaunchTime float64 `json:"launch_time"`
}

// Target reads process identity without capturing a screen or querying AX.
func (c *Controller) Target(ctx context.Context, pid int) (Target, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	desktopMu.Lock()
	defer desktopMu.Unlock()
	if err := ctx.Err(); err != nil {
		return Target{}, err
	}
	return c.target(ctx, pid)
}

// target requires both native-operation locks to be held by the caller.
func (c *Controller) target(ctx context.Context, pid int) (Target, error) {
	if pid < 1 || pid > math.MaxInt32 {
		return Target{}, errors.New("select a running app PID returned by apps")
	}
	// NSWorkspace.runningApplications requires a Cocoa main run loop to update.
	// The Go bridge has no such loop; resolve the exact PID directly instead of
	// trusting its cached catalog when an app starts or restarts during a session.
	v, err := c.backend.call(ctx, Request{Action: "resolve_target", PID: pid})
	if err != nil {
		return Target{}, err
	}
	var state struct {
		Target Target `json:"target"`
	}
	if !v.OK || json.Unmarshal(v.State, &state) != nil {
		return Target{}, errors.New("unable to resolve computer control app")
	}
	app := state.Target
	if app.PID == pid && app.Name != "" && app.BundleID != "" && finite(app.LaunchTime) && app.LaunchTime > 0 {
		return app, nil
	}
	return Target{}, errors.New("selected app is unavailable; list apps again")
}
