package computeruse

import (
	"context"
	"encoding/json"
	"errors"
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
	v, err := c.backend.call(ctx, Request{Action: "apps"})
	if err != nil {
		return Target{}, err
	}
	var catalog struct {
		Apps []Target `json:"apps"`
	}
	if !v.OK || json.Unmarshal(v.State, &catalog) != nil {
		return Target{}, errors.New("unable to resolve computer control app")
	}
	for _, app := range catalog.Apps {
		if app.PID == pid && app.Name != "" && app.BundleID != "" && app.LaunchTime > 0 {
			return app, nil
		}
	}
	return Target{}, errors.New("selected app is unavailable; list apps again")
}
