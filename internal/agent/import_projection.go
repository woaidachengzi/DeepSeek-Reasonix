package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"reasonix/internal/provider"
)

// SaveImportedProjection preserves full display history and the original model
// workset independently. It is an offline migration operation: callers own the
// profile gate and must not expose the new identity before every write succeeds.
// Existing transcripts/checkpoints are never replaced. On a later write error,
// a partial new transcript can remain and must be treated as an incomplete import.
func SaveImportedProjection(path string, history, model []provider.Message, workspaceID, modelRef string) error {
	if !filepath.IsAbs(path) || filepath.Ext(path) != ".jsonl" || len(history) == 0 || len(model) == 0 {
		return fmt.Errorf("import requires an absolute JSONL path and nonempty history/model projections")
	}
	for _, candidate := range []string{path, ContextStatePath(path)} {
		if _, err := os.Lstat(candidate); err == nil {
			return fmt.Errorf("import destination already exists: %s", candidate)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	session := NewSession("")
	session.Messages = append([]provider.Message(nil), history...)
	if err := session.SaveIfAbsent(path); err != nil {
		return err
	}
	// Save assigns stable message IDs. Hash the exact saved canonical prefix,
	// rather than the unnormalized legacy payload, for restart authentication.
	saved, err := LoadSession(path)
	if err != nil {
		return err
	}
	canonical := saved.Snapshot()
	now := time.Now().UTC()
	state := CompactionState{
		SchemaVersion:  compactionStateSchemaCurrent,
		Generation:     1,
		PromptCacheKey: promptCacheKey(workspaceID, BranchID(path), modelRef),
		Projection: ContextProjection{
			Messages:          append([]provider.Message(nil), model...),
			ProjectionVersion: 1, CoveredCount: len(canonical),
			CoveredPrefixHash: coveredPrefixHash(canonical, len(canonical)),
			PinnedContextHash: pinnedContextCoverageHash(canonical, len(canonical)), CreatedAt: now,
		},
		UpdatedAt: now,
	}
	if err := SaveCompactionState(path, state); err != nil {
		return err
	}
	return os.Chmod(ContextStatePath(path), 0600)
}
