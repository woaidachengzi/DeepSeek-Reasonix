package builtin

import (
	"encoding/json"
	"testing"

	"reasonix/internal/computeruse"
	"reasonix/internal/sandbox"
	"reasonix/internal/tool"
)

func TestComputerUseHonorsWorkspaceSandbox(t *testing.T) {
	c := newComputerUse(true)
	if c.ReadOnly() || c.ProviderVisible(t.Context()) {
		t.Fatal("desktop tool escaped sandbox/read-only classification")
	}
	if _, _, err := c.ExecuteWithImages(t.Context(), json.RawMessage(`{"action":"apps"}`)); err == nil {
		t.Fatal("sandboxed direct execution succeeded")
	}
	if computeruse.Available() {
		for _, candidate := range (Workspace{Dir: t.TempDir(), Bash: sandbox.Spec{Mode: "enforce"}}).Tools("computer_use") {
			bound, ok := candidate.(computerUse)
			if !ok || bound.ProviderVisible(t.Context()) {
				t.Fatal("workspace did not bind the desktop sandbox boundary")
			}
		}
		var found bool
		for _, candidate := range (Workspace{Dir: t.TempDir()}).Tools("computer_use") {
			found = true
			if _, ok := candidate.(tool.ImageTool); !ok {
				t.Fatal("computer tool lost screenshot image channel")
			}
		}
		if !found {
			t.Fatal("native desktop tool not registered")
		}
	}
}

func TestComputerUseErrorsNeverRetry(t *testing.T) {
	c := newComputerUse(false)
	_, _, err := c.ExecuteWithImages(t.Context(), json.RawMessage(`{"action":"click","pid":1}`))
	classified, ok := err.(interface{ RetryableToolError() bool })
	if !ok || classified.RetryableToolError() {
		t.Fatal("desktop errors must opt out of automatic tool replay")
	}
}
