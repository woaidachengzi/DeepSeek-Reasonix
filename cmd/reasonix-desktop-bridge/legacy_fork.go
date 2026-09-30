package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	appconfig "reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
)

func (r *controllerRuntime) PrepareLegacyConversationFork(turn int) (desktopbridge.WorkspaceConversationRewindPlan, error) {
	if turn < 0 {
		return desktopbridge.WorkspaceConversationRewindPlan{}, fmt.Errorf("invalid checkpoint turn")
	}
	if _, dag := r.controller.SessionHead(); dag || appconfig.DesktopSessionIdentityPath() == "" {
		return desktopbridge.WorkspaceConversationRewindPlan{Turn: turn,
			DisabledReason: "this session cannot create a separate registered conversation fork"}, nil
	}
	plan, err := r.controller.PrepareRewind(turn, control.RewindConversation)
	if err != nil {
		return desktopbridge.WorkspaceConversationRewindPlan{}, err
	}
	return desktopbridge.WorkspaceConversationRewindPlan{
		PlanID: plan.PlanID, Turn: plan.Turn, CanConversation: plan.CanConversation,
		DisabledReason: plan.DisabledReason,
	}, nil
}

func (r *controllerRuntime) CommitLegacyConversationFork(planID string) (desktopbridge.LegacyConversationForkResult, error) {
	if !validRevertID(planID) {
		return desktopbridge.LegacyConversationForkResult{}, fmt.Errorf("invalid legacy conversation fork plan ID")
	}
	if _, dag := r.controller.SessionHead(); dag {
		return desktopbridge.LegacyConversationForkResult{}, fmt.Errorf("legacy conversation fork requires an old-format session")
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
	ctx := context.Background()
	identities, err := sessionidentity.Open(ctx, appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
	if err != nil {
		return desktopbridge.LegacyConversationForkResult{}, err
	}
	defer identities.Close()
	if err := identities.Reserve(ctx, r.controller.SessionDir(), sessionidentity.Candidate{
		ID: childID, Path: path, WorkspaceRoot: r.controller.WorkspaceRoot(),
	}); err != nil {
		return desktopbridge.LegacyConversationForkResult{}, bridgeIdentityConflict(err)
	}
	if err := r.controller.CommitLegacyConversationFork(planID, path); err != nil {
		if _, statErr := os.Lstat(path); os.IsNotExist(statErr) {
			// A rejected or stale plan must not leave a visible reserved session.
			if deleteErr := identities.BeginDelete(ctx, childID, path); deleteErr != nil {
				return desktopbridge.LegacyConversationForkResult{}, fmt.Errorf("fork rejected: %w; cleanup of %s failed: %v", err, childID, deleteErr)
			}
			if deleteErr := identities.FinishDelete(ctx, childID, path); deleteErr != nil {
				return desktopbridge.LegacyConversationForkResult{}, fmt.Errorf("fork rejected: %w; cleanup of %s failed: %v", err, childID, deleteErr)
			}
			return desktopbridge.LegacyConversationForkResult{}, err
		}
		// A transcript was durably created before a later sidecar failure.
		// Publish its identity so the user can still reopen the conversation.
		if readyErr := identities.MarkReady(ctx, childID, path); readyErr != nil {
			return desktopbridge.LegacyConversationForkResult{}, fmt.Errorf("fork %s exists but registration failed: %w", childID, readyErr)
		}
		return desktopbridge.LegacyConversationForkResult{OK: true, SessionID: childID}, nil
	}
	if err := identities.MarkReady(ctx, childID, path); err != nil {
		return desktopbridge.LegacyConversationForkResult{SessionID: childID}, err
	}
	return desktopbridge.LegacyConversationForkResult{OK: true, SessionID: childID}, nil
}
