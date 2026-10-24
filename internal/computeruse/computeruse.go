// Package computeruse owns app-scoped desktop observation and input. Native
// operations are serialized across sessions; observation receipts belong to one
// controller and are never accepted by another conversation.
package computeruse

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

// ObservationTTL permits a model round trip; every action still captures and
// validates current native state immediately before consuming its receipt.
const ObservationTTL = 2 * time.Minute

type Request struct {
	Action          string   `json:"action"`
	PID             int      `json:"pid,omitempty"`
	WindowID        int      `json:"window_id,omitempty"`
	Observation     string   `json:"observation,omitempty"`
	Element         string   `json:"element,omitempty"`
	X               *float64 `json:"x,omitempty"`
	Y               *float64 `json:"y,omitempty"`
	CoordinateSpace string   `json:"coordinate_space,omitempty"`
	Text            string   `json:"text,omitempty"`
	Key             string   `json:"key,omitempty"`
	Modifiers       []string `json:"modifiers,omitempty"`
	Delta           int      `json:"delta,omitempty"`
	// VerifiedFocus is stamped by the host, never trusted from tool arguments.
	VerifiedFocus json.RawMessage `json:"verified_focus,omitempty"`
	// VerifiedPointer is copied from a move receipt, never from tool arguments.
	VerifiedPointer *PointerPosition `json:"verified_pointer,omitempty"`
}

type Result struct {
	State                 json.RawMessage `json:"state"`
	Observation           string          `json:"observation,omitempty"`
	ObservationTTLMS      int64           `json:"observation_ttl_ms,omitempty"`
	ObservationAgeMS      int64           `json:"observation_age_ms,omitempty"`
	NextAction            string          `json:"next_action,omitempty"`
	Dispatched            bool            `json:"dispatched"`
	DispatchMethod        string          `json:"dispatch_method,omitempty"`
	DispatchUnknown       bool            `json:"dispatch_unknown,omitempty"`
	StateChanged          bool            `json:"state_changed,omitempty"`
	BlockedReason         string          `json:"blocked_reason,omitempty"`
	InputVerification     string          `json:"input_verification,omitempty"`
	Unchanged             bool            `json:"unchanged,omitempty"`
	ImageGeometry         *ImageGeometry  `json:"image_geometry,omitempty"`
	PointerPreview        *PointerPreview `json:"pointer_preview,omitempty"`
	Cursor                *CursorSnapshot `json:"cursor,omitempty"`
	ObservationValidation string          `json:"observation_validation,omitempty"`
	Image                 string          `json:"-"`
}

type nativeResult struct {
	OK                bool             `json:"ok"`
	Error             string           `json:"error,omitempty"`
	State             json.RawMessage  `json:"state"`
	Image             string           `json:"image,omitempty"`
	WindowID          int              `json:"window_id,omitempty"`
	Dispatched        bool             `json:"dispatched"`
	DispatchMethod    string           `json:"dispatch_method,omitempty"`
	DispatchUnknown   bool             `json:"dispatch_unknown,omitempty"`
	BlockedReason     string           `json:"blocked_reason,omitempty"`
	InputVerification string           `json:"input_verification,omitempty"`
	Unchanged         bool             `json:"unchanged,omitempty"`
	Pointer           *PointerPosition `json:"pointer,omitempty"`
	Cursor            *PointerPosition `json:"cursor,omitempty"`
}

type backend interface {
	call(context.Context, Request) (nativeResult, error)
}
type receipt struct {
	token       string
	pid, window int
	digest      [32]byte
	identity    [32]byte
	expires     time.Time
	issued      time.Time
	snapshot    nativeResult
}
type Controller struct {
	backend backend
	receipt receipt
	mu      sync.Mutex
	now     func() time.Time
}

var desktopMu sync.Mutex

func New() *Controller { return &Controller{backend: nativeBackend{}, now: time.Now} }

// ValidateRequest checks arguments before an interactive host asks for access.
func ValidateRequest(r Request) error { return validate(r) }

func validate(r Request) error {
	switch r.Action {
	case "status", "apps":
		return nil
	case "observe", "activate", "move", "confirm_click", "click", "fill", "type", "key", "scroll":
	default:
		return errors.New("unknown computer_use action")
	}
	if r.PID <= 0 || r.PID > math.MaxInt32 {
		return errors.New("select a running app PID returned by apps")
	}
	if r.WindowID < 0 || uint64(r.WindowID) > math.MaxUint32 {
		return errors.New("invalid window ID")
	}
	if r.Action == "observe" {
		return nil
	}
	if r.Observation == "" {
		return errors.New("observe this app before acting; pass its observation receipt")
	}
	return validateInput(r)
}

func validateInput(r Request) error {
	if err := validatePointerAction(r); err != nil {
		return err
	}
	if err := validateCoordinateSpace(r); err != nil {
		return err
	}
	if (r.Action == "click" || r.Action == "fill") && r.Element == "" && (r.X == nil || r.Y == nil) {
		return fmt.Errorf("%s requires an observed element or screenshot x/y coordinates", r.Action)
	}
	if r.X != nil && (math.IsNaN(*r.X) || math.IsInf(*r.X, 0)) || r.Y != nil && (math.IsNaN(*r.Y) || math.IsInf(*r.Y, 0)) {
		return errors.New("coordinates must be finite")
	}
	if len(r.Text) > 16384 {
		return errors.New("text exceeds 16 KiB; split type input or shorten the complete fill value")
	}
	if (r.Action == "type" || r.Action == "fill") && r.Text == "" {
		return fmt.Errorf("%s requires nonempty text", r.Action)
	}
	if len(r.Modifiers) > 4 {
		return errors.New("too many key modifiers")
	}
	for _, m := range r.Modifiers {
		if m != "command" && m != "shift" && m != "option" && m != "control" {
			return fmt.Errorf("unsupported modifier %q", m)
		}
	}
	if r.Action == "scroll" && (r.Delta == 0 || r.Delta < -100 || r.Delta > 100) {
		return errors.New("scroll delta must be between -100 and 100 and nonzero")
	}
	if r.Action == "key" && !supportedKey(r.Key) {
		return errors.New("unsupported key")
	}

	return nil
}

func digest(v nativeResult) [32]byte {
	// Include pixels as well as accessibility state: a canvas may change without
	// producing an AX revision. A changed screen requires a fresh observation.
	h := sha256.New()
	h.Write(canonicalState(v.State))
	h.Write(framePixels(v))
	var d [32]byte
	copy(d[:], h.Sum(nil))
	return d
}

func identity(v nativeResult) [32]byte {
	var app struct {
		PID    int     `json:"pid"`
		Window int     `json:"window_id"`
		Bundle string  `json:"bundle_id"`
		Launch float64 `json:"launch_time"`
	}
	_ = json.Unmarshal(v.State, &app)
	b, _ := json.Marshal(app)
	return sha256.Sum256(b)
}

func (c *Controller) observe(ctx context.Context, pid, window int) (nativeResult, error) {
	v, err := c.backend.call(ctx, Request{Action: "observe", PID: pid, WindowID: window})
	if err == nil && !v.OK {
		err = errors.New(v.Error)
	}
	return v, err
}

// Preserve native input facts even if the subsequent observation or transport
// fails. This projection never issues a receipt; only publish may do that.
func nativeOutcome(v nativeResult) Result {
	geometry := observationImageGeometry(v)
	return Result{State: v.State, Image: v.Image, Dispatched: v.Dispatched, DispatchMethod: v.DispatchMethod, DispatchUnknown: v.DispatchUnknown, BlockedReason: v.BlockedReason, InputVerification: v.InputVerification, Unchanged: v.Unchanged, ImageGeometry: geometry, Cursor: observedCursor(v, geometry)}
}

func validatedNativeOutcome(v nativeResult, validation string) Result {
	result := nativeOutcome(v)
	result.ObservationValidation = validation
	return result
}

func (c *Controller) publish(v nativeResult, pid int) (Result, error) {
	result := nativeOutcome(v)
	if v.WindowID == 0 {
		return result, nil
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return result, err
	}
	now := c.now()
	c.receipt = receipt{token: hex.EncodeToString(token[:]), pid: pid, window: v.WindowID, digest: digest(v), identity: identity(v), expires: now.Add(ObservationTTL), issued: now, snapshot: v}
	result.Observation = c.receipt.token
	result.ObservationTTLMS = ObservationTTL.Milliseconds()
	return result, nil
}

func (c *Controller) Execute(ctx context.Context, r Request) (Result, error) {
	return c.execute(ctx, r, nil)
}

// ExecuteForTarget rechecks the approved process incarnation inside the native
// operation lock, before any screen capture or input.
func (c *Controller) ExecuteForTarget(ctx context.Context, r Request, target Target) (Result, error) {
	return c.execute(ctx, r, &target)
}

func (c *Controller) execute(ctx context.Context, r Request, target *Target) (Result, error) {
	r.VerifiedFocus = nil
	r.VerifiedPointer = nil
	if err := validate(r); err != nil {
		return Result{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	desktopMu.Lock()
	defer desktopMu.Unlock()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if target != nil {
		current, err := c.target(ctx, r.PID)
		if err != nil || current != *target {
			return Result{}, errors.New("approved app changed; list apps and request access again")
		}
	}
	if r.Action == "observe" {
		c.receipt = receipt{}
	}
	var before nativeResult
	validation := ""
	if r.Action != "status" && r.Action != "apps" && r.Action != "observe" {
		prior := c.receipt
		if r.Observation != prior.token || r.PID != prior.pid {
			return Result{BlockedReason: "invalid_observation"}, errors.New("observation belongs to another app/session or was already consumed; observe again; no input sent")
		}
		if !c.now().Before(prior.expires) {
			c.receipt = receipt{}
			return expiredObservation(prior, c.now()), errors.New("observation expired; no input was sent. Call observe, inspect the fresh result, then issue one action; this is not evidence of a broken input channel")
		}
		current, err := c.observe(ctx, r.PID, prior.window)
		if err != nil {
			return Result{}, err
		}
		changed := digest(current) != prior.digest
		if changed && caretBlinkActionAllowed(r, prior.snapshot, current) {
			changed = false
			validation = "caret_only"
		}
		// Activation sends no content input. Stage Manager can animate or hide
		// an inactive window, so only its app/window identity must stay fixed.
		if r.Action == "activate" {
			changed = identity(current) != prior.identity
		}
		// Native capture and frame comparison take time. Recheck freshness
		// after that work, immediately before accepting the verified receipt.
		if !c.now().Before(prior.expires) {
			c.receipt = receipt{}
			return expiredObservation(prior, c.now()), errors.New("observation expired during verification; no input was sent. Call observe before another action; this is not evidence of a broken input channel")
		}
		if changed {
			result, e := c.publish(current, r.PID)
			if e != nil {
				return result, e
			}
			result.StateChanged = true
			result.BlockedReason = observationChangeReason(prior.snapshot, current)
			return result, errors.New("app state changed; no input sent; inspect the new observation before retrying")
		}
		r.WindowID = prior.window
		before = current
		pinned, err := pinVerifiedInput(r, prior.snapshot, current)
		if err != nil {
			c.receipt = receipt{}
			return Result{BlockedReason: "pointer_preview_required", NextAction: "observe"}, err
		}
		r = pinned
		// Consume before dispatch. An uncertain or failed action must never be
		// automatically replayed using the same receipt.
		c.receipt = receipt{}
		resolved, err := windowPointRequest(r, before)
		if err != nil {
			result := nativeOutcome(before)
			result.BlockedReason, result.NextAction = "coordinate_mapping_unavailable", "observe"
			return result, err
		}
		r = resolved
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	v, err := c.backend.call(ctx, r)
	if err != nil {
		return verifiedInputResult(validatedNativeOutcome(v, validation), r, before, v), err
	}
	if !v.OK {
		return verifiedInputResult(validatedNativeOutcome(v, validation), r, before, v), errors.New(v.Error)
	}
	result, err := c.publishAction(v, r)
	result.ObservationValidation = validation
	markCursorClick(&result, r, before)
	return verifiedInputResult(result, r, before, v), err
}

func supportedKey(key string) bool {
	switch key {
	case "return", "tab", "escape", "backspace", "space", "left", "right", "up", "down", "a", "c", "v", "x", "z":
		return true
	default:
		return false
	}
}
