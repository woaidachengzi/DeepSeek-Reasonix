package control

import (
	"fmt"
	"strings"

	"reasonix/internal/config"
)

// RemoveMCPServer removes writable config before disconnecting the live server.
// A persistence failure must not produce a false-successful session-only removal.
// MCPs contributed by installed plugin packages cannot be removed independently.
func (c *Controller) RemoveMCPServer(name string) (disconnected bool, err error) {
	name = strings.TrimSpace(name)
	cfg, lerr := config.LoadForRootReadOnly(c.workspaceRoot)
	if lerr != nil {
		return false, lerr
	}
	if owner, ok := cfg.PluginPackageOwner(name); ok {
		return false, fmt.Errorf("MCP server %q is managed by plugin %q; disable or remove the plugin instead", name, owner)
	}
	entry, removed, _, rerr := config.RemovePluginFromEffectiveSourceForRoot(c.workspaceRoot, name)
	if rerr != nil {
		return false, rerr
	}
	if !removed {
		if entry.Name != "" {
			return false, fmt.Errorf("no removable MCP server named %q", name)
		}
		var known bool
		entry, known = c.staleMCPEntry(name)
		if !known {
			return false, fmt.Errorf("no removable MCP server named %q", name)
		}
		if entry.Source == config.MCPSourcePluginPackage {
			return false, fmt.Errorf("MCP server %q is managed by a plugin; disable or remove the plugin instead", name)
		}
	}
	if entry.Source != "" {
		_ = config.DefaultMCPActivationStore().ClearServer(entry, c.workspaceRoot)
	}
	removedState := reconcileRemovedMCPState(c.workspaceRoot, name)
	if c.capabilityRuntime != nil {
		// Revoke before touching the shared Host so an overlapping resolver cannot
		// reuse a sibling tab's still-connected client.
		c.capabilityRuntime.RemoveServer(name)
	}
	disconnected = c.mcp.disconnect(name)
	if !disconnected {
		c.mcp.removeToolPrefix(name)
	}
	if host := c.mcp.hostRef(); host != nil {
		host.ClearFailure(name)
	}
	// A lower-priority same-name declaration may now be effective. Restore its
	// cached/on-demand surface without starting a process; otherwise ensure the
	// removed name stays absent.
	if removedState.fallbackFound {
		enabled := removedState.fallback.ShouldAutoStart()
		if resolved, resolveErr := config.DefaultMCPActivationStore().IsEnabled(removedState.fallback, c.workspaceRoot); resolveErr == nil {
			enabled = resolved
		}
		if enabled {
			_, _ = c.RegisterMCPServerOnDemand(removedState.fallback)
		} else {
			c.syncCapabilityRuntimeFromConfig(name, &enabled)
		}
	} else {
		c.syncCapabilityRuntimeFromConfig(name, nil)
	}
	return disconnected, removedState.cleanupErr
}
