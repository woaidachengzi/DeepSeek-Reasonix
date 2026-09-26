package sessionidentity

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const sessionEventLookupChunkSize = 500

// SessionEvent is one canonical schema-2 event. Payload is the exact JSON
// object consumed by the existing DAG reducer; the remaining fields are
// indexed columns and are checked against that payload before persistence.
type SessionEvent struct {
	ID        string
	Type      string
	HeadID    string
	ParentID  string
	MessageID string
	WriterID  string
	CreatedAt int64
	Payload   json.RawMessage
}

// StoredSessionEvent adds the transaction-assigned sequence to a canonical
// event. Sequence is local to a session generation and starts at one.
type StoredSessionEvent struct {
	Sequence int64
	SessionEvent
}

type SessionEventStreamStatus struct {
	Generation         int64
	LastSequence       int64
	ProjectionSequence int64
	ProjectionSHA256   string
	CheckpointSequence int64
	CheckpointSHA256   string
	ImportSourceSHA256 string
	ImportVerified     bool
}

var ErrEventConflict = errors.New("session event sequence or identity conflict")
var ErrEventImportConflict = errors.New("session event import source changed")

// AppendEvents commits a complete batch atomically. Retrying the exact batch
// after an uncertain commit is idempotent; partial overlap or a reused event
// ID with different bytes is rejected.
func (s *Store) AppendEvents(ctx context.Context, sessionID string, events []SessionEvent) (int64, error) {
	if len(events) == 0 {
		return s.LastEventSequence(ctx, sessionID)
	}
	for i := range events {
		if err := normalizeEvent(&events[i]); err != nil {
			return 0, err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := ensureEventStream(ctx, tx, sessionID); err != nil {
		return 0, err
	}
	prior, err := countExistingEventIDs(ctx, tx, sessionID, events)
	if err != nil {
		return 0, err
	}
	if prior != 0 {
		if prior != len(events) {
			return 0, ErrEventConflict
		}
		for _, event := range events {
			digest, err := validateEvent(event)
			if err != nil {
				return 0, err
			}
			var stored string
			if err := tx.QueryRowContext(ctx, "SELECT payload_sha256 FROM session_events WHERE session_id=? AND event_id=?", sessionID, event.ID).Scan(&stored); err != nil {
				return 0, err
			}
			if stored != digest {
				return 0, ErrEventConflict
			}
		}
		var last int64
		if err := tx.QueryRowContext(ctx, "SELECT coalesce(max(sequence),0) FROM session_events WHERE session_id=?", sessionID).Scan(&last); err != nil {
			return 0, err
		}
		return last, tx.Commit()
	}
	var sequence int64
	if err := tx.QueryRowContext(ctx, "SELECT coalesce(max(sequence),0) FROM session_events WHERE session_id=?", sessionID).Scan(&sequence); err != nil {
		return 0, err
	}
	for _, event := range events {
		digest, err := validateEvent(event)
		if err != nil {
			return 0, err
		}
		sequence++
		if _, err := tx.ExecContext(ctx, `INSERT INTO session_events
			(session_id,sequence,event_id,event_type,head_id,parent_id,message_id,writer_id,created_at_ms,payload_json,payload_sha256)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)`, sessionID, sequence, event.ID, event.Type, event.HeadID, event.ParentID, event.MessageID,
			event.WriterID, event.CreatedAt, []byte(event.Payload), digest); err != nil {
			return 0, fmt.Errorf("append session event: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE session_event_streams SET updated_at_ms=? WHERE session_id=?", time.Now().UTC().UnixMilli(), sessionID); err != nil {
		return 0, err
	}
	return sequence, tx.Commit()
}

func countExistingEventIDs(ctx context.Context, tx *sql.Tx, sessionID string, events []SessionEvent) (int, error) {
	count := 0
	for start := 0; start < len(events); start += sessionEventLookupChunkSize {
		end := min(start+sessionEventLookupChunkSize, len(events))
		args := eventIDs(sessionID, events[start:end])
		var chunk int
		query := "SELECT count(*) FROM session_events WHERE session_id=? AND event_id IN (" + placeholders(end-start) + ")"
		if err := tx.QueryRowContext(ctx, query, args...).Scan(&chunk); err != nil {
			return 0, err
		}
		count += chunk
	}
	return count, nil
}

// ReadEvents returns a bounded, sequence-ordered snapshot of canonical event
// payloads. A zero limit means no explicit limit and is reserved for trusted
// internal callers that have already applied their replay budget.
func (s *Store) ReadEvents(ctx context.Context, sessionID string, after int64, limit int) ([]StoredSessionEvent, error) {
	query := `SELECT sequence,event_id,event_type,head_id,parent_id,message_id,writer_id,created_at_ms,payload_json,payload_sha256
		FROM session_events WHERE session_id=? AND sequence>? ORDER BY sequence`
	args := []any{sessionID, after}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []StoredSessionEvent
	for rows.Next() {
		var item StoredSessionEvent
		var payload []byte
		var digest string
		if err := rows.Scan(&item.Sequence, &item.ID, &item.Type, &item.HeadID, &item.ParentID, &item.MessageID, &item.WriterID, &item.CreatedAt, &payload, &digest); err != nil {
			return nil, err
		}
		item.Payload = append(json.RawMessage(nil), payload...)
		want, err := validateEvent(item.SessionEvent)
		if err != nil || want != digest {
			return nil, fmt.Errorf("verify session event %s: %w", item.ID, ErrEventConflict)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) LastEventSequence(ctx context.Context, sessionID string) (int64, error) {
	var last int64
	err := s.db.QueryRowContext(ctx, "SELECT coalesce(max(sequence),0) FROM session_events WHERE session_id=?", sessionID).Scan(&last)
	return last, err
}

// ReplaceEvents atomically compacts a stream during a lease-protected DAG
// rotation. The expected generation is a compare-and-swap guard against a
// concurrent append or a stale rotator.
func (s *Store) ReplaceEvents(ctx context.Context, sessionID string, expectedGeneration, expectedSequence int64, events []SessionEvent) (int64, int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	if err := ensureEventStream(ctx, tx, sessionID); err != nil {
		return 0, 0, err
	}
	var generation, sequence int64
	if err := tx.QueryRowContext(ctx, `SELECT st.generation,coalesce(max(ev.sequence),0) FROM session_event_streams st
		LEFT JOIN session_events ev ON ev.session_id=st.session_id WHERE st.session_id=? GROUP BY st.session_id`, sessionID).Scan(&generation, &sequence); err != nil {
		return 0, 0, err
	}
	if generation != expectedGeneration || sequence != expectedSequence {
		return generation, 0, ErrEventConflict
	}
	for i := range events {
		if err := normalizeEvent(&events[i]); err != nil {
			return generation, 0, err
		}
		if _, err := validateEvent(events[i]); err != nil {
			return generation, 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM session_events WHERE session_id=?", sessionID); err != nil {
		return generation, 0, err
	}
	for i, event := range events {
		digest, _ := validateEvent(event)
		if _, err := tx.ExecContext(ctx, `INSERT INTO session_events
			(session_id,sequence,event_id,event_type,head_id,parent_id,message_id,writer_id,created_at_ms,payload_json,payload_sha256)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)`, sessionID, i+1, event.ID, event.Type, event.HeadID, event.ParentID, event.MessageID,
			event.WriterID, event.CreatedAt, []byte(event.Payload), digest); err != nil {
			return generation, 0, fmt.Errorf("replace session events: %w", err)
		}
	}
	generation++
	if _, err := tx.ExecContext(ctx, `UPDATE session_event_streams SET generation=?,projection_sequence=0,
		projection_sha256='',checkpoint_sequence=0,checkpoint_sha256='',import_source_sha256='',import_verified=0,updated_at_ms=? WHERE session_id=?`, generation, time.Now().UTC().UnixMilli(), sessionID); err != nil {
		return generation, 0, err
	}
	if err := tx.Commit(); err != nil {
		return generation, 0, err
	}
	return generation, int64(len(events)), nil
}

// MarkEventProjection records a successfully published compatibility JSONL
// projection only when its generation and watermark still match the database.
func (s *Store) MarkEventProjection(ctx context.Context, sessionID string, generation, sequence int64, projectionSHA256 string) error {
	if len(projectionSHA256) != 64 {
		return errors.New("invalid session event projection fingerprint")
	}
	if _, err := hex.DecodeString(projectionSHA256); err != nil {
		return errors.New("invalid session event projection fingerprint")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentGeneration, last int64
	if err := tx.QueryRowContext(ctx, `SELECT st.generation,coalesce(max(ev.sequence),0)
		FROM session_event_streams st LEFT JOIN session_events ev ON ev.session_id=st.session_id
		WHERE st.session_id=? GROUP BY st.session_id`, sessionID).Scan(&currentGeneration, &last); err != nil {
		return err
	}
	if currentGeneration != generation || sequence > last {
		return ErrEventConflict
	}
	if _, err := tx.ExecContext(ctx, `UPDATE session_event_streams SET projection_sequence=?,projection_sha256=?,updated_at_ms=?
		WHERE session_id=? AND generation=?`, sequence, projectionSHA256, time.Now().UTC().UnixMilli(), sessionID, generation); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) EventStreamStatus(ctx context.Context, sessionID string) (SessionEventStreamStatus, error) {
	var status SessionEventStreamStatus
	err := s.db.QueryRowContext(ctx, `SELECT st.generation,coalesce(max(ev.sequence),0),st.projection_sequence,
		st.projection_sha256,st.checkpoint_sequence,st.checkpoint_sha256,st.import_source_sha256,st.import_verified FROM session_event_streams st
		LEFT JOIN session_events ev ON ev.session_id=st.session_id WHERE st.session_id=? GROUP BY st.session_id`, sessionID).
		Scan(&status.Generation, &status.LastSequence, &status.ProjectionSequence, &status.ProjectionSHA256,
			&status.CheckpointSequence, &status.CheckpointSHA256, &status.ImportSourceSHA256, &status.ImportVerified)
	return status, err
}

// MarkEventImportVerified records that the SQLite reducer produced the same
// semantic DAG state as the source JSONL reducer for this imported generation.
func (s *Store) MarkEventImportVerified(ctx context.Context, sessionID string, generation, expectedSequence int64, sourceSHA256 string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentGeneration, last int64
	var currentFingerprint string
	if err := tx.QueryRowContext(ctx, `SELECT st.generation,st.import_source_sha256,coalesce(max(ev.sequence),0)
		FROM session_event_streams st LEFT JOIN session_events ev ON ev.session_id=st.session_id
		WHERE st.session_id=? GROUP BY st.session_id`, sessionID).
		Scan(&currentGeneration, &currentFingerprint, &last); err != nil {
		return err
	}
	if currentGeneration != generation || last != expectedSequence || currentFingerprint != sourceSHA256 || currentFingerprint == "" {
		return ErrEventImportConflict
	}
	if _, err := tx.ExecContext(ctx, `UPDATE session_event_streams SET import_verified=1,updated_at_ms=?
		WHERE session_id=? AND generation=? AND import_source_sha256=?`, time.Now().UTC().UnixMilli(), sessionID, generation, sourceSHA256); err != nil {
		return err
	}
	return tx.Commit()
}

// MarkCheckpointProjection records the transcript checkpoint only if no newer
// event committed while that projection was being serialized.
func (s *Store) MarkCheckpointProjection(ctx context.Context, sessionID string, generation, sequence int64, checkpointSHA256 string) error {
	if len(checkpointSHA256) != 64 {
		return errors.New("invalid session checkpoint fingerprint")
	}
	if _, err := hex.DecodeString(checkpointSHA256); err != nil {
		return errors.New("invalid session checkpoint fingerprint")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentGeneration, last int64
	if err := tx.QueryRowContext(ctx, `SELECT st.generation,coalesce(max(ev.sequence),0)
		FROM session_event_streams st LEFT JOIN session_events ev ON ev.session_id=st.session_id
		WHERE st.session_id=? GROUP BY st.session_id`, sessionID).Scan(&currentGeneration, &last); err != nil {
		return err
	}
	if currentGeneration != generation || sequence != last {
		return ErrEventConflict
	}
	if _, err := tx.ExecContext(ctx, `UPDATE session_event_streams SET checkpoint_sequence=?,checkpoint_sha256=?,updated_at_ms=?
		WHERE session_id=? AND generation=?`, sequence, checkpointSHA256, time.Now().UTC().UnixMilli(), sessionID, generation); err != nil {
		return err
	}
	return tx.Commit()
}

// ImportEvents installs a legacy JSONL event stream once, keyed by the SHA-256
// of the original source bytes. It refuses to replace a stream already owned
// by another import or by a live SQLite writer.
func (s *Store) ImportEvents(ctx context.Context, sessionID, sourceSHA256 string, generation int64, events []SessionEvent) (bool, error) {
	if len(sourceSHA256) != 64 {
		return false, errors.New("invalid session event import fingerprint")
	}
	if generation < 1 {
		return false, errors.New("invalid session event generation")
	}
	if _, err := hex.DecodeString(sourceSHA256); err != nil {
		return false, errors.New("invalid session event import fingerprint")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if err := ensureEventStream(ctx, tx, sessionID); err != nil {
		return false, err
	}
	var prior string
	var priorGeneration int64
	if err := tx.QueryRowContext(ctx, "SELECT import_source_sha256,generation FROM session_event_streams WHERE session_id=?", sessionID).Scan(&prior, &priorGeneration); err != nil {
		return false, err
	}
	if prior != "" {
		if prior != sourceSHA256 || priorGeneration != generation {
			return false, ErrEventImportConflict
		}
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM session_events WHERE session_id=?", sessionID).Scan(&count); err != nil {
			return false, err
		}
		if count != len(events) {
			return false, ErrEventImportConflict
		}
		return false, tx.Commit()
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM session_events WHERE session_id=?", sessionID).Scan(&count); err != nil {
		return false, err
	}
	if count != 0 {
		return false, ErrEventImportConflict
	}
	for i, event := range events {
		if err := normalizeEvent(&event); err != nil {
			return false, err
		}
		digest, err := validateEvent(event)
		if err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO session_events
			(session_id,sequence,event_id,event_type,head_id,parent_id,message_id,writer_id,created_at_ms,payload_json,payload_sha256)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)`, sessionID, i+1, event.ID, event.Type, event.HeadID, event.ParentID, event.MessageID,
			event.WriterID, event.CreatedAt, []byte(event.Payload), digest); err != nil {
			return false, fmt.Errorf("import session event: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE session_event_streams SET generation=?,import_source_sha256=?,updated_at_ms=? WHERE session_id=?`,
		generation, sourceSHA256, time.Now().UTC().UnixMilli(), sessionID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func ensureEventStream(ctx context.Context, tx *sql.Tx, sessionID string) error {
	if sessionID == "" {
		return errors.New("session event identity is empty")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO session_event_streams(session_id,updated_at_ms)
		VALUES(?,?) ON CONFLICT(session_id) DO NOTHING`, sessionID, time.Now().UTC().UnixMilli()); err != nil {
		return fmt.Errorf("open session event stream: %w", err)
	}
	return nil
}

func validateEvent(event SessionEvent) (string, error) {
	if event.ID == "" || event.Type == "" || len(event.Payload) == 0 || !json.Valid(event.Payload) {
		return "", errors.New("invalid canonical session event")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &object); err != nil {
		return "", err
	}
	for field, want := range map[string]string{"type": event.Type, "head": event.HeadID, "parent": event.ParentID, "writer": event.WriterID} {
		if want == "" {
			continue
		}
		var got string
		if err := json.Unmarshal(object[field], &got); err != nil || got != want {
			return "", fmt.Errorf("session event indexed %s does not match payload", field)
		}
	}
	if raw := object["id"]; len(raw) > 0 {
		var got string
		if err := json.Unmarshal(raw, &got); err != nil || got != event.ID {
			return "", errors.New("session event indexed id does not match payload")
		}
	}
	if raw := object["at"]; len(raw) > 0 {
		var at time.Time
		if err := json.Unmarshal(raw, &at); err != nil || (!at.IsZero() && event.CreatedAt != at.UnixMilli()) {
			return "", errors.New("session event indexed timestamp does not match payload")
		}
	}
	digest := sha256.Sum256(event.Payload)
	return hex.EncodeToString(digest[:]), nil
}

func normalizeEvent(event *SessionEvent) error {
	if event == nil || len(event.Payload) == 0 || !json.Valid(event.Payload) {
		return errors.New("invalid canonical session event")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &object); err != nil {
		return err
	}
	if event.ID == "" {
		if raw := object["id"]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &event.ID); err != nil {
				return err
			}
		}
		if event.ID == "" {
			digest := sha256.Sum256(event.Payload)
			event.ID = "sha256:" + hex.EncodeToString(digest[:])
		}
	}
	if event.Type == "" {
		return errors.New("invalid canonical session event")
	}
	return nil
}

func placeholders(n int) string {
	result := "?"
	for i := 1; i < n; i++ {
		result += ",?"
	}
	return result
}

func eventIDs(sessionID string, events []SessionEvent) []any {
	args := make([]any, 1, len(events)+1)
	args[0] = sessionID
	for _, event := range events {
		args = append(args, event.ID)
	}
	return args
}
