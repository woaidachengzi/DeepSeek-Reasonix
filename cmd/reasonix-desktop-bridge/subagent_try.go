package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/boot"
	configpkg "reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/permission"
	"reasonix/internal/sandbox"
	"reasonix/internal/tool"
	"reasonix/internal/tool/builtin"
)

const (
	maxSubagentTryTaskBytes   = 16 << 10
	maxSubagentTryPromptBytes = 64 << 10
	maxSubagentTryResultRunes = 128 << 10
)

type previewSubagentTryRequest struct {
	WorkspaceRoot string                      `json:"workspaceRoot"`
	Input         previewSubagentProfileInput `json:"input"`
	Task          string                      `json:"task"`
}

type subagentTryRun struct {
	cancel    context.CancelFunc
	mu        sync.Mutex
	output    strings.Builder
	runes     int
	truncated bool
}

type previewSubagentTryStatus struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Running         bool   `json:"running"`
	Output          string `json:"output"`
}

type previewSubagentTryResponse struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Result          string `json:"result"`
}

type previewSubagentTryCancelResponse struct {
	ProtocolVersion int  `json:"protocolVersion"`
	Cancelled       bool `json:"cancelled"`
}

func (b *bridgeServer) trySubagentProfile(w http.ResponseWriter, r *http.Request) {
	var request previewSubagentTryRequest
	if err := decodeJSONBody(w, r, 128<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid subagent try request")
		return
	}
	task := strings.TrimSpace(request.Task)
	prompt := strings.TrimSpace(request.Input.SystemPrompt)
	if task == "" || len(task) > maxSubagentTryTaskBytes || prompt == "" || len(prompt) > maxSubagentTryPromptBytes {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "task and system prompt are required and must fit the Preview size limits")
		return
	}
	if len(request.Input.AllowedTools) > 128 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "subagent try tool list exceeds the Preview limit")
		return
	}
	for _, name := range request.Input.AllowedTools {
		if strings.TrimSpace(name) == "" || len(name) > 128 {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "subagent try tool names must be between 1 and 128 characters")
			return
		}
	}
	root, err := normalizeSkillsWorkspace(request.WorkspaceRoot)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid subagent workspace")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
	run := &subagentTryRun{cancel: cancel}
	b.subagentTryMu.Lock()
	if b.subagentTryRun != nil {
		b.subagentTryMu.Unlock()
		cancel()
		writeProtocolError(w, http.StatusConflict, "busy", "another subagent try run is still in progress")
		return
	}
	b.subagentTryRun = run
	b.subagentTryMu.Unlock()
	defer func() {
		b.subagentTryMu.Lock()
		if b.subagentTryRun == run {
			b.subagentTryRun = nil
		}
		b.subagentTryMu.Unlock()
		cancel()
	}()

	result, err := runReadOnlySubagentTry(ctx, root, request.Input, task, subagentTryTextSink{run: run})
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			writeProtocolError(w, http.StatusRequestTimeout, "cancelled", "subagent try run was cancelled")
			return
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			writeProtocolError(w, http.StatusGatewayTimeout, "timeout", "subagent try run reached the 3 minute time limit")
			return
		}
		writeProtocolError(w, http.StatusBadRequest, "try_failed", err.Error())
		return
	}
	if len([]rune(result)) > maxSubagentTryResultRunes {
		result = string([]rune(result)[:maxSubagentTryResultRunes]) + "\n… output truncated"
	}
	writeJSON(w, http.StatusOK, previewSubagentTryResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Result: result})
}

func (b *bridgeServer) subagentProfileTryStatus(w http.ResponseWriter, _ *http.Request) {
	b.subagentTryMu.Lock()
	run := b.subagentTryRun
	b.subagentTryMu.Unlock()
	status := previewSubagentTryStatus{ProtocolVersion: desktopbridge.ProtocolVersion, Running: run != nil}
	if run != nil {
		status.Output = run.snapshot()
	}
	writeJSON(w, http.StatusOK, status)
}

type subagentTryTextSink struct{ run *subagentTryRun }

func (s subagentTryTextSink) Emit(e event.Event) {
	if s.run == nil {
		return
	}
	switch e.Kind {
	case event.StreamAttempt:
		if e.StreamAttempt.Action == event.StreamAttemptBegin || e.StreamAttempt.Action == event.StreamAttemptDiscard {
			s.run.reset()
		}
	case event.Text:
		if e.Text != "" {
			s.run.append(e.Text)
		}
	case event.Message:
		s.run.replace(e.Text)
	}
}

func (r *subagentTryRun) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.output.Reset()
	r.runes = 0
	r.truncated = false
}

func (r *subagentTryRun) replace(text string) {
	r.reset()
	r.append(text)
}

func (r *subagentTryRun) append(text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	remaining := maxSubagentTryResultRunes - r.runes
	if remaining <= 0 {
		r.truncated = true
		return
	}
	runes := []rune(text)
	if len(runes) > remaining {
		runes = runes[:remaining]
		r.truncated = true
	}
	_, _ = r.output.WriteString(string(runes))
	r.runes += len(runes)
}

func (r *subagentTryRun) snapshot() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	output := r.output.String()
	if r.truncated {
		output += "\n… output truncated"
	}
	return output
}

func (b *bridgeServer) cancelSubagentProfileTry(w http.ResponseWriter, _ *http.Request) {
	b.subagentTryMu.Lock()
	run := b.subagentTryRun
	b.subagentTryMu.Unlock()
	if run != nil {
		run.cancel()
	}
	writeJSON(w, http.StatusOK, previewSubagentTryCancelResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Cancelled: run != nil})
}

func runReadOnlySubagentTry(ctx context.Context, root string, input previewSubagentProfileInput, task string, sink event.Sink) (string, error) {
	task = strings.TrimSpace(task)
	prompt := strings.TrimSpace(input.SystemPrompt)
	if task == "" {
		return "", fmt.Errorf("task is required")
	}
	if prompt == "" {
		return "", fmt.Errorf("system prompt is required")
	}
	cfg, err := configpkg.LoadForRootReadOnly(root)
	if err != nil {
		return "", err
	}
	modelRef := strings.TrimSpace(input.Model)
	if modelRef == "" {
		modelRef = strings.TrimSpace(cfg.Agent.SubagentModel)
	}
	if modelRef == "" {
		modelRef = cfg.DefaultModel
	}
	entry, ok := cfg.ResolveModel(modelRef)
	if !ok {
		return "", fmt.Errorf("unknown model %q", modelRef)
	}
	model := *entry
	if effort := strings.TrimSpace(input.Effort); effort != "" {
		normalized, err := configpkg.NormalizeEffort(&model, effort)
		if err != nil {
			return "", err
		}
		model.Effort = normalized
		if model.Kind == "anthropic" && model.Effort != "" && strings.TrimSpace(model.Thinking) == "" {
			model.Thinking = "adaptive"
		}
	}
	provider, err := boot.NewProviderWithProxy(&model, cfg.NetworkProxySpec())
	if err != nil {
		return "", err
	}
	registry := readOnlySubagentTryToolRegistry(cfg, root, input.AllowedTools)
	policy := permission.New(cfg.Permissions.Mode, cfg.Permissions.Allow, cfg.Permissions.Ask, cfg.Permissions.Deny).
		WithAllowDynamicBashFallback(cfg.Permissions.AllowDynamicBash)
	return agent.RunReadOnlySubAgentWithSession(ctx, provider, registry, agent.NewSession(prompt), task, agent.Options{
		MaxSteps:      12,
		Temperature:   cfg.Agent.Temperature,
		Pricing:       model.Price,
		ContextWindow: model.ContextWindow,
		Gate:          control.BuildHeadlessApprovalGate(policy, control.ToolApprovalAsk),
	}, sink)
}

func readOnlySubagentTryToolRegistry(cfg *configpkg.Config, root string, allowedTools []string) *tool.Registry {
	writeRoots := cfg.WriteRootsForRoot(root)
	forbidReadRoots := boot.RuntimeForbidReadRoots(cfg, root)
	workspace := builtin.Workspace{
		Dir:             root,
		WriteRoots:      writeRoots,
		ForbidReadRoots: forbidReadRoots,
		Bash: sandbox.Spec{
			Mode: cfg.BashMode(), WriteRoots: writeRoots,
			ForbidReadRoots: forbidReadRoots, Network: cfg.Sandbox.Network,
		},
		BashTimeout:   time.Duration(cfg.BashTimeoutSeconds()) * time.Second,
		Search:        builtin.ResolveSearch(cfg.Tools.Search.Engine, cfg.Tools.Search.RgPath, io.Discard),
		ProxySpec:     cfg.NetworkProxySpec(),
		ReadPaths:     builtin.NewPathResolver(),
		SessionGuard:  builtin.NewSessionDataGuard(configpkg.MemoryUserDir(), cfg.AllowWriteRoots()),
		ManagedConfig: builtin.NewManagedConfigPaths(configpkg.ReasonixManagedConfigPaths()),
	}
	parent := tool.NewRegistry()
	for _, candidate := range workspace.Tools() {
		parent.Add(candidate)
	}
	return agent.ReadOnlySubagentToolRegistry(parent, allowedTools)
}
