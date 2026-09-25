package sessionidentity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkInventory measures the complete read-only physical audit used by
// each guarded sidebar page, including path-conflict detection. Setup is not
// timed and no real profile is read.
func BenchmarkInventory(b *testing.B) {
	for _, count := range []int{50, 500} {
		b.Run(fmt.Sprintf("identities-%d", count), func(b *testing.B) {
			ctx := context.Background()
			identities, sessionDir, _ := inventoryBenchmarkFixture(b, count)
			b.Cleanup(func() { _ = identities.Close() })
			b.ResetTimer()
			for range b.N {
				if _, err := Inventory(ctx, identities, sessionDir, ""); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkInventoryWithReadOnlyOpen includes the integrity-checked read-only
// connection that the bridge creates for each physical inventory request.
func BenchmarkInventoryWithReadOnlyOpen(b *testing.B) {
	for _, count := range []int{50, 500} {
		b.Run(fmt.Sprintf("identities-%d", count), func(b *testing.B) {
			ctx := context.Background()
			identities, sessionDir, dbPath := inventoryBenchmarkFixture(b, count)
			if err := identities.Close(); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for range b.N {
				reader, err := OpenReadOnly(ctx, dbPath, filepath.Dir(sessionDir))
				if err != nil {
					b.Fatal(err)
				}
				_, inventoryErr := Inventory(ctx, reader, sessionDir, "")
				closeErr := reader.Close()
				if inventoryErr != nil {
					b.Fatal(inventoryErr)
				}
				if closeErr != nil {
					b.Fatal(closeErr)
				}
			}
		})
	}
}

// BenchmarkInventoryIdentityList isolates the path-validation work for
// registered rows before catalog and physical-file reconciliation.
func BenchmarkInventoryIdentityList(b *testing.B) {
	for _, count := range []int{50, 500} {
		b.Run(fmt.Sprintf("identities-%d", count), func(b *testing.B) {
			ctx := context.Background()
			identities, _, _ := inventoryBenchmarkFixture(b, count)
			b.Cleanup(func() { _ = identities.Close() })
			b.ResetTimer()
			for range b.N {
				if _, err := identities.List(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkListVisible measures the snapshot-bound first-page read. The
// structural digest must still cover every visible identity on each request.
func BenchmarkListVisible(b *testing.B) {
	for _, count := range []int{50, 500} {
		b.Run(fmt.Sprintf("identities-%d", count), func(b *testing.B) {
			ctx := context.Background()
			identities, _, _ := inventoryBenchmarkFixture(b, count)
			b.Cleanup(func() { _ = identities.Close() })
			b.ResetTimer()
			for range b.N {
				if _, err := identities.ListVisible(ctx, 50, nil, ""); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func inventoryBenchmarkFixture(b *testing.B, count int) (*Store, string, string) {
	b.Helper()
	ctx := context.Background()
	root := b.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		b.Fatal(err)
	}
	dbPath := filepath.Join(root, "desktop", "identity.sqlite")
	identities, err := Open(ctx, dbPath, root)
	if err != nil {
		b.Fatal(err)
	}
	tx, err := identities.db.BeginTx(ctx, nil)
	if err != nil {
		b.Fatal(err)
	}
	for index := range count {
		id := fmt.Sprintf("peer-%03d", index)
		relative := "sessions/tauri-" + id + ".jsonl"
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(relative)), []byte("{}\n"), 0o600); err != nil {
			b.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO sessions
			(id, relative_path, position, state, created_at_ms, updated_at_ms)
			VALUES (?, ?, ?, 'ready', 0, 0)`, id, relative, index); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	return identities, sessionDir, dbPath
}
