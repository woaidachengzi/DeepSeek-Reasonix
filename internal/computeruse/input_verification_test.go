package computeruse

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestReadbackCannotConfirmAnotherTarget(t *testing.T) {
	before := inputReadbackState(t, "", 0, 0, true)
	for _, change := range [][2]string{
		{`"frontmost":true`, `"frontmost":false`},
		{`"window_id":11`, `"window_id":12`},
		{`"identifier":""`, `"identifier":"other-field"`},
		{`"role":"AXTextField"`, `"role":"AXButton"`},
		{`"subrole":""`, `"subrole":"AXSecureTextField"`},
	} {
		after := inputReadbackState(t, "hello", 5, 0, true)
		after.State = bytes.ReplaceAll(after.State, []byte(change[0]), []byte(change[1]))
		result := verifiedInputResult(Result{Dispatched: true}, Request{Action: "type", Text: "hello"}, before, after)
		if result.InputVerification != "unconfirmed" {
			t.Fatalf("confirmed a different input target: %s", change[1])
		}
	}
}

func TestUncertainTypingIsNeverReplayed(t *testing.T) {
	c, f := fixture()
	r := observed(t, c)
	f.failAction = true
	request := Request{Action: "type", PID: 42, Observation: r.Observation, Text: "fixture"}
	result, err := c.Execute(t.Context(), request)
	if err == nil || !result.Dispatched || result.InputVerification != "unconfirmed" {
		t.Fatalf("uncertain input was misreported: %+v %v", result, err)
	}
	count := len(f.calls)
	if _, err = c.Execute(t.Context(), request); err == nil || len(f.calls) != count {
		t.Fatal("uncertain input was replayed")
	}
}

func inputReadbackState(t *testing.T, value string, location, length int, hashed bool) nativeResult {
	t.Helper()
	f := focusedInput{Role: "AXTextField", Bounds: frameRect{X: 10, Y: 10, Width: 100, Height: 30}, Value: &value, Selection: &textSelection{Location: location, Length: length}}
	if hashed {
		hash := sha256.Sum256([]byte(value))
		f.ValueHash = hex.EncodeToString(hash[:])
		f.ValueLength = len(utf16.Encode([]rune(value)))
		if len([]rune(value)) > 512 {
			short := string([]rune(value)[:512])
			f.Value = &short
		}
	}
	raw, err := json.Marshal(map[string]any{"pid": 42, "window_id": 11, "launch_time": 1, "frontmost": true, "focus": f})
	if err != nil {
		t.Fatal(err)
	}
	return nativeResult{State: raw}
}

func TestTypedTextReadbackChecksExpectedReplacement(t *testing.T) {
	tests := []struct {
		name, old, text, after string
		location, length       int
		hashed                 bool
		want                   string
	}{
		{"empty field", "", "hello", "hello", 0, 0, false, "confirmed"},
		{"selection replacement", "hello", "i", "hi", 1, 4, false, "confirmed"},
		{"UTF16 selection", "A😀B", "源源", "A源源B", 1, 2, true, "confirmed"},
		{"dropped event", "", "hello", "", 0, 0, true, "unconfirmed"},
		{"recipient chip", "", "fixture@example.invalid", "", 0, 0, true, "unconfirmed"},
		{"long inserted text", "", strings.Repeat("字", 600), strings.Repeat("字", 600), 0, 0, true, "confirmed"},
		{"unknown old suffix", strings.Repeat("字", 600), "x", strings.Repeat("字", 600) + "x", 600, 0, true, "unconfirmed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := inputReadbackState(t, tc.old, tc.location, tc.length, tc.hashed)
			after := inputReadbackState(t, tc.after, tc.location+len(utf16.Encode([]rune(tc.text))), 0, tc.hashed)
			result := verifiedInputResult(Result{Dispatched: true}, Request{Action: "type", Text: tc.text}, before, after)
			if result.InputVerification != tc.want {
				t.Fatalf("readback %q want %q", result.InputVerification, tc.want)
			}
			blocked := verifiedInputResult(Result{}, Request{Action: "type", Text: tc.text}, before, after)
			if blocked.InputVerification != "" {
				t.Fatal("blocked input acquired a readback result")
			}
		})
	}
}

func TestChangedFullValueHashInvalidatesLongField(t *testing.T) {
	a, b := inputReadbackState(t, strings.Repeat("x", 600), 0, 0, true), inputReadbackState(t, strings.Repeat("x", 599)+"y", 0, 0, true)
	if digest(a) == digest(b) {
		t.Fatal("a change beyond the displayed prefix was ignored")
	}
	if observationChangeReason(a, b) != "focus_changed" {
		t.Fatal("field edit was not explained as focus/content change")
	}
}
