package sessionidentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const offlineSnapshotVersion = 1

// SnapshotFile is one verified member of an offline profile backup. Paths are
// relative to the snapshot directory, never absolute restore destinations.
type SnapshotFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// SnapshotManifest is written last. Consumers must still call
// VerifyOfflineSnapshot before using it: a backup can be damaged after the
// manifest is published.
type SnapshotManifest struct {
	Version       int            `json:"version"`
	SourceProfile string         `json:"sourceProfile"`
	SourceCatalog string         `json:"sourceCatalog"`
	Directories   []string       `json:"directories"`
	Files         []SnapshotFile `json:"files"`
}

// CreateOfflineSnapshot copies the entire Preview profile plus its host-owned
// workbench catalog into a new private directory outside the profile. The
// caller must first stop all profile writers: existing binaries do not honor
// a common profile lock, so this function intentionally does not claim that a
// lock taken here would make a running profile consistent. It never removes or
// changes source files. On failure a partial directory is left without a
// manifest and must not be used for recovery.
func CreateOfflineSnapshot(ctx context.Context, profileRoot, catalogPath, destinationParent string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	root, err := existingPlainDirectory(profileRoot)
	if err != nil {
		return "", fmt.Errorf("profile root: %w", err)
	}
	if filepath.Dir(root) == root {
		return "", errors.New("filesystem root cannot be a profile snapshot source")
	}
	catalog, err := existingPlainFile(catalogPath)
	if err != nil {
		return "", fmt.Errorf("workbench catalog: %w", err)
	}
	destination, err := existingPlainDirectory(destinationParent)
	if err != nil {
		return "", fmt.Errorf("snapshot destination: %w", err)
	}
	if withinDirectory(root, destination) {
		return "", errors.New("snapshot destination must be outside the profile")
	}
	snapshotDir, err := os.MkdirTemp(destination, "reasonix-session-snapshot-")
	if err != nil {
		return "", fmt.Errorf("create snapshot directory: %w", err)
	}
	if err := os.Chmod(snapshotDir, 0o700); err != nil {
		return snapshotDir, err
	}
	manifest := SnapshotManifest{Version: offlineSnapshotVersion, SourceProfile: root, SourceCatalog: catalog,
		Directories: []string{"profile", "catalog"}}
	profileCopy := filepath.Join(snapshotDir, "profile")
	if err := os.Mkdir(profileCopy, 0o700); err != nil {
		return snapshotDir, err
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("snapshot refuses symlink: %s", path)
		}
		dest := filepath.Join(profileCopy, rel)
		if entry.IsDir() {
			if err := os.Mkdir(dest, 0o700); err != nil {
				return err
			}
			manifest.Directories = append(manifest.Directories, filepath.ToSlash(filepath.Join("profile", rel)))
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("snapshot refuses non-regular file: %s", path)
		}
		item, err := copyVerifiedSnapshotFile(ctx, path, dest)
		if err != nil {
			return err
		}
		item.Path = filepath.ToSlash(filepath.Join("profile", rel))
		manifest.Files = append(manifest.Files, item)
		return nil
	})
	if err != nil {
		return snapshotDir, fmt.Errorf("copy profile into incomplete snapshot %s: %w", snapshotDir, err)
	}
	if err := os.Mkdir(filepath.Join(snapshotDir, "catalog"), 0o700); err != nil {
		return snapshotDir, err
	}
	item, err := copyVerifiedSnapshotFile(ctx, catalog, filepath.Join(snapshotDir, "catalog", "workbench-sessions.json"))
	if err != nil {
		return snapshotDir, fmt.Errorf("copy catalog into incomplete snapshot %s: %w", snapshotDir, err)
	}
	item.Path = "catalog/workbench-sessions.json"
	manifest.Files = append(manifest.Files, item)
	sort.Strings(manifest.Directories)
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return snapshotDir, err
	}
	encoded = append(encoded, '\n')
	manifestPath := filepath.Join(snapshotDir, "manifest.json")
	file, err := os.OpenFile(manifestPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return snapshotDir, err
	}
	if _, err := file.Write(encoded); err != nil {
		_ = file.Close()
		return snapshotDir, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return snapshotDir, err
	}
	if err := file.Close(); err != nil {
		return snapshotDir, err
	}
	if _, err := VerifyOfflineSnapshot(ctx, snapshotDir); err != nil {
		return snapshotDir, fmt.Errorf("verify incomplete snapshot %s: %w", snapshotDir, err)
	}
	return snapshotDir, nil
}

// VerifyOfflineSnapshot checks the manifest and every member before a recovery
// drill. It does not write or restore the source profile.
func VerifyOfflineSnapshot(ctx context.Context, snapshotDir string) (SnapshotManifest, error) {
	root, err := existingPlainDirectory(snapshotDir)
	if err != nil {
		return SnapshotManifest{}, err
	}
	manifestPath, err := existingPlainFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return SnapshotManifest{}, fmt.Errorf("snapshot is incomplete: %w", err)
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		return SnapshotManifest{}, err
	}
	encoded, err := io.ReadAll(io.LimitReader(file, 16<<20+1))
	_ = file.Close()
	if err != nil || len(encoded) > 16<<20 {
		return SnapshotManifest{}, errors.New("snapshot manifest is unreadable or too large")
	}
	var manifest SnapshotManifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		return SnapshotManifest{}, err
	}
	if manifest.Version != offlineSnapshotVersion || len(manifest.Files) == 0 {
		return SnapshotManifest{}, errors.New("snapshot manifest version or contents are invalid")
	}
	directorySeen := make(map[string]bool, len(manifest.Directories))
	for _, name := range manifest.Directories {
		rel := filepath.FromSlash(name)
		if rel == "" || filepath.IsAbs(rel) || filepath.Clean(rel) != rel || directorySeen[rel] ||
			!(name == "profile" || name == "catalog" || strings.HasPrefix(name, "profile/")) {
			return SnapshotManifest{}, errors.New("snapshot manifest contains an invalid directory")
		}
		directorySeen[rel] = true
		if err := rejectSymlinkComponents(root, rel); err != nil {
			return SnapshotManifest{}, err
		}
		info, err := os.Lstat(filepath.Join(root, rel))
		if err != nil || !info.IsDir() {
			return SnapshotManifest{}, fmt.Errorf("snapshot directory missing or changed: %s", name)
		}
	}
	if !directorySeen["profile"] || !directorySeen["catalog"] {
		return SnapshotManifest{}, errors.New("snapshot manifest omits required directories")
	}
	seen := make(map[string]bool, len(manifest.Files))
	for _, item := range manifest.Files {
		if err := ctx.Err(); err != nil {
			return SnapshotManifest{}, err
		}
		rel := filepath.FromSlash(item.Path)
		if rel == "" || filepath.IsAbs(rel) || filepath.Clean(rel) != rel ||
			strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || seen[rel] ||
			!(strings.HasPrefix(item.Path, "profile/") || item.Path == "catalog/workbench-sessions.json") {
			return SnapshotManifest{}, errors.New("snapshot manifest contains an invalid path")
		}
		seen[rel] = true
		path := filepath.Join(root, rel)
		if !withinDirectory(root, path) {
			return SnapshotManifest{}, errors.New("snapshot member escapes its directory")
		}
		if err := rejectSymlinkComponents(root, rel); err != nil {
			return SnapshotManifest{}, err
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != item.Size {
			return SnapshotManifest{}, fmt.Errorf("snapshot member missing or changed: %s", item.Path)
		}
		digest, err := fileSHA256(ctx, path)
		if err != nil || digest != item.SHA256 {
			return SnapshotManifest{}, fmt.Errorf("snapshot member hash mismatch: %s", item.Path)
		}
	}
	if !seen[filepath.FromSlash("catalog/workbench-sessions.json")] {
		return SnapshotManifest{}, errors.New("snapshot manifest omits the workbench catalog")
	}
	// Completeness is the other half of verification: every regular file and
	// directory under the snapshot root must appear in the manifest. Without
	// this walk a manifest that simply forgot a session file would still
	// verify, and StageOfflineSnapshot would silently drop that file.
	diskFiles := make(map[string]bool)
	diskDirs := make(map[string]bool)
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("snapshot contains an unlisted symlink: %s", filepath.ToSlash(rel))
		}
		slashed := filepath.ToSlash(rel)
		if entry.IsDir() {
			diskDirs[slashed] = true
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("snapshot contains a non-regular member: %s", slashed)
		}
		// The manifest is the index itself; it is not a profile member.
		if slashed == "manifest.json" {
			return nil
		}
		diskFiles[slashed] = true
		return nil
	})
	if walkErr != nil {
		return SnapshotManifest{}, fmt.Errorf("walk snapshot directory: %w", walkErr)
	}
	for name := range diskDirs {
		if !directorySeen[filepath.FromSlash(name)] {
			return SnapshotManifest{}, fmt.Errorf("snapshot directory is missing from the manifest: %s", name)
		}
	}
	for name := range diskFiles {
		if !seen[filepath.FromSlash(name)] {
			return SnapshotManifest{}, fmt.Errorf("snapshot file is missing from the manifest: %s", name)
		}
	}
	for name := range seen {
		if !diskFiles[name] {
			return SnapshotManifest{}, fmt.Errorf("manifest member is missing on disk: %s", filepath.ToSlash(name))
		}
	}
	return manifest, nil
}

// StageOfflineSnapshot proves a snapshot can be restored by copying it into a
// newly allocated directory. It never overwrites a live profile or catalog.
// The result contains profile/ and catalog/workbench-sessions.json, ready for
// inspection before any separately authorized recovery operation.
func StageOfflineSnapshot(ctx context.Context, snapshotDir, destinationParent string) (string, error) {
	manifest, err := VerifyOfflineSnapshot(ctx, snapshotDir)
	if err != nil {
		return "", err
	}
	source, err := existingPlainDirectory(snapshotDir)
	if err != nil {
		return "", err
	}
	destination, err := existingPlainDirectory(destinationParent)
	if err != nil {
		return "", err
	}
	if withinDirectory(source, destination) {
		return "", errors.New("recovery staging must be outside the snapshot")
	}
	staged, err := os.MkdirTemp(destination, "reasonix-session-recovery-")
	if err != nil {
		return "", err
	}
	for _, name := range manifest.Directories {
		if err := os.MkdirAll(filepath.Join(staged, filepath.FromSlash(name)), 0o700); err != nil {
			return staged, fmt.Errorf("incomplete recovery staging %s: %w", staged, err)
		}
	}
	for _, item := range manifest.Files {
		rel := filepath.FromSlash(item.Path)
		copyPath := filepath.Join(staged, rel)
		if err := os.MkdirAll(filepath.Dir(copyPath), 0o700); err != nil {
			return staged, fmt.Errorf("incomplete recovery staging %s: %w", staged, err)
		}
		copied, err := copyVerifiedSnapshotFile(ctx, filepath.Join(source, rel), copyPath)
		if err != nil || copied.Size != item.Size || copied.SHA256 != item.SHA256 {
			return staged, fmt.Errorf("incomplete recovery staging %s: member %s changed", staged, item.Path)
		}
	}
	return staged, nil
}

func existingPlainDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("path is not a plain directory")
	}
	return filepath.EvalSymlinks(abs)
}

func existingPlainFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("path is not a plain file")
	}
	return filepath.EvalSymlinks(abs)
}

func withinDirectory(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || (!filepath.IsAbs(rel) && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func copyVerifiedSnapshotFile(ctx context.Context, source, destination string) (SnapshotFile, error) {
	before, err := os.Lstat(source)
	if err != nil || !before.Mode().IsRegular() {
		return SnapshotFile{}, fmt.Errorf("snapshot source is not a regular file: %s", source)
	}
	reader, err := os.Open(source)
	if err != nil {
		return SnapshotFile{}, err
	}
	defer reader.Close()
	opened, err := reader.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return SnapshotFile{}, fmt.Errorf("snapshot source changed before copy: %s", source)
	}
	writer, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return SnapshotFile{}, err
	}
	written, copyErr := io.Copy(writer, contextReader{ctx: ctx, reader: reader})
	if copyErr == nil {
		mode := os.FileMode(0o600)
		if before.Mode().Perm()&0o111 != 0 {
			mode |= 0o100
		}
		copyErr = writer.Chmod(mode)
	}
	if copyErr == nil {
		copyErr = writer.Sync()
	}
	closeErr := writer.Close()
	if copyErr != nil {
		return SnapshotFile{}, copyErr
	}
	if closeErr != nil {
		return SnapshotFile{}, closeErr
	}
	after, err := os.Lstat(source)
	if err != nil || !os.SameFile(before, after) || after.Size() != written {
		return SnapshotFile{}, fmt.Errorf("snapshot source changed during copy: %s", source)
	}
	copyHash, err := fileSHA256(ctx, destination)
	if err != nil {
		return SnapshotFile{}, err
	}
	sourceHash, err := fileSHA256(ctx, source)
	if err != nil || sourceHash != copyHash {
		return SnapshotFile{}, fmt.Errorf("snapshot source changed during verification: %s", source)
	}
	return SnapshotFile{Size: written, SHA256: copyHash}, nil
}

func rejectSymlinkComponents(root, rel string) error {
	path := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("snapshot member contains a symlink: %s", rel)
		}
	}
	return nil
}
