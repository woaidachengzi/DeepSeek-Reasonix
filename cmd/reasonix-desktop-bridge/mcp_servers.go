package main

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

// mcpServerView is the only shape in which an MCP server reaches the renderer.
// Credential material is write-only: EnvKeys and HeaderKeys name the keys a
// server expects so a user knows what to fill in, but no value is ever returned.
type mcpServerView struct {
	Name                 string   `json:"name"`
	Type                 string   `json:"type"`
	Source               string   `json:"source"`
	Scope                string   `json:"scope"`
	ConfigPath           string   `json:"configPath"`
	Command              string   `json:"command,omitempty"`
	Args                 []string `json:"args,omitempty"`
	URL                  string   `json:"url,omitempty"`
	EnvKeys              []string `json:"envKeys,omitempty"`
	HeaderKeys           []string `json:"headerKeys,omitempty"`
	StartupTimeoutSecond int      `json:"startupTimeoutSeconds,omitempty"`
	CallTimeoutSecond    int      `json:"callTimeoutSeconds,omitempty"`
	AutoStart            *bool    `json:"autoStart,omitempty"`
	Tier                 string   `json:"tier,omitempty"`
	ManagedByPackage     bool     `json:"managedByPackage,omitempty"`
}

type mcpServerListResponse struct {
	ProtocolVersion int             `json:"protocolVersion"`
	Servers         []mcpServerView `json:"servers"`
}

// listMCPServers reports the effective MCP servers for one workspace without
// disclosing credentials.
func (b *bridgeServer) listMCPServers(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(r.URL.Query().Get("workspaceRoot"))
	cfg, err := appconfig.LoadForRootReadOnly(root)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to read the MCP configuration")
		return
	}
	servers := make([]mcpServerView, 0, len(cfg.Plugins))
	for _, entry := range cfg.Plugins {
		servers = append(servers, mcpServerViewFor(root, entry))
	}
	sort.SliceStable(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })
	writeJSON(w, http.StatusOK, mcpServerListResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Servers:         servers,
	})
}

func mcpServerViewFor(root string, entry appconfig.PluginEntry) mcpServerView {
	view := mcpServerView{
		Name:                 entry.Name,
		Type:                 strings.TrimSpace(entry.Type),
		Source:               string(entry.Source),
		Scope:                mcpScopeFor(entry.Source),
		ConfigPath:           appconfig.MCPConfigPathForEntry(root, entry),
		Command:              entry.Command,
		Args:                 append([]string(nil), entry.Args...),
		URL:                  entry.URL,
		EnvKeys:              sortedKeys(entry.Env),
		HeaderKeys:           sortedKeys(entry.Headers),
		StartupTimeoutSecond: entry.StartupTimeoutSeconds,
		CallTimeoutSecond:    entry.CallTimeoutSeconds,
		AutoStart:            entry.AutoStart,
		Tier:                 entry.Tier,
		ManagedByPackage:     entry.Source == appconfig.MCPSourcePluginPackage,
	}
	if view.Type == "" {
		view.Type = "stdio"
	}
	return view
}

// mcpScopeFor keeps the project/global distinction the settings UI needs
// without exposing the internal source enum.
func mcpScopeFor(source appconfig.MCPConfigSource) string {
	switch source {
	case appconfig.MCPSourceProjectConfig, appconfig.MCPSourceProjectMCPJSON:
		return "project"
	case appconfig.MCPSourceUserConfig, appconfig.MCPSourceLegacyUser:
		return "global"
	default:
		return "other"
	}
}

// sortedKeys returns the key names of a credential map, never its values.
func sortedKeys(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type mcpServerUpsertRequest struct {
	Scope   string `json:"scope"`
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Command string `json:"command,omitempty"`
	// Args and Env are pointers so "absent" is distinguishable from "empty":
	// omitting a field keeps the stored value, sending an empty map clears it,
	// and credentials are never read back to be echoed.
	Args      *[]string          `json:"args,omitempty"`
	Env       *map[string]string `json:"env,omitempty"`
	URL       string             `json:"url,omitempty"`
	Headers   *map[string]string `json:"headers,omitempty"`
	AutoStart *bool              `json:"autoStart,omitempty"`
	Tier      *string            `json:"tier,omitempty"`
}

// upsertMCPServer adds a server, or edits the one with the same name. Editing
// preserves every field the request omits, including stored credentials, so a
// user can change a command without retyping a token — and never sees one.
func (b *bridgeServer) upsertMCPServer(w http.ResponseWriter, r *http.Request) {
	var request mcpServerUpsertRequest
	if err := decodeJSONBody(w, r, 256<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid MCP server request")
		return
	}
	root := strings.TrimSpace(r.URL.Query().Get("workspaceRoot"))
	name := strings.TrimSpace(request.Name)
	if name == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "MCP server name is required")
		return
	}
	source, err := mcpSourceForScope(request.Scope)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	entry := appconfig.PluginEntry{Name: name, Source: source}
	// Seed the merge from the declaration this name already has, in any source.
	// A project entry must not be silently promoted to a global one just because
	// the request asked for the default scope.
	if cfg, err := appconfig.LoadForRootReadOnly(root); err == nil {
		for _, existing := range cfg.Plugins {
			if existing.Name == name {
				entry = existing
				entry.Source = source
				break
			}
		}
	}
	if entry.Source == appconfig.MCPSourcePluginPackage {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "this MCP server is managed by an installed plugin package")
		return
	}

	if trimmed := strings.TrimSpace(request.Type); trimmed != "" {
		entry.Type = trimmed
	}
	if request.Command != "" {
		entry.Command = request.Command
	}
	if request.URL != "" {
		entry.URL = request.URL
	}
	if request.Args != nil {
		entry.Args = append([]string(nil), (*request.Args)...)
	}
	if request.Env != nil {
		entry.Env = *request.Env
	}
	if request.Headers != nil {
		entry.Headers = *request.Headers
	}
	if request.AutoStart != nil {
		entry.AutoStart = request.AutoStart
	}
	if request.Tier != nil {
		entry.Tier = strings.TrimSpace(*request.Tier)
	}

	path, err := appconfig.UpsertPluginInSourceForRoot(root, entry)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "MCP server could not be saved")
		return
	}
	b.writeMCPServers(w, root, path, "saved", name)
}

type mcpServerDeleteRequest struct {
	Name string `json:"name"`
}

type mcpServerMutationResponse struct {
	ProtocolVersion int             `json:"protocolVersion"`
	Status          string          `json:"status"`
	ConfigPath      string          `json:"configPath,omitempty"`
	Server          *mcpServerView  `json:"server,omitempty"`
	Servers         []mcpServerView `json:"servers"`
}

// deleteMCPServer removes a server from whichever configuration file owns it.
func (b *bridgeServer) deleteMCPServer(w http.ResponseWriter, r *http.Request) {
	var request mcpServerDeleteRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid MCP server request")
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "MCP server name is required")
		return
	}
	root := strings.TrimSpace(r.URL.Query().Get("workspaceRoot"))
	removed, err := appconfig.RemovePluginFromSourcesForRoot(root, name)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to remove the MCP server")
		return
	}
	if !removed {
		writeProtocolError(w, http.StatusNotFound, "not_found", "no such MCP server")
		return
	}
	b.writeMCPServers(w, root, "", "removed", name)
}

// writeMCPServers answers with the redacted server that was written and the
// resulting list, so the settings UI never needs a second round trip.
func (b *bridgeServer) writeMCPServers(w http.ResponseWriter, root, path, status, name string) {
	cfg, err := appconfig.LoadForRootReadOnly(root)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to read the MCP configuration")
		return
	}
	servers := make([]mcpServerView, 0, len(cfg.Plugins))
	for _, entry := range cfg.Plugins {
		servers = append(servers, mcpServerViewFor(root, entry))
	}
	sort.SliceStable(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })
	response := mcpServerMutationResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Status:          status,
		ConfigPath:      path,
		Servers:         servers,
	}
	if name = strings.TrimSpace(name); name != "" {
		for i := range servers {
			if servers[i].Name == name {
				response.Server = &servers[i]
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

// mcpSourceForScope maps the scope the settings UI names onto the config
// source the kernel writes to. An empty scope means the user's global config.
func mcpSourceForScope(scope string) (appconfig.MCPConfigSource, error) {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", "global", "user":
		return appconfig.MCPSourceUserConfig, nil
	case "project", "workspace":
		return appconfig.MCPSourceProjectConfig, nil
	default:
		return appconfig.MCPSourceUnknown, errors.New("MCP server scope must be project or global")
	}
}
