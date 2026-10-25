package computeruse

import (
	"testing"
)

func TestTargetUsesExactLiveIdentityAndNeverCachedCatalog(t *testing.T) {
	c, f := fixture()
	f.state = `{"apps":[{"pid":41,"name":"Old","bundle_id":"example.old","launch_time":50}],"target":{"pid":42,"name":"New","bundle_id":"example.new","launch_time":100}}`
	target, err := c.Target(t.Context(), 42)
	if err != nil || target.PID != 42 || target.BundleID != "example.new" || target.LaunchTime != 100 {
		t.Fatalf("new app identity was not resolved: %+v %v", target, err)
	}
	if len(f.calls) != 1 || f.calls[0].Action != "resolve_target" || f.calls[0].PID != 42 {
		t.Fatalf("identity used an app catalog instead of the exact PID: %+v", f.calls)
	}
	for _, state := range []string{
		`{"target":{}}`,
		`{"target":{"pid":43,"name":"Other","bundle_id":"example.new","launch_time":100}}`,
		`{"target":{"pid":42,"name":"New","bundle_id":"example.new","launch_time":0}}`,
		`{"apps":[{"pid":42,"name":"Old","bundle_id":"example.old","launch_time":50}]}`,
	} {
		f.state = state
		if _, err := c.Target(t.Context(), 42); err == nil {
			t.Fatalf("invalid identity or stale catalog accepted: %s", state)
		}
	}
	// The internal identity operation is never a model-visible action.
	if ValidateRequest(Request{Action: "resolve_target", PID: 42}) == nil {
		t.Fatal("internal identity route became a public desktop operation")
	}
}
