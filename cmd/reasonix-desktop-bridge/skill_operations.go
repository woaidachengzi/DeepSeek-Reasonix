package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/installsource"
)

type previewSkillInstallRequest struct {
	Source        string `json:"source"`
	Scope         string `json:"scope"`
	WorkspaceRoot string `json:"workspaceRoot"`
	PlanID        string `json:"planId"`
	AcceptRisk    bool   `json:"acceptRisk"`
}

type previewSkillInstallAction struct {
	Name      string `json:"name"`
	Target    string `json:"target"`
	RiskLevel string `json:"riskLevel"`
}

type previewSkillInstallPlan struct {
	ProtocolVersion int                         `json:"protocolVersion"`
	PlanID          string                      `json:"planId"`
	Actions         []previewSkillInstallAction `json:"actions"`
	WarningCount    int                         `json:"warningCount"`
	Warnings        []string                    `json:"warnings"`
}

type previewSkillInstallResult struct {
	ProtocolVersion int                `json:"protocolVersion"`
	Status          string             `json:"status"`
	FailedNames     []string           `json:"failedNames"`
	Settings        skillsSettingsView `json:"settings"`
}

func validPreviewSkillSource(source string) bool {
	if len(source) == 0 || len(source) > 4096 || strings.ContainsRune(source, 0) {
		return false
	}
	if filepath.IsAbs(source) {
		info, err := os.Stat(source)
		return err == nil && (info.IsDir() || info.Mode().IsRegular() && strings.EqualFold(filepath.Ext(source), ".md"))
	}
	return validPreviewGitHubSource(source)
}

func normalizePreviewSkillInstall(input *previewSkillInstallRequest) bool {
	input.Source = strings.TrimSpace(input.Source)
	if !validPreviewSkillSource(input.Source) || (input.Scope != "global" && input.Scope != "project") {
		return false
	}
	root, err := normalizeSkillsWorkspace(input.WorkspaceRoot)
	if err != nil || input.Scope == "project" && root == "" {
		return false
	}
	input.WorkspaceRoot = root
	return true
}

func previewSkillToolRequest(input previewSkillInstallRequest, apply bool) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"source": input.Source, "kind": "skill", "scope": input.Scope,
		"mode": "copy", "strict": true, "replace": false,
		"apply": apply, "planId": input.PlanID,
	})
	return raw
}

func planPreviewSkill(r *http.Request, input previewSkillInstallRequest) (installPluginToolResponse, bool) {
	tool := installsource.NewTool(installsource.Options{ProjectRoot: input.WorkspaceRoot})
	out, err := tool.Execute(r.Context(), previewSkillToolRequest(input, false))
	if err != nil || len(out) > 2<<20 {
		return installPluginToolResponse{}, false
	}
	var plan installPluginToolResponse
	if json.Unmarshal([]byte(out), &plan) != nil || !plan.OK || plan.Status != "planned" || plan.PlanID == "" || len(plan.Actions) == 0 || len(plan.Actions) > 200 {
		return installPluginToolResponse{}, false
	}
	for _, action := range plan.Actions {
		if action.Kind != "skill" || action.Action != "copy_skill" || action.Name == "" || action.Target == "" {
			return installPluginToolResponse{}, false
		}
	}
	return plan, true
}

func (b *bridgeServer) planSkillInstall(w http.ResponseWriter, r *http.Request) {
	var input previewSkillInstallRequest
	if err := decodeJSONBody(w, r, 64<<10, &input); err != nil || input.PlanID != "" || input.AcceptRisk || !normalizePreviewSkillInstall(&input) {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "select a local skill or public GitHub repository and a valid scope")
		return
	}
	plan, ok := planPreviewSkill(r, input)
	if !ok {
		writeProtocolError(w, http.StatusBadRequest, "plan_failed", "skill source could not be reviewed for copying")
		return
	}
	view := previewSkillInstallPlan{ProtocolVersion: desktopbridge.ProtocolVersion, PlanID: plan.PlanID, Actions: []previewSkillInstallAction{}, WarningCount: len(plan.Warnings), Warnings: previewPlanWarnings(plan.Warnings)}
	for _, action := range plan.Actions {
		view.Actions = append(view.Actions, previewSkillInstallAction{Name: action.Name, Target: action.Target, RiskLevel: action.RiskLevel})
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) installSkill(w http.ResponseWriter, r *http.Request) {
	var input previewSkillInstallRequest
	if err := decodeJSONBody(w, r, 64<<10, &input); err != nil || !normalizePreviewSkillInstall(&input) || !strings.HasPrefix(input.PlanID, "sha256:") {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "review the skill plan before installing")
		return
	}
	b.packageOpsMu.Lock()
	defer b.packageOpsMu.Unlock()
	plan, ok := planPreviewSkill(r, input)
	if !ok || plan.PlanID != input.PlanID {
		writeProtocolError(w, http.StatusConflict, "plan_changed", "skill plan changed; review it again")
		return
	}
	for _, action := range plan.Actions {
		if action.RiskLevel == "high" && !input.AcceptRisk {
			writeProtocolError(w, http.StatusBadRequest, "risk_not_accepted", "confirm the high-risk skill plan before installing")
			return
		}
	}
	tool := installsource.NewTool(installsource.Options{ProjectRoot: input.WorkspaceRoot})
	out, err := tool.Execute(r.Context(), previewSkillToolRequest(input, true))
	if err != nil || len(out) > 2<<20 {
		writeProtocolError(w, http.StatusConflict, "install_failed", "skill installation failed or the reviewed plan changed")
		return
	}
	var result installPluginToolResponse
	if json.Unmarshal([]byte(out), &result) != nil || result.PlanID != input.PlanID || (result.Status != "done" && result.Status != "partial" && result.Status != "failed") {
		writeProtocolError(w, http.StatusConflict, "install_failed", "skill installation did not match the reviewed plan")
		return
	}
	settings, readErr := loadSkillsSettings(input.WorkspaceRoot)
	if readErr != nil {
		writeProtocolError(w, http.StatusInternalServerError, "read_failed", "skill installation finished but inventory could not be read")
		return
	}
	failedNames := []string{}
	for _, action := range result.Actions {
		if action.Status == "failed" {
			failedNames = append(failedNames, action.Name)
		}
	}
	writeJSON(w, http.StatusOK, previewSkillInstallResult{ProtocolVersion: desktopbridge.ProtocolVersion, Status: result.Status, FailedNames: failedNames, Settings: settings})
}
