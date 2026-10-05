package sessionarchive

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchivePersistenceRetryRestoreAndRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archives.json")
	entry := Entry{SessionID: "first", Title: "History", WorkspaceRoot: "/work"}
	initial, err := Set(path, entry, true)
	if err != nil || len(initial) != 1 {
		t.Fatalf("archive: %v %v", initial, err)
	}
	stat, _ := os.Stat(path)
	if stat.Mode().Perm() != 0o600 {
		t.Fatal("archive is not owner-only")
	}
	retry, err := Set(path, entry, true)
	if err != nil || retry[0].ArchivedAtMS != initial[0].ArchivedAtMS {
		t.Fatal("retry changed archive identity/date")
	}
	loaded, err := List(path)
	if err != nil || len(loaded) != 1 || loaded[0] != initial[0] {
		t.Fatal("restart lost archive")
	}
	if _, err := Set(path, Entry{SessionID: "../escape"}, true); err == nil {
		t.Fatal("invalid identifier accepted")
	}
	if restored, err := Set(path, entry, false); err != nil || len(restored) != 0 {
		t.Fatal("restore failed")
	}
	if err := os.WriteFile(path, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Set(path, entry, true); err == nil {
		t.Fatal("corrupt file was silently overwritten")
	}
	if data, _ := os.ReadFile(path); string(data) != "broken" {
		t.Fatal("corrupt state changed")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "secret"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := List(path); err == nil {
		t.Fatal("archive symlink followed")
	}
}

func TestArchiveReadDoesNotCreateProfileAndRejectsParentLink(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "missing", "archives.json")
	if entries, err := List(path); err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("read created directory")
	}
	linked := filepath.Join(root, "linked")
	if err := os.Symlink(t.TempDir(), linked); err != nil {
		t.Fatal(err)
	}
	if _, err := List(filepath.Join(linked, "archives.json")); err == nil {
		t.Fatal("linked parent accepted")
	}
}
