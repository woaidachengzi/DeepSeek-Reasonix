package main

import (
	"context"
	"net/http"
	"os"
	"strings"

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
	sessionstore "reasonix/internal/store"
)

// sessionInventoryResponse is the wire body of the read-only session listing.
// It carries no session content: only identities, paths and diagnostics.
type sessionInventoryResponse struct {
	ProtocolVersion int                              `json:"protocolVersion"`
	SessionDir      string                           `json:"sessionDir"`
	IdentityPath    string                           `json:"identityPath"`
	IdentityStore   bool                             `json:"identityStoreExists"`
	Entries         []sessionidentity.InventoryEntry `json:"entries"`
	Unclaimed       []string                         `json:"unclaimed"`
	Errors          []string                         `json:"errors"`
}

type sessionShadowSnapshotResponse struct {
	ProtocolVersion int                      `json:"protocolVersion"`
	Directory       sessionListResponse      `json:"directory"`
	Inventory       sessionInventoryResponse `json:"inventory"`
}

// sessionInventory answers what the Preview profile holds without changing it.
//
// The identity path is resolved here rather than accepted from the request: it
// is the store's own location, and a host-supplied path would let a caller read
// or create an identity database outside the profile. The workbench catalog
// belongs to the host, so it is passed optionally and only read.
func (b *bridgeServer) sessionInventory(w http.ResponseWriter, r *http.Request) {
	sessionDir := appconfig.SessionDir()
	if strings.TrimSpace(sessionDir) == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session directory is unavailable")
		return
	}
	identityPath := appconfig.DesktopSessionIdentityPath()
	catalogPath := strings.TrimSpace(r.URL.Query().Get("catalog"))

	var identities *sessionidentity.Store
	identityStore := false
	if identityPath != "" {
		// Existing schema versions are upgraded during bridge startup. This
		// read-only request never creates the database or mutates session rows.
		exists, err := sessionidentity.IdentityDatabaseExists(identityPath, appconfig.SessionProfileRoot())
		if err != nil {
			b.writeRuntimeError(w, err, "unable to inspect the session identity store")
			return
		}
		if exists {
			store, err := sessionidentity.OpenReadOnly(r.Context(), identityPath, appconfig.SessionProfileRoot())
			if err != nil {
				b.writeRuntimeError(w, err, "unable to open the session identity store")
				return
			}
			defer func() { _ = store.Close() }()
			identities = store
			identityStore = true
		}
	}

	report, err := bridgeSessionInventory(r.Context(), identities, sessionDir, catalogPath)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to list the desktop bridge sessions")
		return
	}
	writeJSON(w, http.StatusOK, sessionInventoryEnvelope(report, identityPath, identityStore))
}

// sessionShadowSnapshot reuses the identity rows already validated by the
// physical inventory instead of resolving every transcript path again for a
// separate complete directory response. The host still verifies each page.
func (b *bridgeServer) sessionShadowSnapshot(w http.ResponseWriter, r *http.Request) {
	sessionDir, identityPath := appconfig.SessionDir(), appconfig.DesktopSessionIdentityPath()
	if strings.TrimSpace(sessionDir) == "" || strings.TrimSpace(identityPath) == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session identity store is unavailable")
		return
	}
	exists, err := sessionidentity.IdentityDatabaseExists(identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		b.writeRuntimeError(w, err, "unable to inspect the session identity store")
		return
	}
	var identities *sessionidentity.Store
	if exists {
		identities, err = sessionidentity.OpenReadOnly(r.Context(), identityPath, appconfig.SessionProfileRoot())
		if err != nil {
			b.writeRuntimeError(w, err, "unable to open the session identity store")
			return
		}
		defer func() { _ = identities.Close() }()
	}
	report, err := bridgeSessionInventory(r.Context(), identities, sessionDir, "")
	if err != nil {
		b.writeRuntimeError(w, err, "unable to audit the session directory")
		return
	}
	snapshot, err := report.VisibleSnapshot(sessionidentity.MaxVisibleSnapshotSize)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to read the session directory snapshot")
		return
	}
	writeJSON(w, http.StatusOK, sessionShadowSnapshotResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Directory: sessionListResponse{
			ProtocolVersion: desktopbridge.ProtocolVersion,
			Sessions:        sessionListEntries(snapshot.Records), Total: snapshot.Total, SnapshotID: snapshot.SnapshotID,
		},
		Inventory: sessionInventoryEnvelope(report, identityPath, exists),
	})
}

func sessionInventoryEnvelope(report sessionidentity.InventoryReport, identityPath string, identityStore bool) sessionInventoryResponse {
	return sessionInventoryResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		SessionDir:      report.SessionDir,
		IdentityPath:    identityPath,
		IdentityStore:   identityStore,
		Entries:         report.Entries,
		Unclaimed:       report.Unclaimed,
		Errors:          report.Errors,
	}
}

func bridgeSessionInventory(ctx context.Context, identities *sessionidentity.Store, sessionDir, catalogPath string) (sessionidentity.InventoryReport, error) {
	report, err := sessionidentity.Inventory(ctx, identities, sessionDir, catalogPath)
	if err != nil {
		return sessionidentity.InventoryReport{}, err
	}
	if identities != nil {
		auditIdentitySidecarTitles(&report)
		pendingIDs, err := identities.PendingManualTitleRenameIDs(ctx)
		if err != nil {
			return sessionidentity.InventoryReport{}, err
		}
		for _, id := range pendingIDs {
			report.Errors = append(report.Errors, id+": session title rename recovery is pending")
		}
	}
	return report, nil
}

// A crash between the legacy sidecar write and the identity title CAS can
// leave the host catalog and SQLite matching while the reopened controller
// displays a different title. Surface that as physical drift so the host's
// existing shadow gate cannot declare the directory clean. This is read-only:
// without a durable rename intent, inventory cannot choose which title wins.
func auditIdentitySidecarTitles(report *sessionidentity.InventoryReport) {
	for index := range report.Entries {
		entry := &report.Entries[index]
		if entry.Source != sessionidentity.InventoryFromIdentity || !entry.Exists || entry.Detail != "" {
			continue
		}
		requiresMirror := entry.TitleSource == sessionidentity.TitleUser && entry.Title != ""
		metaPath := sessionstore.SessionMeta(entry.Path)
		info, err := os.Lstat(metaPath)
		if os.IsNotExist(err) {
			if requiresMirror {
				report.Errors = append(report.Errors, entry.ID+": session title metadata is absent")
			}
			continue
		}
		detail := ""
		switch {
		case err != nil || !info.Mode().IsRegular():
			detail = "session title metadata is unreadable"
		default:
			meta, present, loadErr := agent.LoadBranchMeta(entry.Path)
			if loadErr != nil {
				detail = "session title metadata is unreadable"
			} else if !present && requiresMirror {
				detail = "session title metadata is unreadable"
			} else if present && meta.CustomTitle != entry.Title && (meta.CustomTitle != "" || requiresMirror) {
				detail = "session title metadata differs from identity"
			}
		}
		if detail != "" {
			report.Errors = append(report.Errors, entry.ID+": "+detail)
		}
	}
}
