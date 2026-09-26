package sessionidentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionEventBatchAppendIsAtomicAndIdempotent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := Open(ctx, filepath.Join(root, "desktop", "session-state-v1.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Reserve(ctx, sessionDir, Candidate{ID: "tauri-events", Path: filepath.Join(sessionDir, "tauri-events.jsonl")}); err != nil {
		t.Fatal(err)
	}
	events := []SessionEvent{
		{ID: "e1", Type: "log", CreatedAt: 1, Payload: json.RawMessage(`{"schema_version":2,"type":"log","at":"1970-01-01T00:00:00.001Z","generation":1}`)},
		{ID: "m1", Type: "message", HeadID: "main", ParentID: "p0", WriterID: "w1", CreatedAt: 2, Payload: json.RawMessage(`{"schema_version":2,"type":"message","id":"m1","head":"main","parent":"p0","writer":"w1","at":"1970-01-01T00:00:00.002Z"}`)},
	}
	last, err := s.AppendEvents(ctx, "tauri-events", events)
	if err != nil || last != 2 {
		t.Fatalf("AppendEvents = %d, %v; want sequence 2", last, err)
	}
	if last, err = s.AppendEvents(ctx, "tauri-events", events); err != nil || last != 2 {
		t.Fatalf("retry AppendEvents = %d, %v; want sequence 2", last, err)
	}
	changed := append([]SessionEvent(nil), events...)
	changed[1].Payload = json.RawMessage(`{"schema_version":2,"type":"message","id":"m1","head":"main","parent":"p0","writer":"w1","at":"1970-01-01T00:00:00.002Z","target":"changed"}`)
	if _, err := s.AppendEvents(ctx, "tauri-events", changed); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("conflicting retry error = %v; want ErrEventConflict", err)
	}
	rows, err := s.ReadEvents(ctx, "tauri-events", 0, 10)
	if err != nil || len(rows) != 2 || rows[0].Sequence != 1 || rows[1].Sequence != 2 {
		t.Fatalf("ReadEvents = %#v, %v", rows, err)
	}
	if string(rows[1].Payload) != string(events[1].Payload) {
		t.Fatalf("stored payload changed: %s", rows[1].Payload)
	}
}

func TestSessionEventAppendLargeBatchUsesBoundedSQLiteLookups(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := Open(ctx, filepath.Join(root, "desktop", "session-state-v1.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Reserve(ctx, sessionDir, Candidate{ID: "tauri-large-batch", Path: filepath.Join(sessionDir, "tauri-large-batch.jsonl")}); err != nil {
		t.Fatal(err)
	}
	events := make([]SessionEvent, 1200)
	for i := range events {
		id := fmt.Sprintf("event-%d", i)
		events[i] = SessionEvent{Type: "probe", Payload: json.RawMessage(fmt.Sprintf(`{"type":"probe","id":%q}`, id))}
	}
	last, err := s.AppendEvents(ctx, "tauri-large-batch", events)
	if err != nil || last != int64(len(events)) {
		t.Fatalf("AppendEvents large batch = %d, %v; want %d", last, err, len(events))
	}
	if last, err = s.AppendEvents(ctx, "tauri-large-batch", events); err != nil || last != int64(len(events)) {
		t.Fatalf("idempotent large batch = %d, %v; want %d", last, err, len(events))
	}
}

func TestSessionEventImportFingerprintIsIdempotentAndImmutable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := Open(ctx, filepath.Join(root, "desktop", "session-state-v1.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Reserve(ctx, sessionDir, Candidate{ID: "tauri-import", Path: filepath.Join(sessionDir, "tauri-import.jsonl")}); err != nil {
		t.Fatal(err)
	}
	events := []SessionEvent{{Type: "log", CreatedAt: 1, Payload: json.RawMessage(`{"schema_version":2,"type":"log","at":"1970-01-01T00:00:00.001Z","generation":1}`)}}
	inserted, err := s.ImportEvents(ctx, "tauri-import", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1, events)
	if err != nil || !inserted {
		t.Fatalf("ImportEvents = %v, %v; want inserted", inserted, err)
	}
	inserted, err = s.ImportEvents(ctx, "tauri-import", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1, events)
	if err != nil || inserted {
		t.Fatalf("idempotent ImportEvents = %v, %v; want existing", inserted, err)
	}
	if _, err := s.ImportEvents(ctx, "tauri-import", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, events); !errors.Is(err, ErrEventImportConflict) {
		t.Fatalf("changed source error = %v; want ErrEventImportConflict", err)
	}
}

func TestSessionEventRotationUsesGenerationCASAndProjectionWatermark(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := Open(ctx, filepath.Join(root, "desktop", "session-state-v1.sqlite"), root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Reserve(ctx, sessionDir, Candidate{ID: "tauri-rotate", Path: filepath.Join(sessionDir, "tauri-rotate.jsonl")}); err != nil {
		t.Fatal(err)
	}
	initial := []SessionEvent{{Type: "log", CreatedAt: 1, Payload: json.RawMessage(`{"schema_version":2,"type":"log","at":"1970-01-01T00:00:00.001Z","generation":1}`)}}
	if _, err := s.AppendEvents(ctx, "tauri-rotate", initial); err != nil {
		t.Fatal(err)
	}
	status, err := s.EventStreamStatus(ctx, "tauri-rotate")
	if err != nil || status.Generation != 1 || status.LastSequence != 1 {
		t.Fatalf("initial event stream status = %#v, %v", status, err)
	}
	rotated := []SessionEvent{{Type: "log", CreatedAt: 2, Payload: json.RawMessage(`{"schema_version":2,"type":"log","at":"1970-01-01T00:00:00.002Z","generation":2}`)}}
	generation, sequence, err := s.ReplaceEvents(ctx, "tauri-rotate", status.Generation, status.LastSequence, rotated)
	if err != nil || generation != 2 || sequence != 1 {
		t.Fatalf("ReplaceEvents = generation %d sequence %d, %v", generation, sequence, err)
	}
	if _, _, err := s.ReplaceEvents(ctx, "tauri-rotate", status.Generation, status.LastSequence, initial); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("stale rotation error = %v; want ErrEventConflict", err)
	}
	if err := s.MarkEventProjection(ctx, "tauri-rotate", generation, sequence, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"); err != nil {
		t.Fatal(err)
	}
	status, err = s.EventStreamStatus(ctx, "tauri-rotate")
	if err != nil || status.ProjectionSequence != 1 || status.ProjectionSHA256 == "" {
		t.Fatalf("projected event stream status = %#v, %v", status, err)
	}
}

func TestIdentitySchemaSixMigratesToEventStoreSchemaSeven(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(root, "desktop", "session-state-v1.sqlite")
	s, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"DROP TABLE session_events",
		"DROP TABLE session_event_streams",
		"PRAGMA user_version=6",
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("prepare schema v6: %v", err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, dbPath, root)
	if err != nil {
		t.Fatalf("migrate schema v6: %v", err)
	}
	defer s.Close()
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 9 {
		t.Fatalf("schema version = %d, %v; want 9", version, err)
	}
}
