package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"reasonix/internal/config"
	"reasonix/internal/fileutil"
	"reasonix/internal/provider"
	"reasonix/internal/sessionidentity"
	"reasonix/internal/store"
)

const previewSQLiteEventsEnv = "REASONIX_PREVIEW_SQLITE_EVENTS"

var sessionEventStores sync.Map // map[identity database path]*sessionidentity.Store

type sqliteCheckpointTarget struct {
	db         *sessionidentity.Store
	sessionID  string
	generation int64
	sequence   int64
}

func sqliteSessionEventStore(sessionPath string) (*sessionidentity.Store, string, bool, error) {
	if os.Getenv(previewSQLiteEventsEnv) != "1" {
		return nil, "", false, nil
	}
	sessionPath = transcriptPathForDAGLog(sessionPath)
	root := config.SessionProfileRoot()
	databasePath := config.DesktopSessionIdentityPath()
	if root == "" || databasePath == "" {
		return nil, "", false, errors.New("managed Preview session event store path is unavailable")
	}
	abs, err := filepath.Abs(sessionPath)
	if err != nil {
		return nil, "", false, err
	}
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(abs))
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, "", false, nil // exports and custom paths keep the established file backend.
	}
	fileID := BranchID(sessionPath)
	if !strings.HasPrefix(fileID, "tauri-") {
		return nil, "", false, nil
	}
	sessionID := strings.TrimPrefix(fileID, "tauri-")
	if sessionID == "" {
		return nil, "", false, nil
	}
	key := filepath.Clean(databasePath)
	if cached, ok := sessionEventStores.Load(key); ok {
		return cached.(*sessionidentity.Store), sessionID, true, nil
	}
	opened, err := sessionidentity.Open(context.Background(), databasePath, root)
	if err != nil {
		return nil, "", false, fmt.Errorf("open managed Preview session event database: %w", err)
	}
	actual, loaded := sessionEventStores.LoadOrStore(key, opened)
	if loaded {
		_ = opened.Close()
		opened = actual.(*sessionidentity.Store)
	}
	return opened, sessionID, true, nil
}

func ensureSQLiteDAGImport(ctx context.Context, sessionPath string, db *sessionidentity.Store, sessionID string, limits sessionReplayLimits) (bool, error) {
	status, err := db.EventStreamStatus(ctx, sessionID)
	if err == nil && status.LastSequence > 0 && (status.ImportSourceSHA256 == "" || status.ImportVerified) {
		return true, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	path := store.SessionEventLog(sessionPath)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if status.ImportSourceSHA256 != "" && !status.ImportVerified {
				return false, errors.New("unverified SQLite import has no legacy event log for shadow validation")
			}
			return status.Generation > 0, nil
		}
		return false, err
	}
	if info.IsDir() || info.Size() == 0 {
		if status.ImportSourceSHA256 != "" && !status.ImportVerified {
			return false, errors.New("unverified SQLite import has an empty legacy event log")
		}
		return status.Generation > 0, nil
	}
	var header struct {
		SchemaVersion int    `json:"schema_version"`
		Type          string `json:"type"`
	}
	headerFile, err := os.Open(path)
	if err != nil {
		return false, err
	}
	headerErr := json.NewDecoder(io.LimitReader(headerFile, sessionEventProbeMaxBytes)).Decode(&header)
	_ = headerFile.Close()
	if headerErr != nil || header.SchemaVersion != sessionDAGSchemaVersion || header.Type != sessionDAGTypeLog {
		if status.ImportSourceSHA256 != "" && !status.ImportVerified {
			return false, errors.New("unverified SQLite import has an invalid legacy event log header")
		}
		return false, nil
	}
	if info.Size() > limits.maxBytes {
		return false, sessionReplayLimitError(path, "encoded_bytes", info.Size(), limits.maxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	digest := sha256.Sum256(data)
	sourceSHA := hex.EncodeToString(digest[:])
	if status.ImportSourceSHA256 != "" && status.ImportSourceSHA256 != sourceSHA {
		return false, sessionidentity.ErrEventImportConflict
	}
	legacy, err := replaySessionDAGFile(ctx, path, limits)
	if err != nil {
		return false, err
	}
	if legacy.damaged || legacy.holes != 0 {
		return false, fmt.Errorf("refusing to import damaged session event log")
	}
	verify, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if !bytes.Equal(data, verify) {
		return false, fmt.Errorf("session event log changed during SQLite import")
	}
	events, err := decodeSQLiteSessionEvents(data)
	if err != nil {
		return false, err
	}
	if _, err := db.ImportEvents(ctx, sessionID, sourceSHA, legacy.generation, events); err != nil {
		return false, fmt.Errorf("import session event log into SQLite: %w", err)
	}
	status, err = db.EventStreamStatus(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if status.Generation != legacy.generation {
		return false, fmt.Errorf("SQLite event import generation mismatch")
	}
	rows, err := db.ReadEvents(ctx, sessionID, 0, limits.maxRecords+1)
	if err != nil {
		return false, err
	}
	if int64(len(rows)) != status.LastSequence || len(rows) != legacy.records {
		return false, fmt.Errorf("SQLite import shadow replay event count differs from legacy JSONL")
	}
	var shadow bytes.Buffer
	for _, row := range rows {
		shadow.Write(row.Payload)
		shadow.WriteByte('\n')
	}
	shadowState, err := replaySessionDAGBytes(ctx, path, shadow.Bytes(), limits, true)
	if err != nil {
		return false, fmt.Errorf("replay imported SQLite session events: %w", err)
	}
	if !sessionDAGShadowSemanticsEqual(legacy, shadowState) {
		return false, fmt.Errorf("SQLite import shadow replay differs from legacy JSONL")
	}
	finalSource, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	finalDigest := sha256.Sum256(finalSource)
	if hex.EncodeToString(finalDigest[:]) != sourceSHA {
		return false, fmt.Errorf("session event log changed during SQLite shadow validation")
	}
	if status.ImportSourceSHA256 != "" || status.ImportVerified {
		if err := db.MarkEventImportVerified(ctx, sessionID, status.Generation, status.LastSequence, sourceSHA); err != nil {
			return false, fmt.Errorf("mark SQLite legacy import verified: %w", err)
		}
	} else if status.ImportSourceSHA256 == "" {
		status, err = db.EventStreamStatus(ctx, sessionID)
		if err != nil {
			return false, err
		}
		if status.ImportSourceSHA256 != "" {
			if err := db.MarkEventImportVerified(ctx, sessionID, status.Generation, status.LastSequence, sourceSHA); err != nil {
				return false, fmt.Errorf("mark SQLite legacy import verified: %w", err)
			}
		}
	}
	return status.LastSequence > 0, nil
}

func sessionDAGShadowSemanticsEqual(legacy, sqlite *sessionDAGState) bool {
	if legacy == nil || sqlite == nil || legacy.generation != sqlite.generation || legacy.upgradedFrom != sqlite.upgradedFrom ||
		legacy.records != sqlite.records || legacy.collectionItems != sqlite.collectionItems || legacy.selectedHead() != sqlite.selectedHead() ||
		!reflect.DeepEqual(legacy.headOrder, sqlite.headOrder) || !reflect.DeepEqual(legacy.orphans, sqlite.orphans) ||
		!reflect.DeepEqual(legacy.writers, sqlite.writers) || !reflect.DeepEqual(legacy.patches, sqlite.patches) || !reflect.DeepEqual(legacy.redactions, sqlite.redactions) {
		return false
	}
	legacySelected, sqliteSelected := legacy.selectedHead(), sqlite.selectedHead()
	for _, id := range legacy.headOrder {
		legacyHead, sqliteHead := legacy.heads[id], sqlite.heads[id]
		if legacyHead == nil || sqliteHead == nil || !reflect.DeepEqual(legacy.headRecord(id, legacySelected), sqlite.headRecord(id, sqliteSelected)) ||
			!reflect.DeepEqual(legacyHead.system, sqliteHead.system) || !reflect.DeepEqual(legacyHead.compaction, sqliteHead.compaction) ||
			!reflect.DeepEqual(legacyHead.openTurn, sqliteHead.openTurn) {
			return false
		}
		legacyMessages, legacyTimes := legacy.materialize(id)
		sqliteMessages, sqliteTimes := sqlite.materialize(id)
		if !reflect.DeepEqual(legacyMessages, sqliteMessages) || !reflect.DeepEqual(legacyTimes, sqliteTimes) {
			return false
		}
	}
	return true
}

func decodeSQLiteSessionEvents(data []byte) ([]sessionidentity.SessionEvent, error) {
	lines := bytes.Split(data, []byte{'\n'})
	events := make([]sessionidentity.SessionEvent, 0, len(lines))
	for index, line := range lines {
		if index == len(lines)-1 && len(line) == 0 {
			continue
		}
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(bytes.TrimSpace(line)) == 0 {
			return nil, fmt.Errorf("session event log contains an empty record")
		}
		var entry sessionDAGEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("decode session event for SQLite import: %w", err)
		}
		if entry.SchemaVersion != sessionDAGSchemaVersion {
			return nil, fmt.Errorf("session event log uses unsupported schema %d", entry.SchemaVersion)
		}
		events = append(events, sessionidentity.SessionEvent{
			ID: entry.ID, Type: entry.Type, HeadID: entry.Head, ParentID: entry.Parent,
			MessageID: entry.ID, WriterID: entry.Writer, CreatedAt: entry.At.UnixMilli(),
			Payload: append(json.RawMessage(nil), line...),
		})
	}
	if len(events) == 0 {
		return nil, errors.New("session event log has no importable records")
	}
	return events, nil
}

func sqliteDAGEventBytes(ctx context.Context, sessionPath string, limits sessionReplayLimits) ([]byte, bool, error) {
	db, sessionID, enabled, err := sqliteSessionEventStore(sessionPath)
	if err != nil || !enabled {
		return nil, false, err
	}
	active, err := ensureSQLiteDAGImport(ctx, sessionPath, db, sessionID, limits)
	if err != nil || !active {
		return nil, active, err
	}
	status, err := db.EventStreamStatus(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if status.LastSequence > int64(limits.maxRecords) {
		return nil, false, sessionReplayLimitError(sessionPath, "event_records", status.LastSequence, int64(limits.maxRecords))
	}
	rows, err := db.ReadEvents(ctx, sessionID, 0, limits.maxRecords+1)
	if err != nil {
		return nil, false, err
	}
	var data bytes.Buffer
	for _, row := range rows {
		if err := checkSQLiteProjectionBudget(sessionPath, int64(data.Len()), len(row.Payload)+1, limits.maxBytes); err != nil {
			return nil, false, err
		}
		data.Write(row.Payload)
		data.WriteByte('\n')
	}
	if int64(len(rows)) != status.LastSequence {
		return nil, false, errors.New("SQLite session event sequence is not contiguous")
	}
	return data.Bytes(), len(rows) > 0, nil
}

func appendSQLiteDAGEvents(sessionPath string, entries []sessionDAGEntry, encoded []byte) (int64, bool, error) {
	return appendSQLiteDAGEventsWithinLimits(sessionPath, entries, encoded, defaultSessionReplayLimits)
}

func appendSQLiteDAGEventsWithinLimits(sessionPath string, entries []sessionDAGEntry, encoded []byte, limits sessionReplayLimits) (int64, bool, error) {
	db, sessionID, enabled, err := sqliteSessionEventStore(sessionPath)
	if err != nil || !enabled {
		return 0, enabled, err
	}
	ctx := context.Background()
	if _, err := ensureSQLiteDAGImport(ctx, sessionPath, db, sessionID, defaultSessionReplayLimits); err != nil {
		return 0, true, err
	}
	_, err = db.EventStreamStatus(ctx, sessionID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, true, err
	}
	previousSize := int64(0)
	if info, statErr := os.Stat(store.SessionEventLog(sessionPath)); statErr == nil {
		previousSize = info.Size()
	}
	events := make([]sessionidentity.SessionEvent, 0, len(entries))
	for _, entry := range entries {
		payload, err := json.Marshal(entry)
		if err != nil {
			return 0, true, err
		}
		events = append(events, sessionidentity.SessionEvent{
			ID: entry.ID, Type: entry.Type, HeadID: entry.Head, ParentID: entry.Parent,
			MessageID: entry.ID, WriterID: entry.Writer, CreatedAt: entry.At.UnixMilli(), Payload: payload,
		})
	}
	if _, err := db.AppendEventsWithinLimits(ctx, sessionID, events, int64(limits.maxRecords), limits.maxBytes); err != nil {
		return 0, true, fmt.Errorf("commit session events to SQLite: %w", err)
	}
	fileutil.Crash("dag-sqlite-committed", store.SessionEventLog(sessionPath))
	status, err := db.EventStreamStatus(ctx, sessionID)
	if err != nil {
		return 0, true, err
	}
	if status.LastSequence > int64(defaultSessionReplayLimits.maxRecords) {
		return 0, true, sessionReplayLimitError(sessionPath, "event_records", status.LastSequence, int64(defaultSessionReplayLimits.maxRecords))
	}
	projectionPath := store.SessionEventLog(sessionPath)
	projectionSize, projectionErr := writeSQLiteDAGProjection(ctx, db, sessionID, status.Generation, status.LastSequence, projectionPath, defaultSessionReplayLimits)
	if projectionErr != nil {
		// SQLite commit is authoritative. A stale projection is rebuilt on the
		// next managed Preview read before the compatibility reader is exposed.
		slog.Warn("session: SQLite event projection pending", "path", sessionPath, "err", projectionErr)
		return previousSize + int64(len(encoded)), true, nil
	}
	return projectionSize, true, nil
}

func replaceSQLiteDAGEvents(sessionPath string, entries []sessionDAGEntry, expectedSequence int64) (bool, error) {
	return replaceSQLiteDAGEventsWithinLimits(sessionPath, entries, expectedSequence, defaultSessionReplayLimits)
}

func replaceSQLiteDAGEventsWithinLimits(sessionPath string, entries []sessionDAGEntry, expectedSequence int64, limits sessionReplayLimits) (bool, error) {
	db, sessionID, enabled, err := sqliteSessionEventStore(sessionPath)
	if err != nil || !enabled {
		return enabled, err
	}
	ctx := context.Background()
	if _, err := ensureSQLiteDAGImport(ctx, sessionPath, db, sessionID, defaultSessionReplayLimits); err != nil {
		return true, err
	}
	if err := checkSQLiteDAGReplacementBudget(sessionPath, entries, limits); err != nil {
		return true, err
	}
	status, err := db.EventStreamStatus(ctx, sessionID)
	if err != nil {
		return true, err
	}
	if status.LastSequence != expectedSequence {
		return true, fmt.Errorf("rotate SQLite session events: %w", sessionidentity.ErrEventConflict)
	}
	events := make([]sessionidentity.SessionEvent, 0, len(entries))
	for _, entry := range entries {
		payload, err := json.Marshal(entry)
		if err != nil {
			return true, err
		}
		events = append(events, sessionidentity.SessionEvent{
			ID: entry.ID, Type: entry.Type, HeadID: entry.Head, ParentID: entry.Parent,
			MessageID: entry.ID, WriterID: entry.Writer, CreatedAt: entry.At.UnixMilli(), Payload: payload,
		})
	}
	generation, sequence, err := db.ReplaceEvents(ctx, sessionID, status.Generation, expectedSequence, events)
	if err != nil {
		return true, fmt.Errorf("rotate SQLite session events: %w", err)
	}
	fileutil.Crash("dag-sqlite-rotated", store.SessionEventLog(sessionPath))
	if _, err := writeSQLiteDAGProjection(ctx, db, sessionID, generation, sequence, store.SessionEventLog(sessionPath), limits); err != nil {
		// Rotation has committed in SQLite; the compatibility projection can be
		// rebuilt from the new generation on the next managed Preview read.
		slog.Warn("session: SQLite event projection pending after rotation", "path", sessionPath, "err", err)
	}
	return true, nil
}

func checkSQLiteDAGReplacementBudget(path string, entries []sessionDAGEntry, limits sessionReplayLimits) error {
	if len(entries) > limits.maxRecords {
		return sessionReplayLimitError(path, "event_records", int64(len(entries)), int64(limits.maxRecords))
	}
	var projectionBytes int64
	for _, entry := range entries {
		payload, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if err := checkSQLiteProjectionBudget(path, projectionBytes, len(payload)+1, limits.maxBytes); err != nil {
			return err
		}
		projectionBytes += int64(len(payload)) + 1
	}
	return nil
}

func writeSQLiteDAGProjection(ctx context.Context, db *sessionidentity.Store, sessionID string, generation, sequence int64, projectionPath string, limits sessionReplayLimits) (int64, error) {
	rows, err := db.ReadEvents(ctx, sessionID, 0, limits.maxRecords+1)
	if err != nil {
		return 0, err
	}
	var data bytes.Buffer
	for _, row := range rows {
		if err := checkSQLiteProjectionBudget(projectionPath, int64(data.Len()), len(row.Payload)+1, limits.maxBytes); err != nil {
			return 0, err
		}
		data.Write(row.Payload)
		data.WriteByte('\n')
	}
	if int64(len(rows)) != sequence {
		return 0, errors.New("SQLite session event sequence changed while rebuilding projection")
	}
	if err := os.MkdirAll(filepath.Dir(projectionPath), 0o755); err != nil {
		return 0, err
	}
	staged, err := fileutil.StageAtomicWrite(projectionPath, data.Bytes(), 0o600)
	if err != nil {
		return 0, err
	}
	if err := fileutil.PublishStagedWrite(staged, projectionPath); err != nil {
		_ = os.Remove(staged)
		return 0, err
	}
	fileutil.Crash("dag-sqlite-projection-published", projectionPath)
	digest := sha256.Sum256(data.Bytes())
	if err := db.MarkEventProjection(ctx, sessionID, generation, sequence, hex.EncodeToString(digest[:])); err != nil {
		return 0, err
	}
	return int64(data.Len()), nil
}

func checkSQLiteProjectionBudget(path string, currentBytes int64, nextBytes int, maxBytes int64) error {
	if currentBytes < 0 || nextBytes < 0 || maxBytes < 0 || currentBytes > maxBytes || int64(nextBytes) > maxBytes-currentBytes {
		value := currentBytes + int64(nextBytes)
		return sessionReplayLimitError(path, "encoded_bytes", value, maxBytes)
	}
	return nil
}

func importSQLiteProjection(ctx context.Context, path string, limits sessionReplayLimits) ([]byte, bool, error) {
	path = transcriptPathForDAGLog(path)
	data, enabled, err := sqliteDAGEventBytes(ctx, path, limits)
	if err != nil || !enabled {
		return data, enabled, err
	}
	db, sessionID, active, err := sqliteSessionEventStore(path)
	if err != nil || !active || len(data) == 0 {
		return data, active, err
	}
	status, err := db.EventStreamStatus(ctx, sessionID)
	if err != nil {
		return nil, false, err
	}
	digest := sha256.Sum256(data)
	projectionMatches := false
	if status.ProjectionSequence == status.LastSequence && status.ProjectionSHA256 == hex.EncodeToString(digest[:]) {
		if onDisk, readErr := os.ReadFile(store.SessionEventLog(path)); readErr == nil && bytes.Equal(onDisk, data) {
			projectionMatches = true
		}
	}
	if !projectionMatches {
		if err := os.MkdirAll(filepath.Dir(store.SessionEventLog(path)), 0o755); err != nil {
			slog.Warn("session: SQLite event projection rebuild pending", "path", path, "err", err)
			return data, true, nil
		}
		staged, err := fileutil.StageAtomicWrite(store.SessionEventLog(path), data, 0o600)
		if err != nil {
			slog.Warn("session: SQLite event projection rebuild pending", "path", path, "err", err)
			return data, true, nil
		}
		if err := fileutil.PublishStagedWrite(staged, store.SessionEventLog(path)); err != nil {
			_ = os.Remove(staged)
			slog.Warn("session: SQLite event projection rebuild pending", "path", path, "err", err)
			return data, true, nil
		}
		digest = sha256.Sum256(data)
		if err := db.MarkEventProjection(ctx, sessionID, status.Generation, status.LastSequence, hex.EncodeToString(digest[:])); err != nil {
			slog.Warn("session: SQLite event projection watermark pending", "path", path, "err", err)
			return data, true, nil
		}
	}
	return data, true, nil
}

func transcriptPathForDAGLog(path string) string {
	if strings.HasSuffix(path, ".events.jsonl") {
		return strings.TrimSuffix(path, ".events.jsonl") + ".jsonl"
	}
	return path
}

func captureSQLiteCheckpointTarget(path string) (sqliteCheckpointTarget, bool, error) {
	db, sessionID, enabled, err := sqliteSessionEventStore(path)
	if err != nil || !enabled {
		return sqliteCheckpointTarget{}, enabled, err
	}
	status, err := db.EventStreamStatus(context.Background(), sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return sqliteCheckpointTarget{}, false, nil
	}
	if err != nil {
		return sqliteCheckpointTarget{}, true, err
	}
	if status.LastSequence == 0 {
		return sqliteCheckpointTarget{}, false, nil
	}
	return sqliteCheckpointTarget{db: db, sessionID: sessionID, generation: status.Generation, sequence: status.LastSequence}, true, nil
}

func markSQLiteCheckpointProjection(target sqliteCheckpointTarget, path string) error {
	if target.db == nil || target.sequence == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	return target.db.MarkCheckpointProjection(context.Background(), target.sessionID, target.generation, target.sequence, hex.EncodeToString(digest[:]))
}

func repairSQLiteCheckpointProjection(ctx context.Context, path string, messages []provider.Message) {
	target, active, err := captureSQLiteCheckpointTarget(path)
	if err != nil || !active {
		if err != nil {
			slog.Warn("session: could not inspect SQLite checkpoint watermark", "path", path, "err", err)
		}
		return
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		if data, readErr := os.ReadFile(path); readErr == nil {
			digest := sha256.Sum256(data)
			status, statusErr := target.db.EventStreamStatus(ctx, target.sessionID)
			if statusErr == nil && status.LastSequence == target.sequence && status.CheckpointSequence == target.sequence && status.CheckpointSHA256 == hex.EncodeToString(digest[:]) {
				return
			}
		}
	}
	if err := writeSessionMessagesContext(ctx, path, messages); err != nil {
		slog.Warn("session: SQLite checkpoint projection pending", "path", path, "err", err)
	}
}
