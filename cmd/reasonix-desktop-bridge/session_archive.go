package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/sessionarchive"
	"reasonix/internal/sessionidentity"
)

type sessionArchiveResponse struct {
	ProtocolVersion int                    `json:"protocolVersion"`
	Sessions        []sessionarchive.Entry `json:"sessions"`
}

func sessionArchivePath() string {
	return filepath.Join(filepath.Dir(appconfig.DesktopSessionIdentityPath()), "session-archives-v1.json")
}

func (b *bridgeServer) listSessionArchives(w http.ResponseWriter, r *http.Request) {
	entries, err := sessionarchive.List(sessionArchivePath())
	if err != nil {
		b.writeRuntimeError(w, err, "unable to read archived conversations; check profile permissions")
		return
	}
	writeJSON(w, http.StatusOK, sessionArchiveResponse{desktopbridge.ProtocolVersion, entries})
}

func (b *bridgeServer) changeSessionArchive(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID string `json:"sessionId"`
		Archived  bool   `json:"archived"`
	}
	if decodeJSONBody(w, r, 4<<10, &request) != nil || !sessionpath.ValidID(request.SessionID) {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid session archive request")
		return
	}
	var entries []sessionarchive.Entry
	err := b.runtimes.ChangeArchive(request.SessionID, request.Archived, func(owned bool) error {
		exists, err := sessionidentity.IdentityDatabaseExists(appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
		if err != nil {
			return err
		}
		if !exists {
			return desktopbridge.ErrSessionNotFound
		}
		store, err := sessionidentity.Open(r.Context(), appconfig.DesktopSessionIdentityPath(), appconfig.SessionProfileRoot())
		if err != nil {
			return err
		}
		defer store.Close()
		record, found, err := store.Get(r.Context(), request.SessionID)
		if err != nil {
			return err
		}
		if !found {
			return desktopbridge.ErrSessionNotFound
		}
		expected, err := bridgeSessionPath(appconfig.SessionDir(), request.SessionID)
		if err != nil || record.Path != expected || (record.State != sessionidentity.StateReady && record.State != sessionidentity.StateReserved) {
			return desktopbridge.ErrSessionConflict
		}
		if _, pending, err := store.PendingManualTitleRename(r.Context(), request.SessionID); err != nil {
			return err
		} else if pending {
			return desktopbridge.ErrSessionConflict
		}
		if err := store.CheckTranscriptPathUnique(r.Context(), request.SessionID, expected); err != nil {
			return err
		}
		info, err := os.Lstat(expected)
		// A fresh reserved conversation may only own metadata: normal shutdown
		// does not synthesize an empty transcript. Preserve that state on archive.
		if err != nil && !(errors.Is(err, os.ErrNotExist) && record.State == sessionidentity.StateReserved) {
			return err
		}
		if err == nil && !info.Mode().IsRegular() {
			return fmt.Errorf("invalid archive transcript")
		}
		if !owned {
			lease, err := agent.TryAcquireSessionLease(expected)
			if err != nil {
				return desktopbridge.ErrSessionConflict
			}
			defer lease.Release()
		}
		entries, err = sessionarchive.Set(sessionArchivePath(), sessionarchive.Entry{SessionID: record.ID, Title: record.Title, WorkspaceRoot: record.WorkspaceRoot}, request.Archived)
		return err
	})
	if err != nil {
		b.writeRuntimeError(w, err, "unable to change conversation archive; finish active work or retry")
		return
	}
	writeJSON(w, http.StatusOK, sessionArchiveResponse{desktopbridge.ProtocolVersion, entries})
}

func (r *controllerRuntime) CheckArchiveIdle() error {
	status := r.controller.RuntimeStatus()
	if status.Running || status.PendingPrompt || status.BackgroundJobs > 0 {
		return desktopbridge.ErrSessionConflict
	}
	return nil
}
func (r *controllerRuntime) PrepareArchive() error {
	if err := r.CheckArchiveIdle(); err != nil {
		return err
	}
	return r.controller.SnapshotForShutdown()
}
func (r *controllerRuntime) ReleaseArchived() {
	r.cancelMCPOAuthFlows()
	r.stopTurnSnapshotMonitor()
	r.controller.Close()
	_ = r.lifecycleSink.Close()
}
