package shellsafe

import (
	"path/filepath"
	"strings"
)

// runtimeVersionProbe recognizes only a bare, case-sensitive version flag.
// Python -v imports/runs code; neither it nor a version prefix with more
// arguments establishes a read-only invocation. Share this with legacy and
// nested-substitution readers so they cannot reopen the same permission gap.
func runtimeVersionProbe(fields []string) (runtime, version bool) {
	if len(fields) == 0 {
		return false, false
	}
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(fields[0]), ".exe"))
	switch base {
	case "node":
		return true, len(fields) == 2 && (fields[1] == "-v" || fields[1] == "--version")
	case "python", "python3":
		return true, len(fields) == 2 && (fields[1] == "-V" || fields[1] == "--version")
	default:
		return false, false
	}
}
