package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"reasonix/internal/mcpregistry"
)

func TestMCPMarketplaceSearchAndResolveUseLiveRegistryMetadata(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0.1/servers" || r.URL.Query().Get("version") != "latest" {
			t.Errorf("Registry request = %s", r.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"servers": []any{
			map[string]any{"server": map[string]any{
				"name": "io.example/remote", "title": "Remote", "version": "1.0.0",
				"remotes": []any{map[string]any{"type": "streamable-http", "url": "https://mcp.example.test/mcp"}},
			}},
		}})
	}))
	defer registry.Close()

	_, home := mcpTestBridge(t)
	// Inject a local Registry client into a bridge using the private test profile.
	b := newBridgeServer(testToken, "instance", nil)
	b.mcpRegistry = mcpregistry.New(filepath.Join(home, "registry-cache.json"))
	b.mcpRegistry.BaseURL = registry.URL
	handler := b.handler()

	search := mcpRequest(t, handler, http.MethodGet, "/v1/mcp/marketplace?query=remote", "")
	if search.Code != http.StatusOK {
		t.Fatalf("search status = %d, body = %s", search.Code, search.Body.String())
	}
	var listing mcpMarketplaceResponse
	if err := json.Unmarshal(search.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if listing.Cached || len(listing.Servers) != 1 || !listing.Servers[0].Installable || listing.Servers[0].Transport != "http" {
		t.Fatalf("listing = %+v", listing)
	}
	resolved := mcpRequest(t, handler, http.MethodPost, "/v1/mcp/marketplace/resolve", `{"name":"io.example/remote"}`)
	if resolved.Code != http.StatusOK {
		t.Fatalf("resolve status = %d, body = %s", resolved.Code, resolved.Body.String())
	}
	var result mcpMarketplaceResolveResponse
	if err := json.Unmarshal(resolved.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Server.URL != "https://mcp.example.test/mcp" || result.Server.SuggestedName != "remote" {
		t.Fatalf("resolved = %+v", result.Server)
	}
	if response := mcpRequest(t, handler, http.MethodPost, "/v1/mcp/marketplace/resolve", `{"name":""}`); response.Code != http.StatusBadRequest {
		t.Fatalf("blank name status = %d, want 400", response.Code)
	}
	registry.Close()
	// Search may show cached metadata while offline, but resolve must fail.
	cached := mcpRequest(t, handler, http.MethodGet, "/v1/mcp/marketplace?query=remote", "")
	if cached.Code != http.StatusOK {
		t.Fatalf("cached search status = %d, body = %s", cached.Code, cached.Body.String())
	}
	if err := json.Unmarshal(cached.Body.Bytes(), &listing); err != nil || !listing.Cached {
		t.Fatalf("cached listing = %+v, err=%v", listing, err)
	}
	if response := mcpRequest(t, handler, http.MethodPost, "/v1/mcp/marketplace/resolve", `{"name":"io.example/remote"}`); response.Code != http.StatusBadGateway {
		t.Fatalf("offline resolve status = %d, want 502", response.Code)
	}
}
