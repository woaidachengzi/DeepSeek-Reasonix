// Package sessionidentity keeps Preview session IDs independent of transcript paths.
package sessionidentity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"reasonix/internal/desktopbridge/sessionpath"

	_ "modernc.org/sqlite"
)

const schemaVersion = 9
const visibleSnapshotIndex = "sessions_visible_structure_snapshot"
const pendingDeleteCursorIndex = "sessions_deletion_recovery_cursor"
const visibleStructureRevisionTable = "session_directory_revision"

var visibleStructureRevisionTriggers = []struct {
	name string
	ddl  string
}{
	{
		name: "sessions_directory_revision_insert_v1",
		ddl: `CREATE TRIGGER IF NOT EXISTS sessions_directory_revision_insert_v1
		AFTER INSERT ON sessions BEGIN
			UPDATE session_directory_revision SET revision=revision+1 WHERE singleton=1;
		END`,
	},
	{
		name: "sessions_directory_revision_delete_v1",
		ddl: `CREATE TRIGGER IF NOT EXISTS sessions_directory_revision_delete_v1
		AFTER DELETE ON sessions BEGIN
			UPDATE session_directory_revision SET revision=revision+1 WHERE singleton=1;
		END`,
	},
	{
		name: "sessions_directory_revision_update_v1",
		ddl: `CREATE TRIGGER IF NOT EXISTS sessions_directory_revision_update_v1
		AFTER UPDATE OF id, relative_path, workspace_root, position, state ON sessions
		WHEN OLD.id IS NOT NEW.id OR OLD.relative_path IS NOT NEW.relative_path OR
			OLD.workspace_root IS NOT NEW.workspace_root OR OLD.position IS NOT NEW.position OR
			OLD.state IS NOT NEW.state
		BEGIN
			UPDATE session_directory_revision SET revision=revision+1 WHERE singleton=1;
		END`,
	},
}

const createTitleIntentTable = `CREATE TABLE session_title_intents (
	session_id TEXT PRIMARY KEY REFERENCES sessions(id),
	relative_path TEXT NOT NULL,
	expected_revision INTEGER NOT NULL CHECK (expected_revision >= 0),
	previous_sidecar_title TEXT NOT NULL,
	new_title TEXT NOT NULL,
	created_at_ms INTEGER NOT NULL
)`

var ErrPathChanged = errors.New("session identity path changed without an explicit move")
var ErrTranscriptPathConflict = errors.New("session transcript path already belongs to another identity")
var ErrSessionStateConflict = errors.New("session identity lifecycle state conflict")
var ErrTitleConflict = errors.New("session title revision changed")
var ErrTitleProtected = errors.New("session title is protected from automatic replacement")
var ErrSessionNotFound = errors.New("session identity not found")
var ErrInvalidTitle = errors.New("invalid session title")
var ErrTitleIntentRequired = errors.New("user-authority session title requires a durable rename intent")
var ErrDirectoryChanged = errors.New("session directory changed while paging")

// TitleSource records the authority of a title, not merely whether AI wrote
// its text. A user-requested AI rename has user authority; an imported legacy
// title cannot safely be classified as manual or generated.
type TitleSource string

const (
	TitleFallback      TitleSource = "fallback"
	TitleGenerated     TitleSource = "generated"
	TitleUser          TitleSource = "user"
	TitleLegacyUnknown TitleSource = "legacy_unknown"
)

// TitleOperation is intentionally an action, not a caller-provided source.
// This prevents a background generator from claiming user authority.
type TitleOperation int

const (
	TitleManualRename TitleOperation = iota
	TitleAutomaticGeneration
	TitleFirstMessage
	TitleUserRequestedGeneration
)

// SessionState is the persistent lifecycle state of a desktop session identity.
type SessionState string

const (
	StateReserved SessionState = "reserved"
	StateReady    SessionState = "ready"
	StateMissing  SessionState = "missing"
	StateDeleting SessionState = "deleting"
	StateDeleted  SessionState = "deleted"
)

type Candidate struct {
	ID            string
	Path          string
	WorkspaceRoot string
	Title         string
	Position      int
}

type Record struct {
	Candidate
	relativePath  string
	State         SessionState
	Missing       bool
	TitleSource   TitleSource
	TitleRevision int64
	CreatedAtMS   int64
	UpdatedAtMS   int64
}

// Cursor is the stable keyset boundary for visible-session pagination.
type Cursor struct {
	Position   int    `json:"position"`
	ID         string `json:"id"`
	SnapshotID string `json:"snapshotId,omitempty"`
	Total      int    `json:"total,omitempty"`
}

// Page contains one bounded page plus a cursor only when more rows remain.
type Page struct {
	Records    []Record
	NextCursor *Cursor
	Total      int
	SnapshotID string
}

// PendingDelete contains only display metadata for a deletion explicitly
// started by the user. It deliberately has no transcript path.
type PendingDelete struct {
	ID    string
	Title string
}

// PendingDeleteCursor is an opaque keyset position in the separate deletion
// recovery list. Session IDs are immutable, so retries and workbench reorders
// cannot move the continuation boundary.
type PendingDeleteCursor struct {
	ID string
}

type PendingDeletePage struct {
	Sessions   []PendingDelete
	NextCursor *PendingDeleteCursor
}

const MaxVisiblePageSize = 200
const MaxVisibleSnapshotSize = 10_000
const MaxPendingDeletes = 10_000
const MaxPendingDeletePageSize = 200

type Store struct {
	db               *sql.DB
	profileRoot      string
	identityPath     string
	identityFileInfo os.FileInfo
}

// Open opens the identity database. Existing databases require an explicit
// state/profile root; a new empty database may bind that root when its first
// import or reservation is supplied. Production bridge callers pass it here.
func Open(ctx context.Context, path string, profileRoots ...string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("session identity path is empty")
	}
	if len(profileRoots) > 1 {
		return nil, errors.New("only one session profile root may be supplied")
	}
	profileRoot := ""
	if len(profileRoots) == 1 {
		var err error
		profileRoot, err = normalizeProfileRoot(profileRoots[0])
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	unlock := lockIdentityOpen()
	defer unlock()
	if err := validateIdentityDatabaseLocation(abs, profileRoot); err != nil {
		return nil, err
	}
	if err := validateIdentityDatabasePath(abs, true); err != nil {
		return nil, err
	}
	artifactsExist, err := identityDatabaseArtifactsExist(abs)
	if err != nil {
		return nil, err
	}
	if profileRoot == "" && artifactsExist {
		return nil, errors.New("session profile root is required for an existing identity database")
	}
	if err := validateExistingIdentityDatabase(ctx, abs); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return nil, fmt.Errorf("create session identity directory: %w", err)
	}
	_ = os.Chmod(filepath.Dir(abs), 0o700)
	if err := validateIdentityDatabaseLocation(abs, profileRoot); err != nil {
		return nil, err
	}
	slash := filepath.ToSlash(abs)
	if runtime.GOOS == "windows" && len(slash) >= 2 && slash[1] == ':' {
		slash = "/" + slash
	}
	databaseURL := &url.URL{Scheme: "file", Path: slash}
	query := databaseURL.Query()
	query.Add("_pragma", "busy_timeout(2000)")
	databaseURL.RawQuery = query.Encode()
	dsn := databaseURL.String()
	db, err := sql.Open("sqlite", dsn)
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
		return fail(err)
	}
	for _, pragma := range []string{"PRAGMA busy_timeout=2000", "PRAGMA foreign_keys=ON"} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fail(err)
		}
	}
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil {
		return fail(err)
	}
	if integrity != "ok" {
		return fail(fmt.Errorf("session identity quick check: %s", integrity))
	}
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if version != 0 && profileRoot == "" {
		return fail(errors.New("session profile root is required for an existing identity database"))
	}
	if version > schemaVersion {
		return fail(fmt.Errorf("session identity schema %d is newer than supported %d", version, schemaVersion))
	}
	if version == 0 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fail(err)
		}
		if _, err := tx.ExecContext(ctx, `CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			relative_path TEXT NOT NULL UNIQUE,
			workspace_root TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			title_source TEXT NOT NULL DEFAULT 'fallback' CHECK (title_source IN ('fallback','generated','user','legacy_unknown')),
			title_revision INTEGER NOT NULL DEFAULT 0 CHECK (title_revision >= 0),
			position INTEGER NOT NULL DEFAULT 0,
			state TEXT NOT NULL DEFAULT 'ready' CHECK (state IN ('reserved','ready','missing','deleting','deleted')),
			created_at_ms INTEGER NOT NULL,
			updated_at_ms INTEGER NOT NULL
		)`); err != nil {
			_ = tx.Rollback()
			return fail(err)
		}
		if _, err := tx.ExecContext(ctx, "CREATE INDEX sessions_state_position_id ON sessions(state, position, id)"); err != nil {
			_ = tx.Rollback()
			return fail(err)
		}
		if _, err := tx.ExecContext(ctx, createTitleIntentTable); err != nil {
			_ = tx.Rollback()
			return fail(err)
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=5"); err != nil {
			_ = tx.Rollback()
			return fail(err)
		}
		if err := tx.Commit(); err != nil {
			return fail(err)
		}
		version = 5
	} else if version == 1 {
		// Legacy catalog titles may have been derived from the first user message,
		// manually renamed, or copied from a sidecar. Preserve them without
		// pretending that all nonempty values were explicitly user-authored.
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fail(err)
		}
		for _, statement := range []string{
			"ALTER TABLE sessions ADD COLUMN title_source TEXT NOT NULL DEFAULT 'fallback' CHECK (title_source IN ('fallback','generated','user','legacy_unknown'))",
			"ALTER TABLE sessions ADD COLUMN title_revision INTEGER NOT NULL DEFAULT 0 CHECK (title_revision >= 0)",
			"UPDATE sessions SET title_source=CASE WHEN title='' THEN 'fallback' ELSE 'legacy_unknown' END",
			"PRAGMA user_version=2",
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				_ = tx.Rollback()
				return fail(fmt.Errorf("migrate session identity to v2: %w", err))
			}
		}
		if err := tx.Commit(); err != nil {
			return fail(err)
		}
		version = 2
	}
	if version == 2 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fail(err)
		}
		for _, statement := range []string{
			"ALTER TABLE sessions ADD COLUMN state TEXT NOT NULL DEFAULT 'ready' CHECK (state IN ('reserved','ready','missing','deleting','deleted'))",
			"UPDATE sessions SET state=CASE WHEN missing=1 THEN 'missing' ELSE 'ready' END",
			"CREATE INDEX sessions_state_position_id ON sessions(state, position, id)",
			"PRAGMA user_version=3",
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				_ = tx.Rollback()
				return fail(fmt.Errorf("migrate session identity to v3: %w", err))
			}
		}
		if err := tx.Commit(); err != nil {
			return fail(err)
		}
		version = 3
	}
	if version == 3 {
		if profileRoot == "" {
			return fail(errors.New("session profile root is required to migrate absolute identity paths"))
		}
		rows, err := db.QueryContext(ctx, "SELECT id, path FROM sessions ORDER BY id")
		if err != nil {
			return fail(err)
		}
		type pathMigration struct{ id, relative string }
		migrations := make([]pathMigration, 0)
		for rows.Next() {
			var id, absolute string
			if err := rows.Scan(&id, &absolute); err != nil {
				_ = rows.Close()
				return fail(err)
			}
			relative, err := relativeTranscriptPath(profileRoot, id, absolute)
			if err != nil {
				_ = rows.Close()
				return fail(fmt.Errorf("migrate session identity path for %s: %w", id, err))
			}
			migrations = append(migrations, pathMigration{id: id, relative: relative})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fail(err)
		}
		if err := rows.Close(); err != nil {
			return fail(err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fail(err)
		}
		if _, err := tx.ExecContext(ctx, "ALTER TABLE sessions RENAME COLUMN path TO relative_path"); err != nil {
			_ = tx.Rollback()
			return fail(err)
		}
		for _, migration := range migrations {
			if _, err := tx.ExecContext(ctx, "UPDATE sessions SET relative_path=? WHERE id=?", migration.relative, migration.id); err != nil {
				_ = tx.Rollback()
				return fail(err)
			}
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=4"); err != nil {
			_ = tx.Rollback()
			return fail(err)
		}
		if err := tx.Commit(); err != nil {
			return fail(err)
		}
		version = 4
	}
	if version == 4 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fail(err)
		}
		if _, err := tx.ExecContext(ctx, createTitleIntentTable); err != nil {
			_ = tx.Rollback()
			return fail(fmt.Errorf("migrate session identity to v5: %w", err))
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=5"); err != nil {
			_ = tx.Rollback()
			return fail(fmt.Errorf("migrate session identity to v5: %w", err))
		}
		if err := tx.Commit(); err != nil {
			return fail(err)
		}
		version = 5
	}
	if version == 5 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fail(err)
		}
		generation := make([]byte, 16)
		if _, err := rand.Read(generation); err != nil {
			_ = tx.Rollback()
			return fail(fmt.Errorf("generate session directory revision id: %w", err))
		}
		if _, err := tx.ExecContext(ctx, `CREATE TABLE session_directory_revision (
			singleton INTEGER PRIMARY KEY CHECK (singleton=1),
			generation TEXT NOT NULL,
			revision INTEGER NOT NULL CHECK (revision >= 0)
		)`); err != nil {
			_ = tx.Rollback()
			return fail(fmt.Errorf("create session directory revision: %w", err))
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO session_directory_revision(singleton,generation,revision) VALUES(1,?,0)`, hex.EncodeToString(generation)); err != nil {
			_ = tx.Rollback()
			return fail(fmt.Errorf("initialize session directory revision: %w", err))
		}
		for _, trigger := range visibleStructureRevisionTriggers {
			if _, err := tx.ExecContext(ctx, trigger.ddl); err != nil {
				_ = tx.Rollback()
				return fail(fmt.Errorf("create session directory revision trigger %s: %w", trigger.name, err))
			}
		}
		if _, err := tx.ExecContext(ctx, `CREATE INDEX sessions_deletion_recovery_cursor
			ON sessions(id) WHERE state='deleting'`); err != nil {
			_ = tx.Rollback()
			return fail(fmt.Errorf("create session deletion recovery cursor index: %w", err))
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=6"); err != nil {
			_ = tx.Rollback()
			return fail(fmt.Errorf("migrate session identity to v6: %w", err))
		}
		if err := tx.Commit(); err != nil {
			return fail(err)
		}
		version = 6
	}
	if version == 6 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fail(err)
		}
		for _, statement := range []string{
			`CREATE TABLE session_event_streams (
				session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
				generation INTEGER NOT NULL DEFAULT 1 CHECK (generation > 0),
				projection_sequence INTEGER NOT NULL DEFAULT 0 CHECK (projection_sequence >= 0),
				projection_sha256 TEXT NOT NULL DEFAULT '',
				import_source_sha256 TEXT NOT NULL DEFAULT '',
				updated_at_ms INTEGER NOT NULL
			)`,
			`CREATE TABLE session_events (
				session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
				sequence INTEGER NOT NULL CHECK (sequence > 0),
				event_id TEXT NOT NULL,
				event_type TEXT NOT NULL,
				head_id TEXT NOT NULL DEFAULT '',
				parent_id TEXT NOT NULL DEFAULT '',
				message_id TEXT NOT NULL DEFAULT '',
				writer_id TEXT NOT NULL DEFAULT '',
				created_at_ms INTEGER NOT NULL,
				payload_json BLOB NOT NULL,
				payload_sha256 TEXT NOT NULL,
				PRIMARY KEY (session_id, sequence),
				UNIQUE (session_id, event_id)
			)`,
			`CREATE INDEX session_events_head_sequence ON session_events(session_id, head_id, sequence)`,
			`PRAGMA user_version=7`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				_ = tx.Rollback()
				return fail(fmt.Errorf("migrate session identity to v7: %w", err))
			}
		}
		if err := tx.Commit(); err != nil {
			return fail(err)
		}
		version = 7
	}
	if version == 7 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fail(err)
		}
		for _, statement := range []string{
			"ALTER TABLE session_event_streams ADD COLUMN checkpoint_sequence INTEGER NOT NULL DEFAULT 0 CHECK (checkpoint_sequence >= 0)",
			"ALTER TABLE session_event_streams ADD COLUMN checkpoint_sha256 TEXT NOT NULL DEFAULT ''",
			"PRAGMA user_version=8",
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				_ = tx.Rollback()
				return fail(fmt.Errorf("migrate session identity to v8: %w", err))
			}
		}
		if err := tx.Commit(); err != nil {
			return fail(err)
		}
		version = 8
	}
	if version == 8 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fail(err)
		}
		for _, statement := range []string{
			"ALTER TABLE session_event_streams ADD COLUMN import_verified INTEGER NOT NULL DEFAULT 0 CHECK (import_verified IN (0,1))",
			"PRAGMA user_version=9",
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				_ = tx.Rollback()
				return fail(fmt.Errorf("migrate session identity to v9: %w", err))
			}
		}
		if err := tx.Commit(); err != nil {
			return fail(err)
		}
		version = 9
	}
	// Visible-page snapshot validation hashes these columns in position/id
	// order. A covering index keeps that required full-structure check off the
	// sessions table on continuation pages while preserving its exact digest.
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS sessions_visible_structure_snapshot
		ON sessions(position, id, state, relative_path, workspace_root)`); err != nil {
		return fail(fmt.Errorf("create session identity snapshot index: %w", err))
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS sessions_deletion_recovery_cursor
		ON sessions(id) WHERE state='deleting'`); err != nil {
		return fail(fmt.Errorf("create session deletion recovery cursor index: %w", err))
	}
	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return fail(fmt.Errorf("read session identity journal mode: %w", err))
	}
	// Switching journal mode is a persistent database change and needs an
	// exclusive lock. Once WAL is established, repeating this PRAGMA on every
	// Open can race with a read-only inventory using an already-open Store and
	// fail with SQLITE_BUSY. Only perform the transition when it is necessary.
	if !strings.EqualFold(journalMode, "wal") {
		if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
			return fail(fmt.Errorf("enable session identity WAL mode: %w", err))
		}
	}
	if _, err := db.ExecContext(ctx, "PRAGMA synchronous=FULL"); err != nil {
		return fail(err)
	}
	_ = os.Chmod(abs, 0o600)
	_ = os.Chmod(abs+"-wal", 0o600)
	_ = os.Chmod(abs+"-shm", 0o600)
	return &Store{db: db, profileRoot: profileRoot}, nil
}

func normalizeProfileRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("session profile root is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve session profile root: %w", err)
	}
	return filepath.Clean(abs), nil
}

func relativeTranscriptPath(profileRoot, id, transcriptPath string) (string, error) {
	relative, _, err := relativeTranscriptPathWithIdentity(profileRoot, id, transcriptPath)
	return relative, err
}

func relativeTranscriptPathWithIdentity(profileRoot, id, transcriptPath string) (string, string, error) {
	root, err := normalizeProfileRoot(profileRoot)
	if err != nil {
		return "", "", err
	}
	path, err := filepath.Abs(transcriptPath)
	if err != nil {
		return "", "", err
	}
	resolvedRoot, err := validateCandidateWithResolvedRoot(root, Candidate{ID: id, Path: path})
	if err != nil {
		return "", "", err
	}
	return relativeTranscriptPathAfterValidation(root, resolvedRoot, path)
}

func relativeTranscriptPathAfterValidation(root, resolvedRoot, path string) (string, string, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", errors.New("transcript path escapes session profile root")
	}
	resolvedPath, err := resolveIdentityPath(path)
	if err != nil {
		return "", "", fmt.Errorf("resolve transcript path: %w", err)
	}
	resolvedRelative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || resolvedRelative == "." || resolvedRelative == ".." || filepath.IsAbs(resolvedRelative) ||
		strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) {
		return "", "", errors.New("transcript path escapes session profile root through a symlink")
	}
	return filepath.ToSlash(relative), resolvedPath, nil
}

func resolveTranscriptPath(profileRoot, id, relative string) (string, error) {
	path, _, err := resolveTranscriptPathWithIdentity(profileRoot, id, relative)
	return path, err
}

func resolveTranscriptPathWithIdentity(profileRoot, id, relative string) (string, string, error) {
	root, err := normalizeProfileRoot(profileRoot)
	if err != nil {
		return "", "", err
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", "", errors.New("session identity contains an invalid relative path")
	}
	path := filepath.Join(root, clean)
	_, resolved, err := relativeTranscriptPathWithIdentity(root, id, path)
	if err != nil {
		return "", "", err
	}
	return path, resolved, nil
}

// ensureTranscriptPathAvailable prevents two IDs from claiming one physical
// transcript through internal symlink aliases or hard links. The lexical
// relative_path UNIQUE constraint remains useful, but cannot express physical
// filesystem identity. Callers run this inside the same SQLite transaction as
// the insert so a detected conflict rolls back the complete import/reserve.
// candidateResolvedPath is freshly validated by the caller in that transaction,
// so it does not repeat the candidate's final symlink resolution here.
func ensureTranscriptPathAvailable(ctx context.Context, tx *sql.Tx, profileRoot, id, transcriptPath, candidateResolvedPath string) error {
	root, err := normalizeProfileRoot(profileRoot)
	if err != nil {
		return err
	}
	// A missing candidate cannot be a hard link or final-component symlink to
	// an existing peer. For peers in the exact same lexical parent, only an
	// identical or case-folded name can still alias; avoid resolving/statting
	// every sibling transcript on the common fresh-reservation path. Resolve
	// the parent before and after peer enumeration so a changed parent symlink
	// fails closed instead of reusing the caller's stale candidate path.
	candidateMissing := false
	if _, statErr := os.Lstat(transcriptPath); errors.Is(statErr, os.ErrNotExist) {
		candidateMissing = true
	}
	if candidateMissing {
		resolvedParent, parentErr := resolveIdentityPath(filepath.Dir(transcriptPath))
		candidateMissing = parentErr == nil && filepath.Clean(resolvedParent) == filepath.Clean(filepath.Dir(candidateResolvedPath))
	}
	var resolvedRoot string
	siblingShortcutUsed := false
	rows, err := tx.QueryContext(ctx, "SELECT id, relative_path FROM sessions WHERE id<>?", id)
	if err != nil {
		return err
	}
	type existingIdentityPath struct{ id, path, resolved string }
	var existing []existingIdentityPath
	for rows.Next() {
		var otherID, relative string
		if err := rows.Scan(&otherID, &relative); err != nil {
			_ = rows.Close()
			return err
		}
		cleanRelative := filepath.Clean(filepath.FromSlash(relative))
		if cleanRelative == "." || cleanRelative == ".." || filepath.IsAbs(cleanRelative) || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) {
			_ = rows.Close()
			return fmt.Errorf("resolve existing transcript identity %s: invalid relative path", otherID)
		}
		otherPath := filepath.Join(root, cleanRelative)
		if candidateMissing && filepath.Dir(filepath.Clean(otherPath)) == filepath.Dir(filepath.Clean(transcriptPath)) {
			if filepath.Clean(otherPath) == filepath.Clean(transcriptPath) || sameCaseInsensitivePath(otherPath, transcriptPath) {
				_ = rows.Close()
				return fmt.Errorf("%w: %s and %s", ErrTranscriptPathConflict, id, otherID)
			}
			info, statErr := os.Lstat(otherPath)
			if errors.Is(statErr, os.ErrNotExist) || (statErr == nil && info.Mode().IsRegular()) {
				siblingShortcutUsed = true
				// A missing candidate cannot alias an absent peer or a regular
				// sibling. Keep its lexical path for a final same-file recheck if
				// another writer creates the candidate during this transaction.
				existing = append(existing, existingIdentityPath{id: otherID, path: otherPath})
				continue
			}
			if statErr != nil {
				_ = rows.Close()
				return fmt.Errorf("inspect existing transcript identity %s: %w", otherID, statErr)
			}
			// Symlinks and non-regular entries still need full resolution and
			// validation, including a broken final-component symlink.
		}
		if resolvedRoot == "" {
			resolvedRoot, err = filepath.EvalSymlinks(root)
			if err != nil {
				_ = rows.Close()
				return fmt.Errorf("resolve session profile root: %w", err)
			}
		}
		resolvedOtherPath, otherResolved, resolveErr := resolveTranscriptPathWithResolvedRoot(root, resolvedRoot, otherID, relative)
		if resolveErr != nil {
			_ = rows.Close()
			return fmt.Errorf("resolve existing transcript identity %s: %w", otherID, resolveErr)
		}
		existing = append(existing, existingIdentityPath{id: otherID, path: resolvedOtherPath, resolved: otherResolved})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if siblingShortcutUsed {
		resolvedParent, parentErr := resolveIdentityPath(filepath.Dir(transcriptPath))
		if parentErr != nil || filepath.Clean(resolvedParent) != filepath.Clean(filepath.Dir(candidateResolvedPath)) {
			return fmt.Errorf("session transcript parent changed during path uniqueness check")
		}
	}
	if len(existing) == 0 {
		return nil
	}
	// Check path aliases before file identity. They also conflict when neither
	// transcript exists yet, as is common while reserving a fresh session.
	for _, other := range existing {
		if other.resolved == "" {
			continue
		}
		if candidateResolvedPath == other.resolved {
			return fmt.Errorf("%w: %s and %s", ErrTranscriptPathConflict, id, other.id)
		}
		if sameCaseInsensitivePath(candidateResolvedPath, other.resolved) {
			return fmt.Errorf("%w: case-only paths are conservatively treated as aliases on macOS/Windows (%s and %s); macOS volume case sensitivity can vary", ErrTranscriptPathConflict, id, other.id)
		}
	}
	candidateInfo, candidateErr := os.Stat(transcriptPath)
	if candidateErr != nil && !errors.Is(candidateErr, os.ErrNotExist) {
		return fmt.Errorf("inspect candidate transcript identity: %w", candidateErr)
	}
	// A missing candidate has no file identity that could match a peer. Peers
	// outside the same lexical parent were fully resolved above; same-parent
	// peers were retained for the final candidate identity check if the file
	// appears while this scan runs.
	if candidateErr == nil {
		for _, other := range existing {
			otherInfo, otherErr := os.Stat(other.path)
			if otherErr == nil && os.SameFile(candidateInfo, otherInfo) {
				return fmt.Errorf("%w: %s and %s", ErrTranscriptPathConflict, id, other.id)
			}
			if otherErr != nil && !errors.Is(otherErr, os.ErrNotExist) {
				return fmt.Errorf("inspect existing transcript identity %s: %w", other.id, otherErr)
			}
		}
	}
	// The candidate may have been created or replaced while other identities
	// were inspected. Recheck it and compare every peer again if its file
	// identity changed. This does not replace the profile writer gate, but it
	// avoids an avoidable N-fold candidate stat in the steady state.
	latestInfo, latestErr := os.Stat(transcriptPath)
	if latestErr != nil && !errors.Is(latestErr, os.ErrNotExist) {
		return fmt.Errorf("inspect candidate transcript identity: %w", latestErr)
	}
	if latestErr == nil && (candidateErr != nil || !os.SameFile(candidateInfo, latestInfo)) {
		for _, other := range existing {
			otherInfo, otherErr := os.Stat(other.path)
			if otherErr == nil && os.SameFile(latestInfo, otherInfo) {
				return fmt.Errorf("%w: %s and %s", ErrTranscriptPathConflict, id, other.id)
			}
			if otherErr != nil && !errors.Is(otherErr, os.ErrNotExist) {
				return fmt.Errorf("inspect existing transcript identity %s: %w", other.id, otherErr)
			}
		}
	}
	return nil
}

func resolveTranscriptPathWithResolvedRoot(root, resolvedRoot, id, relative string) (string, string, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", "", errors.New("session identity contains an invalid relative path")
	}
	path := filepath.Join(root, clean)
	if err := validateCandidateUnderResolvedRoot(root, resolvedRoot, Candidate{ID: id, Path: path}); err != nil {
		return "", "", err
	}
	_, resolved, err := relativeTranscriptPathAfterValidation(root, resolvedRoot, path)
	if err != nil {
		return "", "", err
	}
	return path, resolved, nil
}

// CheckTranscriptPathUnique verifies that an existing identity still has an
// exclusive physical claim to its transcript. Runtime resume uses this to
// keep legacy duplicate identities from opening the same conversation under
// multiple IDs. The transaction acquires SQLite's writer reservation before
// comparing rows so the result is serialized with import/reserve mutations.
func (s *Store) CheckTranscriptPathUnique(ctx context.Context, id, transcriptPath string) error {
	if err := validateCandidate(s.profileRoot, Candidate{ID: id, Path: transcriptPath}); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "UPDATE sessions SET updated_at_ms=updated_at_ms WHERE id=?", id); err != nil {
		return err
	}
	var storedRelative string
	if err := tx.QueryRowContext(ctx, "SELECT relative_path FROM sessions WHERE id=?", id).Scan(&storedRelative); errors.Is(err, sql.ErrNoRows) {
		return ErrSessionNotFound
	} else if err != nil {
		return err
	}
	relativePath, candidatePath, err := relativeTranscriptPathWithIdentity(s.profileRoot, id, transcriptPath)
	if err != nil {
		return err
	}
	if storedRelative != relativePath {
		return fmt.Errorf("%w: %s", ErrPathChanged, id)
	}
	if err := ensureTranscriptPathAvailable(ctx, tx, s.profileRoot, id, transcriptPath, candidatePath); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) bindProfileRoot(root string) error {
	resolved, err := normalizeProfileRoot(root)
	if err != nil {
		return err
	}
	if s.profileRoot == "" {
		s.profileRoot = resolved
		return nil
	}
	if filepath.Clean(s.profileRoot) != resolved {
		return errors.New("session profile root changed while the identity store is open")
	}
	return nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Import(ctx context.Context, sessionDir string, candidates []Candidate) error {
	if s.profileRoot == "" {
		if err := s.bindImportProfileRoot(sessionDir); err != nil {
			return err
		}
	}
	if err := normalizeImportPathRoot(s.profileRoot, sessionDir); err != nil {
		return err
	}
	return s.importCandidates(ctx, sessionDir, candidates, false, false, false)
}

// ImportLegacyCatalog migrates the host's bounded legacy catalog without
// overwriting existing identity metadata. Missing transcripts are retained as
// visible missing rows so migration never silently drops a legacy entry. A
// matching terminal identity is preserved as already processed, allowing an
// interrupted host-catalog cleanup to leave the rest of the import usable.
func (s *Store) ImportLegacyCatalog(ctx context.Context, sessionDir string, candidates []Candidate) error {
	if len(candidates) > 50 {
		return errors.New("legacy catalog contains too many sessions")
	}
	for _, candidate := range candidates {
		if !ValidWorkbenchCatalogTitle(candidate.Title) || !ValidWorkbenchCatalogWorkspaceRoot(candidate.WorkspaceRoot) {
			return errors.New("legacy catalog metadata is invalid")
		}
	}
	if s.profileRoot == "" {
		if err := s.bindImportProfileRoot(sessionDir); err != nil {
			return err
		}
	}
	if err := normalizeImportPathRoot(s.profileRoot, sessionDir); err != nil {
		return err
	}
	return s.importCandidates(ctx, sessionDir, candidates, false, true, true)
}

func normalizeImportPathRoot(profileRoot, transcriptRoot string) error {
	profile, err := normalizeProfileRoot(profileRoot)
	if err != nil {
		return err
	}
	transcripts, err := normalizeProfileRoot(transcriptRoot)
	if err != nil {
		return err
	}
	profile, err = resolveIdentityPath(profile)
	if err != nil {
		return err
	}
	transcripts, err = resolveIdentityPath(transcripts)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(profile, transcripts)
	if err != nil || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("session transcript directory escapes profile root")
	}
	return nil
}

func (s *Store) bindImportProfileRoot(sessionDir string) error {
	root := sessionDir
	if filepath.Base(filepath.Clean(sessionDir)) == "sessions" {
		root = filepath.Dir(sessionDir)
	}
	return s.bindProfileRoot(root)
}

func (s *Store) importCandidates(ctx context.Context, sessionDir string, candidates []Candidate, requirePresent, preserveMissing, preserveExisting bool) error {
	root, err := filepath.Abs(sessionDir)
	if err != nil || strings.TrimSpace(sessionDir) == "" {
		return errors.New("session directory is invalid")
	}
	prepared := make([]Record, 0, len(candidates))
	seenIDs := make(map[string]bool, len(candidates))
	seenPaths := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if err := validateCandidate(root, candidate); err != nil {
			return err
		}
		candidate.Path = filepath.Clean(candidate.Path)
		if seenIDs[candidate.ID] || seenPaths[candidate.Path] {
			return errors.New("duplicate session ID or path in import")
		}
		seenIDs[candidate.ID], seenPaths[candidate.Path] = true, true
		info, err := os.Lstat(candidate.Path)
		missing := errors.Is(err, os.ErrNotExist)
		if err != nil && !missing {
			return fmt.Errorf("inspect transcript: %w", err)
		}
		if requirePresent && missing {
			return fmt.Errorf("%w: selected transcript is missing: %s", ErrImportReviewChanged, candidate.Path)
		}
		if !missing && !info.Mode().IsRegular() {
			return fmt.Errorf("transcript is not a regular file: %s", candidate.Path)
		}
		state := StateReady
		if missing {
			state = StateMissing
		}
		prepared = append(prepared, Record{Candidate: candidate, State: state, Missing: missing})
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UnixMilli()
	for _, incoming := range prepared {
		if requirePresent {
			info, err := os.Lstat(incoming.Path)
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("%w: selected transcript changed before registration: %s", ErrImportReviewChanged, incoming.Path)
			}
		}
		var current Record
		var currentRelativePath string
		err := tx.QueryRowContext(ctx, `SELECT relative_path, workspace_root, title, position, state, created_at_ms, updated_at_ms
			FROM sessions WHERE id=?`, incoming.ID).Scan(&currentRelativePath, &current.WorkspaceRoot, &current.Title,
			&current.Position, &current.State, &current.CreatedAtMS, &current.UpdatedAtMS)
		if errors.Is(err, sql.ErrNoRows) {
			if incoming.Missing && !preserveMissing {
				continue
			}
			relativePath, candidatePath, err := relativeTranscriptPathWithIdentity(s.profileRoot, incoming.ID, incoming.Path)
			if err != nil {
				return err
			}
			if err := ensureTranscriptPathAvailable(ctx, tx, s.profileRoot, incoming.ID, incoming.Path, candidatePath); err != nil {
				return err
			}
			source := TitleFallback
			if incoming.Title != "" {
				source = TitleLegacyUnknown
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO sessions
				(id, relative_path, workspace_root, title, title_source, title_revision, position, state, created_at_ms, updated_at_ms)
				VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?)`, incoming.ID, relativePath,
				incoming.WorkspaceRoot, incoming.Title, source, incoming.Position, incoming.State, now, now)
			if err != nil {
				return fmt.Errorf("register session %s: %w", incoming.ID, err)
			}
			continue
		}
		if err != nil {
			return err
		}
		current.Path, err = resolveTranscriptPath(s.profileRoot, incoming.ID, currentRelativePath)
		if err != nil {
			return err
		}
		if current.Path != incoming.Path {
			return fmt.Errorf("%w: %s", ErrPathChanged, incoming.ID)
		}
		if preserveExisting {
			// Legacy catalog migration is idempotent and never revives or mutates
			// an existing identity. A stale JSON row left after a crash must not
			// make the rest of the import fail when its identity is already
			// deleting or tombstoned.
			continue
		}
		if current.State == StateDeleting || current.State == StateDeleted {
			return fmt.Errorf("%w: %s", ErrSessionStateConflict, incoming.ID)
		}
		// Import is registration/reconciliation, never a title command. The
		// catalog can be stale after a manual rename or automatic generation.
		incoming.Title = current.Title
		if incoming.WorkspaceRoot == "" {
			incoming.WorkspaceRoot = current.WorkspaceRoot
		}
		newState := incoming.State
		if current.State == StateReserved && incoming.Missing {
			newState = StateReserved
		}
		if current.State == StateMissing && incoming.State == StateReady && !requirePresent {
			// A file reappearing at the same path is not proof that this is the
			// original transcript. Only the reviewed, presence-checked import may
			// recover a missing identity.
			newState = StateMissing
		}
		if current.Title == incoming.Title && current.WorkspaceRoot == incoming.WorkspaceRoot &&
			current.Position == incoming.Position && current.State == newState {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET title=?, workspace_root=?, position=?, state=?, updated_at_ms=? WHERE id=?`,
			incoming.Title, incoming.WorkspaceRoot, incoming.Position, newState, now, incoming.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func validateCandidate(root string, candidate Candidate) error {
	_, err := validateCandidateWithResolvedRoot(root, candidate)
	return err
}

// validateCandidateWithResolvedRoot returns the root resolved by the parent
// path check, so callers that also validate the transcript itself need not
// resolve the same root a second time. The parent check must remain separate:
// a parent can escape the profile even if the final file links back inside.
func validateCandidateWithResolvedRoot(root string, candidate Candidate) (string, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve session root: %w", err)
	}
	if err := validateCandidateUnderResolvedRoot(root, resolvedRoot, candidate); err != nil {
		return "", err
	}
	return resolvedRoot, nil
}

func validateCandidateUnderResolvedRoot(root, resolvedRoot string, candidate Candidate) error {
	if candidate.ID == "" || len(candidate.ID) > 128 || candidate.Position < 0 {
		return errors.New("invalid session identity candidate")
	}
	for _, c := range []byte(candidate.ID) {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			return errors.New("invalid session identity candidate")
		}
	}
	if !filepath.IsAbs(candidate.Path) || filepath.Ext(candidate.Path) != ".jsonl" {
		return errors.New("transcript path is invalid")
	}
	rel, err := filepath.Rel(root, filepath.Clean(candidate.Path))
	if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("transcript path escapes session root")
	}
	// A lexical relative path is not enough: root/sessions may be a symlink
	// outside the profile. Resolve the nearest existing parent so even a
	// missing transcript below a linked directory cannot be imported later.
	parent := filepath.Dir(candidate.Path)
	for {
		resolvedParent, resolveErr := filepath.EvalSymlinks(parent)
		if resolveErr == nil {
			resolvedRel, relErr := filepath.Rel(resolvedRoot, resolvedParent)
			if relErr != nil || resolvedRel == ".." || filepath.IsAbs(resolvedRel) ||
				strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) {
				return errors.New("transcript path escapes session root through a symlink")
			}
			break
		}
		if !errors.Is(resolveErr, os.ErrNotExist) {
			return fmt.Errorf("resolve transcript parent: %w", resolveErr)
		}
		if parent == root {
			return fmt.Errorf("resolve transcript parent: %w", resolveErr)
		}
		parent = filepath.Dir(parent)
	}
	return nil
}

func (s *Store) List(ctx context.Context) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, relative_path, workspace_root, title, title_source, title_revision, position, state,
		created_at_ms, updated_at_ms FROM sessions ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []Record
	for rows.Next() {
		record, err := scanIdentityRecord(rows, s.profileRoot)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// ListPendingDeletes exposes deleting identities through a separate recovery
// path without making them openable or part of normal session pagination.
func (s *Store) ListPendingDeletes(ctx context.Context) ([]PendingDelete, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, title FROM sessions
		WHERE state='deleting' ORDER BY position, id LIMIT ?`, MaxPendingDeletes+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deletions := make([]PendingDelete, 0)
	for rows.Next() {
		var pending PendingDelete
		if err := rows.Scan(&pending.ID, &pending.Title); err != nil {
			return nil, err
		}
		if len(deletions) == MaxPendingDeletes {
			return nil, errors.New("too many pending session deletions")
		}
		// Legacy imports can retain titles that the host or UI refuses to
		// display. Keep the deletion discoverable with a neutral title without
		// rewriting the stored metadata or dropping any pending identity.
		if !ValidWorkbenchCatalogTitle(pending.Title) {
			pending.Title = ""
		}
		deletions = append(deletions, pending)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return deletions, nil
}

// ListPendingDeletesPage keeps interrupted deletion recovery visible without
// turning a large failure backlog into one unbounded response. Schema 6's
// partial ID index lets these keyset pages avoid sorting unrelated sessions.
func (s *Store) ListPendingDeletesPage(ctx context.Context, limit int, cursor *PendingDeleteCursor) (PendingDeletePage, error) {
	if limit < 1 || limit > MaxPendingDeletePageSize {
		return PendingDeletePage{}, fmt.Errorf("pending deletion page limit must be between 1 and %d", MaxPendingDeletePageSize)
	}
	if cursor != nil && !sessionpath.ValidID(cursor.ID) {
		return PendingDeletePage{}, errors.New("pending deletion page cursor is invalid")
	}

	query := `SELECT id, title FROM sessions WHERE state='deleting'`
	args := make([]any, 0, 4)
	if cursor != nil {
		query += ` AND id>?`
		args = append(args, cursor.ID)
	}
	query += ` ORDER BY id LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return PendingDeletePage{}, err
	}
	defer rows.Close()
	page := PendingDeletePage{Sessions: make([]PendingDelete, 0, limit)}
	for rows.Next() {
		var pending PendingDelete
		if err := rows.Scan(&pending.ID, &pending.Title); err != nil {
			return PendingDeletePage{}, err
		}
		if len(page.Sessions) == limit {
			last := page.Sessions[len(page.Sessions)-1]
			page.NextCursor = &PendingDeleteCursor{ID: last.ID}
			break
		}
		if !ValidWorkbenchCatalogTitle(pending.Title) {
			pending.Title = ""
		}
		page.Sessions = append(page.Sessions, pending)
	}
	if err := rows.Err(); err != nil {
		return PendingDeletePage{}, err
	}
	return page, nil
}

type identityScanner interface{ Scan(...any) error }

func scanIdentityRecord(scanner identityScanner, profileRoot string) (Record, error) {
	var record Record
	var relativePath string
	err := scanner.Scan(&record.ID, &relativePath, &record.WorkspaceRoot, &record.Title, &record.TitleSource,
		&record.TitleRevision, &record.Position, &record.State, &record.CreatedAtMS, &record.UpdatedAtMS)
	if err == nil {
		record.relativePath = relativePath
		record.Path, err = resolveTranscriptPath(profileRoot, record.ID, relativePath)
	}
	record.Missing = record.State == StateMissing
	return record, err
}

// Get returns one identity record without reconciling it against the disk.
func (s *Store) Get(ctx context.Context, id string) (Record, bool, error) {
	record, err := scanIdentityRecord(s.db.QueryRowContext(ctx, `SELECT id, relative_path, workspace_root, title, title_source,
		title_revision, position, state, created_at_ms, updated_at_ms FROM sessions WHERE id=?`, id), s.profileRoot)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	return record, true, nil
}

// ListVisible returns a keyset-paginated view of sessions. Tombstones and
// in-progress deletion rows are deliberately hidden; missing sessions remain
// visible so the host can explain and recover them.
func (s *Store) ListVisible(ctx context.Context, limit int, cursor *Cursor, workspaceRoot string) (Page, error) {
	if limit < 1 || limit > MaxVisiblePageSize {
		return Page{}, fmt.Errorf("session page limit must be between 1 and %d", MaxVisiblePageSize)
	}
	if cursor != nil && (cursor.Position < 0 || cursor.ID == "" || cursor.Total < 0 || cursor.Total > MaxVisibleSnapshotSize ||
		(cursor.SnapshotID != "" && !validSnapshotID(cursor.SnapshotID))) {
		return Page{}, errors.New("session page cursor is invalid")
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		workspaceRoot = ""
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Page{}, err
	}
	defer func() { _ = tx.Rollback() }()
	snapshotID, total, err := visibleSnapshotID(ctx, tx, workspaceRoot)
	if err != nil {
		return Page{}, err
	}
	if total < 0 {
		if cursor != nil && cursor.Total > 0 && cursor.SnapshotID != "" {
			total = cursor.Total
		} else if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions
			WHERE state NOT IN ('deleting','deleted') AND (?='' OR workspace_root=?)`, workspaceRoot, workspaceRoot).Scan(&total); err != nil {
			return Page{}, err
		}
	}
	if total > MaxVisibleSnapshotSize {
		return Page{}, fmt.Errorf("session directory exceeds the maximum snapshot size of %d", MaxVisibleSnapshotSize)
	}
	if cursor != nil && cursor.SnapshotID != "" && cursor.SnapshotID != snapshotID {
		return Page{}, ErrDirectoryChanged
	}
	query := `SELECT id, relative_path, workspace_root, title, title_source, title_revision, position, state,
		created_at_ms, updated_at_ms FROM sessions
		WHERE state NOT IN ('deleting','deleted') AND (?='' OR workspace_root=?)`
	args := []any{workspaceRoot, workspaceRoot}
	if cursor != nil {
		query += ` AND (position>? OR (position=? AND id>?))`
		args = append(args, cursor.Position, cursor.Position, cursor.ID)
	}
	query += ` ORDER BY position, id LIMIT ?`
	args = append(args, limit+1)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	page := Page{Records: make([]Record, 0, limit), Total: total, SnapshotID: snapshotID}
	for rows.Next() {
		record, err := scanIdentityRecord(rows, s.profileRoot)
		if err != nil {
			return Page{}, err
		}
		if len(page.Records) == limit {
			last := page.Records[len(page.Records)-1]
			page.NextCursor = &Cursor{Position: last.Position, ID: last.ID, SnapshotID: snapshotID, Total: total}
			break
		}
		page.Records = append(page.Records, record)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if err := rows.Close(); err != nil {
		return Page{}, err
	}
	if err := tx.Commit(); err != nil {
		return Page{}, err
	}
	return page, nil
}

// ListVisibleSnapshot reads the complete bounded visible directory and its
// structural snapshot ID in one SQLite read transaction. Its ID uses the same
// revision/hash rules as ListVisible so the host can bind its shadow audit to
// subsequent page cursors.
func (s *Store) ListVisibleSnapshot(ctx context.Context, limit int, workspaceRoot string) (Page, error) {
	if limit < 1 || limit > MaxVisibleSnapshotSize {
		return Page{}, fmt.Errorf("session directory snapshot limit must be between 1 and %d", MaxVisibleSnapshotSize)
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		workspaceRoot = ""
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Page{}, err
	}
	defer func() { _ = tx.Rollback() }()
	snapshotID, _, err := visibleSnapshotID(ctx, tx, workspaceRoot)
	if err != nil {
		return Page{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, relative_path, workspace_root, title, title_source,
		title_revision, position, state, created_at_ms, updated_at_ms FROM sessions
		WHERE state NOT IN ('deleting','deleted') AND (?='' OR workspace_root=?)
		ORDER BY position, id LIMIT ?`, workspaceRoot, workspaceRoot, limit+1)
	if err != nil {
		return Page{}, err
	}
	page := Page{Records: make([]Record, 0, limit)}
	for rows.Next() {
		if len(page.Records) == limit {
			_ = rows.Close()
			return Page{}, fmt.Errorf("session directory snapshot exceeds %d entries", limit)
		}
		var record Record
		var relativePath string
		if err := rows.Scan(&record.ID, &relativePath, &record.WorkspaceRoot, &record.Title,
			&record.TitleSource, &record.TitleRevision, &record.Position, &record.State,
			&record.CreatedAtMS, &record.UpdatedAtMS); err != nil {
			_ = rows.Close()
			return Page{}, err
		}
		record.relativePath = relativePath
		record.Path, err = resolveTranscriptPath(s.profileRoot, record.ID, relativePath)
		if err != nil {
			_ = rows.Close()
			return Page{}, err
		}
		record.Missing = record.State == StateMissing
		page.Records = append(page.Records, record)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return Page{}, err
	}
	if err := rows.Close(); err != nil {
		return Page{}, err
	}
	page.Total = len(page.Records)
	page.SnapshotID = snapshotID
	if err := tx.Commit(); err != nil {
		return Page{}, err
	}
	return page, nil
}

func visibleSnapshotID(ctx context.Context, tx *sql.Tx, workspaceRoot string) (string, int, error) {
	var generation string
	var revision int64
	var revisionAvailable bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM sqlite_master WHERE type='table' AND name=?
	)`, visibleStructureRevisionTable).Scan(&revisionAvailable); err != nil {
		return "", 0, err
	}
	if revisionAvailable {
		for _, trigger := range visibleStructureRevisionTriggers {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
				SELECT 1 FROM sqlite_master WHERE type='trigger' AND name=?
			)`, trigger.name).Scan(&exists); err != nil {
				return "", 0, err
			}
			if !exists {
				revisionAvailable = false
				break
			}
		}
	}
	if revisionAvailable {
		err := tx.QueryRowContext(ctx, `SELECT generation, revision FROM session_directory_revision WHERE singleton=1`).Scan(&generation, &revision)
		if err != nil {
			revisionAvailable = false
		}
	}
	if revisionAvailable {
		h := sha256.New()
		_, _ = h.Write([]byte("reasonix-session-directory-revision-v1\x00"))
		writeSnapshotPart(h, generation)
		var revisionBytes [8]byte
		binary.BigEndian.PutUint64(revisionBytes[:], uint64(revision))
		_, _ = h.Write(revisionBytes[:])
		writeSnapshotPart(h, workspaceRoot)
		return hex.EncodeToString(h.Sum(nil)), -1, nil
	}
	var hasCoveringIndex bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM sqlite_master WHERE type='index' AND name=?
	)`, visibleSnapshotIndex).Scan(&hasCoveringIndex); err != nil {
		return "", 0, err
	}
	query := `SELECT id, relative_path, workspace_root,
		position, state FROM sessions
		WHERE state NOT IN ('deleting','deleted') AND (?='' OR workspace_root=?)
		ORDER BY position, id`
	if hasCoveringIndex {
		query = `SELECT id, relative_path, workspace_root, position, state
			FROM sessions INDEXED BY sessions_visible_structure_snapshot
			WHERE state NOT IN ('deleting','deleted') AND (?='' OR workspace_root=?)
			ORDER BY position, id`
	}
	rows, err := tx.QueryContext(ctx, query, workspaceRoot, workspaceRoot)
	if err != nil {
		return "", 0, err
	}
	hasher := newVisibleSnapshotHasher()
	total := 0
	for rows.Next() {
		var id, relativePath, workspace, state string
		var position int64
		if err := rows.Scan(&id, &relativePath, &workspace, &position, &state); err != nil {
			_ = rows.Close()
			return "", 0, err
		}
		hasher.add(id, relativePath, workspace, position, SessionState(state))
		total++
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return "", 0, err
	}
	if err := rows.Close(); err != nil {
		return "", 0, err
	}
	return hasher.sum(), total, nil
}

func writeSnapshotPart(h hash.Hash, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = h.Write(size[:])
	_, _ = h.Write([]byte(value))
}

type visibleSnapshotHasher struct{ h hash.Hash }

func newVisibleSnapshotHasher() visibleSnapshotHasher {
	h := sha256.New()
	_, _ = h.Write([]byte("reasonix-session-directory-v2\x00"))
	return visibleSnapshotHasher{h: h}
}

func (hasher visibleSnapshotHasher) add(id, relativePath, workspaceRoot string, position int64, state SessionState) {
	writeString := func(value string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hasher.h.Write(length[:])
		_, _ = hasher.h.Write([]byte(value))
	}
	writeInt := func(value int64) {
		var encoded [8]byte
		binary.BigEndian.PutUint64(encoded[:], uint64(value))
		_, _ = hasher.h.Write(encoded[:])
	}
	writeString(id)
	writeString(relativePath)
	writeString(workspaceRoot)
	writeInt(position)
	writeString(string(state))
}

func (hasher visibleSnapshotHasher) sum() string {
	return hex.EncodeToString(hasher.h.Sum(nil))
}

func validSnapshotID(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

// Reserve claims an unused session ID and path before the first transcript or
// sidecar is written. Repeating the same reservation is idempotent.
func (s *Store) Reserve(ctx context.Context, sessionDir string, candidate Candidate) error {
	if s.profileRoot == "" {
		if err := s.bindProfileRoot(filepath.Dir(sessionDir)); err != nil {
			return err
		}
	}
	if err := normalizeImportPathRoot(s.profileRoot, sessionDir); err != nil {
		return err
	}
	if err := validateCandidate(sessionDir, candidate); err != nil {
		return err
	}
	if _, err := os.Lstat(candidate.Path); err == nil {
		return fmt.Errorf("%w: cannot reserve an existing transcript", ErrSessionStateConflict)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect reserved transcript: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := scanIdentityRecord(tx.QueryRowContext(ctx, `SELECT id, relative_path, workspace_root, title, title_source,
		title_revision, position, state, created_at_ms, updated_at_ms FROM sessions WHERE id=?`, candidate.ID), s.profileRoot)
	if err == nil {
		if current.Path != candidate.Path {
			return fmt.Errorf("%w: %s", ErrPathChanged, candidate.ID)
		}
		if current.State != StateReserved {
			return fmt.Errorf("%w: %s is already %s", ErrSessionStateConflict, candidate.ID, current.State)
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	relativePath, candidatePath, err := relativeTranscriptPathWithIdentity(s.profileRoot, candidate.ID, candidate.Path)
	if err != nil {
		return err
	}
	if err := ensureTranscriptPathAvailable(ctx, tx, s.profileRoot, candidate.ID, candidate.Path, candidatePath); err != nil {
		return err
	}
	var position int
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM sessions").Scan(&position); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	_, err = tx.ExecContext(ctx, `INSERT INTO sessions
		(id, relative_path, workspace_root, title, title_source, title_revision, position, state, created_at_ms, updated_at_ms)
		VALUES (?, ?, ?, ?, 'fallback', 0, ?, 'reserved', ?, ?)`, candidate.ID, relativePath,
		candidate.WorkspaceRoot, candidate.Title, position, now, now)
	if err != nil {
		return fmt.Errorf("reserve session identity: %w", err)
	}
	return tx.Commit()
}

// MarkReady advances a reserved identity after a transcript has been observed
// and successfully loaded. Repeating it for a ready record is harmless.
func (s *Store) MarkReady(ctx context.Context, id, transcriptPath string) error {
	if err := validateCandidate(s.profileRoot, Candidate{ID: id, Path: transcriptPath}); err != nil {
		return err
	}
	info, err := os.Lstat(transcriptPath)
	if err != nil || !info.Mode().IsRegular() {
		if err == nil {
			err = errors.New("transcript is not a regular file")
		}
		return fmt.Errorf("mark session ready: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Acquire SQLite's writer reservation before reading the lifecycle state.
	// Without this no-op write, concurrent MarkReady/BeginDelete calls can both
	// establish read snapshots and then one fails to promote its snapshot with
	// SQLITE_BUSY instead of observing the committed fence.
	if _, err := tx.ExecContext(ctx, "UPDATE sessions SET updated_at_ms=updated_at_ms WHERE id=?", id); err != nil {
		return err
	}
	var storedRelative string
	var state SessionState
	err = tx.QueryRowContext(ctx, "SELECT relative_path, state FROM sessions WHERE id=?", id).Scan(&storedRelative, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s cannot become ready", ErrSessionStateConflict, id)
	}
	if err != nil {
		return err
	}
	relativePath, candidatePath, err := relativeTranscriptPathWithIdentity(s.profileRoot, id, transcriptPath)
	if err != nil {
		return err
	}
	if storedRelative != relativePath {
		return fmt.Errorf("%w: %s", ErrPathChanged, id)
	}
	if state == StateReady {
		return tx.Commit()
	}
	if state != StateReserved {
		return fmt.Errorf("%w: %s cannot become ready from %s", ErrSessionStateConflict, id, state)
	}
	if err := ensureTranscriptPathAvailable(ctx, tx, s.profileRoot, id, transcriptPath, candidatePath); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE sessions SET state='ready', updated_at_ms=?
		WHERE id=? AND relative_path=? AND state='reserved'`, time.Now().UnixMilli(), id, relativePath)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("%w: %s cannot become ready", ErrSessionStateConflict, id)
	}
	return tx.Commit()
}

// MarkMissing records that a previously ready transcript disappeared.
func (s *Store) MarkMissing(ctx context.Context, id, transcriptPath string) error {
	relativePath, err := relativeTranscriptPath(s.profileRoot, id, transcriptPath)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(transcriptPath); err == nil {
		return fmt.Errorf("%w: transcript still exists", ErrSessionStateConflict)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect missing transcript: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET state='missing', updated_at_ms=?
		WHERE id=? AND relative_path=? AND state='ready'`, time.Now().UnixMilli(), id, relativePath)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 1 {
		return nil
	}
	record, exists, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if exists && record.Path == transcriptPath && record.State == StateMissing {
		return nil
	}
	return fmt.Errorf("%w: %s cannot become missing", ErrSessionStateConflict, id)
}

// HasRegisteredID checks whether an ID has ever been claimed by this identity
// store. A missing transcript does not release its ID for a fresh session.
// This works on OpenReadOnly stores and does not reconcile or modify rows.
func (s *Store) HasRegisteredID(ctx context.Context, id string) (bool, error) {
	var registered bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id=?)`, id).Scan(&registered)
	return registered, err
}

// SetTitle writes generated and fallback titles directly. User-authority
// titles must use the durable title-intent transaction so a crash between
// SQLite and the legacy sidecar has recoverable evidence. The caller names
// the action, never the desired source, and supplies the revision it observed.
// The revision and protection policy are checked by the UPDATE.
func (s *Store) SetTitle(ctx context.Context, id string, expectedRevision int64, title string, operation TitleOperation) error {
	if strings.TrimSpace(title) == "" || !utf8.ValidString(title) || utf8.RuneCountInString(title) > 120 ||
		strings.IndexFunc(title, unicode.IsControl) >= 0 {
		return ErrInvalidTitle
	}
	if expectedRevision < 0 {
		return ErrTitleConflict
	}
	var source TitleSource
	var guard string
	switch operation {
	case TitleManualRename, TitleUserRequestedGeneration:
		return ErrTitleIntentRequired
	case TitleAutomaticGeneration:
		source = TitleGenerated
		guard = "title_source IN ('fallback','generated') AND NOT EXISTS (SELECT 1 FROM session_title_intents WHERE session_id=sessions.id)"
	case TitleFirstMessage:
		source = TitleFallback
		guard = "title_source='fallback' AND title='' AND NOT EXISTS (SELECT 1 FROM session_title_intents WHERE session_id=sessions.id)"
	default:
		return errors.New("invalid session title operation")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET title=?, title_source=?,
		title_revision=title_revision+1, updated_at_ms=? WHERE id=? AND title_revision=? AND `+guard,
		title, source, time.Now().UnixMilli(), id, expectedRevision)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 1 {
		return nil
	}
	var revision int64
	if err := s.db.QueryRowContext(ctx, `SELECT title_revision FROM sessions WHERE id=?`, id).Scan(&revision); errors.Is(err, sql.ErrNoRows) {
		return ErrSessionNotFound
	} else if err != nil {
		return err
	}
	if revision != expectedRevision {
		return ErrTitleConflict
	}
	var pending bool
	if err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM session_title_intents WHERE session_id=?)", id).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return ErrTitleConflict
	}
	return ErrTitleProtected
}
