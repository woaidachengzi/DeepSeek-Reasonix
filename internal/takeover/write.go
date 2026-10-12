package takeover

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/fileutil"
	"reasonix/internal/profilegate"
	"reasonix/internal/sessionarchive"
	"reasonix/internal/sessionidentity"
)

type Receipt struct {
	Version        int       `json:"version"`
	ProfileRoot    string    `json:"profileRoot"`
	BackupSnapshot string    `json:"backupSnapshot"`
	Rows           []Row     `json:"rows"`
	CompletedAt    time.Time `json:"completedAt"`
}

// Write publishes new Tauri transcripts only in the selected original/staged
// profile. Original Wails records and config remain untouched. A failed import
// deliberately leaves no completion receipt, so the installation cannot start.
func Write(ctx context.Context, profile, snapshot string, rows []Row) (Receipt, error) {
	return write(ctx, profile, snapshot, rows, false)
}

func WriteVerified(ctx context.Context, profile, snapshot string, rows []Row) (Receipt, error) {
	return write(ctx, profile, snapshot, rows, true)
}

func write(ctx context.Context, profile, snapshot string, rows []Row, verifySource bool) (Receipt, error) {
	root, err := filepath.Abs(profile)
	if err != nil {
		return Receipt{}, err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Receipt{}, fmt.Errorf("takeover profile must be a plain existing directory")
	}
	release, err := profilegate.TryAcquireDesktop(root, root)
	if err != nil {
		return Receipt{}, err
	}
	defer release()
	if verifySource {
		manifest, err := sessionidentity.VerifyOfflineSnapshot(ctx, snapshot)
		if err != nil {
			return Receipt{}, err
		}
		for _, member := range manifest.Files {
			if len(member.Path) < len("profile/") || member.Path[:len("profile/")] != "profile/" {
				continue
			}
			rel := filepath.FromSlash(member.Path[len("profile/"):])
			file, err := fileutil.OpenFileBeneath(root, rel)
			if err != nil {
				return Receipt{}, err
			}
			info, err := file.Stat()
			if err != nil || !info.Mode().IsRegular() || info.Size() != member.Size {
				file.Close()
				return Receipt{}, fmt.Errorf("destination source no longer matches the backed-up regular file")
			}
			digest := sha256.New()
			written, copyErr := io.Copy(digest, file)
			closeErr := file.Close()
			if copyErr != nil {
				return Receipt{}, copyErr
			}
			if closeErr != nil {
				return Receipt{}, closeErr
			}
			if written != member.Size || hex.EncodeToString(digest.Sum(nil)) != member.SHA256 {
				return Receipt{}, fmt.Errorf("destination source changed since backup; create a fresh snapshot before takeover")
			}
		}
	}
	receiptPath := filepath.Join(root, "tauri-takeover-v1.json")
	database := filepath.Join(root, "desktop", "session-state-v1.sqlite")
	for _, path := range []string{receiptPath, database, database + "-wal", database + "-shm"} {
		if _, err := os.Lstat(path); err == nil {
			return Receipt{}, fmt.Errorf("takeover target already contains Tauri state; refusing replacement")
		} else if !os.IsNotExist(err) {
			return Receipt{}, err
		}
	}
	if len(rows) == 0 {
		return Receipt{}, fmt.Errorf("no authoritative source sessions selected")
	}
	dir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Receipt{}, err
	}
	candidates := make([]sessionidentity.Candidate, 0, len(rows))
	seen := map[string]bool{}
	for position, row := range rows {
		if err := ctx.Err(); err != nil {
			return Receipt{}, err
		}
		if seen[row.SessionID] {
			return Receipt{}, fmt.Errorf("duplicate imported identity")
		}
		seen[row.SessionID] = true
		path, err := sessionpath.TranscriptPath(dir, row.SessionID)
		if err != nil {
			return Receipt{}, err
		}
		if row.OriginalRelativePath != "" {
			path = filepath.Join(root, filepath.FromSlash(row.OriginalRelativePath))
			bound, err := sessionpath.LegacyID(root, path)
			if err != nil || bound != row.SessionID {
				return Receipt{}, fmt.Errorf("original transcript no longer matches its imported identity")
			}
			if _, err := agent.LoadSession(path); err != nil {
				return Receipt{}, fmt.Errorf("original selected transcript could not be loaded: %w", err)
			}
			candidates = append(candidates, sessionidentity.Candidate{ID: row.SessionID, Path: path, WorkspaceRoot: row.WorkspaceRoot, Title: row.Title, Position: position})
			continue
		}
		if err := agent.SaveImportedProjection(path, row.Export.Messages, row.Export.ModelMessages, "", row.Export.ModelRef); err != nil {
			return Receipt{}, err
		}
		preview, turns := agent.SessionPreviewFromMessages(row.Export.Messages)
		scope := "project"
		if row.WorkspaceRoot == "" {
			scope = "global"
		}
		if err := agent.SaveBranchMetaPreserveUpdated(path, agent.BranchMeta{ID: agent.BranchID(path), Scope: scope, WorkspaceRoot: row.WorkspaceRoot, TopicID: row.TopicID, TopicTitle: row.Title, CustomTitle: row.Title, Name: row.Title, Model: row.Export.ModelRef, Preview: preview, Turns: turns, CreatedAt: row.CreatedAt, SchemaVersion: agent.BranchMetaCountsVersion}); err != nil {
			return Receipt{}, err
		}
		candidates = append(candidates, sessionidentity.Candidate{ID: row.SessionID, Path: path, WorkspaceRoot: row.WorkspaceRoot, Title: row.Title, Position: position})
	}
	identities, err := sessionidentity.Open(ctx, database, root)
	if err != nil {
		return Receipt{}, err
	}
	err = identities.Import(ctx, root, candidates)
	closeErr := identities.Close()
	if err != nil {
		return Receipt{}, err
	}
	if closeErr != nil {
		return Receipt{}, closeErr
	}
	for _, row := range rows {
		if row.Archived {
			if _, err := sessionarchive.Set(filepath.Join(root, "desktop", "session-archives-v1.json"), sessionarchive.Entry{SessionID: row.SessionID, Title: row.Title, WorkspaceRoot: row.WorkspaceRoot}, true); err != nil {
				return Receipt{}, err
			}
		}
	}
	receipt := Receipt{Version: 1, ProfileRoot: root, BackupSnapshot: snapshot, Rows: rows, CompletedAt: time.Now().UTC()}
	encoded, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return Receipt{}, err
	}
	if err := fileutil.AtomicWriteFileStrict(receiptPath, append(encoded, '\n'), 0600); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}
