package control

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/config"
	"reasonix/internal/plugin"
	"reasonix/internal/tool"
)

type staleMCPTestTool struct {
	fakeControlTool
	owner string
}

func (staleMCPTestTool) MCPServerName() string    { return "mock" }
func (staleMCPTestTool) MCPRawToolName() string   { return "echo" }
func (t staleMCPTestTool) MCPPackageName() string { return t.owner }

func staleRemovalController(t *testing.T, retained string) (*Controller, *agent.MCPCapabilityRuntime, *tool.Registry, *plugin.Host) {
	t.Helper()
	home := isolateControlConfigHome(t)
	t.Setenv("REASONIX_HOME", filepath.Join(home, ".reasonix"))
	workspace := t.TempDir()
	reg := tool.NewRegistry()
	reg.Add(fakeControlTool{name: "mcp__other__echo"})
	host := plugin.NewHost()
	runtime := agent.NewMCPCapabilityRuntime(context.Background(), host, nil, reg, nil)
	spec := plugin.Spec{Name: "mock", Command: "never-started"}
	if retained == "runtime" || retained == "package-runtime" {
		source := config.MCPSourceProjectConfig
		if retained == "package-runtime" {
			source = config.MCPSourcePluginPackage
		}
		entry := config.PluginEntry{Name: "mock", Source: source}
		runtime.ConfigureServers([]config.PluginEntry{entry}, []plugin.Spec{spec}, map[string]bool{"mock": true})
		reg.Add(fakeControlTool{name: "mcp__mock__connect"})
	} else if retained == "registry" || retained == "package-registry" {
		owner := ""
		if retained == "package-registry" {
			owner = "installed-package"
		}
		reg.Add(staleMCPTestTool{fakeControlTool{name: "mcp__mock__echo"}, owner})
	}
	host.RecordFailure(spec, errors.New("unreachable"))
	c := New(Options{Host: host, Registry: reg, CapabilityRuntime: runtime, WorkspaceRoot: workspace})
	t.Cleanup(func() { c.Close(); host.Close() })
	return c, runtime, reg, host
}

func TestRemoveMCPAfterExternalSourceDisappears(t *testing.T) {
	for _, retained := range []string{"runtime", "registry", "failure"} {
		t.Run(retained, func(t *testing.T) {
			c, runtime, reg, host := staleRemovalController(t, retained)
			if _, err := c.RemoveMCPServer(" mock "); err != nil {
				t.Fatal(err)
			}
			if _, found := runtime.ServerIdentity("mock"); found || len(host.Failures()) != 0 {
				t.Fatal("removed MCP retained runtime identity or failure")
			}
			for _, name := range reg.AllNames() {
				if strings.HasPrefix(name, "mcp__mock__") {
					t.Fatalf("stale tool survived: %s", name)
				}
			}
			if _, found := reg.Get("mcp__other__echo"); !found {
				t.Fatal("unrelated MCP tool was removed")
			}
			if _, err := c.RemoveMCPServer("mock"); err == nil {
				t.Fatal("unknown MCP was reported removed")
			}
		})
	}
}

func TestStalePluginMCPStillRequiresPluginRemoval(t *testing.T) {
	for _, retained := range []string{"package-runtime", "package-registry"} {
		t.Run(retained, func(t *testing.T) {
			c, _, reg, host := staleRemovalController(t, retained)
			if _, err := c.RemoveMCPServer("mock"); err == nil || !strings.Contains(err.Error(), "managed by a plugin") {
				t.Fatalf("package-owned stale MCP removal: %v", err)
			}
			if len(reg.AllNames()) != 2 || len(host.Failures()) != 1 {
				t.Fatal("rejected removal modified runtime state")
			}
		})
	}
}

func TestMalformedMCPSourceCannotAuthorizeStaleRemoval(t *testing.T) {
	for _, file := range []string{"reasonix.toml", ".mcp.json"} {
		t.Run(file, func(t *testing.T) {
			c, runtime, reg, host := staleRemovalController(t, "runtime")
			path := filepath.Join(c.workspaceRoot, file)
			if err := os.WriteFile(path, []byte("[invalid"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := c.RemoveMCPServer("mock"); err == nil {
				t.Fatal("unreadable declaration was mistaken for an external removal")
			}
			if _, found := runtime.ServerIdentity("mock"); !found || len(reg.AllNames()) != 2 || len(host.Failures()) != 1 {
				t.Fatal("failed removal changed retained state")
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != "[invalid" {
				t.Fatal("malformed source was rewritten")
			}
		})
	}
}
