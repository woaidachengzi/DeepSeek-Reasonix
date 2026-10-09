package bot

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrDesktopNotificationDenied = errors.New("desktop notification audience is unavailable")
var ErrDesktopNotificationUnknown = errors.New("desktop notification delivery is unconfirmed")

// DesktopNotification contains deliberately separate safe summary and private
// detail. Callers must construct Summary from approved event kinds, never raw
// tool output/errors/reasoning. Only DM/direct delivery can include PrivateDetail.
// No arbitrary webhook, attachment, card URL or reply target crosses this API.
type DesktopNotification struct {
	Summary       string
	PrivateDetail string
}

// SendDesktopNotification rechecks current admission/admin and exact started
// transport for a persisted watch actor. Persistence alone conveys no authority.
// It sends once; transport error/cancellation is unknown, never an automatic retry.
// Stop cancels and awaits these sends. Adapter.Send must honor its context and
// must not synchronously call Gateway.Stop from this outbound callback.
func (gw *BotGateway) SendDesktopNotification(ctx context.Context, route DesktopWatchRoute, actor string, notification DesktopNotification) (SendResult, error) {
	if gw.cfg.Desktop == nil || ctx == nil || ctx.Err() != nil || !validDesktopNotificationIdentity(route, actor) || !validDesktopNotificationText(notification.Summary, 512) || strings.TrimSpace(notification.Summary) == "" || !validDesktopNotificationText(notification.PrivateDetail, 8192) {
		return SendResult{}, ErrDesktopNotificationDenied
	}
	gw.lifecycleMu.Lock()
	if !gw.started || gw.stopped || gw.runContext == nil || gw.startDone == nil {
		gw.lifecycleMu.Unlock()
		return SendResult{}, ErrDesktopNotificationDenied
	}
	select {
	case <-gw.startDone:
	default:
		gw.lifecycleMu.Unlock()
		return SendResult{}, ErrDesktopNotificationDenied
	}
	runCtx := gw.runContext
	// Add is fenced by stopped under lifecycleMu, so Stop's Wait cannot race a
	// newly admitted sender after closure. Do not hold either gateway lock on IO.
	gw.desktopSendWG.Add(1)
	gw.lifecycleMu.Unlock()
	defer gw.desktopSendWG.Done()
	var target AdapterBinding
	gw.mu.Lock()
	for _, binding := range gw.adapters {
		if binding.ID == route.ConnectionID && binding.Domain == route.Domain && binding.Platform == route.Platform {
			target = binding
			break
		}
	}
	gw.mu.Unlock()
	identity := InboundMessage{Platform: route.Platform, ConnectionID: route.ConnectionID, Domain: route.Domain, ChatType: route.ChatType, ChatID: route.ChatID, UserID: actor}
	if target.Adapter == nil || !gw.checkAllowlist(route.Platform, identity) || !gw.checkCommandRole(route.Platform, identity, "admin") || runCtx.Err() != nil {
		return SendResult{}, ErrDesktopNotificationDenied
	}
	text := notification.Summary
	if (route.ChatType == ChatDM || route.ChatType == ChatDirect) && notification.PrivateDetail != "" {
		text += "\n" + notification.PrivateDetail
	}
	sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	stopRunCancel := context.AfterFunc(runCtx, cancel)
	defer func() { stopRunCancel(); cancel() }()
	if sendCtx.Err() != nil || runCtx.Err() != nil {
		return SendResult{}, ErrDesktopNotificationDenied
	}
	result, err := target.Adapter.Send(sendCtx, OutboundMessage{ConnectionID: route.ConnectionID, Domain: route.Domain, ChatID: route.ChatID, ChatType: route.ChatType, Text: text})
	unknown := err != nil || sendCtx.Err() != nil || runCtx.Err() != nil
	var healthErr error
	if unknown {
		healthErr = ErrDesktopNotificationUnknown
	}
	// Adapter health is exposed to settings too: do not store raw SDK errors.
	gw.markAdapterSend(target, healthErr)
	// Preserve echo suppression even for a partial/unknown send with delivery
	// IDs; this bookkeeping is not a claim of confirmed delivery to the caller.
	for _, id := range result.DeliveredMessageIDs() {
		gw.rememberOutboundMessage(target.Platform, target.ID, target.Domain, route.ChatID, id)
	}
	if unknown {
		return SendResult{}, ErrDesktopNotificationUnknown
	}
	return result, nil
}

func validDesktopNotificationText(value string, limit int) bool {
	return len(value) <= limit && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func validDesktopNotificationIdentity(route DesktopWatchRoute, actor string) bool {
	for _, value := range []string{actor, route.ConnectionID, route.Domain, route.ChatID} {
		if len(value) > 512 || strings.TrimSpace(value) != value || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") {
			return false
		}
	}
	if actor == "" || route.ConnectionID == "" || route.ChatID == "" {
		return false
	}
	switch route.Platform {
	case PlatformQQ, PlatformFeishu, PlatformWeixin, PlatformDingtalk:
	default:
		return false
	}
	switch route.ChatType {
	case ChatDM, ChatDirect, ChatGroup, ChatGuild, ChatThread:
		return true
	}
	return false
}
