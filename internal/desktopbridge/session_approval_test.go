package desktopbridge

import (
	"context"
	"errors"
	"testing"
)

type approvalRuntime struct {
	fakeRuntime
	mode    string
	refusal error
}

func (r *approvalRuntime) ApprovalMode() string { return r.mode }
func (r *approvalRuntime) SetApprovalMode(mode string) error {
	if r.refusal != nil {
		return r.refusal
	}
	r.mode = mode
	return nil
}
func TestSessionApprovalOwnershipStateAndFailure(t *testing.T) {
	r := &approvalRuntime{fakeRuntime: fakeRuntime{path: "/sessions/a.jsonl", state: "idle"}, mode: "ask"}
	m := NewRuntimeManager(RuntimeFactoryFunc(func(context.Context, OpenRequest) (Runtime, error) { return r, nil }))
	if _, err := m.Open(context.Background(), OpenRequest{SessionID: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SessionApproval("other", "yolo"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatal(err)
	}
	if _, err := m.SessionApproval("a", "bogus"); err == nil {
		t.Fatal("invalid mode accepted")
	}
	for _, state := range []string{"running", "paused"} {
		r.state = state
		if _, err := m.SessionApproval("a", "yolo"); !errors.Is(err, ErrSessionConflict) {
			t.Fatal(state, err)
		}
	}
	r.state = "idle"
	r.refusal = errors.New("disk refused")
	if _, err := m.SessionApproval("a", "auto"); err == nil || r.mode != "ask" {
		t.Fatal("failed write changed live posture")
	}
	r.refusal = nil
	if got, err := m.SessionApproval("a", "auto"); err != nil || got != "auto" {
		t.Fatal(got, err)
	}
	if got, err := m.SessionApproval("a", ""); err != nil || got != "auto" {
		t.Fatal(got, err)
	}
}
