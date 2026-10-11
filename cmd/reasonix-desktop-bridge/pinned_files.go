package main

import (
	"net/http"
	"strings"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/pinnedcontext"
)

type pinnedFilesResponse struct {
	ProtocolVersion uint8                      `json:"protocolVersion"`
	SessionID       string                     `json:"sessionId"`
	Files           []desktopbridge.PinnedFile `json:"files"`
}

func (b *bridgeServer) pinnedFiles(w http.ResponseWriter, r *http.Request) {
	files, err := b.runtimes.PinnedFiles(r.PathValue("id"))
	if err != nil {
		b.writeRuntimeError(w, err, "unable to load pinned files")
		return
	}
	writeJSON(w, http.StatusOK, pinnedFilesResponse{ProtocolVersion: desktopbridge.ProtocolVersion, SessionID: r.PathValue("id"), Files: files})
}

func (b *bridgeServer) changePinnedFile(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(requestIDHeader) == "" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "changing pinned files requires a request ID")
		return
	}
	var request workspaceFileRequest
	if err := decodeJSONBody(w, r, 16<<10, &request); err != nil || strings.TrimSpace(request.Path) == "" || len(request.Path) > 4096 || strings.ContainsRune(request.Path, '\x00') {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid pinned file path")
		return
	}
	if _, err := pinnedcontext.NormalizePath(request.Path); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid pinned file path")
		return
	}
	files, err := b.runtimes.ChangePinnedFile(r.PathValue("id"), request.Path, r.PathValue("pin") == "true")
	if err != nil {
		b.writeRuntimeError(w, err, "unable to change pinned files")
		return
	}
	writeJSON(w, http.StatusOK, pinnedFilesResponse{ProtocolVersion: desktopbridge.ProtocolVersion, SessionID: r.PathValue("id"), Files: files})
}

func pinnedFileViews(infos []pinnedcontext.Info) []desktopbridge.PinnedFile {
	files := make([]desktopbridge.PinnedFile, 0, len(infos))
	for _, info := range infos {
		files = append(files, desktopbridge.PinnedFile{
			Path: info.Path, SizeBytes: info.SizeBytes, TokenEstimate: info.TokenEstimate, Error: info.Error,
		})
	}
	return files
}

func (r *controllerRuntime) PinnedFiles() ([]desktopbridge.PinnedFile, error) {
	root, err := r.LocalWorkspace()
	if err != nil {
		return nil, err
	}
	state, err := pinnedcontext.LoadState(r.controller.SessionPath())
	if err != nil {
		return nil, err
	}
	return pinnedFileViews(pinnedcontext.Build(root, state.Files, nil).Infos), nil
}

func (r *controllerRuntime) ChangePinnedFile(path string, pin bool) ([]desktopbridge.PinnedFile, error) {
	var files []desktopbridge.PinnedFile
	err := r.controller.MutatePinnedContext(func(sessionPath string) error {
		root, err := r.LocalWorkspace()
		if err != nil {
			return err
		}
		infos, err := pinnedcontext.Change(root, sessionPath, path, pin)
		if err == nil {
			files = pinnedFileViews(infos)
		}
		return err
	})
	return files, err
}
