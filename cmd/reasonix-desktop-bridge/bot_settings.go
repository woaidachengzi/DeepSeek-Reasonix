package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"reasonix/internal/botruntime"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

type botSettingsView struct {
	ProtocolVersion          int                      `json:"protocolVersion"`
	ConfigPath               string                   `json:"configPath"`
	Enabled                  bool                     `json:"enabled"`
	MaxSteps                 int                      `json:"maxSteps"`
	DebounceMs               int                      `json:"debounceMs"`
	QueueMode                string                   `json:"queueMode"`
	QueueCap                 int                      `json:"queueCap"`
	QueueDrop                string                   `json:"queueDrop"`
	IgnoreSelfMessages       bool                     `json:"ignoreSelfMessages"`
	SelfUserIDs              map[string][]string      `json:"selfUserIds"`
	AccessControlConfigured  bool                     `json:"accessControlConfigured"`
	PairingEnabled           bool                     `json:"pairingEnabled"`
	PairingRequestTTLMinutes int                      `json:"pairingRequestTtlMinutes"`
	PairingMaxPending        int                      `json:"pairingMaxPendingPerPlatform"`
	AllowlistEnabled         bool                     `json:"allowlistEnabled"`
	AllowAll                 bool                     `json:"allowAll"`
	Allowlist                map[string]botAccessList `json:"allowlist"`
	Channels                 []botChannelSettings     `json:"channels"`
	Routes                   []botRouteSettings       `json:"routes"`
}

type botRouteSettings struct {
	ConnectionID     string `json:"connectionId"`
	Platform         string `json:"platform"`
	ChatType         string `json:"chatType"`
	ChatID           string `json:"chatId"`
	UserID           string `json:"userId"`
	ThreadID         string `json:"threadId"`
	Model            string `json:"model"`
	ToolApprovalMode string `json:"toolApprovalMode"`
	WorkspaceRoot    string `json:"workspaceRoot"`
}

type botAccessList struct {
	Users     []string `json:"users"`
	Groups    []string `json:"groups"`
	Approvers []string `json:"approvers"`
	Admins    []string `json:"admins"`
}

type botConnectionAccessView struct {
	Enabled        bool     `json:"enabled"`
	AllowAll       bool     `json:"allowAll"`
	PairingEnabled bool     `json:"pairingEnabled"`
	Users          []string `json:"users"`
	Groups         []string `json:"groups"`
	Approvers      []string `json:"approvers"`
	Admins         []string `json:"admins"`
}

type botChannelSettings struct {
	ID                 string                   `json:"id"`
	Platform           string                   `json:"platform"`
	Domain             string                   `json:"domain,omitempty"`
	Label              string                   `json:"label"`
	Enabled            bool                     `json:"enabled"`
	Status             string                   `json:"status"`
	CredentialsSet     bool                     `json:"credentialsSet"`
	CredentialMissing  bool                     `json:"credentialMissing"`
	CredentialIdentity string                   `json:"credentialIdentity,omitempty"`
	Model              string                   `json:"model,omitempty"`
	ToolApprovalMode   string                   `json:"toolApprovalMode,omitempty"`
	WorkspaceRoot      string                   `json:"workspaceRoot,omitempty"`
	RuntimeSettings    bool                     `json:"runtimeSettings"`
	Access             *botConnectionAccessView `json:"access,omitempty"`
}

type botConnectionInput struct {
	ID       string `json:"id"`
	Platform string `json:"platform"`
	Domain   string `json:"domain"`
	Label    string `json:"label"`
	Identity string `json:"identity"`
	Secret   string `json:"secret"`
}

type botSettingsChange struct {
	Connection               *botConnectionInput `json:"connection,omitempty"`
	Action                   string              `json:"action"`
	Enabled                  bool                `json:"enabled"`
	ChannelID                string              `json:"channelId,omitempty"`
	Platform                 string              `json:"platform,omitempty"`
	List                     string              `json:"list,omitempty"`
	Mode                     string              `json:"mode,omitempty"`
	Values                   []string            `json:"values,omitempty"`
	Identity                 string              `json:"identity,omitempty"`
	Secret                   string              `json:"secret,omitempty"`
	Model                    *string             `json:"model,omitempty"`
	ToolApprovalMode         *string             `json:"toolApprovalMode,omitempty"`
	WorkspaceRoot            *string             `json:"workspaceRoot,omitempty"`
	MaxSteps                 *int                `json:"maxSteps,omitempty"`
	DebounceMs               *int                `json:"debounceMs,omitempty"`
	QueueMode                *string             `json:"queueMode,omitempty"`
	QueueCap                 *int                `json:"queueCap,omitempty"`
	QueueDrop                *string             `json:"queueDrop,omitempty"`
	IgnoreSelfMessages       *bool               `json:"ignoreSelfMessages,omitempty"`
	PairingRequestTTLMinutes *int                `json:"pairingRequestTtlMinutes,omitempty"`
	PairingMaxPending        *int                `json:"pairingMaxPendingPerPlatform,omitempty"`
	Routes                   []botRouteSettings  `json:"routes,omitempty"`
}

func readBotSettings() (botSettingsView, error) {
	cfg, err := appconfig.LoadForEditReadOnlyStrict(appconfig.UserConfigPath())
	if err != nil {
		return botSettingsView{}, err
	}
	view := botSettingsView{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		ConfigPath:      appconfig.UserConfigPath(), Enabled: cfg.Bot.Enabled,
		MaxSteps: cfg.Bot.MaxSteps, DebounceMs: cfg.Bot.DebounceMs, QueueMode: cfg.Bot.QueueMode,
		QueueCap: cfg.Bot.QueueCap, QueueDrop: cfg.Bot.QueueDrop, IgnoreSelfMessages: cfg.Bot.IgnoreSelfMessages,
		SelfUserIDs: map[string][]string{
			"qq": botStringList(cfg.Bot.SelfUserIDs.QQ), "feishu": botStringList(cfg.Bot.SelfUserIDs.Feishu),
			"weixin": botStringList(cfg.Bot.SelfUserIDs.Weixin), "dingtalk": botStringList(cfg.Bot.SelfUserIDs.Dingtalk),
		},
		AccessControlConfigured:  botruntime.BotConfigHasAccessControl(cfg.Bot),
		PairingEnabled:           cfg.Bot.Pairing.Enabled,
		PairingRequestTTLMinutes: cfg.Bot.Pairing.RequestTTLMinutes,
		PairingMaxPending:        cfg.Bot.Pairing.MaxPendingPerPlatform,
		AllowlistEnabled:         cfg.Bot.Allowlist.Enabled,
		AllowAll:                 cfg.Bot.Allowlist.AllowAll,
		Allowlist: map[string]botAccessList{
			"qq":       {Users: botStringList(cfg.Bot.Allowlist.QQUsers), Groups: botStringList(cfg.Bot.Allowlist.QQGroups), Approvers: botStringList(cfg.Bot.Allowlist.QQApprovers), Admins: botStringList(cfg.Bot.Allowlist.QQAdmins)},
			"feishu":   {Users: botStringList(cfg.Bot.Allowlist.FeishuUsers), Groups: botStringList(cfg.Bot.Allowlist.FeishuGroups), Approvers: botStringList(cfg.Bot.Allowlist.FeishuApprovers), Admins: botStringList(cfg.Bot.Allowlist.FeishuAdmins)},
			"weixin":   {Users: botStringList(cfg.Bot.Allowlist.WeixinUsers), Groups: botStringList(cfg.Bot.Allowlist.WeixinGroups), Approvers: botStringList(cfg.Bot.Allowlist.WeixinApprovers), Admins: botStringList(cfg.Bot.Allowlist.WeixinAdmins)},
			"dingtalk": {Users: botStringList(cfg.Bot.Allowlist.DingtalkUsers), Groups: botStringList(cfg.Bot.Allowlist.DingtalkGroups), Approvers: botStringList(cfg.Bot.Allowlist.DingtalkApprovers), Admins: botStringList(cfg.Bot.Allowlist.DingtalkAdmins)},
		},
		Channels: make([]botChannelSettings, 0, 4+len(cfg.Bot.Connections)),
	}
	view.Channels = append(view.Channels,
		botChannelSettings{ID: "legacy:qq", Platform: "qq", Label: "QQ", Enabled: cfg.Bot.QQ.Enabled, CredentialIdentity: cfg.Bot.QQ.AppID, CredentialsSet: credentialIsSet(cfg.Bot.QQ.AppSecretEnv), CredentialMissing: strings.TrimSpace(cfg.Bot.QQ.AppID) == "" || !credentialIsSet(cfg.Bot.QQ.AppSecretEnv), Model: cfg.Bot.QQ.Model, ToolApprovalMode: cfg.Bot.QQ.ToolApprovalMode, WorkspaceRoot: cfg.Bot.QQ.WorkspaceRoot, RuntimeSettings: true, Access: botConnectionAccess(cfg.Bot.QQ.Access)},
		botChannelSettings{ID: "legacy:feishu", Platform: "feishu", Domain: normalizedBotDomain(cfg.Bot.Feishu.Domain), Label: "Feishu", Enabled: cfg.Bot.Feishu.Enabled, CredentialIdentity: cfg.Bot.Feishu.AppID, CredentialsSet: credentialIsSet(cfg.Bot.Feishu.AppSecretEnv), CredentialMissing: strings.TrimSpace(cfg.Bot.Feishu.AppID) == "" || !credentialIsSet(cfg.Bot.Feishu.AppSecretEnv)},
		botChannelSettings{ID: "legacy:weixin", Platform: "weixin", Label: "WeChat", Enabled: cfg.Bot.Weixin.Enabled, CredentialIdentity: cfg.Bot.Weixin.AccountID, CredentialsSet: credentialIsSet(cfg.Bot.Weixin.TokenEnv), CredentialMissing: strings.TrimSpace(cfg.Bot.Weixin.AccountID) == "" || !credentialIsSet(cfg.Bot.Weixin.TokenEnv)},
		botChannelSettings{ID: "legacy:dingtalk", Platform: "dingtalk", Label: "DingTalk", Enabled: cfg.Bot.Dingtalk.Enabled, CredentialIdentity: cfg.Bot.Dingtalk.ClientID, CredentialsSet: credentialIsSet(cfg.Bot.Dingtalk.SecretEnv) || strings.TrimSpace(cfg.Bot.Dingtalk.ClientSecret) != "", CredentialMissing: strings.TrimSpace(cfg.Bot.Dingtalk.ClientID) == "" || (strings.TrimSpace(cfg.Bot.Dingtalk.SecretEnv) == "" && strings.TrimSpace(cfg.Bot.Dingtalk.ClientSecret) == ""), Model: cfg.Bot.Dingtalk.Model, ToolApprovalMode: cfg.Bot.Dingtalk.ToolApprovalMode, WorkspaceRoot: cfg.Bot.Dingtalk.WorkspaceRoot, RuntimeSettings: true, Access: botConnectionAccess(cfg.Bot.Dingtalk.Access)},
	)
	for _, connection := range cfg.Bot.Connections {
		id := strings.TrimSpace(connection.ID)
		if id == "" {
			id = botruntime.ConnectionRuntimeID(connection)
		}
		credentialSet := credentialIsSet(connection.Credential.AppSecretEnv) || credentialIsSet(connection.Credential.TokenEnv)
		identitySet := strings.TrimSpace(connection.Credential.AppID) != "" || strings.TrimSpace(connection.Credential.AccountID) != ""
		credentialSet = credentialSet && identitySet
		view.Channels = append(view.Channels, botChannelSettings{
			ID: id, Platform: strings.TrimSpace(connection.Provider), Domain: strings.TrimSpace(connection.Domain),
			Label:   firstNonEmpty(strings.TrimSpace(connection.Label), strings.TrimSpace(connection.Domain), strings.TrimSpace(connection.Provider)),
			Enabled: connection.Enabled, Status: strings.TrimSpace(connection.Status), CredentialsSet: credentialSet, Access: botConnectionAccess(connection.Access),
			CredentialMissing: !credentialSet, CredentialIdentity: firstNonEmpty(connection.Credential.AppID, connection.Credential.AccountID),
			Model: connection.Model, ToolApprovalMode: connection.ToolApprovalMode, WorkspaceRoot: connection.WorkspaceRoot, RuntimeSettings: true,
		})
	}
	view.Routes = make([]botRouteSettings, 0, len(cfg.Bot.Routes))
	for _, route := range cfg.Bot.Routes {
		view.Routes = append(view.Routes, botRouteSettings{
			ConnectionID: route.ConnectionID, Platform: route.Platform, ChatType: route.ChatType,
			ChatID: route.ChatID, UserID: route.UserID, ThreadID: route.ThreadID, Model: route.Model,
			ToolApprovalMode: route.ToolApprovalMode, WorkspaceRoot: route.WorkspaceRoot,
		})
	}
	return view, nil
}

func botStringList(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}

func botConnectionAccess(access appconfig.BotAccessConfig) *botConnectionAccessView {
	return &botConnectionAccessView{
		Enabled: access.Enabled, AllowAll: access.AllowAll, PairingEnabled: access.PairingEnabled,
		Users: botStringList(access.Users), Groups: botStringList(access.Groups),
		Approvers: botStringList(access.Approvers), Admins: botStringList(access.Admins),
	}
}

func credentialIsSet(envName string) bool {
	envName = strings.TrimSpace(envName)
	return envName != "" && (strings.TrimSpace(os.Getenv(envName)) != "" || appconfig.ResolveCredentialForRootGlobalFirst(".", envName).Set)
}

func normalizedBotDomain(domain string) string {
	if domain = strings.TrimSpace(domain); domain != "" {
		return domain
	}
	return "feishu"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func changeBotSettings(change botSettingsChange) (botSettingsView, error) {
	change.Action = strings.TrimSpace(change.Action)
	change.ChannelID = strings.TrimSpace(change.ChannelID)
	if change.Action != "create_connection" && change.Action != "remove_connection" && change.Action != "set_enabled" && change.Action != "set_channel_enabled" && change.Action != "set_credentials" && change.Action != "set_pairing" && change.Action != "set_allowlist" && change.Action != "set_allow_all" && change.Action != "set_channel_access_mode" && change.Action != "set_channel_pairing" && change.Action != "set_channel_allowlist" && change.Action != "set_channel_runtime" && change.Action != "set_gateway_runtime" && change.Action != "set_self_user_ids" && change.Action != "set_routes" {
		return botSettingsView{}, fmt.Errorf("invalid bot settings action")
	}
	if change.Action == "create_connection" {
		if change.Connection == nil {
			return botSettingsView{}, fmt.Errorf("missing bot connection")
		}
		input := change.Connection
		input.ID, input.Platform, input.Domain, input.Label, input.Identity = strings.TrimSpace(input.ID), strings.TrimSpace(input.Platform), strings.TrimSpace(input.Domain), strings.TrimSpace(input.Label), strings.TrimSpace(input.Identity)
		if input.ID == "qq" || input.ID == "feishu" || input.ID == "weixin" || input.ID == "dingtalk" || !previewProviderName.MatchString(input.ID) || len(input.ID) > 64 || len(input.Label) > 128 || len(input.Identity) > 512 || input.Identity == "" || len(input.Secret) > 8192 || strings.TrimSpace(input.Secret) == "" || strings.ContainsAny(input.Secret, "\r\n") {
			return botSettingsView{}, fmt.Errorf("invalid bot connection")
		}
		switch input.Platform {
		case "feishu":
			if input.Domain == "" {
				input.Domain = "feishu"
			}
			if input.Domain != "feishu" && input.Domain != "lark" {
				return botSettingsView{}, fmt.Errorf("invalid bot domain")
			}
		case "qq", "weixin":
			if input.Domain == "" {
				input.Domain = input.Platform
			}
			if input.Domain != input.Platform {
				return botSettingsView{}, fmt.Errorf("invalid bot domain")
			}
		default:
			return botSettingsView{}, fmt.Errorf("unsupported bot connection platform")
		}
	}
	if change.Action == "remove_connection" && (!previewProviderName.MatchString(change.ChannelID) || len(change.ChannelID) > 64) {
		return botSettingsView{}, fmt.Errorf("invalid bot connection ID")
	}
	if change.Action == "set_routes" {
		if len(change.Routes) > 200 {
			return botSettingsView{}, fmt.Errorf("too many bot routes")
		}
		for _, route := range change.Routes {
			if !validBotRoute(route) {
				return botSettingsView{}, fmt.Errorf("invalid bot route")
			}
		}
	}
	if change.Action == "set_gateway_runtime" {
		if change.MaxSteps == nil && change.DebounceMs == nil && change.QueueMode == nil && change.QueueCap == nil && change.QueueDrop == nil && change.IgnoreSelfMessages == nil && change.PairingRequestTTLMinutes == nil && change.PairingMaxPending == nil {
			return botSettingsView{}, fmt.Errorf("empty bot runtime settings request")
		}
		if change.MaxSteps != nil && (*change.MaxSteps < 0 || *change.MaxSteps > 100000) ||
			change.DebounceMs != nil && (*change.DebounceMs < 0 || *change.DebounceMs > 60000) ||
			change.QueueCap != nil && (*change.QueueCap < 0 || *change.QueueCap > 10000) ||
			change.PairingRequestTTLMinutes != nil && (*change.PairingRequestTTLMinutes < 0 || *change.PairingRequestTTLMinutes > 525600) ||
			change.PairingMaxPending != nil && (*change.PairingMaxPending < 0 || *change.PairingMaxPending > 10000) {
			return botSettingsView{}, fmt.Errorf("bot runtime setting is outside its supported range")
		}
		if change.QueueMode != nil {
			switch strings.TrimSpace(*change.QueueMode) {
			case "steer", "followup", "collect", "interrupt":
			default:
				return botSettingsView{}, fmt.Errorf("invalid bot queue mode")
			}
		}
		if change.QueueDrop != nil {
			switch strings.TrimSpace(*change.QueueDrop) {
			case "summarize", "old", "new":
			default:
				return botSettingsView{}, fmt.Errorf("invalid bot queue overflow policy")
			}
		}
	}
	if change.Action == "set_self_user_ids" {
		if !validBotAccessPlatform(change.Platform) || len(change.Values) > 100 {
			return botSettingsView{}, fmt.Errorf("invalid bot self user ID request")
		}
		for _, value := range change.Values {
			if value = strings.TrimSpace(value); value == "" || len(value) > 512 {
				return botSettingsView{}, fmt.Errorf("bot self user IDs must be 1–512 characters")
			}
		}
	}
	if change.Action == "set_channel_runtime" {
		if change.ChannelID == "" || change.Model == nil && change.ToolApprovalMode == nil && change.WorkspaceRoot == nil {
			return botSettingsView{}, fmt.Errorf("invalid bot runtime settings request")
		}
		if change.Model != nil && (len(*change.Model) > 512 || strings.ContainsAny(*change.Model, "\r\n\x00")) {
			return botSettingsView{}, fmt.Errorf("invalid bot model")
		}
		if change.ToolApprovalMode != nil {
			switch strings.TrimSpace(*change.ToolApprovalMode) {
			case "", "ask", "auto", "yolo":
			default:
				return botSettingsView{}, fmt.Errorf("invalid bot tool approval mode")
			}
		}
		if change.WorkspaceRoot != nil && (len(*change.WorkspaceRoot) > 4096 || strings.ContainsRune(*change.WorkspaceRoot, '\x00')) {
			return botSettingsView{}, fmt.Errorf("invalid bot workspace root")
		}
	}
	if change.Action == "set_allowlist" || change.Action == "set_channel_allowlist" {
		if !validBotAccessList(change.List) || len(change.Values) > 100 {
			return botSettingsView{}, fmt.Errorf("invalid bot allowlist request")
		}
		if change.Action == "set_allowlist" && !validBotAccessPlatform(change.Platform) {
			return botSettingsView{}, fmt.Errorf("invalid bot allowlist request")
		}
		if change.Action == "set_channel_allowlist" && strings.TrimSpace(change.ChannelID) == "" {
			return botSettingsView{}, fmt.Errorf("invalid bot allowlist request")
		}
		for _, value := range change.Values {
			if value = strings.TrimSpace(value); value == "" || len(value) > 512 {
				return botSettingsView{}, fmt.Errorf("bot allowlist entries must be 1–512 characters")
			}
		}
	}
	if change.Action == "set_credentials" && (len(change.Identity) > 512 || len(change.Secret) > 8192 || strings.TrimSpace(change.Identity) == "" || strings.TrimSpace(change.Secret) == "") {
		return botSettingsView{}, fmt.Errorf("bot identity and credential are required")
	}
	err := appconfig.EditUserConfigWithCredentialsStrict(func(cfg *appconfig.Config) ([]appconfig.CredentialChange, error) {
		if change.Action == "create_connection" {
			input := change.Connection
			if len(cfg.Bot.Connections) >= 100 {
				return nil, fmt.Errorf("too many bot connections")
			}
			for _, entry := range cfg.Bot.Connections {
				if botruntime.ConnectionRuntimeID(entry) == input.ID || strings.TrimSpace(entry.ID) == input.ID {
					return nil, fmt.Errorf("bot connection already exists")
				}
			}
			// A fresh account is disabled until its owner explicitly enables it. Trust
			// starts with pairing, never an implicit allow-all grant.
			now := time.Now().UTC().Format(time.RFC3339)
			entry := appconfig.BotConnectionConfig{ID: input.ID, Provider: input.Platform, Domain: input.Domain, Label: firstNonEmpty(input.Label, input.ID), Status: "connected", CreatedAt: now, UpdatedAt: now, Access: appconfig.BotAccessConfig{Enabled: true, PairingEnabled: true}}
			key := botCredentialKey(input.ID)
			if input.Platform == "weixin" {
				entry.Credential.AccountID = input.Identity
				entry.Credential.TokenEnv = key
			} else {
				entry.Credential.AppID = input.Identity
				entry.Credential.AppSecretEnv = key
			}
			cfg.Bot.Connections = append(cfg.Bot.Connections, entry)
			return []appconfig.CredentialChange{{Key: key, Value: input.Secret}}, nil
		}
		if change.Action == "remove_connection" {
			index := -1
			for i, entry := range cfg.Bot.Connections {
				if botruntime.ConnectionRuntimeID(entry) == change.ChannelID || strings.TrimSpace(entry.ID) == change.ChannelID {
					index = i
					break
				}
			}
			if index < 0 {
				return nil, fmt.Errorf("bot connection not found")
			}
			cfg.Bot.Connections = append(cfg.Bot.Connections[:index], cfg.Bot.Connections[index+1:]...)
			routes := cfg.Bot.Routes[:0]
			for _, route := range cfg.Bot.Routes {
				if strings.TrimSpace(route.ConnectionID) != change.ChannelID {
					routes = append(routes, route)
				}
			}
			cfg.Bot.Routes = routes
			watchers := cfg.Bot.DesktopWatchers[:0]
			for _, watcher := range cfg.Bot.DesktopWatchers {
				if strings.TrimSpace(watcher.ConnectionID) != change.ChannelID {
					watchers = append(watchers, watcher)
				}
			}
			cfg.Bot.DesktopWatchers = watchers
			key := botCredentialKey(change.ChannelID)
			data, _ := json.Marshal(cfg)
			quoted, _ := json.Marshal(key)
			if !strings.Contains(string(data), string(quoted)) {
				return []appconfig.CredentialChange{{Key: key, Remove: true}}, nil
			}
			return nil, nil
		}

		if change.Action == "set_routes" {
			routes := make([]appconfig.BotRouteConfig, 0, len(change.Routes))
			for _, route := range change.Routes {
				route.ConnectionID = strings.TrimSpace(route.ConnectionID)
				route.Platform = strings.TrimSpace(route.Platform)
				route.ChatType = strings.TrimSpace(route.ChatType)
				route.ChatID = strings.TrimSpace(route.ChatID)
				route.UserID = strings.TrimSpace(route.UserID)
				route.ThreadID = strings.TrimSpace(route.ThreadID)
				route.Model = strings.TrimSpace(route.Model)
				route.ToolApprovalMode = strings.TrimSpace(route.ToolApprovalMode)
				route.WorkspaceRoot = strings.TrimSpace(route.WorkspaceRoot)
				if botRouteHasValue(route) {
					routes = append(routes, appconfig.BotRouteConfig{
						ConnectionID: route.ConnectionID, Platform: route.Platform, ChatType: route.ChatType,
						ChatID: route.ChatID, UserID: route.UserID, ThreadID: route.ThreadID, Model: route.Model,
						ToolApprovalMode: route.ToolApprovalMode, WorkspaceRoot: route.WorkspaceRoot,
					})
				}
			}
			cfg.Bot.Routes = routes
			return nil, nil
		}
		if change.Action == "set_gateway_runtime" {
			if change.MaxSteps != nil {
				cfg.Bot.MaxSteps = *change.MaxSteps
			}
			if change.DebounceMs != nil {
				cfg.Bot.DebounceMs = *change.DebounceMs
			}
			if change.QueueMode != nil {
				cfg.Bot.QueueMode = strings.TrimSpace(*change.QueueMode)
			}
			if change.QueueCap != nil {
				cfg.Bot.QueueCap = *change.QueueCap
			}
			if change.QueueDrop != nil {
				cfg.Bot.QueueDrop = strings.TrimSpace(*change.QueueDrop)
			}
			if change.IgnoreSelfMessages != nil {
				cfg.Bot.IgnoreSelfMessages = *change.IgnoreSelfMessages
			}
			if change.PairingRequestTTLMinutes != nil {
				cfg.Bot.Pairing.RequestTTLMinutes = *change.PairingRequestTTLMinutes
			}
			if change.PairingMaxPending != nil {
				cfg.Bot.Pairing.MaxPendingPerPlatform = *change.PairingMaxPending
			}
			return nil, nil
		}
		if change.Action == "set_self_user_ids" {
			values := normalizeBotAccessValues(change.Values)
			switch change.Platform {
			case "qq":
				cfg.Bot.SelfUserIDs.QQ = values
			case "feishu":
				cfg.Bot.SelfUserIDs.Feishu = values
			case "weixin":
				cfg.Bot.SelfUserIDs.Weixin = values
			case "dingtalk":
				cfg.Bot.SelfUserIDs.Dingtalk = values
			}
			return nil, nil
		}
		if change.Action == "set_pairing" {
			cfg.Bot.Pairing.Enabled = change.Enabled
			return nil, nil
		}
		if change.Action == "set_allow_all" {
			cfg.Bot.Allowlist.AllowAll = change.Enabled
			cfg.Bot.Allowlist.Enabled = !change.Enabled
			return nil, nil
		}
		if change.Action == "set_channel_access_mode" {
			access, err := botChannelAccessConfig(cfg, change.ChannelID)
			if err != nil {
				return nil, err
			}
			switch change.Mode {
			case "trusted":
				access.Enabled, access.AllowAll = true, false
			case "everyone":
				access.Enabled, access.AllowAll = false, true
			default:
				return nil, fmt.Errorf("invalid bot access mode")
			}
			return nil, nil
		}
		if change.Action == "set_channel_runtime" {
			var model, approval, workspace *string
			switch change.ChannelID {
			case "legacy:qq":
				model, approval, workspace = &cfg.Bot.QQ.Model, &cfg.Bot.QQ.ToolApprovalMode, &cfg.Bot.QQ.WorkspaceRoot
			case "legacy:dingtalk":
				model, approval, workspace = &cfg.Bot.Dingtalk.Model, &cfg.Bot.Dingtalk.ToolApprovalMode, &cfg.Bot.Dingtalk.WorkspaceRoot
			default:
				for i := range cfg.Bot.Connections {
					connection := &cfg.Bot.Connections[i]
					id := strings.TrimSpace(connection.ID)
					if id == "" {
						id = botruntime.ConnectionRuntimeID(*connection)
					}
					if id == change.ChannelID {
						model, approval, workspace = &connection.Model, &connection.ToolApprovalMode, &connection.WorkspaceRoot
						break
					}
				}
			}
			if model == nil {
				return nil, fmt.Errorf("bot channel %q does not support runtime settings", change.ChannelID)
			}
			if change.Model != nil {
				*model = strings.TrimSpace(*change.Model)
			}
			if change.ToolApprovalMode != nil {
				*approval = strings.TrimSpace(*change.ToolApprovalMode)
			}
			if change.WorkspaceRoot != nil {
				*workspace = strings.TrimSpace(*change.WorkspaceRoot)
			}
			return nil, nil
		}
		if change.Action == "set_channel_pairing" || change.Action == "set_channel_allowlist" {
			access, err := botChannelAccessConfig(cfg, change.ChannelID)
			if err != nil {
				return nil, err
			}
			if change.Action == "set_channel_pairing" {
				access.PairingEnabled = change.Enabled
				return nil, nil
			}
			values := normalizeBotAccessValues(change.Values)
			if err := setBotAccessList(access, change.List, values); err != nil {
				return nil, err
			}
			access.Enabled, access.AllowAll = true, false
			return nil, nil
		}
		if change.Action == "set_allowlist" {
			values := normalizeBotAccessValues(change.Values)
			if err := setBotAllowlist(cfg, change.Platform, change.List, values); err != nil {
				return nil, err
			}
			cfg.Bot.Allowlist.Enabled = true
			cfg.Bot.Allowlist.AllowAll = false
			return nil, nil
		}
		if change.Action == "set_enabled" {
			if change.Enabled && !botruntime.BotConfigHasAccessControl(cfg.Bot) {
				return nil, fmt.Errorf("bot access control is required before enabling the runtime")
			}
			cfg.Bot.Enabled = change.Enabled
			return nil, nil
		}
		if change.Action == "set_credentials" {
			if err := setBotChannelCredential(cfg, change.ChannelID, change.Identity); err != nil {
				return nil, err
			}
			return []appconfig.CredentialChange{{Key: botCredentialKey(change.ChannelID), Value: change.Secret}}, nil
		}
		if change.Enabled && !botruntime.BotConfigHasAccessControl(cfg.Bot) {
			return nil, fmt.Errorf("bot access control is required before enabling a channel")
		}
		switch change.ChannelID {
		case "legacy:qq":
			cfg.Bot.QQ.Enabled = change.Enabled
		case "legacy:feishu":
			cfg.Bot.Feishu.Enabled = change.Enabled
		case "legacy:weixin":
			cfg.Bot.Weixin.Enabled = change.Enabled
		case "legacy:dingtalk":
			cfg.Bot.Dingtalk.Enabled = change.Enabled
		default:
			found := false
			for i := range cfg.Bot.Connections {
				id := strings.TrimSpace(cfg.Bot.Connections[i].ID)
				if id == "" {
					id = botruntime.ConnectionRuntimeID(cfg.Bot.Connections[i])
				}
				if id == change.ChannelID {
					cfg.Bot.Connections[i].Enabled = change.Enabled
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("bot channel %q was not found", change.ChannelID)
			}
		}
		return nil, nil
	})
	if err != nil {
		return botSettingsView{}, err
	}
	return readBotSettings()
}

func validBotRoute(route botRouteSettings) bool {
	for _, value := range []string{route.ConnectionID, route.Platform, route.ChatType, route.ChatID, route.UserID, route.ThreadID, route.Model, route.ToolApprovalMode} {
		if len(value) > 512 || strings.ContainsAny(value, "\r\n\x00") {
			return false
		}
	}
	if len(route.WorkspaceRoot) > 4096 || strings.ContainsRune(route.WorkspaceRoot, '\x00') {
		return false
	}
	if route.Platform != "" && !validBotAccessPlatform(route.Platform) {
		return false
	}
	switch route.ChatType {
	case "", "dm", "group", "guild", "direct", "thread":
	default:
		return false
	}
	switch strings.TrimSpace(route.ToolApprovalMode) {
	case "", "ask", "auto", "yolo":
	default:
		return false
	}
	return true
}

func botRouteHasValue(route botRouteSettings) bool {
	return strings.TrimSpace(route.ConnectionID+route.Platform+route.ChatType+route.ChatID+route.UserID+route.ThreadID+route.Model+route.ToolApprovalMode+route.WorkspaceRoot) != ""
}

func validBotAccessPlatform(value string) bool {
	switch strings.TrimSpace(value) {
	case "qq", "feishu", "weixin", "dingtalk":
		return true
	default:
		return false
	}
}

func validBotAccessList(value string) bool {
	switch strings.TrimSpace(value) {
	case "users", "groups", "approvers", "admins":
		return true
	default:
		return false
	}
}

func normalizeBotAccessValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func setBotAllowlist(cfg *appconfig.Config, platform, list string, values []string) error {
	if cfg == nil || !validBotAccessPlatform(platform) || !validBotAccessList(list) {
		return fmt.Errorf("invalid bot allowlist target")
	}
	var target *[]string
	switch platform + ":" + list {
	case "qq:users":
		target = &cfg.Bot.Allowlist.QQUsers
	case "qq:groups":
		target = &cfg.Bot.Allowlist.QQGroups
	case "qq:approvers":
		target = &cfg.Bot.Allowlist.QQApprovers
	case "qq:admins":
		target = &cfg.Bot.Allowlist.QQAdmins
	case "feishu:users":
		target = &cfg.Bot.Allowlist.FeishuUsers
	case "feishu:groups":
		target = &cfg.Bot.Allowlist.FeishuGroups
	case "feishu:approvers":
		target = &cfg.Bot.Allowlist.FeishuApprovers
	case "feishu:admins":
		target = &cfg.Bot.Allowlist.FeishuAdmins
	case "weixin:users":
		target = &cfg.Bot.Allowlist.WeixinUsers
	case "weixin:groups":
		target = &cfg.Bot.Allowlist.WeixinGroups
	case "weixin:approvers":
		target = &cfg.Bot.Allowlist.WeixinApprovers
	case "weixin:admins":
		target = &cfg.Bot.Allowlist.WeixinAdmins
	case "dingtalk:users":
		target = &cfg.Bot.Allowlist.DingtalkUsers
	case "dingtalk:groups":
		target = &cfg.Bot.Allowlist.DingtalkGroups
	case "dingtalk:approvers":
		target = &cfg.Bot.Allowlist.DingtalkApprovers
	case "dingtalk:admins":
		target = &cfg.Bot.Allowlist.DingtalkAdmins
	default:
		return fmt.Errorf("invalid bot allowlist target")
	}
	*target = values
	return nil
}

func botChannelAccessConfig(cfg *appconfig.Config, channelID string) (*appconfig.BotAccessConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("bot profile is unavailable")
	}
	switch strings.TrimSpace(channelID) {
	case "legacy:qq":
		return &cfg.Bot.QQ.Access, nil
	case "legacy:dingtalk":
		return &cfg.Bot.Dingtalk.Access, nil
	}
	for i := range cfg.Bot.Connections {
		connection := &cfg.Bot.Connections[i]
		id := strings.TrimSpace(connection.ID)
		if id == "" {
			id = botruntime.ConnectionRuntimeID(*connection)
		}
		if id == strings.TrimSpace(channelID) {
			return &connection.Access, nil
		}
	}
	return nil, fmt.Errorf("bot channel %q does not support per-channel access settings", channelID)
}

func setBotAccessList(access *appconfig.BotAccessConfig, list string, values []string) error {
	if access == nil || !validBotAccessList(list) {
		return fmt.Errorf("invalid bot access list")
	}
	switch list {
	case "users":
		access.Users = values
	case "groups":
		access.Groups = values
	case "approvers":
		access.Approvers = values
	case "admins":
		access.Admins = values
	}
	return nil
}

func botCredentialKey(channelID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(channelID)))
	return fmt.Sprintf("REASONIX_BOT_%X_SECRET", sum[:8])
}

func setBotChannelCredential(cfg *appconfig.Config, channelID, identity string) error {
	if cfg == nil {
		return fmt.Errorf("bot profile is unavailable")
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return fmt.Errorf("bot identity is required")
	}
	key := botCredentialKey(channelID)
	switch channelID {
	case "legacy:qq":
		cfg.Bot.QQ.AppID, cfg.Bot.QQ.AppSecretEnv = identity, key
	case "legacy:feishu":
		cfg.Bot.Feishu.AppID, cfg.Bot.Feishu.AppSecretEnv = identity, key
	case "legacy:weixin":
		cfg.Bot.Weixin.AccountID, cfg.Bot.Weixin.TokenEnv = identity, key
	case "legacy:dingtalk":
		cfg.Bot.Dingtalk.ClientID, cfg.Bot.Dingtalk.SecretEnv = identity, key
		cfg.Bot.Dingtalk.ClientSecret = ""
	default:
		for i := range cfg.Bot.Connections {
			connection := &cfg.Bot.Connections[i]
			id := strings.TrimSpace(connection.ID)
			if id == "" {
				id = botruntime.ConnectionRuntimeID(*connection)
			}
			if id != channelID {
				continue
			}
			switch strings.TrimSpace(connection.Provider) {
			case "qq", "feishu", "dingtalk":
				connection.Credential.AppID = identity
				connection.Credential.AppSecretEnv = key
			case "weixin":
				connection.Credential.AccountID = identity
				connection.Credential.TokenEnv = key
			default:
				return fmt.Errorf("unsupported bot channel provider")
			}
			return nil
		}
		return fmt.Errorf("bot channel %q was not found", channelID)
	}
	return nil
}

func (b *bridgeServer) botSettings(w http.ResponseWriter, _ *http.Request) {
	view, err := readBotSettings()
	if err != nil {
		writeProtocolError(w, http.StatusInternalServerError, "internal", "unable to read bot settings")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (b *bridgeServer) changeBotSettings(w http.ResponseWriter, r *http.Request) {
	var change botSettingsChange
	if err := decodeJSONBody(w, r, 64<<10, &change); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "bot settings could not be saved; check the input and retry")
		return
	}
	view, err := changeBotSettings(change)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "bot settings could not be saved; check the input and retry")
		return
	}
	if b.botRuntime != nil {
		b.botRuntime.refreshAsync(nil)
	}
	writeJSON(w, http.StatusOK, view)
}
