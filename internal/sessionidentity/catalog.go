package sessionidentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/config"
)

type catalogEntry struct {
	SessionID     string `json:"sessionId"`
	Title         string `json:"title"`
	WorkspaceRoot string `json:"workspaceRoot"`
}

func (s *Store) ImportWorkbenchCatalog(ctx context.Context, previewRoot, catalogPath string) error {
	file, err := os.Open(catalogPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open workbench catalog: %w", err)
	}
	defer file.Close()
	encoded, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil {
		return fmt.Errorf("read workbench catalog: %w", err)
	}
	if len(encoded) > 1<<20 {
		return errors.New("workbench catalog is too large")
	}
	var entries []catalogEntry
	if err := json.Unmarshal(encoded, &entries); err != nil {
		return fmt.Errorf("decode workbench catalog: %w", err)
	}
	if len(entries) > 50 {
		return errors.New("workbench catalog contains too many sessions")
	}
	candidates := make([]Candidate, 0, len(entries))
	for position, entry := range entries {
		root := strings.TrimSpace(entry.WorkspaceRoot)
		dir := filepath.Join(previewRoot, "sessions")
		if root != "" {
			abs, err := filepath.Abs(root)
			if err != nil {
				return fmt.Errorf("resolve workbench workspace: %w", err)
			}
			root = abs
			dir = filepath.Join(previewRoot, "projects", config.WorkspaceSlug(root), "sessions")
		}
		candidates = append(candidates, Candidate{
			ID: entry.SessionID, Path: filepath.Join(dir, "tauri-"+entry.SessionID+".jsonl"),
			WorkspaceRoot: root, Title: entry.Title, Position: position,
		})
	}
	return s.Import(ctx, previewRoot, candidates)
}
