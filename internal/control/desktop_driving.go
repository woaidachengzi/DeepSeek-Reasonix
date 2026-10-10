package control

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrDesktopDrivingScope = errors.New("desktop driving authority is no longer current")

const desktopDrivingLifetime = 15 * time.Minute
const desktopDrivingKeyLimit = 128
const desktopDrivingVersionLimit = 9_007_199_254_740_991

// ControlVersion fences local input even when a management/no-op input does
// not change the runtime revision. Zero is the first Controller generation.
type DesktopDrivingScope struct {
	TurnSubmitScope
	ControlVersion uint64
}

type desktopDrivingGrant struct {
	capture DesktopDrivingScope
	key     string
	expires time.Time
}
type controllerDesktopDriving struct {
	active  *desktopDrivingGrant
	used    map[string]struct{}
	version uint64
}

func validDesktopDrivingKey(key string) bool {
	if len(key) != 32 || key != strings.ToLower(key) {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}

// These comparisons require runtimeState.mu followed by c.mu, matching scoped
// admission. Epoch is the Controller's own ledger identity, not prompt routing.
func (c *Controller) matchesDrivingOwnerLocked(scope TurnSubmitScope) bool {
	c.turnEvents.mu.RLock()
	defer c.turnEvents.mu.RUnlock()
	ledger := c.turnEvents.ledger
	return c.turnEvents.err == nil && ledger != nil && c.runtimeState.ledger == ledger && c.runtimeState.path == c.sessionPath &&
		c.sessionPath == scope.SessionPath && c.runtimeState.snapshot.RuntimeEpoch == scope.RuntimeEpoch &&
		!c.closed && !c.rotating && !c.rejectDrainingGenerationLocked()
}

func (c *Controller) matchesScopedIdleLocked(scope TurnSubmitScope) bool {
	return c.matchesDrivingOwnerLocked(scope) && scope.Revision != 0 && c.runtimeState.snapshot.Revision == scope.Revision &&
		c.runtimeState.snapshot.Phase == "idle" && !c.runtimeState.snapshot.Running && !c.runtimeState.snapshot.PendingPrompt
}

func (c *Controller) revokeDesktopDrivingLocked() {
	c.desktopDriving.active = nil
	if c.desktopDriving.version < desktopDrivingVersionLimit {
		c.desktopDriving.version++
	}
}

func (c *Controller) revokeDesktopDriving() {
	c.mu.Lock()
	c.revokeDesktopDrivingLocked()
	c.mu.Unlock()
}

func (c *Controller) expireDesktopDrivingLocked() {
	if grant := c.desktopDriving.active; grant != nil && !time.Now().Before(grant.expires) {
		c.revokeDesktopDrivingLocked()
	}
}

// CaptureDesktopDriving samples one currently owned idle admission identity
// and the independent local-control generation. It grants no holder, starts
// no turn, reads no saved history and never renews a lease.
func (c *Controller) CaptureDesktopDriving(ctx context.Context) (DesktopDrivingScope, error) {
	if c == nil || ctx == nil || ctx.Err() != nil {
		return DesktopDrivingScope{}, ErrDesktopDrivingScope
	}
	// Use the existing Controller runtime initialization, then take one cut
	// under the same lock order used by capture/submit admission.
	_ = c.RuntimeStateSnapshot()
	c.runtimeState.mu.Lock()
	defer c.runtimeState.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expireDesktopDrivingLocked()
	scope := TurnSubmitScope{SessionPath: c.sessionPath, RuntimeEpoch: c.runtimeState.snapshot.RuntimeEpoch, Revision: c.runtimeState.snapshot.Revision}
	if ctx.Err() != nil || !c.matchesScopedIdleLocked(scope) || c.running || c.finishing || c.desktopDriving.version >= desktopDrivingVersionLimit {
		return DesktopDrivingScope{}, ErrDesktopDrivingScope
	}
	return DesktopDrivingScope{scope, c.desktopDriving.version}, nil
}

// AcquireDesktopDriving accepts one explicit caller-generated random key for
// an already-owned idle instance. Hosts must generate 128 bits and privately
// bind the key to the authenticated chat/actor; never format it into IM/events.
// Same key/capture is idempotent while active (including its own running turn),
// not a renewal. A local input, release, expiry or owner retirement cannot be
// undone by retrying the same key. Used keys are bounded tombstones for this
// Controller lifetime; exhaustion fails closed, never evicts an unknown grant.
// This API neither starts a turn nor grants permission to restore/switch owners.
func (c *Controller) AcquireDesktopDriving(ctx context.Context, scope DesktopDrivingScope, key string) error {
	if c == nil || ctx == nil || ctx.Err() != nil || !validDesktopDrivingKey(key) || scope.SessionPath == "" || scope.RuntimeEpoch == "" || scope.Revision == 0 || c.ensureWriteAuthorityReady() != nil {
		return ErrDesktopDrivingScope
	}
	c.runtimeState.mu.Lock()
	defer c.runtimeState.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil || !c.matchesDrivingOwnerLocked(scope.TurnSubmitScope) {
		return ErrDesktopDrivingScope
	}
	c.expireDesktopDrivingLocked()
	if scope.ControlVersion != c.desktopDriving.version || c.desktopDriving.version >= desktopDrivingVersionLimit {
		return ErrDesktopDrivingScope
	}
	if grant := c.desktopDriving.active; grant != nil {
		if grant.key == key && grant.capture == scope {
			return nil
		}
		return ErrDesktopDrivingScope
	}
	if _, spent := c.desktopDriving.used[key]; spent || len(c.desktopDriving.used) >= desktopDrivingKeyLimit || !c.matchesScopedIdleLocked(scope.TurnSubmitScope) || c.running || c.finishing {
		return ErrDesktopDrivingScope
	}
	if c.desktopDriving.used == nil {
		c.desktopDriving.used = make(map[string]struct{})
	}
	c.desktopDriving.used[key] = struct{}{}
	c.desktopDriving.active = &desktopDrivingGrant{capture: scope, key: key, expires: time.Now().Add(desktopDrivingLifetime)}
	return nil
}

// DesktopDrivingActive is observation only. The caller must still pass the
// same key through atomic admission and Serve's published-owner/lease gates.
// No expiry extension, transcript read, routing refresh or fallback occurs.
func (c *Controller) DesktopDrivingActive(ctx context.Context, scope DesktopDrivingScope, key string) bool {
	if c == nil || ctx == nil || ctx.Err() != nil || !validDesktopDrivingKey(key) {
		return false
	}
	c.runtimeState.mu.Lock()
	defer c.runtimeState.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expireDesktopDrivingLocked()
	grant := c.desktopDriving.active
	return ctx.Err() == nil && c.matchesDrivingOwnerLocked(scope.TurnSubmitScope) && grant != nil && grant.key == key && grant.capture == scope && grant.capture.ControlVersion == c.desktopDriving.version
}

// ReleaseDesktopDriving only revokes the matching holder. It is permitted
// after actor role loss and idempotent for a spent key, but never releases a
// different chat's replacement grant or cancels a previously accepted turn.
func (c *Controller) ReleaseDesktopDriving(ctx context.Context, scope DesktopDrivingScope, key string) error {
	if c == nil || ctx == nil || ctx.Err() != nil || !validDesktopDrivingKey(key) {
		return ErrDesktopDrivingScope
	}
	c.runtimeState.mu.Lock()
	defer c.runtimeState.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil || !c.matchesDrivingOwnerLocked(scope.TurnSubmitScope) {
		return ErrDesktopDrivingScope
	}
	if grant := c.desktopDriving.active; grant != nil && grant.key == key {
		if grant.capture != scope {
			return ErrDesktopDrivingScope
		}
		c.revokeDesktopDrivingLocked()
		return nil
	}
	if _, spent := c.desktopDriving.used[key]; spent {
		return nil
	}
	return ErrDesktopDrivingScope
}

// SubmitDesktopDriving accepts literal user text using the current idle
// revision, but only under the originally acquired holder key and owner epoch.
// Transport cancellation fences admission, not an already accepted turn.
func (c *Controller) SubmitDesktopDriving(ctx context.Context, scope TurnSubmitScope, key, input string) error {
	if c == nil || !validDesktopDrivingKey(key) || strings.TrimSpace(input) == "" || len(input) > 64<<10 || !utf8.ValidString(input) || strings.ContainsRune(input, 0) {
		return ErrDesktopDrivingScope
	}
	err := c.admitScopedTurnGuarded(ctx, scope, func(turnCtx context.Context) error {
		return c.runRefTurnWithResolverSync(turnCtx, input, input, "", "", c.ResolveScopedRefs)
	}, func() bool {
		c.expireDesktopDrivingLocked()
		grant := c.desktopDriving.active
		return grant != nil && grant.key == key && grant.capture.ControlVersion == c.desktopDriving.version && grant.capture.SessionPath == scope.SessionPath && grant.capture.RuntimeEpoch == scope.RuntimeEpoch
	})
	if err != nil {
		return ErrDesktopDrivingScope
	}
	return nil
}
