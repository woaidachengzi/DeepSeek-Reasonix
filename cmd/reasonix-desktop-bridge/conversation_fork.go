package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/pinnedcontext"
	"reasonix/internal/sessionidentity"
)

func (r *controllerRuntime) ForkConversation(forkToken string) (desktopbridge.LegacyConversationForkResult, error) {
	if appconfig.DesktopSessionIdentityPath() == "" {
		return desktopbridge.LegacyConversationForkResult{}, fmt.Errorf("conversation identity store is unavailable")
	}
	// Capture desired context before creating the child. A malformed source
	// manifest must not silently become an empty context in the new conversation.
	pinned, err := pinnedcontext.LoadState(r.controller.SessionPath())
	if err != nil {
		return desktopbridge.LegacyConversationForkResult{}, fmt.Errorf("load source pinned files before forking: %w", err)
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return desktopbridge.LegacyConversationForkResult{}, err
	}
	childID := "tauri-" + hex.EncodeToString(random[:])
	path, err := bridgeSessionPath(r.controller.SessionDir(), childID)
	if err != nil {
		return desktopbridge.LegacyConversationForkResult{}, err
	}
	title := strings.TrimSpace(r.Title())
	if title == "" {
		title = "对话"
	}
	runes := []rune(title)
	if len(runes) > 100 {
		title = string(runes[:100])
	}
	title += "（分叉）"
	ctx := context.Background()
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		return desktopbridge.LegacyConversationForkResult{}, err
	}
	defer identities.Close()
	if err := identities.Reserve(ctx, r.controller.SessionDir(), sessionidentity.Candidate{ID: childID, Path: path, WorkspaceRoot: r.controller.WorkspaceRoot(), Title: title}); err != nil {
		return desktopbridge.LegacyConversationForkResult{}, bridgeIdentityConflict(err)
	}
	forkErr := r.controller.ForkSessionToPath(path, title, forkToken)
	if forkErr != nil {
		if _, statErr := os.Lstat(path); os.IsNotExist(statErr) {
			if err := identities.BeginDelete(ctx, childID, path); err != nil {
				return desktopbridge.LegacyConversationForkResult{}, fmt.Errorf("fork rejected: %w; cleanup failed: %v", forkErr, err)
			}
			if err := identities.FinishDelete(ctx, childID, path); err != nil {
				return desktopbridge.LegacyConversationForkResult{}, fmt.Errorf("fork rejected: %w; cleanup failed: %v", forkErr, err)
			}
			return desktopbridge.LegacyConversationForkResult{}, forkErr
		}
	}
	// A created transcript remains discoverable even if a later metadata write
	// fails. Return its ID so the UI can direct the user to session history.
	pinnedErr := pinnedcontext.SaveState(path, pinned.Files)
	if pinnedErr != nil {
		slog.Warn("conversation fork pinned context incomplete", "session_id", childID, "err", pinnedErr)
	}
	if err := identities.MarkReady(ctx, childID, path); err != nil {
		// Keep the durable child identity in the normal result envelope. Returning
		// an RPC error here would discard it and invite another creation attempt.
		slog.Warn("conversation fork identity registration incomplete", "session_id", childID, "err", err)
		return desktopbridge.LegacyConversationForkResult{SessionID: childID, Error: "分叉已保存，但身份登记未完成；请重新检查会话目录后打开该对话。"}, nil
	}
	if forkErr != nil {
		return desktopbridge.LegacyConversationForkResult{SessionID: childID, Error: "分叉已保存，部分信息未保存完成；请从历史对话打开。"}, nil
	}
	if pinnedErr != nil {
		return desktopbridge.LegacyConversationForkResult{SessionID: childID, Error: "分叉已保存，但固定文件清单保存失败；请从历史对话打开，并重新固定所需文件。"}, nil
	}
	return desktopbridge.LegacyConversationForkResult{OK: true, SessionID: childID}, nil
}

func (b *bridgeServer) conversationFork(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(requestIDHeader) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "conversation fork requires a request ID")
		return
	}
	var request struct {
		ForkToken string `json:"forkToken,omitempty"`
	}
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid conversation fork request")
		return
	}
	result, err := b.runtimes.ForkConversation(r.PathValue("id"), request.ForkToken)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to fork conversation")
		return
	}
	writeJSON(w, http.StatusOK, legacyConversationForkResultResponse{ProtocolVersion: desktopbridge.ProtocolVersion, Result: result})
}
