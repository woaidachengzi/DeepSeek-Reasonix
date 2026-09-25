package sessionidentity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
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
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("session identity path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	unlock := lockIdentityOpen()
	defer unlock()
	if err := validateIdentityDatabasePath(abs, false); err != nil {
		return nil, err
	}
	if len(profileRoots) != 1 {
		return nil, errors.New("session profile root is required for read-only identity access")
	}
	profileRoot, err := normalizeProfileRoot(profileRoots[0])
	if err != nil {
		return nil, err
	}
	if err := validateIdentityDatabaseLocation(abs, profileRoot); err != nil {
		return nil, err
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
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(cause error) (*Store, error) {
		_ = db.Close()
		return nil, cause
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
	if version != schemaVersion {
		return fail(fmt.Errorf("read-only session inventory requires schema %d; found %d", schemaVersion, version))
	}
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil {
		return fail(fmt.Errorf("check read-only session identity integrity: %w", err))
	}
	if integrity != "ok" {
		return fail(fmt.Errorf("session identity quick check: %s", integrity))
	}
	return &Store{db: db, profileRoot: profileRoot}, nil
}
