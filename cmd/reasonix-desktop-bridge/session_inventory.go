package main

import (
	"net/http"
	"os"
	"strings"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
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
		if info, err := os.Stat(identityPath); err == nil && info.Mode().IsRegular() {
			store, err := sessionidentity.OpenReadOnly(r.Context(), identityPath)
			if err != nil {
				b.writeRuntimeError(w, err, "unable to open the session identity store")
				return
			}
			defer func() { _ = store.Close() }()
			identities = store
			identityStore = true
		}
	}

	report, err := sessionidentity.Inventory(r.Context(), identities, sessionDir, catalogPath)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to list the desktop bridge sessions")
		return
	}
	writeJSON(w, http.StatusOK, sessionInventoryResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		SessionDir:      report.SessionDir,
		IdentityPath:    identityPath,
		IdentityStore:   identityStore,
		Entries:         report.Entries,
		Unclaimed:       report.Unclaimed,
		Errors:          report.Errors,
	})
}
