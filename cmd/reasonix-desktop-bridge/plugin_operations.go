package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/installsource"
	"reasonix/internal/pluginpkg"
)

type previewPluginInstallRequest struct {
	Source     string `json:"source"`
	PlanID     string `json:"planId"`
	AcceptRisk bool   `json:"acceptRisk"`
}

type previewPluginRemoveRequest struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
}

type installPluginToolResponse struct {
	OK      bool   `json:"ok"`
	Status  string `json:"status"`
	PlanID  string `json:"planId"`
	Actions []struct {
		Status       string `json:"status"`
		Kind         string `json:"kind"`
		Action       string `json:"action"`
		Name         string `json:"name"`
		Version      string `json:"version"`
		ManifestKind string `json:"manifestKind"`
		RiskLevel    string `json:"riskLevel"`
		SkillCount   int    `json:"skillCount"`
		AgentCount   int    `json:"agentCount"`
		CommandCount int    `json:"commandCount"`
		HookCount    int    `json:"hookCount"`
		ToolCount    int    `json:"toolCount"`
		PromptCount  int    `json:"promptCount"`
		ThemeCount   int    `json:"themeCount"`
		Runtime      *struct {
			Command      string   `json:"command"`
			Args         []string `json:"args"`
			Intercepts   []string `json:"intercepts"`
			Replaces     []string `json:"replaces"`
			Capabilities []string `json:"capabilities"`
			FullTrust    bool     `json:"fullTrust"`
		} `json:"runtime"`
	} `json:"actions"`
	Warnings []string `json:"warnings"`
}

type previewPluginPlanAction struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	ManifestKind string   `json:"manifestKind"`
	RiskLevel    string   `json:"riskLevel"`
	Skills       int      `json:"skills"`
	Agents       int      `json:"agents"`
	Commands     int      `json:"commands"`
	Hooks        int      `json:"hooks"`
	MCPServers   int      `json:"mcpServers"`
	Prompts      int      `json:"prompts"`
	Themes       int      `json:"themes"`
	Runtime      bool     `json:"runtime"`
	RuntimeCmd   string   `json:"runtimeCommand"`
	Intercepts   []string `json:"intercepts"`
	Replaces     []string `json:"replaces"`
}

type previewPluginInstallPlan struct {
	ProtocolVersion int                       `json:"protocolVersion"`
	PlanID          string                    `json:"planId"`
	Actions         []previewPluginPlanAction `json:"actions"`
	WarningCount    int                       `json:"warningCount"`
}

type previewPluginOperationResult struct {
	ProtocolVersion int                   `json:"protocolVersion"`
	Status          string                `json:"status"`
	FailedNames     []string              `json:"failedNames"`
	Settings        previewPluginSettings `json:"settings"`
}

func validPreviewPluginSource(source string) bool {
	if len(source) == 0 || len(source) > 4096 || strings.ContainsRune(source, 0) {
		return false
	}
	if filepath.IsAbs(source) {
		info, err := os.Stat(source)
		return err == nil && info.IsDir()
	}
	u, err := url.Parse(source)
	if err != nil || u.Scheme != "https" || u.Hostname() != "github.com" || u.Port() != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return len(parts) >= 2 && parts[0] != "" && parts[1] != ""
}

func previewPluginToolRequest(source string, apply bool, planID string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"source": source, "kind": "plugin", "scope": "global", "mode": "copy", "replace": false,
		"apply": apply, "planId": planID,
	})
	return raw
}

func planPreviewPlugin(r *http.Request, source string) (installPluginToolResponse, bool) {
	tool := installsource.NewTool(installsource.Options{})
	out, err := tool.Execute(r.Context(), previewPluginToolRequest(source, false, ""))
	if err != nil || len(out) > 2<<20 {
		return installPluginToolResponse{}, false
	}
	var planned installPluginToolResponse
	if json.Unmarshal([]byte(out), &planned) != nil || !planned.OK || planned.Status != "planned" || planned.PlanID == "" || len(planned.Actions) == 0 || len(planned.Actions) > 16 {
		return installPluginToolResponse{}, false
	}
	for _, action := range planned.Actions {
		if action.Kind != "plugin" || action.Action != "install_plugin_package" || !pluginpkg.IsValidName(action.Name) {
			return installPluginToolResponse{}, false
		}
	}
	return planned, true
}

func (b *bridgeServer) planPluginInstall(w http.ResponseWriter, r *http.Request) {
	var input previewPluginInstallRequest
	if err := decodeJSONBody(w, r, 64<<10, &input); err != nil || input.PlanID != "" || input.AcceptRisk {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid plugin source request")
		return
	}
	input.Source = strings.TrimSpace(input.Source)
	if !validPreviewPluginSource(input.Source) {
		writeProtocolError(w, http.StatusBadRequest, "invalid_source", "select a local plugin folder or a public GitHub repository")
		return
	}
	plan, ok := planPreviewPlugin(r, input.Source)
	if !ok {
		writeProtocolError(w, http.StatusBadRequest, "plan_failed", "plugin source could not be reviewed")
		return
	}
	view := previewPluginInstallPlan{ProtocolVersion: desktopbridge.ProtocolVersion, PlanID: plan.PlanID, Actions: []previewPluginPlanAction{}, WarningCount: len(plan.Warnings)}
	for _, action := range plan.Actions {
		item := previewPluginPlanAction{
			Name: action.Name, Version: action.Version, ManifestKind: action.ManifestKind,
			RiskLevel: action.RiskLevel, Skills: action.SkillCount, Agents: action.AgentCount,
			Commands: action.CommandCount, Hooks: action.HookCount, MCPServers: action.ToolCount,
			Prompts: action.PromptCount, Themes: action.ThemeCount,
			Intercepts: []string{}, Replaces: []string{},
		}
		if action.Runtime != nil {
			item.Runtime = action.Runtime.FullTrust
			item.RuntimeCmd = strings.Join(append([]string{action.Runtime.Command}, action.Runtime.Args...), " ")
			item.Intercepts = append([]string{}, action.Runtime.Intercepts...)
			item.Replaces = append([]string{}, action.Runtime.Replaces...)
		}
		view.Actions = append(view.Actions, item)
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) installPlugin(w http.ResponseWriter, r *http.Request) {
	var input previewPluginInstallRequest
	if err := decodeJSONBody(w, r, 64<<10, &input); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid plugin install request")
		return
	}
	input.Source = strings.TrimSpace(input.Source)
	if !validPreviewPluginSource(input.Source) || !strings.HasPrefix(input.PlanID, "sha256:") {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "review the plugin plan before installing")
		return
	}
	b.pluginOpsMu.Lock()
	defer b.pluginOpsMu.Unlock()
	plan, ok := planPreviewPlugin(r, input.Source)
	if !ok || plan.PlanID != input.PlanID {
		writeProtocolError(w, http.StatusConflict, "plan_changed", "plugin plan changed; review it again")
		return
	}
	for _, action := range plan.Actions {
		if action.RiskLevel == "high" && !input.AcceptRisk {
			writeProtocolError(w, http.StatusBadRequest, "risk_not_accepted", "confirm the high-risk plugin plan before installing")
			return
		}
	}
	tool := installsource.NewTool(installsource.Options{})
	out, err := tool.Execute(r.Context(), previewPluginToolRequest(input.Source, true, input.PlanID))
	if err != nil || len(out) > 2<<20 {
		writeProtocolError(w, http.StatusConflict, "install_failed", "plugin installation failed or the reviewed plan changed")
		return
	}
	var result installPluginToolResponse
	if json.Unmarshal([]byte(out), &result) != nil || (result.Status != "done" && result.Status != "partial") || result.PlanID != input.PlanID {
		writeProtocolError(w, http.StatusConflict, "install_failed", "plugin installation did not match the reviewed plan")
		return
	}
	settings, readErr := loadPreviewPluginSettings()
	if readErr != nil {
		writeProtocolError(w, http.StatusInternalServerError, "read_failed", "plugin installation finished but inventory could not be read")
		return
	}
	failedNames := []string{}
	for _, action := range result.Actions {
		if action.Status == "failed" {
			failedNames = append(failedNames, action.Name)
		}
	}
	writeJSON(w, http.StatusOK, previewPluginOperationResult{ProtocolVersion: desktopbridge.ProtocolVersion, Status: result.Status, FailedNames: failedNames, Settings: settings})
}

func (b *bridgeServer) removePlugin(w http.ResponseWriter, r *http.Request) {
	var input previewPluginRemoveRequest
	if err := decodeJSONBody(w, r, 64<<10, &input); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid plugin removal request")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if !pluginpkg.IsValidName(input.Name) || len(input.Revision) != 64 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid plugin removal request")
		return
	}
	b.pluginOpsMu.Lock()
	defer b.pluginOpsMu.Unlock()
	current, found, err := pluginpkg.FindInstalled(appconfig.ReasonixHomeDir(), input.Name)
	if err != nil || !found || pluginpkg.InstalledRevision(current) != input.Revision {
		writeProtocolError(w, http.StatusConflict, "plugin_changed", "plugin changed; refresh and retry")
		return
	}
	tool := installsource.NewTool(installsource.Options{})
	raw, _ := json.Marshal(map[string]any{"op": "uninstall", "kind": "plugin", "scope": "global", "name": input.Name, "revision": input.Revision})
	out, err := tool.Execute(r.Context(), raw)
	if err != nil || len(out) > 2<<20 {
		writeProtocolError(w, http.StatusConflict, "remove_failed", "plugin could not be removed")
		return
	}
	var result installPluginToolResponse
	if json.Unmarshal([]byte(out), &result) != nil || !result.OK || result.Status != "done" || len(result.Actions) != 1 || result.Actions[0].Kind != "plugin" {
		writeProtocolError(w, http.StatusConflict, "remove_failed", "plugin removal was incomplete; refresh the inventory")
		return
	}
	settings, readErr := loadPreviewPluginSettings()
	if readErr != nil {
		writeProtocolError(w, http.StatusInternalServerError, "read_failed", "plugin removed but inventory could not be read")
		return
	}
	writeJSON(w, http.StatusOK, previewPluginOperationResult{ProtocolVersion: desktopbridge.ProtocolVersion, Status: "done", FailedNames: []string{}, Settings: settings})
}
