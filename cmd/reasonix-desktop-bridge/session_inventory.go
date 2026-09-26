package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"reasonix/internal/agent"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/sessionpath"
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

type sessionShadowAuditSnapshotResponse struct {
	ProtocolVersion int                         `json:"protocolVersion"`
	Directory       sessionListResponse         `json:"directory"`
	Inventory       sessionShadowAuditInventory `json:"inventory"`
}

type sessionShadowAuditInventory struct {
	ProtocolVersion int                       `json:"protocolVersion"`
	Entries         []sessionShadowAuditEntry `json:"entries"`
	UnclaimedCount  int                       `json:"unclaimedCount"`
	ErrorCount      int                       `json:"errorCount"`
	RetiredIDs      []string                  `json:"retiredIds"`
}

type sessionShadowAuditEntry struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Exists   bool   `json:"exists"`
	Readable bool   `json:"readable"`
}

type sessionShadowAuditRequest struct {
	LegacySessionIDs []string `json:"legacySessionIds"`
}

var errSessionShadowIdentityUnavailable = errors.New("session identity store is unavailable")

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
		store, exists, closeIdentity, err := b.readIdentityStore(r.Context(), identityPath, appconfig.SessionProfileRoot())
		if err != nil {
			b.writeRuntimeError(w, err, "unable to inspect the session identity store")
			return
		}
		if exists {
			defer closeIdentity()
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
	report, snapshot, identityPath, identityStore, err := b.readSessionShadowSnapshot(r)
	if err != nil {
		if errors.Is(err, errSessionShadowIdentityUnavailable) {
			writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session identity store is unavailable")
			return
		}
		b.writeRuntimeError(w, err, "unable to audit the session directory")
		return
	}
	writeJSON(w, http.StatusOK, sessionShadowSnapshotResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Directory: sessionListResponse{
			ProtocolVersion: desktopbridge.ProtocolVersion,
			Sessions:        sessionListEntries(snapshot.Records), Total: snapshot.Total, SnapshotID: snapshot.SnapshotID,
		},
		Inventory: sessionInventoryEnvelope(report, identityPath, identityStore),
	})
}

// sessionShadowAuditSnapshot sends only the physical state needed to validate
// the visible directory. The older full shadow-snapshot endpoint remains for
// existing hosts; this bounded projection avoids transmitting every scanned
// path and diagnostic string on each Tauri audit.
func (b *bridgeServer) sessionShadowAuditSnapshot(w http.ResponseWriter, r *http.Request) {
	var request sessionShadowAuditRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid shadow audit request")
		return
	}
	if len(request.LegacySessionIDs) > 50 {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "shadow audit contains too many legacy IDs")
		return
	}
	legacyIDs := make(map[string]struct{}, len(request.LegacySessionIDs))
	for _, id := range request.LegacySessionIDs {
		if !sessionpath.ValidID(id) {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "shadow audit contains an invalid legacy ID")
			return
		}
		if _, duplicate := legacyIDs[id]; duplicate {
			writeProtocolError(w, http.StatusBadRequest, "invalid_request", "shadow audit contains duplicate legacy IDs")
			return
		}
		legacyIDs[id] = struct{}{}
	}
	report, snapshot, _, _, err := b.readSessionShadowSnapshot(r)
	if err != nil {
		if errors.Is(err, errSessionShadowIdentityUnavailable) {
			writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session identity store is unavailable")
			return
		}
		b.writeRuntimeError(w, err, "unable to audit the session directory")
		return
	}
	visibleIDs := make(map[string]struct{}, len(snapshot.Records))
	for _, record := range snapshot.Records {
		visibleIDs[record.ID] = struct{}{}
	}
	entries := make([]sessionShadowAuditEntry, 0, len(snapshot.Records))
	retiredIDs := make([]string, 0)
	for _, entry := range report.Entries {
		if entry.Source != sessionidentity.InventoryFromIdentity {
			continue
		}
		retired := entry.State == sessionidentity.StateDeleting ||
			(entry.State == sessionidentity.StateDeleted && !entry.Exists && entry.Detail == "transcript is absent")
		if _, requested := legacyIDs[entry.ID]; requested && retired {
			retiredIDs = append(retiredIDs, entry.ID)
		}
		if _, visible := visibleIDs[entry.ID]; !visible {
			continue
		}
		entries = append(entries, sessionShadowAuditEntry{
			ID: entry.ID, Source: string(entry.Source), Exists: entry.Exists,
			Readable: entry.Detail == "" || entry.Detail == "transcript is absent",
		})
	}
	writeJSON(w, http.StatusOK, sessionShadowAuditSnapshotResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Directory: sessionListResponse{
			ProtocolVersion: desktopbridge.ProtocolVersion,
			Sessions:        sessionListEntries(snapshot.Records), Total: snapshot.Total, SnapshotID: snapshot.SnapshotID,
		},
		Inventory: sessionShadowAuditInventory{
			ProtocolVersion: desktopbridge.ProtocolVersion,
			Entries:         entries,
			UnclaimedCount:  len(report.Unclaimed),
			ErrorCount:      len(report.Errors),
			RetiredIDs:      retiredIDs,
		},
	})
}

func (b *bridgeServer) readSessionShadowSnapshot(r *http.Request) (sessionidentity.InventoryReport, sessionidentity.Page, string, bool, error) {
	sessionDir, identityPath := appconfig.SessionDir(), appconfig.DesktopSessionIdentityPath()
	if strings.TrimSpace(sessionDir) == "" || strings.TrimSpace(identityPath) == "" {
		return sessionidentity.InventoryReport{}, sessionidentity.Page{}, "", false, errSessionShadowIdentityUnavailable
	}
	identities, exists, closeIdentity, err := b.readIdentityStore(r.Context(), identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		return sessionidentity.InventoryReport{}, sessionidentity.Page{}, "", false, err
	}
	if exists {
		defer closeIdentity()
	}
	report, err := bridgeSessionInventory(r.Context(), identities, sessionDir, "")
	if err != nil {
		return sessionidentity.InventoryReport{}, sessionidentity.Page{}, "", false, err
	}
	snapshot, err := report.VisibleSnapshot(sessionidentity.MaxVisibleSnapshotSize)
	if err != nil {
		return sessionidentity.InventoryReport{}, sessionidentity.Page{}, "", false, err
	}
	return report, snapshot, identityPath, exists, nil
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
