//go:build darwin && cgo

package computeruse

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Compile the production allocation/dispatch gateway into a native test runner.
// Its post callback only inspects envelopes; it never sends input or asks TCC.
func TestComputerUseNativePointerRouting(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "pointer-test")
	compile := exec.CommandContext(ctx, "/usr/bin/xcrun", "--sdk", "macosx", "clang", "-fobjc-arc", "-I.",
		"-framework", "AppKit", "-framework", "ApplicationServices", "native_pointer_darwin.m", "testdata/native_pointer.m", "-o", binary)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile native pointer tests: %v\n%s", err, output)
	}
	output, err := exec.CommandContext(ctx, binary).CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 77 {
		t.Skip("no graphical macOS session; native event envelopes unavailable")
	}
	if err != nil {
		t.Fatalf("native pointer tests: %v\n%s", err, output)
	}
	t.Log(string(output))
}
