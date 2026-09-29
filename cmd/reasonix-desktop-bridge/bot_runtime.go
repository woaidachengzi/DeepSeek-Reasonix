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
)

// previewBotRuntime runs the configured IM adapters inside the Preview
// sidecar. Desktop is deliberately nil until the sidecar can bridge bot
// commands to all Preview sessions; /desktop commands must not target a
// different (stable/Wails) profile.
type previewBotRuntime struct {
	lifecycleMu sync.Mutex
	mu          sync.Mutex
	cancel      context.CancelFunc
	gateway     *bot.BotGateway
	status      botRuntimeStatusView
	closed      bool
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
	go func() {
		if err := r.refresh(parent); err != nil {
			slog.Warn("Preview bot runtime refresh failed", "err", err)
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
		ControlToken:       os.Getenv(strings.TrimSpace(cfg.Bot.Control.TokenEnv)),
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
		// Desktop remains nil: this Preview sidecar cannot safely expose Wails
		// sessions or their approval/watch state.
	}
	bindings := botruntime.AdapterBindings(cfg, plan.enabled, nil, logger)
	if len(bindings) == 0 {
		cancel()
		r.setStatus(botRuntimeStatusView{ProtocolVersion: 1, Status: "stopped", Message: "No bot adapters are configured", DesktopBridgeAvailable: false})
		return nil
	}
	gateway := bot.NewGatewayWithAdapterBindings(gwCfg, bindings, logger)
	if err := gateway.Start(ctx); err != nil {
		cancel()
		gateway.Stop()
		r.setStatus(botRuntimeStatusView{ProtocolVersion: 1, Status: "error", Message: "Bot adapters could not start", DesktopBridgeAvailable: false})
		return err
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
	r.status = botRuntimeStatusView{
		ProtocolVersion: 1, Running: true, Status: status, Message: message,
		Connections: gateway.AdapterCount(), StartedAt: time.Now().UTC().Format(time.RFC3339),
		AdapterHealth: health, DesktopBridgeAvailable: false,
	}
	r.mu.Unlock()
	return nil
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
	cancel, gateway := r.cancel, r.gateway
	r.cancel, r.gateway = nil, nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if gateway != nil {
		gateway.Stop()
	}
}

func (r *previewBotRuntime) stop() {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	r.closed = true
	r.stopCurrent()
	r.setStatus(botRuntimeStatusView{ProtocolVersion: 1, Status: "stopped", Message: "Preview bot runtime stopped", DesktopBridgeAvailable: false})
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
	return status
}

func (b *bridgeServer) botRuntimeStatus(w http.ResponseWriter, _ *http.Request) {
	if b.botRuntime == nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "Preview bot runtime is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, b.botRuntime.snapshot())
}
