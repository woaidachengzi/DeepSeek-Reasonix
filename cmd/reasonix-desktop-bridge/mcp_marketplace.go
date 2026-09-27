package main

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/mcpregistry"
)

type mcpMarketplaceResponse struct {
	ProtocolVersion int                 `json:"protocolVersion"`
	Servers         []mcpregistry.Entry `json:"servers"`
	Cached          bool                `json:"cached"`
	Warning         string              `json:"warning,omitempty"`
}

type mcpMarketplaceResolveRequest struct {
	Name string `json:"name"`
}

type mcpMarketplaceResolveResponse struct {
	ProtocolVersion int               `json:"protocolVersion"`
	Server          mcpregistry.Entry `json:"server"`
}

func (b *bridgeServer) marketplaceClient() *mcpregistry.Client {
	if b.mcpRegistry != nil {
		return b.mcpRegistry
	}
	// Keep the regenerable browsing cache inside this Preview profile.
	cachePath := ""
	if home := appconfig.ReasonixHomeDir(); home != "" {
		cachePath = filepath.Join(home, "cache", "mcp-registry-v0.1.json")
	}
	return mcpregistry.New(cachePath)
}

func (b *bridgeServer) searchMCPMarketplace(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if utf8.RuneCountInString(query) > 120 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "MCP Registry search is too long")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	result, err := b.marketplaceClient().Search(ctx, query, 50)
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "registry_unavailable", err.Error())
		return
	}
	servers := result.Entries
	if servers == nil {
		servers = []mcpregistry.Entry{}
	}
	writeJSON(w, http.StatusOK, mcpMarketplaceResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Servers:         servers,
		Cached:          result.Cached,
		Warning:         result.Warning,
	})
}

// Resolve re-fetches live Registry metadata before the renderer prepares an
// install form. Cached browse results are never trusted as installation data.
func (b *bridgeServer) resolveMCPMarketplace(w http.ResponseWriter, r *http.Request) {
	var request mcpMarketplaceResolveRequest
	if err := decodeJSONBody(w, r, 4<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid MCP Registry request")
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" || utf8.RuneCountInString(name) > 240 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "valid MCP Registry server name is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	entry, _, err := b.marketplaceClient().Resolve(ctx, name)
	if err != nil {
		writeProtocolError(w, http.StatusBadGateway, "registry_unavailable", err.Error())
		return
	}
	if _, err := entry.PluginEntry(""); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "manual_setup_required", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, mcpMarketplaceResolveResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Server:          entry,
	})
}
