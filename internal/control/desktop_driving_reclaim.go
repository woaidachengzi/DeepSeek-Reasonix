package control

import (
	"context"
	"time"
)

// A one-shot observation of an already acquired exact grant. This creates no
// grant, renews no expiry and never exposes input, key or scope in a frame.
// Retired remains separate from Done so owner/lease retirement can cancel an
// in-flight notification after a local reclaim has already been observed.
type DesktopDrivingReclaimObservation struct {
	owner                *Controller
	scope                DesktopDrivingScope
	key                  string
	done, retired        chan struct{}
	reclaimed, signalled bool // Controller.mu
	stop                 func() bool
	timer                *time.Timer
}

func (c *Controller) ObserveDesktopDrivingReclaim(ctx context.Context, scope DesktopDrivingScope, key string) (*DesktopDrivingReclaimObservation, error) {
	if c == nil || ctx == nil || ctx.Err() != nil || !validDesktopDrivingKey(key) {
		return nil, ErrDesktopDrivingScope
	}
	c.runtimeState.mu.Lock()
	defer c.runtimeState.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expireDesktopDrivingLocked()
	grant := c.desktopDriving.active
	if ctx.Err() != nil || !c.matchesDrivingOwnerLocked(scope.TurnSubmitScope) || grant == nil || grant.key != key || grant.capture != scope || scope.ControlVersion != c.desktopDriving.version || len(c.desktopDriving.reclaims) >= 8 {
		return nil, ErrDesktopDrivingScope
	}
	o := &DesktopDrivingReclaimObservation{owner: c, scope: scope, key: key, done: make(chan struct{}), retired: make(chan struct{})}
	if c.desktopDriving.reclaims == nil {
		c.desktopDriving.reclaims = make(map[*DesktopDrivingReclaimObservation]struct{})
	}
	c.desktopDriving.reclaims[o] = struct{}{}
	o.stop = context.AfterFunc(ctx, o.Close)
	// This is the original grant expiry, not an observation renewal. Expiry
	// wakes a blocked stream even if no later command queries driving state.
	o.timer = time.AfterFunc(time.Until(grant.expires), o.Close)
	return o, nil
}

func (o *DesktopDrivingReclaimObservation) Done() <-chan struct{}    { return o.done }
func (o *DesktopDrivingReclaimObservation) Retired() <-chan struct{} { return o.retired }
func (o *DesktopDrivingReclaimObservation) Reclaimed() bool {
	o.owner.mu.Lock()
	defer o.owner.mu.Unlock()
	return o.reclaimed
}
func (o *DesktopDrivingReclaimObservation) Close() {
	o.owner.mu.Lock()
	defer o.owner.mu.Unlock()
	o.owner.endDesktopDrivingReclaimLocked(o)
}
func (c *Controller) endDesktopDrivingReclaimLocked(o *DesktopDrivingReclaimObservation) {
	if _, ok := c.desktopDriving.reclaims[o]; !ok {
		return
	}
	delete(c.desktopDriving.reclaims, o)
	if o.stop != nil {
		o.stop()
	}
	if o.timer != nil {
		o.timer.Stop()
	}
	if !o.signalled {
		o.signalled = true
		close(o.done)
	}
	close(o.retired)
}
func (c *Controller) retireDesktopDrivingReclaimsLocked(key string) {
	for o := range c.desktopDriving.reclaims {
		if key == "" || o.key == key {
			c.endDesktopDrivingReclaimLocked(o)
		}
	}
}
func (c *Controller) signalDesktopDrivingReclaimLocked(grant *desktopDrivingGrant) {
	for o := range c.desktopDriving.reclaims {
		if !o.signalled && o.key == grant.key && o.scope == grant.capture {
			o.reclaimed, o.signalled = true, true
			close(o.done)
		}
	}
}
