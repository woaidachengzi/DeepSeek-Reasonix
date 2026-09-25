package sessionidentity

import (
	"context"
	"database/sql"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenSkipsWALCopyWhenNoSchemaPageWasWritten(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "desktop", "identity.sqlite")
	store, err := Open(ctx, path, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.ExecContext(ctx, `INSERT INTO sessions
		(id, relative_path, created_at_ms, updated_at_ms) VALUES
		('ordinary', 'sessions/tauri-ordinary.jsonl', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	walBytes, err := os.ReadFile(path + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	if len(walBytes) < 32 {
		t.Fatal("ordinary data write did not leave a WAL header")
	}
	pageSize := int64(binary.BigEndian.Uint32(walBytes[8:12]))
	wal, err := os.Open(path + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	canSkipCopy, err := walLeavesSchemaPageUntouched(wal, pageSize)
	if err != nil {
		t.Fatal(err)
	}
	if err := wal.Close(); err != nil {
		t.Fatal(err)
	}
	if !canSkipCopy {
		t.Fatal("ordinary row write unexpectedly modified the WAL schema page")
	}
	for name, content := range map[string][]byte{
		"incomplete": append(append([]byte(nil), walBytes...), 0),
		"bad-size": func() []byte {
			altered := append([]byte(nil), walBytes...)
			binary.BigEndian.PutUint32(altered[8:12], 513)
			return altered
		}(),
		"unknown-version": func() []byte {
			altered := append([]byte(nil), walBytes...)
			binary.BigEndian.PutUint32(altered[4:8], 3007001)
			return altered
		}(),
	} {
		fixture := filepath.Join(root, name+".wal")
		if err := os.WriteFile(fixture, content, 0o600); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(fixture)
		if err != nil {
			t.Fatal(err)
		}
		canSkipCopy, probeErr := walLeavesSchemaPageUntouched(file, pageSize)
		_ = file.Close()
		if probeErr != nil || canSkipCopy {
			t.Fatalf("%s WAL schema probe = %v, %v; want fallback", name, canSkipCopy, probeErr)
		}
	}
	wal, err = os.Open(path + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	canSkipCopy, probeErr := walLeavesSchemaPageUntouched(wal, pageSize+512)
	_ = wal.Close()
	if probeErr != nil || canSkipCopy {
		t.Fatalf("mismatched database page size probe = %v, %v; want fallback", canSkipCopy, probeErr)
	}
	reopened, err := Open(ctx, path, root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var count int
	if err := reopened.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions WHERE id='ordinary'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("reopened session count = %d, %v", count, err)
	}
}

func TestWALSchemaPageProbeFallsBackOnPageOneFrame(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "desktop", "identity.sqlite")
	store, err := Open(ctx, path, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.ExecContext(ctx, "PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	walBytes, err := os.ReadFile(path + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	if len(walBytes) <= 32 || binary.BigEndian.Uint32(walBytes[32:36]) != 1 {
		t.Fatal("future schema fixture did not write page 1 to WAL")
	}
	pageSize := int64(binary.BigEndian.Uint32(walBytes[8:12]))
	file, err := os.Open(path + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	canSkipCopy, probeErr := walLeavesSchemaPageUntouched(file, pageSize)
	_ = file.Close()
	if probeErr != nil || canSkipCopy {
		t.Fatalf("page-one WAL schema probe = %v, %v; want fallback", canSkipCopy, probeErr)
	}
}
