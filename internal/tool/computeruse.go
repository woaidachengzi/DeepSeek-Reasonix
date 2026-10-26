package tool

import "context"

// ComputerUseTarget is resolved from the native app catalog, never model labels.
type ComputerUseTarget struct {
	PID        int     `json:"pid"`
	Name       string  `json:"name"`
	BundleID   string  `json:"bundle_id"`
	LaunchTime float64 `json:"launch_time"`
}

// ComputerUseApprover requires app-scoped human consent before screenshots or
// input. The interactive host owns the task lifetime and grants; nil fails closed.
type ComputerUseApprover interface {
	ApproveComputerUse(context.Context, ComputerUseTarget) (bool, error)
}

// Browser profile attachment is a separate residual boundary. Ordinary app
// consent does not authorize enabling debugging or attaching to logged-in tabs.
type BrowserProfileApprover interface {
	ApproveBrowserProfile(context.Context, ComputerUseTarget, int) (bool, error)
}

// Foreground access is a single action, never covered by ordinary app/profile
// access. Hosts must obtain a fresh human decision; unattended hosts refuse.
type ComputerUseForegroundApprover interface {
	ApproveComputerUseForeground(context.Context, ComputerUseTarget, int, string) (bool, error)
}

type ComputerUsePreview struct {
	Target   ComputerUseTarget
	WindowID int
	Action   string
	Image    string
	Cursor   *ComputerUseCursor
}

type ComputerUseCursor struct {
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	ScreenX     float64 `json:"screenX"`
	ScreenY     float64 `json:"screenY"`
	Clicked     bool    `json:"clicked"`
	ImageWidth  int     `json:"imageWidth"`
	ImageHeight int     `json:"imageHeight"`
}

type computerUseApproverKey struct{}
type computerUsePreviewKey struct{}
type computerUseLifetimeKey struct{}

// The host run owns this context. Per-call timeouts must not extend it or let
// a replacement task reuse the driver's element tokens and process.
func WithComputerUseLifetime(ctx, lifetime context.Context) context.Context {
	return context.WithValue(ctx, computerUseLifetimeKey{}, lifetime)
}

func ComputerUseLifetimeFrom(ctx context.Context) context.Context {
	lifetime, _ := ctx.Value(computerUseLifetimeKey{}).(context.Context)
	return lifetime
}

func WithComputerUseApprover(ctx context.Context, approver ComputerUseApprover) context.Context {
	return context.WithValue(ctx, computerUseApproverKey{}, approver)
}

func ComputerUseApproverFrom(ctx context.Context) ComputerUseApprover {
	approver, _ := ctx.Value(computerUseApproverKey{}).(ComputerUseApprover)
	return approver
}

func WithComputerUsePreview(ctx context.Context, publish func(ComputerUsePreview)) context.Context {
	return context.WithValue(ctx, computerUsePreviewKey{}, publish)
}

func PublishComputerUsePreview(ctx context.Context, preview ComputerUsePreview) {
	if ctx.Err() != nil {
		return
	}
	if publish, ok := ctx.Value(computerUsePreviewKey{}).(func(ComputerUsePreview)); ok && publish != nil {
		publish(preview)
	}
}
