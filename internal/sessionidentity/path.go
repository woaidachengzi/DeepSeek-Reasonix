package sessionidentity

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var identityOpenMu sync.Mutex

// sameCaseInsensitivePath conservatively treats case-only path differences as
// aliases on the platforms whose normal filesystems are case-insensitive.
// Volume-level case sensitivity can vary on macOS, so this can reject a valid
// case-distinct pair on a case-sensitive volume; it avoids allowing two absent
// reservations to race into one file on the common case-insensitive volumes.
func sameCaseInsensitivePath(a, b string) bool {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return false
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// lockIdentityOpen serializes connection setup in this process. A canonical
// per-path key is not stable while an ancestor is being created: macOS may
// report /var for one opener and /private/var for another. Connection setup
// is short and profilegate still excludes other bridge processes, so one
// process-wide lock is safer than allowing simultaneous schema creation.
func lockIdentityOpen() func() {
	identityOpenMu.Lock()
	return identityOpenMu.Unlock
}

// validateIdentityDatabasePath rejects symlinked or non-regular SQLite files
// before SQLite opens the database or its journal sidecars. This is a
// best-effort path check; it does not eliminate a concurrent path swap.
func validateIdentityDatabasePath(path string, allowMissingDatabase bool) error {
	_, err := inspectIdentityDatabasePath(path, allowMissingDatabase)
	return err
}

func inspectIdentityDatabasePath(path string, allowMissingDatabase bool) (os.FileInfo, error) {
	var databaseInfo os.FileInfo
	sidecarExists := false
	for index, candidate := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect session identity database path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("session identity database path is a symlink: %s", candidate)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("session identity database path is not a regular file: %s", candidate)
		}
		if index == 0 {
			databaseInfo = info
		} else {
			sidecarExists = true
		}
	}
	if databaseInfo == nil && sidecarExists {
		return nil, errors.New("session identity database is missing while SQLite sidecars remain")
	}
	if databaseInfo == nil && !allowMissingDatabase {
		return nil, fmt.Errorf("inspect session identity database: %w", os.ErrNotExist)
	}
	return databaseInfo, nil
}

// IdentityDatabaseExists distinguishes a genuinely new profile from a missing
// main database with leftover WAL/SHM/journal evidence. Read-only callers may
// return an empty directory only in the former case.
func IdentityDatabaseExists(path, profileRoot string) (bool, error) {
	if strings.TrimSpace(profileRoot) == "" {
		return false, errors.New("session profile root is required to inspect identity database")
	}
	if err := validateIdentityDatabaseLocation(path, profileRoot); err != nil {
		return false, err
	}
	info, err := inspectIdentityDatabasePath(path, true)
	if err != nil {
		return false, err
	}
	return info != nil, nil
}

func identityDatabaseArtifactsExist(path string) (bool, error) {
	for _, candidate := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		if _, err := os.Lstat(candidate); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("inspect session identity database path: %w", err)
		}
	}
	return false, nil
}

// validateExistingIdentityDatabase validates SQLite's stable file header before
// SQLite opens an existing database. A future user_version must be rejected
// before a WAL-mode open can create or update its shared-memory sidecars; an
// existing empty or truncated file must not be mistaken for a new database.
func validateExistingIdentityDatabase(ctx context.Context, path string) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil // A genuinely new database will be created by SQLite.
	} else if err != nil {
		return fmt.Errorf("inspect session identity database header: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read session identity database header: %w", err)
	}
	defer file.Close()
	header := make([]byte, 100)
	if _, err := io.ReadFull(file, header); err != nil || string(header[:16]) != "SQLite format 3\x00" {
		return errors.New("existing session identity database has an invalid SQLite header")
	}
	version := binary.BigEndian.Uint32(header[60:64])
	if version > schemaVersion {
		return fmt.Errorf("session identity schema %d is newer than supported %d", version, schemaVersion)
	}
	walInfo, err := os.Stat(path + "-wal")
	if errors.Is(err, os.ErrNotExist) || (err == nil && walInfo.Size() == 0) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect session identity WAL before open: %w", err)
	}
	walFile, err := os.Open(path + "-wal")
	if err != nil {
		return fmt.Errorf("inspect session identity WAL before open: %w", err)
	}
	defer walFile.Close()
	var magic [4]byte
	if _, err := io.ReadFull(walFile, magic[:]); err != nil {
		return fmt.Errorf("inspect session identity WAL header: %w", err)
	}
	if binary.BigEndian.Uint32(magic[:]) != 0x377f0682 && binary.BigEndian.Uint32(magic[:]) != 0x377f0683 {
		return errors.New("session identity WAL header is invalid")
	}
	// user_version lives on SQLite page 1. A WAL containing only complete
	// non-page-1 frames cannot override the version already checked in the
	// main file header. Unknown or changing WAL layouts keep the copy check.
	mainPageSize := int64(binary.BigEndian.Uint16(header[16:18]))
	if mainPageSize == 1 {
		mainPageSize = 65536
	}
	canSkipCopy, err := walLeavesSchemaPageUntouched(walFile, mainPageSize)
	if err != nil {
		return fmt.Errorf("inspect session identity WAL frames: %w", err)
	}
	if canSkipCopy {
		return nil
	}
	if err := inspectWALSchemaOnCopy(ctx, path); err != nil {
		return err
	}
	return nil
}

// walLeavesSchemaPageUntouched is deliberately conservative: it only accepts
// an unchanged, complete WAL with valid frame spacing and no page-1 frame.
// It does not try to decide which frames SQLite would commit or recover.
func walLeavesSchemaPageUntouched(file *os.File, mainPageSize int64) (bool, error) {
	before, err := file.Stat()
	if err != nil {
		return false, err
	}
	if before.Size() < 32 {
		return false, nil
	}
	var header [32]byte
	if _, err := file.ReadAt(header[:], 0); err != nil {
		return false, err
	}
	magic := binary.BigEndian.Uint32(header[:4])
	if magic != 0x377f0682 && magic != 0x377f0683 {
		return false, nil
	}
	// Do not interpret frame offsets for a WAL format newer than the layout
	// documented by SQLite (format version 3007000).
	if binary.BigEndian.Uint32(header[4:8]) != 3007000 {
		return false, nil
	}
	pageSize := int64(binary.BigEndian.Uint32(header[8:12]))
	if pageSize != mainPageSize || pageSize < 512 || pageSize > 65536 || pageSize&(pageSize-1) != 0 {
		return false, nil
	}
	frameSize := pageSize + 24
	if (before.Size()-32)%frameSize != 0 {
		return false, nil
	}
	var pageNumber [4]byte
	for offset := int64(32); offset < before.Size(); offset += frameSize {
		if _, err := file.ReadAt(pageNumber[:], offset); err != nil {
			return false, err
		}
		if binary.BigEndian.Uint32(pageNumber[:]) <= 1 {
			return false, nil
		}
	}
	after, err := file.Stat()
	if err != nil {
		return false, err
	}
	return before.Size() == after.Size() && before.ModTime() == after.ModTime() && os.SameFile(before, after), nil
}

func inspectWALSchemaOnCopy(ctx context.Context, path string) error {
	dir, err := os.MkdirTemp("", "reasonix-session-schema-check-")
	if err != nil {
		return fmt.Errorf("create isolated schema check: %w", err)
	}
	defer os.RemoveAll(dir)
	copyPath := filepath.Join(dir, "identity.sqlite")
	for _, suffix := range []string{"", "-wal", "-journal"} {
		if err := copyIdentityArtifact(path+suffix, copyPath+suffix); err != nil {
			if suffix != "" && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("copy identity database for schema check: %w", err)
		}
	}
	slash := filepath.ToSlash(copyPath)
	if runtime.GOOS == "windows" && len(slash) >= 2 && slash[1] == ':' {
		slash = "/" + slash
	}
	uri := (&url.URL{Scheme: "file", Path: slash}).String()
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return err
	}
	defer db.Close()
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read session identity schema from WAL snapshot: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("session identity schema %d is newer than supported %d", version, schemaVersion)
	}
	return nil
}

func copyIdentityArtifact(source, destination string) error {
	reader, err := os.Open(source)
	if err != nil {
		return err
	}
	defer reader.Close()
	writer, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(writer, reader)
	closeErr := writer.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func validateIdentityDatabaseLocation(path, profileRoot string) error {
	if profileRoot == "" {
		return nil
	}
	root, err := resolveIdentityPath(profileRoot)
	if err != nil {
		return fmt.Errorf("resolve session profile root: %w", err)
	}
	parent, err := resolveIdentityPath(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("resolve session identity directory: %w", err)
	}
	relative, err := filepath.Rel(root, parent)
	if err != nil || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("session identity database directory escapes profile root")
	}
	return nil
}

func resolveIdentityPath(path string) (string, error) {
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	current := path
	var missing []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		// A failed resolution is not proof that the path is absent: a
		// dangling symlink itself exists and must remain an error. On the
		// common existing-path case EvalSymlinks already checked the path,
		// so avoid an additional Lstat before it.
		if _, statErr := os.Lstat(current); statErr == nil {
			return "", resolveErr
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path, nil
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}
