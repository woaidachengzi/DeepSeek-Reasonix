package main

import (
	"reasonix/internal/desktopbridge"
	"reasonix/internal/pinnedcontext"
)

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
