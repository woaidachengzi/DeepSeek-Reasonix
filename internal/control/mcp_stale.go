package control

import (
	"reasonix/internal/config"
	"reasonix/internal/tool"
)

func (c *Controller) staleMCPEntry(name string) (config.PluginEntry, bool) {
	entry, found := c.capabilityRuntime.ServerIdentity(name)
	if entry.Source == config.MCPSourcePluginPackage {
		return entry, true
	}
	host := c.mcp.hostRef()
	if host != nil {
		for _, server := range host.Servers() {
			if server.Name != name {
				continue
			}
			source := config.MCPConfigSource(server.ConfigSource)
			if source == config.MCPSourcePluginPackage {
				return config.PluginEntry{Name: name, Source: source}, true
			}
			if !found {
				entry, found = config.PluginEntry{Name: name, Source: source}, true
			}
		}
	}
	if reg := c.mcp.registry(); reg != nil {
		for _, toolName := range reg.AllNames() {
			registered, _ := reg.Get(toolName)
			if meta, ok := registered.(tool.MCPMetadata); ok && meta.MCPServerName() == name {
				if owned, ok := registered.(tool.MCPPackageMetadata); ok && owned.MCPPackageName() != "" {
					return config.PluginEntry{Name: name, Source: config.MCPSourcePluginPackage}, true
				}
				if !found {
					entry, found = config.PluginEntry{Name: name}, true
				}
			}
		}
	}
	if !found && host != nil {
		for _, failure := range host.Failures() {
			if failure.Name == name {
				return config.PluginEntry{Name: name}, true
			}
		}
	}
	return entry, found
}
