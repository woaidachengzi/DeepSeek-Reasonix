package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
)

type scanImportCandidatesRequest struct {
	CatalogPath string `json:"catalogPath"`
}

type scanImportCandidatesResponse struct {
	ProtocolVersion int                                   `json:"protocolVersion"`
	Candidates      []sessionidentity.ScanImportCandidate `json:"candidates"`
	BlockedCount    int                                   `json:"blockedCount"`
}

type scanImportApplyRequest struct {
	CatalogPath string                                `json:"catalogPath"`
	Selected    []sessionidentity.ScanImportSelection `json:"selected"`
}

type scanImportApplyResponse struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Applied         int      `json:"applied"`
	SessionIDs      []string `json:"sessionIds"`
}

func validateWorkbenchCatalogPath(path, sessionDir string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("workbench catalog path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil || filepath.Clean(abs) != abs {
		return "", errors.New("workbench catalog path is invalid")
	}
	info, err := os.Lstat(abs)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err == nil && !info.Mode().IsRegular() {
		return "", errors.New("workbench catalog must be a regular file")
	}
	rel, err := filepath.Rel(filepath.Clean(sessionDir), abs)
	if err != nil || rel == "." || (rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return "", errors.New("workbench catalog must be outside the sessions directory")
	}
	return abs, nil
}

func (b *bridgeServer) scanImportCandidates(w http.ResponseWriter, r *http.Request) {
	var request scanImportCandidatesRequest
	if err := decodeJSONBody(w, r, 64<<10, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid scan import inventory request")
		return
	}
	sessionDir, identityPath := appconfig.SessionDir(), appconfig.DesktopSessionIdentityPath()
	if sessionDir == "" || identityPath == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session profile is unavailable")
		return
	}
	catalogPath, err := validateWorkbenchCatalogPath(request.CatalogPath, sessionDir)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "workbench catalog path is invalid")
		return
	}
	identities, exists, closeIdentity, err := b.readIdentityStore(r.Context(), identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		b.writeRuntimeError(w, err, "unable to inspect the session identity store")
		return
	}
	if exists {
		defer closeIdentity()
	}
	listing, err := sessionidentity.ListScanImportCandidates(r.Context(), identities, sessionDir, catalogPath)
	if err != nil {
		b.writeRuntimeError(w, err, "unable to inspect unclaimed session transcripts")
		return
	}
	writeJSON(w, http.StatusOK, scanImportCandidatesResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Candidates:      listing.Candidates, BlockedCount: listing.BlockedCount,
	})
}

func (b *bridgeServer) applyScanImport(w http.ResponseWriter, r *http.Request) {
	var request scanImportApplyRequest
	if err := decodeJSONBody(w, r, 2<<20, &request); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid reviewed scan import request")
		return
	}
	if len(request.Selected) == 0 || len(request.Selected) > sessionidentity.MaxVisibleSnapshotSize {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "scan import selection is empty or too large")
		return
	}
	sessionDir, identityPath := appconfig.SessionDir(), appconfig.DesktopSessionIdentityPath()
	if sessionDir == "" || identityPath == "" {
		writeProtocolError(w, http.StatusServiceUnavailable, "internal", "session profile is unavailable")
		return
	}
	catalogPath, err := validateWorkbenchCatalogPath(request.CatalogPath, sessionDir)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "workbench catalog path is invalid")
		return
	}
	identities, err := sessionidentity.Open(r.Context(), identityPath, appconfig.SessionProfileRoot())
	if err != nil {
		b.writeRuntimeError(w, err, "unable to open the session identity store")
		return
	}
	defer identities.Close()
	plan, err := sessionidentity.PrepareScanImportReview(r.Context(), identities, sessionDir, catalogPath, request.Selected)
	if err != nil {
		writeProtocolError(w, http.StatusConflict, "session_conflict", "selected transcript changed or is no longer eligible for import")
		return
	}
	result, err := identities.ApplyScanImportReviewUnderProfileGate(r.Context(), plan)
	if err != nil {
		if errors.Is(err, sessionidentity.ErrImportReviewChanged) || errors.Is(err, sessionidentity.ErrPathChanged) || errors.Is(err, sessionidentity.ErrSessionStateConflict) {
			writeProtocolError(w, http.StatusConflict, "session_conflict", "session directory changed during reviewed import")
			return
		}
		b.writeRuntimeError(w, err, "unable to apply the reviewed session import")
		return
	}
	ids := make([]string, len(plan.Rows))
	for i, row := range plan.Rows {
		ids[i] = row.ID
	}
	writeJSON(w, http.StatusOK, scanImportApplyResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		Applied:         result.Applied, SessionIDs: ids,
	})
}
