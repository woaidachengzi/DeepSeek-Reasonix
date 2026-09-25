package sessionidentity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkCheckTranscriptPathUnique measures the physical-identity guard on
// realistic bounded Preview directory sizes. Fixture setup is outside the
// timer; the benchmark still performs the full transaction and filesystem
// checks used by resume, not a simplified path-comparison surrogate.
func BenchmarkCheckTranscriptPathUnique(b *testing.B) {
	for _, count := range []int{50, 500} {
		b.Run(fmt.Sprintf("identities-%d", count), func(b *testing.B) {
			ctx := context.Background()
			root := b.TempDir()
			sessionDir := filepath.Join(root, "sessions")
			if err := os.MkdirAll(sessionDir, 0o700); err != nil {
				b.Fatal(err)
			}
			store, err := Open(ctx, filepath.Join(root, "desktop", "identity.sqlite"), root)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = store.Close() })
			tx, err := store.db.BeginTx(ctx, nil)
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
			path := filepath.Join(sessionDir, "tauri-peer-000.jsonl")
			b.ResetTimer()
			for range b.N {
				if err := store.CheckTranscriptPathUnique(ctx, "peer-000", path); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkCheckMissingTranscriptPathUnique covers a reserved identity whose
// transcript has not been written yet, as happens during first open.
func BenchmarkCheckMissingTranscriptPathUnique(b *testing.B) {
	for _, count := range []int{50, 500} {
		b.Run(fmt.Sprintf("identities-%d", count), func(b *testing.B) {
			ctx := context.Background()
			store, sessionDir, _ := inventoryBenchmarkFixture(b, count)
			b.Cleanup(func() { _ = store.Close() })
			path := filepath.Join(sessionDir, "tauri-reserved.jsonl")
			if err := store.Reserve(ctx, sessionDir, Candidate{ID: "reserved", Path: path}); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for range b.N {
				if err := store.CheckTranscriptPathUnique(ctx, "reserved", path); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
