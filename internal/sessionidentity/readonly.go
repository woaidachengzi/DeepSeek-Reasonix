package sessionidentity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "modernc.org/sqlite"
)

// OpenReadOnly opens an existing identity database for inventory inspection.
// Unlike Open, it never creates a directory or database, changes journal mode,
// or migrates a schema. Callers should pass nil to Inventory when no database
// exists yet; a missing or unsupported database is not silently replaced.
func OpenReadOnly(ctx context.Context, path string, profileRoots ...string) (*Store, error) {
	store, exists, err := openReadOnly(ctx, path, profileRoots...)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("inspect session identity database: %w", os.ErrNotExist)
	}
	return store, nil
}

// OpenReadOnlyIfExists opens an existing identity database without a separate
// existence probe. A genuinely absent database returns (nil, false, nil);
// leftover SQLite sidecars and invalid paths remain errors.
func OpenReadOnlyIfExists(ctx context.Context, path, profileRoot string) (*Store, bool, error) {
	return openReadOnly(ctx, path, profileRoot)
}

// ValidateReadOnlyPath confirms that a cached read-only Store still names the
// same regular database file beneath the same profile root. SQLite writes
// preserve the file identity, while an offline database replacement requires
// the bridge to reopen and revalidate the new file.
func (s *Store) ValidateReadOnlyPath(path, profileRoot string) error {
	if s == nil || s.db == nil || s.identityFileInfo == nil || s.identityPath == "" {
		return errors.New("read-only session identity path cannot be validated")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if filepath.Clean(abs) != s.identityPath {
		return errors.New("session identity database path changed while open")
	}
	root, err := normalizeProfileRoot(profileRoot)
	if err != nil {
		return err
	}
	if root != s.profileRoot {
		return errors.New("session profile root changed while identity store is open")
	}
	if err := validateIdentityDatabaseLocation(abs, root); err != nil {
		return err
	}
	current, err := inspectIdentityDatabasePath(abs, false)
	if err != nil {
		return err
	}
	if !os.SameFile(s.identityFileInfo, current) {
		return errors.New("session identity database file changed while open")
	}
	return nil
}

func openReadOnly(ctx context.Context, path string, profileRoots ...string) (*Store, bool, error) {
	if strings.TrimSpace(path) == "" {
		return nil, false, errors.New("session identity path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, false, err
	}
	unlock := lockIdentityOpen()
	defer unlock()
	if len(profileRoots) != 1 {
		info, inspectErr := inspectIdentityDatabasePath(abs, true)
		if inspectErr != nil {
			return nil, false, inspectErr
		}
		if info == nil {
			return nil, false, nil
		}
		return nil, false, errors.New("session profile root is required for read-only identity access")
	}
	profileRoot, err := normalizeProfileRoot(profileRoots[0])
	if err != nil {
		return nil, false, err
	}
	if err := validateIdentityDatabaseLocation(abs, profileRoot); err != nil {
		return nil, false, err
	}
	identityInfo, err := inspectIdentityDatabasePath(abs, true)
	if err != nil {
		return nil, false, err
	}
	if identityInfo == nil {
		return nil, false, nil
	}
	slash := filepath.ToSlash(abs)
	if runtime.GOOS == "windows" && len(slash) >= 2 && slash[1] == ':' {
		slash = "/" + slash
	}
	u := &url.URL{Scheme: "file", Path: slash}
	query := u.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "busy_timeout(2000)")
	u.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, false, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(cause error) (*Store, bool, error) {
		_ = db.Close()
		return nil, false, cause
	}
	if err := db.PingContext(ctx); err != nil {
		return fail(fmt.Errorf("ping read-only session identity store: %w", err))
	}
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout=2000"); err != nil {
		return fail(fmt.Errorf("configure read-only session identity busy timeout: %w", err))
	}
	// query_only protects this connection even if a future caller accidentally
	// invokes a write method on the returned Store.
	if _, err := db.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
		return fail(fmt.Errorf("set read-only session identity query mode: %w", err))
	}
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fail(fmt.Errorf("read session identity schema version: %w", err))
	}
	if version != 5 && version != 6 && version != 7 && version != 8 && version != schemaVersion {
		return fail(fmt.Errorf("read-only session inventory requires schema 5, 6, 7, 8, or %d; found %d", schemaVersion, version))
	}
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil {
		return fail(fmt.Errorf("check read-only session identity integrity: %w", err))
	}
	if integrity != "ok" {
		return fail(fmt.Errorf("session identity quick check: %s", integrity))
	}
	openedInfo, err := inspectIdentityDatabasePath(abs, false)
	if err != nil {
		return fail(err)
	}
	if !os.SameFile(identityInfo, openedInfo) {
		return fail(errors.New("session identity database file changed while opening"))
	}
	return &Store{
		db:               db,
		profileRoot:      profileRoot,
		identityPath:     filepath.Clean(abs),
		identityFileInfo: openedInfo,
	}, true, nil
}
