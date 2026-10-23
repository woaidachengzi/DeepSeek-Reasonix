//go:build darwin && cgo

package computeruse

import (
	"encoding/json"
	"testing"
)

// This exercises the actual compiled Cocoa bridge without prompting for TCC
// permissions, capturing any screen, launching an app, or sending input.
func TestComputerUseNativeStatusAndCatalog(t *testing.T) {
	c := New()
	for _, action := range []string{"status", "apps"} {
		result, err := c.Execute(t.Context(), Request{Action: action})
		if err != nil || !json.Valid(result.State) || result.Dispatched || result.Image != "" {
			t.Fatalf("%s: %+v %v", action, result, err)
		}
		t.Logf("%s: %s", action, result.State)
	}
}
