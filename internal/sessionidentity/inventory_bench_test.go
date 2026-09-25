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
			root := b.TempDir()
			sessionDir := filepath.Join(root, "sessions")
			if err := os.MkdirAll(sessionDir, 0o700); err != nil {
				b.Fatal(err)
			}
			identities, err := Open(ctx, filepath.Join(root, "desktop", "identity.sqlite"), root)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = identities.Close() })
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
			b.ResetTimer()
			for range b.N {
				if _, err := Inventory(ctx, identities, sessionDir, ""); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
