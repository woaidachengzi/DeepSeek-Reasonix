package agent

import (
	"strings"

	"reasonix/internal/config"
)

// ServerIdentity returns retained provenance without refreshing configuration
// or exposing the server's launch parameters.
func (r *MCPCapabilityRuntime) ServerIdentity(name string) (config.PluginEntry, bool) {
	if r == nil {
		return config.PluginEntry{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	server, found := r.servers[strings.TrimSpace(name)]
	if !found {
		return config.PluginEntry{}, false
	}
	return config.PluginEntry{Name: server.entry.Name, Source: server.entry.Source}, true
}
