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
