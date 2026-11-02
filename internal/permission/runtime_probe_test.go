package permission

import (
	"encoding/json"
	"testing"
)

func TestRuntimeCodeCannotUseReadOnlyFallback(t *testing.T) {
	policy := New("ask", nil, nil, nil)
	approver := &stubApprover{}
	gate := NewGate(policy, approver)
	for _, command := range []string{"python3 -v script.py", "python -V script.py", "node -v script.js", `echo "$(python3 -v script.py)"`} {
		args, _ := json.Marshal(map[string]string{"command": command})
		if BashCommandIsReadOnly(args) {
			t.Errorf("read-only execution allowed %q", command)
		}
		if decision := policy.Decide("bash", BashCommandIsReadOnly(args), args); decision != Ask {
			t.Errorf("%q: %s, want ask", command, decision)
		}
		before := approver.calls
		if allow, _, err := gate.Check(t.Context(), "bash", args, false); err != nil || allow || approver.calls != before+1 {
			t.Errorf("%q bypassed interactive approval: allowed=%t, err=%v", command, allow, err)
		}
	}
	for _, command := range []string{"python3 -V", "node -v"} {
		args, _ := json.Marshal(map[string]string{"command": command})
		if !BashCommandIsReadOnly(args) || policy.Decide("bash", BashCommandIsReadOnly(args), args) != Allow {
			t.Errorf("bare version probe denied: %q", command)
		}
	}
}
