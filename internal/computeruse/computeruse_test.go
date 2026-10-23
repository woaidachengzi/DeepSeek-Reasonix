package computeruse

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeDesktop struct {
	state, image string
	calls        []Request
	failAction   bool
	onObserve    func()
}

func (f *fakeDesktop) call(_ context.Context, r Request) (nativeResult, error) {
	f.calls = append(f.calls, r)
	if r.Action == "observe" && f.onObserve != nil {
		f.onObserve()
	}
	if r.Action != "observe" && f.failAction {
		return nativeResult{Dispatched: true}, errors.New("observation failed after dispatch")
	}
	return nativeResult{OK: true, State: json.RawMessage(f.state), Image: f.image, WindowID: 11, Dispatched: r.Action != "observe"}, nil
}
func fixture() (*Controller, *fakeDesktop) {
	f := &fakeDesktop{state: `{"pid":42,"window_id":11,"frontmost":true}`, image: "pixels-one"}
	return &Controller{backend: f, now: time.Now}, f
}
func observed(t *testing.T, c *Controller) Result {
	t.Helper()
	r, err := c.Execute(t.Context(), Request{Action: "observe", PID: 42})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestComputerUseRequiresOwnedFreshObservation(t *testing.T) {
	for _, kind := range []string{"missing", "foreign-controller", "wrong-pid", "expired"} {
		t.Run(kind, func(t *testing.T) {
			c, f := fixture()
			r := observed(t, c)
			request := Request{Action: "key", PID: 42, Key: "return", Observation: r.Observation}
			switch kind {
			case "missing":
				request.Observation = ""
			case "foreign-controller":
				c, _ = fixture()
			case "wrong-pid":
				request.PID = 99
			case "expired":
				c.now = func() time.Time { return time.Now().Add(time.Minute) }
			}
			f.calls = nil
			if _, err := c.Execute(t.Context(), request); err == nil {
				t.Fatal("unowned observation accepted")
			}
			if len(f.calls) != 0 {
				t.Fatal("invalid receipt reached native dispatch")
			}
		})
	}
}
func TestComputerUseChangedPixelsSendNoInput(t *testing.T) {
	c, f := fixture()
	r := observed(t, c)
	f.image = "different pixels"
	f.calls = nil
	next, err := c.Execute(t.Context(), Request{Action: "key", PID: 42, Key: "return", Observation: r.Observation})
	if err == nil || !next.StateChanged || next.Dispatched || next.Observation == "" || next.Observation == r.Observation || next.Image != f.image {
		t.Fatalf("changed frame: %+v err=%v", next, err)
	}
	if len(f.calls) != 1 || f.calls[0].Action != "observe" {
		t.Fatal("stale visual state dispatched input")
	}
}

func TestComputerUseExpiryDuringVerificationSendsNoInput(t *testing.T) {
	c, f := fixture()
	now := time.Now()
	c.now = func() time.Time { return now }
	r := observed(t, c)
	f.onObserve = func() { now = now.Add(30 * time.Second) }
	f.calls = nil
	request := Request{Action: "key", PID: 42, Key: "return", Observation: r.Observation}
	result, err := c.Execute(t.Context(), request)
	if err == nil || result.Dispatched || len(f.calls) != 1 || f.calls[0].Action != "observe" {
		t.Fatalf("expired during capture: %+v calls=%+v err=%v", result, f.calls, err)
	}
	if c.receipt.token != "" {
		t.Fatal("expired receipt retained")
	}
}

func TestComputerUseActivationAllowsWindowAnimationButNotAppReplacement(t *testing.T) {
	for _, replaced := range []bool{false, true} {
		c, f := fixture()
		f.state = `{"pid":42,"window_id":11,"launch_time":100,"bounds":{"width":80},"frontmost":false}`
		f.image = ""
		r := observed(t, c)
		f.state = `{"pid":42,"window_id":11,"launch_time":100,"bounds":{"width":90},"frontmost":false}`
		if replaced {
			f.state = `{"pid":42,"window_id":11,"launch_time":200}`
		}
		f.calls = nil
		result, err := c.Execute(t.Context(), Request{Action: "activate", PID: 42, Observation: r.Observation})
		if replaced {
			if err == nil || result.Dispatched || len(f.calls) != 1 {
				t.Fatal("activation accepted a replaced app identity")
			}
		} else if err != nil || !result.Dispatched || len(f.calls) != 2 {
			t.Fatalf("window animation prevented activation: %+v %v", result, err)
		}
	}
}
func TestComputerUseConsumesReceiptAndPreservesUncertainDispatch(t *testing.T) {
	c, f := fixture()
	r := observed(t, c)
	f.failAction = true
	request := Request{Action: "key", PID: 42, Key: "return", Observation: r.Observation}
	result, err := c.Execute(t.Context(), request)
	if err == nil || !result.Dispatched {
		t.Fatalf("uncertain dispatch lost: %+v %v", result, err)
	}
	before := len(f.calls)
	if _, err = c.Execute(t.Context(), request); err == nil || len(f.calls) != before {
		t.Fatal("uncertain action replayed")
	}
}
func TestComputerUseDispatchPinsObservedWindowAndReturnsNextReceipt(t *testing.T) {
	c, f := fixture()
	r := observed(t, c)
	next, err := c.Execute(t.Context(), Request{Action: "key", PID: 42, WindowID: 999, Key: "tab", Observation: r.Observation})
	if err != nil || !next.Dispatched || next.Observation == "" || next.Observation == r.Observation {
		t.Fatalf("action result=%+v err=%v", next, err)
	}
	if last := f.calls[len(f.calls)-1]; last.WindowID != 11 {
		t.Fatal("caller replaced observed window identity")
	}
}
func TestComputerUseValidationAndCancellationPrecedeNativeCalls(t *testing.T) {
	for _, r := range []Request{
		{Action: "unknown"}, {Action: "observe", PID: 0},
		{Action: "key", PID: 42, Observation: "x", Key: "unsupported"},
		{Action: "type", PID: 42, Observation: "x", Text: strings.Repeat("x", 16385)},
		{Action: "type", PID: 42, Observation: "x"},
		{Action: "scroll", PID: 42, Observation: "x", Delta: 101},
		{Action: "click", PID: 42, Observation: "x"},
	} {
		c, f := fixture()
		if _, err := c.Execute(t.Context(), r); err == nil || len(f.calls) != 0 {
			t.Fatalf("invalid request reached native: %+v", r)
		}
	}
	c, f := fixture()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.Execute(ctx, Request{Action: "apps"}); !errors.Is(err, context.Canceled) || len(f.calls) != 0 {
		t.Fatal("canceled operation reached native")
	}
}
func TestComputerUseSerializesSharedDesktop(t *testing.T) {
	_, f := fixture()
	var wg sync.WaitGroup
	for range 10 {
		c := &Controller{backend: f, now: time.Now}
		wg.Go(func() { _, _ = c.Execute(t.Context(), Request{Action: "observe", PID: 42}) })
	}
	wg.Wait()
	if len(f.calls) != 10 {
		t.Fatalf("shared backend lost calls: %d", len(f.calls))
	}
}
