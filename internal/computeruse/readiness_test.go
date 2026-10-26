package computeruse

import "testing"

func TestReadinessGNOMEShellVersion(t *testing.T) {
	for input, want := range map[string]int{"GNOME Shell 50.1\n": 50, "GNOME Shell 49.5": 49, "GNOME Shell unknown": 0, "mutter 50.1": 0, "": 0} {
		if got := gnomeShellMajor(input); got != want {
			t.Fatalf("version %q: got %d want %d", input, got, want)
		}
	}
}
