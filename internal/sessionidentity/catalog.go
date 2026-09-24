package sessionidentity

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"reasonix/internal/desktopbridge/sessionpath"
)

type catalogEntry struct {
	SessionID     string `json:"sessionId"`
	Title         string `json:"title"`
	WorkspaceRoot string `json:"workspaceRoot"`
}

// ImportWorkbenchCatalog registers the sessions the Preview host lists.
//
// sessionDir must be the directory the writer actually uses, so this importer
// shares one path rule with the bridge instead of re-deriving a layout: a
// session's workspace is UI metadata and never selects its transcript
// directory. The catalog keeps its own order, titles and workspaces.
func (s *Store) ImportWorkbenchCatalog(ctx context.Context, sessionDir, catalogPath string) error {
	entries, err := readCatalog(catalogPath)
	if err != nil {
		return err
	}
	if len(entries) > 50 {
		return errors.New("workbench catalog contains too many sessions")
	}
	candidates := make([]Candidate, 0, len(entries))
	for position, entry := range entries {
		workspace := strings.TrimSpace(entry.WorkspaceRoot)
		if workspace != "" {
			abs, err := filepath.Abs(workspace)
			if err != nil {
				return fmt.Errorf("resolve workbench workspace: %w", err)
			}
			workspace = abs
		}
		path, err := sessionpath.TranscriptPath(sessionDir, entry.SessionID)
		if err != nil {
			return fmt.Errorf("workbench session %s: %w", entry.SessionID, err)
		}
		candidates = append(candidates, Candidate{
			ID: entry.SessionID, Path: path,
			WorkspaceRoot: workspace, Title: entry.Title, Position: position,
		})
	}
	return s.ImportLegacyCatalog(ctx, sessionDir, candidates)
}
