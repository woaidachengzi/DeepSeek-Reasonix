package sessionidentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	reordered := []SessionEvent{events[1], events[0]}
	if _, err := s.AppendEvents(ctx, "tauri-events", reordered); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("reordered batch retry error = %v; want ErrEventConflict", err)
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

func TestSessionEventAppendLimitsAreAtomicAndKeepExactRetriesIdempotent(t *testing.T) {
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
	if err := s.Reserve(ctx, sessionDir, Candidate{ID: "tauri-bounded-events", Path: filepath.Join(sessionDir, "tauri-bounded-events.jsonl")}); err != nil {
		t.Fatal(err)
	}
	if err := s.Reserve(ctx, sessionDir, Candidate{ID: "tauri-byte-limited-events", Path: filepath.Join(sessionDir, "tauri-byte-limited-events.jsonl")}); err != nil {
		t.Fatal(err)
	}
	first := SessionEvent{Type: "probe", Payload: json.RawMessage(`{"type":"probe","id":"first"}`)}
	firstBytes := int64(len(first.Payload) + 1)
	if last, err := s.AppendEventsWithinLimits(ctx, "tauri-bounded-events", []SessionEvent{first}, 1, firstBytes); err != nil || last != 1 {
		t.Fatalf("append event at exact limits = %d, %v; want 1, nil", last, err)
	}
	if last, err := s.AppendEventsWithinLimits(ctx, "tauri-bounded-events", []SessionEvent{first}, 1, firstBytes); err != nil || last != 1 {
		t.Fatalf("exact limited retry = %d, %v; want 1, nil", last, err)
	}
	second := SessionEvent{Type: "probe", Payload: json.RawMessage(`{"type":"probe","id":"second"}`)}
	if _, err := s.AppendEventsWithinLimits(ctx, "tauri-bounded-events", []SessionEvent{second}, 1, firstBytes+int64(len(second.Payload)+1)); !errors.Is(err, ErrEventLimitExceeded) {
		t.Fatalf("record limit error = %v; want ErrEventLimitExceeded", err)
	}
	last, err := s.LastEventSequence(ctx, "tauri-bounded-events")
	if err != nil || last != 1 {
		t.Fatalf("sequence after record-limit rejection = %d, %v; want 1, nil", last, err)
	}

	byteLimit := firstBytes + int64(len(second.Payload))
	if last, err := s.AppendEventsWithinLimits(ctx, "tauri-byte-limited-events", []SessionEvent{first}, 10, byteLimit); err != nil || last != 1 {
		t.Fatalf("append first byte-limited event = %d, %v; want 1, nil", last, err)
	}
	if _, err := s.AppendEventsWithinLimits(ctx, "tauri-byte-limited-events", []SessionEvent{second}, 10, byteLimit); !errors.Is(err, ErrEventLimitExceeded) {
		t.Fatalf("projection byte limit error = %v; want ErrEventLimitExceeded", err)
	}
	last, err = s.LastEventSequence(ctx, "tauri-byte-limited-events")
	if err != nil || last != 1 {
		t.Fatalf("sequence after byte-limit rejection = %d, %v; want 1, nil", last, err)
	}
	rows, err := s.ReadEvents(ctx, "tauri-byte-limited-events", 0, 0)
	if err != nil || len(rows) != 1 || rows[0].ID != "first" {
		t.Fatalf("events after byte-limit rejection = %#v, %v; want original event only", rows, err)
	}
}

func TestSessionEventWritersFailClosedOnCorruptExistingStream(t *testing.T) {
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
	for _, id := range []string{"tauri-corrupt-append", "tauri-corrupt-rotation"} {
		if err := s.Reserve(ctx, sessionDir, Candidate{ID: id, Path: filepath.Join(sessionDir, id+".jsonl")}); err != nil {
			t.Fatal(err)
		}
	}
	initial := SessionEvent{ID: "first", Type: "probe", Payload: json.RawMessage(`{"type":"probe","id":"first"}`)}
	if _, err := s.AppendEvents(ctx, "tauri-corrupt-append", []SessionEvent{initial}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendEvents(ctx, "tauri-corrupt-rotation", []SessionEvent{initial}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE session_events SET payload_json=? WHERE session_id=?`,
		[]byte(`{"type":"probe","id":"tampered"}`), "tauri-corrupt-append"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE session_events SET payload_json=? WHERE session_id=?`,
		[]byte(`{"type":"probe","id":"tampered"}`), "tauri-corrupt-rotation"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendEvents(ctx, "tauri-corrupt-append", []SessionEvent{{Type: "probe", Payload: json.RawMessage(`{"type":"probe","id":"second"}`)}}); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("append to corrupt stream error = %v; want ErrEventConflict", err)
	}
	if _, _, err := s.ReplaceEvents(ctx, "tauri-corrupt-rotation", 1, 1, []SessionEvent{{Type: "probe", Payload: json.RawMessage(`{"type":"probe","id":"replacement"}`)}}); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("rotation of corrupt stream error = %v; want ErrEventConflict", err)
	}
	for _, id := range []string{"tauri-corrupt-append", "tauri-corrupt-rotation"} {
		status, err := s.EventStreamStatus(ctx, id)
		if err != nil || status.Generation != 1 || status.LastSequence != 1 {
			t.Fatalf("corrupt stream %s changed after rejected writer: status=%#v err=%v", id, status, err)
		}
		var count int
		if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM session_events WHERE session_id=?", id).Scan(&count); err != nil || count != 1 {
			t.Fatalf("corrupt stream %s row count = %d, %v; want 1", id, count, err)
		}
	}
}

func TestSessionEventConcurrentWritersSerializeSequencesAndExactRetries(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(root, "desktop", "session-state-v1.sqlite")
	first, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := first.Reserve(ctx, sessionDir, Candidate{ID: "tauri-concurrent", Path: filepath.Join(sessionDir, "tauri-concurrent.jsonl")}); err != nil {
		t.Fatal(err)
	}

	const writers = 16
	start := make(chan struct{})
	errCh := make(chan error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		writer := first
		if i%2 != 0 {
			writer = second
		}
		id := fmt.Sprintf("parallel-%02d", i)
		event := SessionEvent{ID: id, Type: "probe", Payload: json.RawMessage(fmt.Sprintf(`{"type":"probe","id":%q}`, id))}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := writer.AppendEvents(ctx, "tauri-concurrent", []SessionEvent{event}); err != nil {
				errCh <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent distinct append: %v", err)
	}

	shared := SessionEvent{ID: "shared-retry", Type: "probe", Payload: json.RawMessage(`{"type":"probe","id":"shared-retry"}`)}
	start = make(chan struct{})
	results := make(chan struct {
		last int64
		err  error
	}, writers)
	for i := 0; i < writers; i++ {
		writer := first
		if i%2 != 0 {
			writer = second
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			last, err := writer.AppendEvents(ctx, "tauri-concurrent", []SessionEvent{shared})
			results <- struct {
				last int64
				err  error
			}{last: last, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for result := range results {
		if result.err != nil || result.last != writers+1 {
			t.Fatalf("concurrent exact retry = sequence %d, %v; want %d", result.last, result.err, writers+1)
		}
	}
	rows, err := first.ReadEvents(ctx, "tauri-concurrent", 0, writers+1)
	if err != nil || len(rows) != writers+1 {
		t.Fatalf("events after concurrent writes = %d, %v; want %d", len(rows), err, writers+1)
	}
	for i, row := range rows {
		if row.Sequence != int64(i+1) {
			t.Fatalf("concurrent event sequence at index %d = %d", i, row.Sequence)
		}
	}
}

func TestSessionEventAppendRollsBackOnSQLiteFull(t *testing.T) {
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
	if err := s.Reserve(ctx, sessionDir, Candidate{ID: "tauri-full", Path: filepath.Join(sessionDir, "tauri-full.jsonl")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendEvents(ctx, "tauri-full", []SessionEvent{{
		ID: "committed", Type: "probe", Payload: json.RawMessage(`{"type":"probe","payload":"seed"}`),
	}}); err != nil {
		t.Fatalf("append seed event: %v", err)
	}
	var pageCount int64
	if err := s.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("PRAGMA max_page_count=%d", pageCount)); err != nil {
		t.Fatalf("set SQLite page limit: %v", err)
	}
	largePayload, err := json.Marshal(struct {
		Type    string `json:"type"`
		Payload string `json:"payload"`
	}{Type: "probe", Payload: strings.Repeat("x", 2<<20)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AppendEvents(ctx, "tauri-full", []SessionEvent{{
		ID: "must-rollback", Type: "probe", Payload: largePayload,
	}})
	if err == nil {
		t.Fatal("append succeeded after SQLite reached max_page_count")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "full") {
		t.Fatalf("append failure = %v; want SQLite full-database error", err)
	}
	last, err := s.LastEventSequence(ctx, "tauri-full")
	if err != nil || last != 1 {
		t.Fatalf("last sequence after SQLITE_FULL = %d, %v; want original committed sequence 1", last, err)
	}
	rows, err := s.ReadEvents(ctx, "tauri-full", 0, 10)
	if err != nil || len(rows) != 1 || rows[0].ID != "committed" {
		t.Fatalf("events after SQLITE_FULL = %#v, %v; want only the seed event", rows, err)
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

func TestSessionEventConcurrentRotationAllowsOneCASWinner(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(root, "desktop", "session-state-v1.sqlite")
	first, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Open(ctx, dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := first.Reserve(ctx, sessionDir, Candidate{ID: "tauri-rotate-race", Path: filepath.Join(sessionDir, "tauri-rotate-race.jsonl")}); err != nil {
		t.Fatal(err)
	}
	initial := []SessionEvent{{Type: "log", CreatedAt: 1, Payload: json.RawMessage(`{"schema_version":2,"type":"log","at":"1970-01-01T00:00:00.001Z","generation":1}`)}}
	if _, err := first.AppendEvents(ctx, "tauri-rotate-race", initial); err != nil {
		t.Fatal(err)
	}
	status, err := first.EventStreamStatus(ctx, "tauri-rotate-race")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, writer := range []*Store{first, second} {
		events := []SessionEvent{{Type: "log", CreatedAt: int64(2 + i), Payload: json.RawMessage(fmt.Sprintf(`{"schema_version":2,"type":"log","at":"1970-01-01T00:00:00.00%dZ","generation":2}`, 2+i))}}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, err := writer.ReplaceEvents(ctx, "tauri-rotate-race", status.Generation, status.LastSequence, events)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes, failures := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("concurrent rotations: successes=%d failures=%d; want one CAS winner", successes, failures)
	}
	final, err := first.EventStreamStatus(ctx, "tauri-rotate-race")
	if err != nil || final.Generation != status.Generation+1 || final.LastSequence != 1 {
		t.Fatalf("rotation race final stream = %#v, %v", final, err)
	}
	rows, err := first.ReadEvents(ctx, "tauri-rotate-race", 0, 10)
	if err != nil || len(rows) != 1 || rows[0].Type != "log" {
		t.Fatalf("rotation race events = %#v, %v; want one complete replacement", rows, err)
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
