package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"reasonix/internal/bot"
	"reasonix/internal/botruntime"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

// previewBotRuntime runs the configured IM adapters inside the Preview
// sidecar. Desktop binds only this sidecar's published RuntimeManager/tunnels,
// never stable/Wails profiles or reconstructed saved sessions.
type previewBotRuntime struct {
	bindingFactory func(*appconfig.Config, map[bot.Platform]bool, *slog.Logger) []bot.AdapterBinding
	lifecycleMu    sync.Mutex
	mu             sync.Mutex
	cancel         context.CancelFunc
	gateway        *bot.BotGateway
	status         botRuntimeStatusView
	closed         bool
	parent         context.Context
	pending        int
	desktopManager *desktopbridge.RuntimeManager
	desktopRemotes *previewRemoteSessions
	desktopStream  *desktopbridge.OwnedEventStream
	watch          *previewDesktopWatchStore
	desktop        *previewDesktopHost
	cleanupErr     error // lifecycleMu; unknown cleanup must not auto-start a new generation
}

// Configure once before refresh; callers supply actual sidecar-owned resources.
func (r *previewBotRuntime) configureDesktop(manager *desktopbridge.RuntimeManager, remotes *previewRemoteSessions, stream *desktopbridge.OwnedEventStream) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.closed || r.desktopStream != nil {
		return
	}
	r.desktopManager, r.desktopRemotes, r.desktopStream = manager, remotes, stream
}

type botRuntimeStatusView struct {
	ProtocolVersion        int                         `json:"protocolVersion"`
	Running                bool                        `json:"running"`
	Status                 string                      `json:"status"`
	Message                string                      `json:"message"`
	Connections            int                         `json:"connections"`
	StartedAt              string                      `json:"startedAt"`
	Platforms              map[string]string           `json:"platforms,omitempty"`
	AdapterHealth          []bot.AdapterHealthSnapshot `json:"adapterHealth,omitempty"`
	DesktopBridgeAvailable bool                        `json:"desktopBridgeAvailable"`
	Refreshing             bool                        `json:"refreshing"`
}

func newPreviewBotRuntime() *previewBotRuntime {
	return &previewBotRuntime{status: botRuntimeStatusView{
		ProtocolVersion:        1,
		Status:                 "stopped",
		Message:                "Preview bot is disabled",
		DesktopBridgeAvailable: false,
	}}
}

func (r *previewBotRuntime) refreshAsync(parent context.Context) {
	r.mu.Lock()
	if parent != nil {
		r.parent = parent
	}
	if r.parent == nil {
		r.parent = context.Background()
	}
	parent = r.parent
	r.pending++
	r.mu.Unlock()
	go func() {
		defer func() { r.mu.Lock(); r.pending--; r.mu.Unlock() }()
		if err := r.refresh(parent); err != nil {
			slog.Warn("Preview bot runtime refresh failed")
		}
	}()
}

func (r *previewBotRuntime) refresh(parent context.Context) error {
	if parent == nil {
		parent = context.Background()
	}
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.closed {
		return nil
	}
	path := appconfig.UserConfigPath()
	cfg, err := appconfig.LoadForEditReadOnlyStrict(path)
	if err != nil {
		r.stopCurrent()
		r.setStatus(botRuntimeStatusView{
			ProtocolVersion: 1, Status: "error", Message: "Preview bot configuration is unavailable",
			DesktopBridgeAvailable: false,
		})
		return err
	}
	plan := previewBotRuntimePlan(cfg)
	r.stopCurrent()
	if !plan.start {
		r.setStatus(botRuntimeStatusView{
			ProtocolVersion:        1,
			Status:                 plan.status,
			Message:                plan.message,
			DesktopBridgeAvailable: false,
		})
		return nil
	}
	if r.cleanupErr != nil {
		r.setStatus(botRuntimeStatusView{ProtocolVersion: 1, Status: "blocked", Message: "Desktop takeover cleanup is unconfirmed; inspect the original owner before restarting"})
		return errPreviewDesktopBinding
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, cancel := context.WithCancel(parent)
	channels := botruntime.ChannelConfigs(cfg.Bot.Connections, true, true)
	connectionChannels := botruntime.ConnectionChannelConfigs(cfg.Bot.Connections, true, true)
	channels, connectionChannels = previewBotLegacyChannels(cfg, channels, connectionChannels)
	gwCfg := bot.GatewayConfig{
		Model:              botruntime.ModelName(cfg, ""),
		ToolApprovalMode:   cfg.Bot.ToolApprovalMode,
		MaxSteps:           cfg.Bot.MaxSteps,
		QueueMode:          cfg.Bot.QueueMode,
		QueueCap:           cfg.Bot.QueueCap,
		QueueDrop:          cfg.Bot.QueueDrop,
		PairingEnabled:     cfg.Bot.Pairing.Enabled,
		PairingTTL:         time.Duration(cfg.Bot.Pairing.RequestTTLMinutes) * time.Minute,
		PairingMaxPending:  cfg.Bot.Pairing.MaxPendingPerPlatform,
		IgnoreSelfMessages: cfg.Bot.IgnoreSelfMessages,
		SelfUserIDs: map[bot.Platform][]string{
			bot.PlatformQQ: cfg.Bot.SelfUserIDs.QQ, bot.PlatformFeishu: cfg.Bot.SelfUserIDs.Feishu,
			bot.PlatformWeixin: cfg.Bot.SelfUserIDs.Weixin, bot.PlatformDingtalk: cfg.Bot.SelfUserIDs.Dingtalk,
		},
		ControlEnabled:     cfg.Bot.Control.Enabled,
		ControlAddr:        cfg.Bot.Control.Addr,
		ControlToken:       appconfig.ResolveCredentialForRootGlobalFirst(".", strings.TrimSpace(cfg.Bot.Control.TokenEnv)).Value,
		Channels:           channels,
		ConnectionChannels: connectionChannels,
		Routes:             botruntime.RouteConfigs(cfg.Bot.Routes, true, true),
		ConnectionAccess:   botruntime.ConnectionAccessConfigs(cfg),
		Enabled:            plan.enabled,
		Allowlist: bot.AllowlistConfig{
			Enabled: cfg.Bot.Allowlist.Enabled, AllowAll: cfg.Bot.Allowlist.AllowAll,
			Users: map[bot.Platform][]string{
				bot.PlatformQQ: cfg.Bot.Allowlist.QQUsers, bot.PlatformFeishu: cfg.Bot.Allowlist.FeishuUsers,
				bot.PlatformWeixin: cfg.Bot.Allowlist.WeixinUsers, bot.PlatformDingtalk: cfg.Bot.Allowlist.DingtalkUsers,
			},
			Approvers: map[bot.Platform][]string{
				bot.PlatformQQ: cfg.Bot.Allowlist.QQApprovers, bot.PlatformFeishu: cfg.Bot.Allowlist.FeishuApprovers,
				bot.PlatformWeixin: cfg.Bot.Allowlist.WeixinApprovers, bot.PlatformDingtalk: cfg.Bot.Allowlist.DingtalkApprovers,
			},
			Admins: map[bot.Platform][]string{
				bot.PlatformQQ: cfg.Bot.Allowlist.QQAdmins, bot.PlatformFeishu: cfg.Bot.Allowlist.FeishuAdmins,
				bot.PlatformWeixin: cfg.Bot.Allowlist.WeixinAdmins, bot.PlatformDingtalk: cfg.Bot.Allowlist.DingtalkAdmins,
			},
			Groups: map[bot.Platform][]string{
				bot.PlatformQQ: cfg.Bot.Allowlist.QQGroups, bot.PlatformFeishu: cfg.Bot.Allowlist.FeishuGroups,
				bot.PlatformWeixin: cfg.Bot.Allowlist.WeixinGroups, bot.PlatformDingtalk: cfg.Bot.Allowlist.DingtalkGroups,
			},
		},
		Debounce:       time.Duration(cfg.Bot.DebounceMs) * time.Millisecond,
		ModelResolver:  botruntime.ModelResolver(cfg),
		OnInbound:      botruntime.NewRemoteRememberer(logger),
		OnSessionReady: botruntime.NewSessionRemembererWithWorkspace(logger, ""),
	}
	bindings := []bot.AdapterBinding(nil)
	if r.bindingFactory != nil {
		bindings = r.bindingFactory(cfg, plan.enabled, logger)
	} else {
		bindings = botruntime.AdapterBindings(cfg, plan.enabled, nil, logger)
	}
	if len(bindings) == 0 {
		cancel()
		r.setStatus(botRuntimeStatusView{ProtocolVersion: 1, Status: "stopped", Message: "No bot adapters are configured", DesktopBridgeAvailable: false})
		return nil
	}
	var desktop *previewDesktopHost
	if r.desktopStream != nil {
		if r.watch == nil {
			r.watch, err = newPreviewDesktopWatchStore(path)
		}
		if err == nil {
			desktop, err = newPreviewDesktopHost(ctx, r.desktopManager, r.desktopRemotes, r.desktopStream, r.watch)
		}
		if err != nil {
			cancel()
			r.setStatus(botRuntimeStatusView{ProtocolVersion: 1, Status: "error", Message: "Preview desktop host is unavailable; refresh the current owners before restarting"})
			return errPreviewDesktopBinding
		}
		gwCfg.Desktop = desktop
	}
	gateway := bot.NewGatewayWithAdapterBindings(gwCfg, bindings, logger)
	if err := gateway.Start(ctx); err != nil {
		cancel()
		gateway.Stop()
		if desktop != nil {
			_ = desktop.Shutdown(context.Background())
		}
		r.setStatus(botRuntimeStatusView{ProtocolVersion: 1, Status: "error", Message: "Bot adapters could not start", DesktopBridgeAvailable: false})
		return err
	}
	if desktop != nil {
		if err := desktop.Start(ctx, gateway); err != nil {
			desktop.StopIngress()
			cancel()
			gateway.Stop()
			r.cleanupErr = desktop.Shutdown(context.Background())
			r.setStatus(botRuntimeStatusView{ProtocolVersion: 1, Status: "error", Message: "Preview desktop observation could not start; refresh the current owners before restarting"})
			return errPreviewDesktopBinding
		}
	}
	health := gateway.AdapterHealth()
	status := "running"
	message := fmt.Sprintf("%d bot connection(s) running", gateway.AdapterCount())
	if len(gateway.StartErrors()) > 0 {
		status = "degraded"
		message = fmt.Sprintf("%d bot connection(s) running; %d failed to start", gateway.AdapterCount(), len(gateway.StartErrors()))
	}
	r.mu.Lock()
	r.cancel = cancel
	r.gateway = gateway
	r.desktop = desktop
	r.status = botRuntimeStatusView{
		ProtocolVersion: 1, Running: true, Status: status, Message: message,
		Connections: gateway.AdapterCount(), StartedAt: time.Now().UTC().Format(time.RFC3339),
		AdapterHealth: health, DesktopBridgeAvailable: desktop != nil && desktop.Available(),
	}
	r.mu.Unlock()
	if desktop != nil {
		go r.watchDesktopGeneration(desktop, gateway)
	}
	return nil
}

func (r *previewBotRuntime) watchDesktopGeneration(host *previewDesktopHost, gateway *bot.BotGateway) {
	<-host.notifications.Done()
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	r.mu.Lock()
	current := r.desktop == host && r.gateway == gateway
	r.mu.Unlock()
	if !current {
		return
	} // A retired generation cannot stop its replacement.
	r.stopCurrent()
	message := "Preview desktop observation ended; refresh current owners and restart the bot"
	status := "error"
	if r.cleanupErr != nil {
		status = "blocked"
		message = "Desktop takeover cleanup is unconfirmed; inspect the original owner before restarting"
	}
	r.setStatus(botRuntimeStatusView{ProtocolVersion: 1, Status: status, Message: message})
}

type previewBotPlan struct {
	start   bool
	status  string
	message string
	enabled map[bot.Platform]bool
}

func previewBotRuntimePlan(cfg *appconfig.Config) previewBotPlan {
	if cfg == nil {
		return previewBotPlan{status: "error", message: "Preview bot configuration is unavailable"}
	}
	if !cfg.Bot.Enabled {
		return previewBotPlan{status: "stopped", message: "Preview bot is disabled"}
	}
	if !botruntime.BotConfigHasAccessControl(cfg.Bot) {
		return previewBotPlan{status: "blocked", message: "Bot access control is required before starting"}
	}
	enabled, unknown := botruntime.EnabledPlatforms(cfg, nil)
	if len(unknown) > 0 {
		return previewBotPlan{status: "error", message: "Unknown bot channel configured"}
	}
	if !botruntime.HasEnabledPlatform(enabled) {
		return previewBotPlan{status: "stopped", message: "No bot channels are enabled"}
	}
	return previewBotPlan{start: true, status: "running", message: "Bot runtime can start", enabled: enabled}
}

func previewBotLegacyChannels(cfg *appconfig.Config, channels map[bot.Platform]bot.ChannelConfig, connectionChannels map[string]bot.ChannelConfig) (map[bot.Platform]bot.ChannelConfig, map[string]bot.ChannelConfig) {
	if cfg == nil {
		return channels, connectionChannels
	}
	for platform, channel := range map[bot.Platform]bot.ChannelConfig{
		bot.PlatformQQ:       {Model: strings.TrimSpace(cfg.Bot.QQ.Model), ToolApprovalMode: cfg.Bot.QQ.ToolApprovalMode, WorkspaceRoot: cfg.Bot.QQ.WorkspaceRoot},
		bot.PlatformDingtalk: {Model: strings.TrimSpace(cfg.Bot.Dingtalk.Model), ToolApprovalMode: cfg.Bot.Dingtalk.ToolApprovalMode, WorkspaceRoot: cfg.Bot.Dingtalk.WorkspaceRoot, SessionMappings: botruntime.SessionMappings(cfg.Bot.Dingtalk.SessionMappings)},
	} {
		if channel.Model == "" && channel.ToolApprovalMode == "" && channel.WorkspaceRoot == "" && len(channel.SessionMappings) == 0 {
			continue
		}
		if channels == nil {
			channels = make(map[bot.Platform]bot.ChannelConfig)
		}
		if _, exists := channels[platform]; !exists {
			channels[platform] = channel
		}
		if connectionChannels == nil {
			connectionChannels = make(map[string]bot.ChannelConfig)
		}
		if _, exists := connectionChannels[string(platform)]; !exists {
			connectionChannels[string(platform)] = channel
		}
	}
	return channels, connectionChannels
}

func (r *previewBotRuntime) stopCurrent() {
	r.mu.Lock()
	cancel, gateway, desktop := r.cancel, r.gateway, r.desktop
	r.cancel, r.gateway, r.desktop = nil, nil, nil
	r.mu.Unlock()
	if desktop != nil {
		desktop.StopIngress()
	}
	if cancel != nil {
		cancel()
	}
	if gateway != nil {
		gateway.Stop()
	}
	if desktop != nil {
		if err := desktop.Shutdown(context.Background()); err != nil {
			r.cleanupErr = errPreviewDesktopBinding
		}
	}
}

func (r *previewBotRuntime) stop() {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	r.closed = true
	r.stopCurrent()
	if r.watch != nil {
		r.watch.Close()
	}
	message := "Preview bot runtime stopped"
	if r.cleanupErr != nil {
		message = "Preview bot runtime stopped; desktop takeover cleanup is unconfirmed, inspect the original owner"
	}
	r.setStatus(botRuntimeStatusView{ProtocolVersion: 1, Status: "stopped", Message: message, DesktopBridgeAvailable: false})
}

func (r *previewBotRuntime) setStatus(status botRuntimeStatusView) {
	r.mu.Lock()
	r.status = status
	r.mu.Unlock()
}

func (r *previewBotRuntime) snapshot() botRuntimeStatusView {
	r.mu.Lock()
	defer r.mu.Unlock()
	status := r.status
	if r.gateway != nil {
		status.AdapterHealth = r.gateway.AdapterHealth()
		status.Connections = r.gateway.AdapterCount()
	}
	status.DesktopBridgeAvailable = r.desktop != nil && r.desktop.Available()
	status.Refreshing = r.pending > 0
	status.AdapterHealth = append([]bot.AdapterHealthSnapshot(nil), status.AdapterHealth...)
	for i := range status.AdapterHealth {
		if status.AdapterHealth[i].LastError != "" {
			status.AdapterHealth[i].LastError = "Connection failed; check credentials and retry."
		}
	}
	return status
}

func (b *bridgeServer) botRuntimeStatus(w http.ResponseWriter, _ *http.Request) {
	if b.botRuntime == nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "Preview bot runtime is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, b.botRuntime.snapshot())
}

type botRuntimeActionRequest struct {
	Action string `json:"action"`
}

func (b *bridgeServer) changeBotRuntime(w http.ResponseWriter, r *http.Request) {
	var input botRuntimeActionRequest
	if err := decodeJSONBody(w, r, 1024, &input); err != nil || input.Action != "restart" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid bot runtime action")
		return
	}
	b.botRuntime.refreshAsync(nil)
	writeJSON(w, http.StatusAccepted, b.botRuntime.snapshot())
}
