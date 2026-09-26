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
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/fileutil"
	"reasonix/internal/provider"
	"reasonix/internal/sessionidentity"
	"reasonix/internal/store"
	"reasonix/internal/turnevent"
)

const (
	sqliteDAGCrashModeEnv = "REASONIX_TEST_SQLITE_DAG_CRASH_MODE"
	sqliteDAGCrashPathEnv = "REASONIX_TEST_SQLITE_DAG_CRASH_PATH"
	sqliteDAGCrashRootEnv = "REASONIX_TEST_SQLITE_DAG_CRASH_ROOT"
	sqliteDAGCrashExit    = 47
)

func prepareSQLiteEventTestSession(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("REASONIX_STATE_HOME", root)
	t.Setenv(previewSQLiteEventsEnv, "1")
	sessionDir := config.SessionDir()
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	id := "sqlite-" + strings.ReplaceAll(t.Name(), "/", "-")
	path := filepath.Join(sessionDir, "tauri-"+id+".jsonl")
	identities, err := sessionidentity.Open(context.Background(), config.DesktopSessionIdentityPath(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := identities.Reserve(context.Background(), sessionDir, sessionidentity.Candidate{ID: id, Path: path}); err != nil {
		_ = identities.Close()
		t.Fatal(err)
	}
	if err := identities.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cached, ok := sessionEventStores.Load(filepath.Clean(config.DesktopSessionIdentityPath())); ok {
			_ = cached.(*sessionidentity.Store).Close()
			sessionEventStores.Delete(filepath.Clean(config.DesktopSessionIdentityPath()))
		}
	})
	return path
}

func TestSQLiteSessionDAGAppendProjectsAndReplaysCommittedEvents(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	base := time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC)
	entries := []sessionDAGEntry{{Type: sessionDAGTypeLog, Generation: 1, At: base}}
	entries = append(entries, dagMessageEntry(t, SessionMainHead, "", "turn-1", dagMsg("user", "question", "message-1"), base.Add(time.Second)))
	entries = append(entries, dagMessageEntry(t, SessionMainHead, "message-1", "turn-1", dagMsg("assistant", "answer", "message-2"), base.Add(2*time.Second)))
	if _, err := appendSessionDAGEntries(path, entries, true); err != nil {
		t.Fatalf("append SQLite DAG: %v", err)
	}
	data, err := os.ReadFile(store.SessionEventLog(path))
	if err != nil {
		t.Fatal(err)
	}
	var lines int
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != len(entries) {
		t.Fatalf("compatibility projection contains %d entries, want %d", lines, len(entries))
	}
	state, err := replaySessionDAG(context.Background(), store.SessionEventLog(path), defaultSessionReplayLimits)
	if err != nil {
		t.Fatalf("replay SQLite DAG: %v", err)
	}
	if !state.sqliteBacked || state.damaged || state.records != len(entries) {
		t.Fatalf("SQLite replay state backing=%v damaged=%v records=%d", state.sqliteBacked, state.damaged, state.records)
	}
	messages, _ := state.materialize(SessionMainHead)
	if got := dagContents(messages); len(got) != 2 || got[0] != "question" || got[1] != "answer" {
		t.Fatalf("SQLite DAG messages = %#v", got)
	}
}

func TestSQLiteSessionEventStoreRequiresPreviewGateAndProfilePath(t *testing.T) {
	managedPath := prepareSQLiteEventTestSession(t)
	t.Setenv(previewSQLiteEventsEnv, "0")
	if db, _, enabled, err := sqliteSessionEventStore(managedPath); err != nil || enabled || db != nil {
		t.Fatalf("event store without Preview capability = db:%v enabled:%v err:%v; want disabled", db != nil, enabled, err)
	}
	t.Setenv(previewSQLiteEventsEnv, "1")
	for _, path := range []string{
		filepath.Join(t.TempDir(), "tauri-outside.jsonl"),
		filepath.Join(config.SessionDir(), "custom-profile.jsonl"),
	} {
		if db, _, enabled, err := sqliteSessionEventStore(path); err != nil || enabled || db != nil {
			t.Fatalf("event store for non-managed path %q = db:%v enabled:%v err:%v; want disabled", path, db != nil, enabled, err)
		}
	}
	if db, _, enabled, err := sqliteSessionEventStore(managedPath); err != nil || !enabled || db == nil {
		t.Fatalf("event store for managed Preview session = db:%v enabled:%v err:%v; want enabled", db != nil, enabled, err)
	}
}

func TestSQLiteSessionDAGReplayEnforcesRecordAndByteBudgets(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	entries := []sessionDAGEntry{
		{Type: sessionDAGTypeLog, Generation: 1, At: time.Now().UTC()},
		dagMessageEntry(t, SessionMainHead, "", "turn-budget", dagMsg("user", "bounded replay", "budget-1"), time.Now().UTC()),
	}
	if _, err := appendSessionDAGEntries(path, entries, true); err != nil {
		t.Fatal(err)
	}
	projection := store.SessionEventLog(path)
	before, err := os.ReadFile(projection)
	if err != nil {
		t.Fatal(err)
	}
	limits := defaultSessionReplayLimits
	limits.maxRecords = 1
	if _, _, err := sqliteDAGEventBytes(context.Background(), projection, limits); !errors.Is(err, ErrSessionReplayLimitExceeded) {
		t.Fatalf("record budget error = %v; want ErrSessionReplayLimitExceeded", err)
	}
	limits = defaultSessionReplayLimits
	limits.maxBytes = 64
	if _, _, err := sqliteDAGEventBytes(context.Background(), projection, limits); !errors.Is(err, ErrSessionReplayLimitExceeded) {
		t.Fatalf("byte budget error = %v; want ErrSessionReplayLimitExceeded", err)
	}
	after, err := os.ReadFile(projection)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("budget refusal changed event projection: err=%v", err)
	}
}

func TestSQLiteSessionDAGAppendBudgetRejectsBeforeCommitting(t *testing.T) {
	for _, tc := range []struct {
		name      string
		maxEvents int
		maxBytes  func(header, next []byte) int64
	}{
		{
			name:      "record limit",
			maxEvents: 1,
			maxBytes:  func(_, _ []byte) int64 { return defaultSessionReplayLimits.maxBytes },
		},
		{
			name:      "projection byte limit",
			maxEvents: defaultSessionReplayLimits.maxRecords,
			maxBytes: func(header, next []byte) int64 {
				return int64(len(header) + len(next) - 1)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := prepareSQLiteEventTestSession(t)
			base := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
			headerEntries := []sessionDAGEntry{{Type: sessionDAGTypeLog, Generation: 1, At: base}}
			headerData, err := encodeSessionDAGEntries(headerEntries, base)
			if err != nil {
				t.Fatal(err)
			}
			limits := defaultSessionReplayLimits
			limits.maxRecords = tc.maxEvents
			limits.maxBytes = defaultSessionReplayLimits.maxBytes
			if _, handled, err := appendSQLiteDAGEventsWithinLimits(path, headerEntries, headerData, limits); err != nil || !handled {
				t.Fatalf("append initial log header: handled=%v err=%v", handled, err)
			}
			headerProjection, err := os.ReadFile(store.SessionEventLog(path))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(headerProjection, headerData) {
				t.Fatal("initial SQLite event projection differs from its canonical encoding")
			}
			nextEntries := []sessionDAGEntry{
				dagMessageEntry(t, SessionMainHead, "", "turn-budgeted", dagMsg("user", "must not partially commit", "budgeted-message"), base.Add(time.Second)),
			}
			nextData, err := encodeSessionDAGEntries(nextEntries, base)
			if err != nil {
				t.Fatal(err)
			}
			limits.maxBytes = tc.maxBytes(headerData, nextData)
			if _, handled, err := appendSQLiteDAGEventsWithinLimits(path, nextEntries, nextData, limits); !handled || !errors.Is(err, sessionidentity.ErrEventLimitExceeded) {
				t.Fatalf("over-budget append: handled=%v err=%v; want ErrEventLimitExceeded", handled, err)
			}
			db, id, enabled, err := sqliteSessionEventStore(path)
			if err != nil || !enabled {
				t.Fatalf("open event store after rejected append: enabled=%v err=%v", enabled, err)
			}
			status, err := db.EventStreamStatus(context.Background(), id)
			if err != nil || status.LastSequence != 1 {
				t.Fatalf("SQLite sequence after rejected append = %d, %v; want 1", status.LastSequence, err)
			}
			after, err := os.ReadFile(store.SessionEventLog(path))
			if err != nil || !bytes.Equal(after, headerProjection) {
				t.Fatalf("projection changed after rejected append: err=%v", err)
			}
			projected, active, err := sqliteDAGEventBytes(context.Background(), store.SessionEventLog(path), defaultSessionReplayLimits)
			if err != nil || !active || !bytes.Equal(projected, headerData) {
				t.Fatalf("committed SQLite state after rejected append: active=%v data_equal=%v err=%v", active, bytes.Equal(projected, headerData), err)
			}
		})
	}
}

func TestSQLiteSessionDAGRotationBudgetRejectsBeforeReplacingGeneration(t *testing.T) {
	for _, tc := range []struct {
		name      string
		entries   func(time.Time) []sessionDAGEntry
		setLimits func(*sessionReplayLimits, []byte)
	}{
		{
			name: "record limit",
			entries: func(at time.Time) []sessionDAGEntry {
				return []sessionDAGEntry{
					{Type: sessionDAGTypeLog, Generation: 2, At: at, Writer: "rotation-writer"},
					{Type: sessionDAGTypeSelect, ID: "selected-head", Head: SessionMainHead, At: at, Writer: "rotation-writer"},
				}
			},
			setLimits: func(limits *sessionReplayLimits, _ []byte) { limits.maxRecords = 1 },
		},
		{
			name: "projection byte limit",
			entries: func(at time.Time) []sessionDAGEntry {
				return []sessionDAGEntry{{Type: sessionDAGTypeLog, Generation: 2, At: at, Writer: strings.Repeat("writer", 16)}}
			},
			setLimits: func(limits *sessionReplayLimits, encoded []byte) { limits.maxBytes = int64(len(encoded) - 1) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := prepareSQLiteEventTestSession(t)
			base := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
			initial := []sessionDAGEntry{{Type: sessionDAGTypeLog, Generation: 1, At: base, Writer: "initial-writer"}}
			initialData, err := encodeSessionDAGEntries(initial, base)
			if err != nil {
				t.Fatal(err)
			}
			if _, handled, err := appendSQLiteDAGEventsWithinLimits(path, initial, initialData, defaultSessionReplayLimits); err != nil || !handled {
				t.Fatalf("append initial generation: handled=%v err=%v", handled, err)
			}
			before, err := os.ReadFile(store.SessionEventLog(path))
			if err != nil {
				t.Fatal(err)
			}
			replacement := tc.entries(base.Add(time.Second))
			replacementData, err := encodeSessionDAGEntries(replacement, base)
			if err != nil {
				t.Fatal(err)
			}
			limits := defaultSessionReplayLimits
			tc.setLimits(&limits, replacementData)
			if handled, err := replaceSQLiteDAGEventsWithinLimits(path, replacement, 1, limits); !handled || !errors.Is(err, ErrSessionReplayLimitExceeded) {
				t.Fatalf("over-budget rotation: handled=%v err=%v; want replay-limit refusal", handled, err)
			}
			db, id, enabled, err := sqliteSessionEventStore(path)
			if err != nil || !enabled {
				t.Fatalf("open event store after rejected rotation: enabled=%v err=%v", enabled, err)
			}
			status, err := db.EventStreamStatus(context.Background(), id)
			if err != nil || status.Generation != 1 || status.LastSequence != 1 {
				t.Fatalf("SQLite stream after rejected rotation = generation %d sequence %d, %v; want 1/1", status.Generation, status.LastSequence, err)
			}
			after, err := os.ReadFile(store.SessionEventLog(path))
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("event projection changed after rejected rotation: err=%v", err)
			}
		})
	}
}

func TestSQLiteSessionEventProjectionAcceptsExactly128MiBAndRejectsOneByteOver(t *testing.T) {
	limit := sessionEventReplayMaxBytes
	if err := checkSQLiteProjectionBudget("projection", limit-1, 1, limit); err != nil {
		t.Fatalf("exact 128 MiB projection boundary rejected: %v", err)
	}
	err := checkSQLiteProjectionBudget("projection", limit, 1, limit)
	var budgetErr *SessionReplayLimitError
	if !errors.As(err, &budgetErr) || budgetErr.Value != limit+1 || budgetErr.Limit != limit {
		t.Fatalf("one-byte-over boundary error = %#v; want value %d limit %d", err, limit+1, limit)
	}
}

func TestSQLiteSessionDAGRoundTripsMessageNear128MiBReplayLimit(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	const messageBytes = 120 << 20
	message := strings.Repeat("m", messageBytes)
	entries := []sessionDAGEntry{
		{Type: sessionDAGTypeLog, Generation: 1, At: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		dagMessageEntry(t, SessionMainHead, "", "turn-large", dagMsg(provider.RoleUser, message, "large-message"), time.Date(2026, 9, 1, 0, 0, 1, 0, time.UTC)),
	}
	if _, err := appendSessionDAGEntries(path, entries, true); err != nil {
		t.Fatalf("append near-limit SQLite message: %v", err)
	}
	state, err := replaySessionDAG(context.Background(), store.SessionEventLog(path), defaultSessionReplayLimits)
	if err != nil || !state.sqliteBacked || state.records != len(entries) {
		t.Fatalf("replay near-limit SQLite message: backing=%v records=%d err=%v", state.sqliteBacked, state.records, err)
	}
	messages, _ := state.materialize(SessionMainHead)
	if len(messages) != 1 || len(messages[0].Content) != messageBytes || messages[0].Content != message {
		t.Fatalf("near-limit message round trip: messages=%d content_bytes=%d", len(messages), func() int {
			if len(messages) == 0 {
				return 0
			}
			return len(messages[0].Content)
		}())
	}
}

func TestSQLiteSessionDAGRoundTripsToolCallImageAndResultForLegacyExport(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	base := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	user := dagMsg(provider.RoleUser, "run the check", "tool-user")
	user.RawContent = "run the check with the attached reference"
	user.Images = []string{"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p1sAAAAASUVORK5CYII="}
	assistant := provider.Message{
		Role:             provider.RoleAssistant,
		ID:               "tool-assistant",
		ReasoningContent: "checking the attached reference before running the command",
		DecisionReceipts: []*provider.DecisionReceipt{{
			ID: "approval-call-1", Kind: "tool", Tool: "bash", Subject: "report.txt", Outcome: "allow_once",
		}},
		ToolCalls: []provider.ToolCall{{
			ID: "call-1", Name: "bash", Arguments: `{"command":"printf ready"}`,
			WriteIntents: []json.RawMessage{json.RawMessage(`{"version":1,"path":"report.txt"}`)},
			Diff:         "report.txt created", Added: 1,
		}},
	}
	toolResult := provider.Message{Role: provider.RoleTool, ID: "tool-result", Name: "bash", ToolCallID: "call-1", Content: "ready"}
	entries := []sessionDAGEntry{
		{Type: sessionDAGTypeLog, Generation: 1, At: base},
		dagMessageEntry(t, SessionMainHead, "", "tool-turn", user, base.Add(time.Second)),
		dagMessageEntry(t, SessionMainHead, user.ID, "tool-turn", assistant, base.Add(2*time.Second)),
		dagMessageEntry(t, SessionMainHead, assistant.ID, "tool-turn", toolResult, base.Add(3*time.Second)),
	}
	if _, err := appendSessionDAGEntries(path, entries, true); err != nil {
		t.Fatalf("append tool-call DAG: %v", err)
	}
	state, err := replaySessionDAG(context.Background(), store.SessionEventLog(path), defaultSessionReplayLimits)
	if err != nil || !state.sqliteBacked {
		t.Fatalf("replay tool-call SQLite DAG: backing=%v err=%v", state.sqliteBacked, err)
	}
	want := []provider.Message{user, assistant, toolResult}
	got, _ := state.materialize(SessionMainHead)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SQLite tool-call history differs:\ngot:  %#v\nwant: %#v", got, want)
	}
	destination := filepath.Join(t.TempDir(), "tool-call-legacy.jsonl")
	if err := ExportSessionSchemaOne(path, destination); err != nil {
		t.Fatalf("export SQLite tool-call history for legacy reader: %v", err)
	}
	legacy, err := LoadSession(destination)
	if err != nil || !reflect.DeepEqual(legacy.Messages, want) {
		t.Fatalf("legacy tool-call round trip differs: got=%#v err=%v", legacy.Messages, err)
	}
}

func TestSQLiteSessionDAGReadsAndReplaysOneHundredThousandEvents(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if _, err := appendSessionDAGEntries(path, []sessionDAGEntry{{Type: sessionDAGTypeLog, Generation: 1, At: base}}, true); err != nil {
		t.Fatal(err)
	}
	db, sessionID, enabled, err := sqliteSessionEventStore(path)
	if err != nil || !enabled {
		t.Fatalf("open managed event store: enabled=%v err=%v", enabled, err)
	}
	const targetRecords = 100_000
	const batchSize = 5_000
	for start := 0; start < targetRecords-1; start += batchSize {
		end := min(start+batchSize, targetRecords-1)
		events := make([]sessionidentity.SessionEvent, 0, end-start)
		for i := start; i < end; i++ {
			entry := sessionDAGEntry{SchemaVersion: sessionDAGSchemaVersion, Type: sessionDAGTypeSelect, ID: fmt.Sprintf("stress-%06d", i), Head: SessionMainHead, At: base}
			payload, err := json.Marshal(entry)
			if err != nil {
				t.Fatal(err)
			}
			events = append(events, sessionidentity.SessionEvent{ID: entry.ID, Type: entry.Type, HeadID: entry.Head, CreatedAt: base.UnixMilli(), Payload: payload})
		}
		if _, err := db.AppendEvents(context.Background(), sessionID, events); err != nil {
			t.Fatalf("append stress batch %d:%d: %v", start, end, err)
		}
	}
	data, active, err := sqliteDAGEventBytes(context.Background(), store.SessionEventLog(path), defaultSessionReplayLimits)
	if err != nil || !active {
		t.Fatalf("read 100k-event SQLite projection: active=%v err=%v", active, err)
	}
	state, err := replaySessionDAGBytes(context.Background(), store.SessionEventLog(path), data, defaultSessionReplayLimits, true)
	if err != nil || state.records != targetRecords || state.generation != 1 {
		t.Fatalf("100k-event replay: records=%d generation=%d err=%v", state.records, state.generation, err)
	}
	limits := defaultSessionReplayLimits
	limits.maxRecords = targetRecords - 1
	if _, _, err := sqliteDAGEventBytes(context.Background(), store.SessionEventLog(path), limits); !errors.Is(err, ErrSessionReplayLimitExceeded) {
		t.Fatalf("100k-event over-budget error = %v; want ErrSessionReplayLimitExceeded", err)
	}
}

func TestSQLiteSessionKeepsApprovalAndCancellationInTurnLedger(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	session := NewSession("system")
	session.Add(dagMsg("user", "persist lifecycle", ""))
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	ledger, err := turnevent.Open(path, BranchID(path))
	if err != nil {
		t.Fatal(err)
	}
	turnID, err := ledger.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		event  event.Event
		status event.TurnStatus
	}{
		{event.Event{Kind: event.TurnStarted}, event.TurnInProgress},
		{event.Event{Kind: event.ApprovalRequest, Approval: event.Approval{ID: "approval-allow", Tool: "bash", Subject: "run command"}}, event.TurnInProgress},
		{event.Event{Kind: event.PromptAnswered, ItemID: "approval-allow"}, event.TurnInProgress},
		{event.Event{Kind: event.Notice, Level: event.LevelInfo, Code: event.NoticeCodeDecisionReceipt, DecisionReceipt: &provider.DecisionReceipt{ID: "approval-allow", Kind: "tool", Tool: "bash", Subject: "run command", Outcome: "allow_once"}}, event.TurnInProgress},
		{event.Event{Kind: event.ApprovalRequest, Approval: event.Approval{ID: "approval-deny", Tool: "bash", Subject: "run another command"}}, event.TurnInProgress},
		{event.Event{Kind: event.PromptAnswered, ItemID: "approval-deny"}, event.TurnInProgress},
		{event.Event{Kind: event.Notice, Level: event.LevelInfo, Code: event.NoticeCodeDecisionReceipt, DecisionReceipt: &provider.DecisionReceipt{ID: "approval-deny", Kind: "tool", Tool: "bash", Subject: "run another command", Outcome: "deny"}}, event.TurnInProgress},
		{event.Event{Kind: event.TurnStatusChanged}, event.TurnCancelling},
		{event.Event{Kind: event.TurnDone, Outcome: "cancelled"}, event.TurnInterrupted},
	} {
		if _, ok, err := ledger.Append(step.event, step.status); err != nil || !ok {
			t.Fatalf("append turn lifecycle kind %d: ok=%v err=%v", step.event.Kind, ok, err)
		}
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := turnevent.Open(path, BranchID(path))
	if err != nil {
		t.Fatalf("reopen turn ledger beside SQLite transcript: %v", err)
	}
	defer reopened.Close()
	rows, err := reopened.EventsAfter(0)
	if err != nil {
		t.Fatalf("turn lifecycle after reopen: %v", err)
	}
	var lifecycle []turnevent.Envelope
	for _, row := range rows {
		if row.TurnID == turnID {
			lifecycle = append(lifecycle, row)
		}
	}
	if len(lifecycle) != 9 || lifecycle[1].Event.Approval == nil || lifecycle[1].Event.Approval.ID != "approval-allow" ||
		lifecycle[3].Event.DecisionReceipt == nil || lifecycle[3].Event.DecisionReceipt.Outcome != "allow_once" ||
		lifecycle[4].Event.Approval == nil || lifecycle[4].Event.Approval.ID != "approval-deny" ||
		lifecycle[6].Event.DecisionReceipt == nil || lifecycle[6].Event.DecisionReceipt.Outcome != "deny" ||
		lifecycle[7].Status != event.TurnCancelling || lifecycle[8].Status != event.TurnInterrupted {
		t.Fatalf("approval/cancellation lifecycle changed beside SQLite transcript: %#v", lifecycle)
	}
}

func TestSQLiteSessionSaveUsesManagedPreviewEventStore(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	session := NewSession("system prompt")
	session.AddBatch(dagMsg("user", "hello", ""), dagMsg("assistant", "world", ""))
	if err := session.Save(path); err != nil {
		t.Fatalf("save managed Preview session: %v", err)
	}
	status, err := func() (sessionidentity.SessionEventStreamStatus, error) {
		db, id, enabled, err := sqliteSessionEventStore(path)
		if err != nil || !enabled {
			return sessionidentity.SessionEventStreamStatus{}, err
		}
		return db.EventStreamStatus(context.Background(), id)
	}()
	if err != nil || status.LastSequence < 4 || status.ProjectionSequence != status.LastSequence {
		t.Fatalf("managed Preview stream status = %#v, %v", status, err)
	}
	loaded, err := LoadSession(path)
	if err != nil {
		t.Fatalf("load managed Preview session: %v", err)
	}
	if got := dagContents(loaded.Snapshot()); len(got) != 3 || got[0] != "system prompt" || got[2] != "world" {
		t.Fatalf("managed Preview messages = %#v", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("compatibility transcript checkpoint was not written: %v", err)
	}
}

func TestSQLiteSessionUpgradesLegacyCheckpointAsLinearHistory(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	t.Setenv(previewSQLiteEventsEnv, "0")
	t.Setenv(SessionLogSchemaEnv, "v1")
	legacy := NewSession("legacy system")
	legacy.AddBatch(dagMsg(provider.RoleUser, "old question", "legacy-user"), dagMsg(provider.RoleAssistant, "old answer", "legacy-assistant"))
	if err := legacy.Save(path); err != nil {
		t.Fatalf("write isolated schema-1 checkpoint: %v", err)
	}
	legacyCheckpoint, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a legacy release that kept only the transcript checkpoint.
	if err := os.Remove(store.SessionEventLog(path)); err != nil {
		t.Fatalf("remove legacy event sidecar for checkpoint-only fixture: %v", err)
	}
	if _, err := os.Stat(store.SessionEventLog(path)); !os.IsNotExist(err) {
		t.Fatalf("schema-1 checkpoint unexpectedly has event history: stat err=%v", err)
	}

	t.Setenv(previewSQLiteEventsEnv, "1")
	t.Setenv(SessionLogSchemaEnv, "")
	loaded, err := LoadSession(path)
	if err != nil {
		t.Fatalf("load legacy checkpoint in Preview: %v", err)
	}
	lease, err := TryAcquireSessionLease(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	loaded.Add(dagMsg(provider.RoleUser, "new question", "new-user"))
	if err := loaded.Save(path); err != nil {
		t.Fatalf("upgrade and append from legacy checkpoint: %v", err)
	}
	state, err := replaySessionDAG(context.Background(), store.SessionEventLog(path), defaultSessionReplayLimits)
	if err != nil || !state.sqliteBacked || state.upgradedFrom != sessionEventSchemaVersion || len(state.heads) != 1 {
		t.Fatalf("legacy checkpoint upgrade state: backing=%v upgradedFrom=%d heads=%d err=%v", state.sqliteBacked, state.upgradedFrom, len(state.heads), err)
	}
	messages, _ := state.materialize(SessionMainHead)
	if got := dagContents(messages); !reflect.DeepEqual(got, []string{"legacy system", "old question", "old answer", "new question"}) {
		t.Fatalf("linear checkpoint migration history = %#v", got)
	}
	if status, err := func() (sessionidentity.SessionEventStreamStatus, error) {
		db, id, enabled, err := sqliteSessionEventStore(path)
		if err != nil || !enabled {
			return sessionidentity.SessionEventStreamStatus{}, err
		}
		return db.EventStreamStatus(context.Background(), id)
	}(); err != nil || !status.ImportVerified || status.ImportSourceSHA256 == "" {
		t.Fatalf("verified checkpoint-upgrade event stream = %#v, %v", status, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("legacy-reader transcript checkpoint missing after upgrade: %v", err)
	}
	if len(legacyCheckpoint) == 0 {
		t.Fatal("legacy fixture checkpoint was empty")
	}
	t.Setenv(previewSQLiteEventsEnv, "0")
	t.Setenv(SessionLogSchemaEnv, "v1")
	oldReader, err := LoadSession(path)
	if err != nil || !reflect.DeepEqual(dagContents(oldReader.Messages), []string{"legacy system", "old question", "old answer", "new question"}) {
		t.Fatalf("old schema-1 reader failed after checkpoint migration: messages=%#v err=%v", dagContents(oldReader.Messages), err)
	}
}

func TestSQLiteSessionDAGImportsLegacyGenerationAndRepairsProjection(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	entries := []sessionDAGEntry{{Type: sessionDAGTypeLog, Generation: 4, At: base}}
	entries = append(entries, dagMessageEntry(t, SessionMainHead, "", "turn-legacy", dagMsg("user", "import me", "legacy-1"), base.Add(time.Second)))
	data, err := encodeSessionDAGEntries(entries, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.SessionEventLog(path), data, 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := replaySessionDAG(context.Background(), store.SessionEventLog(path), defaultSessionReplayLimits)
	if err != nil {
		t.Fatalf("import and replay legacy DAG: %v", err)
	}
	if !state.sqliteBacked || state.generation != 4 || state.records != 2 {
		t.Fatalf("imported replay backing=%v generation=%d records=%d", state.sqliteBacked, state.generation, state.records)
	}
	db, id, _, err := sqliteSessionEventStore(path)
	if err != nil {
		t.Fatal(err)
	}
	status, err := db.EventStreamStatus(context.Background(), id)
	if err != nil || !status.ImportVerified || status.ImportSourceSHA256 == "" {
		t.Fatalf("legacy import verification status = %#v, %v", status, err)
	}
	if err := os.WriteFile(store.SessionEventLog(path), []byte("stale projection\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err = replaySessionDAG(context.Background(), store.SessionEventLog(path), defaultSessionReplayLimits)
	if err != nil || !state.sqliteBacked || state.records != 2 {
		t.Fatalf("SQLite recovery from stale projection = state %#v, err %v", state, err)
	}
	repaired, err := os.ReadFile(store.SessionEventLog(path))
	if err != nil || !bytes.Equal(repaired, data) {
		t.Fatalf("repaired event projection = %q, %v; want imported bytes %q", repaired, err, data)
	}
}

func TestSQLiteSessionUnverifiedImportCannotBypassShadowValidationAfterRestart(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	base := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	entries := []sessionDAGEntry{
		{Type: sessionDAGTypeLog, Generation: 3, At: base},
		dagMessageEntry(t, SessionMainHead, "", "turn-unverified", dagMsg(provider.RoleUser, "pending verification", "pending-1"), base.Add(time.Second)),
	}
	data, err := encodeSessionDAGEntries(entries, base)
	if err != nil {
		t.Fatal(err)
	}
	pathToLog := store.SessionEventLog(path)
	if err := os.WriteFile(pathToLog, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, sessionID, _, err := sqliteSessionEventStore(path)
	if err != nil {
		t.Fatal(err)
	}
	events, err := decodeSQLiteSessionEvents(data)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := replaySessionDAGFile(context.Background(), pathToLog, defaultSessionReplayLimits)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if _, err := db.ImportEvents(context.Background(), sessionID, hex.EncodeToString(digest[:]), legacy.generation, events); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(pathToLog); err != nil {
		t.Fatal(err)
	}
	if _, err := replaySessionDAG(context.Background(), pathToLog, defaultSessionReplayLimits); err == nil {
		t.Fatal("unverified SQLite import was trusted after the source log disappeared")
	}
	status, err := db.EventStreamStatus(context.Background(), sessionID)
	if err != nil || status.ImportVerified {
		t.Fatalf("interrupted import status = %#v err=%v; want unverified", status, err)
	}
}

func TestSQLiteSessionDamagedLegacyImportIsAtomicAndLeavesSourceUntouched(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	base := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	entries := []sessionDAGEntry{
		{Type: sessionDAGTypeLog, Generation: 1, At: base},
		dagMessageEntry(t, SessionMainHead, "", "turn-damaged-import", dagMsg(provider.RoleUser, "valid prefix", "valid-message"), base.Add(time.Second)),
	}
	data, err := encodeSessionDAGEntries(entries, base)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte(`{"schema_version":2,"type":"message","id":`)...)
	data = append(data, '\n')
	pathToLog := store.SessionEventLog(path)
	if err := os.WriteFile(pathToLog, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, sessionID, enabled, err := sqliteSessionEventStore(path)
	if err != nil || !enabled {
		t.Fatalf("open isolated event store: enabled=%v err=%v", enabled, err)
	}
	if _, err := replaySessionDAG(context.Background(), pathToLog, defaultSessionReplayLimits); err == nil {
		t.Fatal("damaged legacy event log was imported")
	}
	last, err := db.LastEventSequence(context.Background(), sessionID)
	if err != nil || last != 0 {
		t.Fatalf("partial events persisted after rejected import: last=%d err=%v", last, err)
	}
	status, err := db.EventStreamStatus(context.Background(), sessionID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("event stream after rejected import = %#v, %v; want no stream", status, err)
	}
	after, err := os.ReadFile(pathToLog)
	if err != nil || !bytes.Equal(after, data) {
		t.Fatalf("rejected import changed source bytes: err=%v", err)
	}
}

func TestSQLiteSessionDAGRotationKeepsReducerStateAndAdvancesGeneration(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	dagLinearLog(t, path)
	lease, err := TryAcquireSessionLease(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	state := dagReplay(t, path)
	if err := sessionDAGSingleWriterProof(path, state, time.Now().UTC()); err != nil {
		t.Fatalf("single writer proof: %v", err)
	}
	if err := rotateSessionDAG(path, state, time.Now().UTC()); err != nil {
		t.Fatalf("rotate SQLite DAG: %v", err)
	}
	rotated := dagReplay(t, path)
	if !rotated.sqliteBacked || rotated.generation != state.generation+1 || rotated.records == 0 {
		t.Fatalf("rotated SQLite DAG backing=%v generation=%d records=%d", rotated.sqliteBacked, rotated.generation, rotated.records)
	}
	messages, _ := rotated.materialize(SessionMainHead)
	if got := dagContents(messages); len(got) != 4 || got[0] != "sys" || got[3] != "q2" {
		t.Fatalf("rotated main history = %#v", got)
	}
}

func TestSQLiteSessionDAGCommittedEventsSurviveProjectionFailure(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	projectionPath := store.SessionEventLog(path)
	if err := os.Mkdir(projectionPath, 0o700); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	entries := []sessionDAGEntry{{Type: sessionDAGTypeLog, Generation: 1, At: base}}
	entries = append(entries, dagMessageEntry(t, SessionMainHead, "", "turn-projection", dagMsg("user", "database wins", "projection-1"), base.Add(time.Second)))
	if _, err := appendSessionDAGEntries(path, entries, true); err != nil {
		t.Fatalf("SQLite commit with projection failure: %v", err)
	}
	state, err := replaySessionDAG(context.Background(), projectionPath, defaultSessionReplayLimits)
	if err != nil || !state.sqliteBacked || state.records != 2 {
		t.Fatalf("replay after projection failure = backing %v records %d, err %v", state.sqliteBacked, state.records, err)
	}
	if err := os.RemoveAll(projectionPath); err != nil {
		t.Fatal(err)
	}
	if _, err := replaySessionDAG(context.Background(), projectionPath, defaultSessionReplayLimits); err != nil {
		t.Fatalf("rebuild removed projection from committed SQLite events: %v", err)
	}
	info, err := os.Stat(projectionPath)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("rebuilt projection stat = %#v, %v", info, err)
	}
}

func TestSQLiteSessionDAGCrashAfterCommitRecoversFromDatabase(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	entries := []sessionDAGEntry{{Type: sessionDAGTypeLog, Generation: 1, At: base}}
	entries = append(entries, dagMessageEntry(t, SessionMainHead, "", "turn-crash", dagMsg("user", "committed", "crash-1"), base.Add(time.Second)))
	previousCrashPoint := fileutil.CrashPoint
	t.Cleanup(func() { fileutil.CrashPoint = previousCrashPoint })
	fileutil.CrashPoint = func(op, _ string) {
		if op == "dag-sqlite-committed" {
			panic("simulated process crash after SQLite commit")
		}
	}
	crashed := false
	func() {
		defer func() {
			crashed = recover() != nil
		}()
		_, _ = appendSessionDAGEntries(path, entries, true)
	}()
	if !crashed {
		t.Fatal("expected injected crash after SQLite commit")
	}
	fileutil.CrashPoint = previousCrashPoint
	state, err := replaySessionDAG(context.Background(), store.SessionEventLog(path), defaultSessionReplayLimits)
	if err != nil || !state.sqliteBacked || state.records != 2 {
		t.Fatalf("replay after commit crash = backing %v records %d, err %v", state.sqliteBacked, state.records, err)
	}
	if _, err := os.Stat(store.SessionEventLog(path)); err != nil {
		t.Fatalf("recovery did not rebuild JSONL projection: %v", err)
	}
}

func TestSQLiteSessionDAGCrashAfterProjectionStageRecoversFromDatabase(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	base := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	first := []sessionDAGEntry{
		{Type: sessionDAGTypeLog, Generation: 1, At: base},
		dagMessageEntry(t, SessionMainHead, "", "turn-stage", dagMsg("user", "first", "stage-1"), base.Add(time.Second)),
	}
	if _, err := appendSessionDAGEntries(path, first, true); err != nil {
		t.Fatalf("append initial SQLite DAG: %v", err)
	}
	projectionPath := store.SessionEventLog(path)
	projectionBefore, err := os.ReadFile(projectionPath)
	if err != nil {
		t.Fatal(err)
	}
	previousCrashPoint := fileutil.CrashPoint
	t.Cleanup(func() { fileutil.CrashPoint = previousCrashPoint })
	fileutil.CrashPoint = func(op, target string) {
		if op == "staged-write-publish" && target == projectionPath {
			panic("simulated process crash after projection stage")
		}
	}
	crashed := false
	func() {
		defer func() { crashed = recover() != nil }()
		entry := dagMessageEntry(t, SessionMainHead, "stage-1", "turn-stage", dagMsg("assistant", "committed", "stage-2"), base.Add(2*time.Second))
		_, _ = appendSessionDAGEntries(path, []sessionDAGEntry{entry}, true)
	}()
	if !crashed {
		t.Fatal("expected injected crash after projection staging")
	}
	fileutil.CrashPoint = previousCrashPoint
	projectionAfterCrash, err := os.ReadFile(projectionPath)
	if err != nil || !bytes.Equal(projectionAfterCrash, projectionBefore) {
		t.Fatalf("projection changed before staged publish: err=%v", err)
	}
	state, err := replaySessionDAG(context.Background(), projectionPath, defaultSessionReplayLimits)
	if err != nil || !state.sqliteBacked || state.records != 3 {
		t.Fatalf("replay after staged projection crash: backing=%v records=%d err=%v", state.sqliteBacked, state.records, err)
	}
	messages, _ := state.materialize(SessionMainHead)
	if got := dagContents(messages); len(got) != 2 || got[1] != "committed" {
		t.Fatalf("recovered history after staged projection crash = %#v", got)
	}
	projectionAfterRecovery, err := os.ReadFile(projectionPath)
	if err != nil || bytes.Equal(projectionAfterRecovery, projectionBefore) {
		t.Fatalf("projection was not rebuilt after staged publish crash: err=%v", err)
	}
}

func TestSQLiteSessionDAGCrashAfterProjectionPublishRepairsWatermark(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	base := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	first := []sessionDAGEntry{
		{Type: sessionDAGTypeLog, Generation: 1, At: base},
		dagMessageEntry(t, SessionMainHead, "", "turn-publish", dagMsg("user", "first", "publish-1"), base.Add(time.Second)),
	}
	if _, err := appendSessionDAGEntries(path, first, true); err != nil {
		t.Fatalf("append initial SQLite DAG: %v", err)
	}
	projectionPath := store.SessionEventLog(path)
	previousCrashPoint := fileutil.CrashPoint
	t.Cleanup(func() { fileutil.CrashPoint = previousCrashPoint })
	fileutil.CrashPoint = func(op, target string) {
		if op == "dag-sqlite-projection-published" && target == projectionPath {
			panic("simulated process crash after projection publish")
		}
	}
	crashed := false
	func() {
		defer func() { crashed = recover() != nil }()
		entry := dagMessageEntry(t, SessionMainHead, "publish-1", "turn-publish", dagMsg("assistant", "committed", "publish-2"), base.Add(2*time.Second))
		_, _ = appendSessionDAGEntries(path, []sessionDAGEntry{entry}, true)
	}()
	if !crashed {
		t.Fatal("expected injected crash after projection publish")
	}
	fileutil.CrashPoint = previousCrashPoint
	db, sessionID, _, err := sqliteSessionEventStore(path)
	if err != nil {
		t.Fatal(err)
	}
	status, err := db.EventStreamStatus(context.Background(), sessionID)
	if err != nil || status.LastSequence != 3 || status.ProjectionSequence >= status.LastSequence {
		t.Fatalf("pre-recovery event watermarks = %#v, %v; want committed events with pending projection mark", status, err)
	}
	projection, err := os.ReadFile(projectionPath)
	if err != nil || !bytes.Contains(projection, []byte(`"id":"publish-2"`)) {
		t.Fatalf("published projection after injected crash does not contain committed event: err=%v", err)
	}
	state, err := replaySessionDAG(context.Background(), projectionPath, defaultSessionReplayLimits)
	if err != nil || !state.sqliteBacked || state.records != 3 {
		t.Fatalf("replay after published projection crash: backing=%v records=%d err=%v", state.sqliteBacked, state.records, err)
	}
	status, err = db.EventStreamStatus(context.Background(), sessionID)
	if err != nil || status.ProjectionSequence != status.LastSequence || status.ProjectionSHA256 == "" {
		t.Fatalf("repaired projection watermark = %#v, %v", status, err)
	}
}

func TestSQLiteSessionDAGRecoversAfterAbruptProjectionCrash(t *testing.T) {
	if mode := os.Getenv(sqliteDAGCrashModeEnv); mode != "" {
		path := os.Getenv(sqliteDAGCrashPathEnv)
		os.Setenv("REASONIX_STATE_HOME", os.Getenv(sqliteDAGCrashRootEnv))
		os.Setenv(previewSQLiteEventsEnv, "1")
		fileutil.CrashPoint = func(op, target string) {
			if op == mode && target == store.SessionEventLog(path) {
				os.Exit(sqliteDAGCrashExit)
			}
		}
		base := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
		entries := []sessionDAGEntry{
			{Type: sessionDAGTypeLog, Generation: 1, At: base},
			dagMessageEntry(t, SessionMainHead, "", "turn-process-crash", dagMsg(provider.RoleUser, "committed before crash", "process-crash-message"), base.Add(time.Second)),
		}
		if _, err := appendSessionDAGEntries(path, entries, true); err != nil {
			t.Fatalf("append before injected process exit: %v", err)
		}
		t.Fatal("crash injection point was not reached")
	}

	for _, mode := range []string{"dag-sqlite-committed", "staged-write-publish", "dag-sqlite-projection-published"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("REASONIX_STATE_HOME", root)
			t.Setenv(previewSQLiteEventsEnv, "1")
			sessionDir := config.SessionDir()
			if err := os.MkdirAll(sessionDir, 0o700); err != nil {
				t.Fatal(err)
			}
			id := "sqlite-crash-" + mode
			path := filepath.Join(sessionDir, "tauri-"+id+".jsonl")
			identities, err := sessionidentity.Open(context.Background(), config.DesktopSessionIdentityPath(), root)
			if err != nil {
				t.Fatal(err)
			}
			if err := identities.Reserve(context.Background(), sessionDir, sessionidentity.Candidate{ID: id, Path: path}); err != nil {
				_ = identities.Close()
				t.Fatal(err)
			}
			if err := identities.Close(); err != nil {
				t.Fatal(err)
			}

			command := exec.Command(os.Args[0], "-test.run=^TestSQLiteSessionDAGRecoversAfterAbruptProjectionCrash$")
			command.Env = append(os.Environ(), sqliteDAGCrashModeEnv+"="+mode, sqliteDAGCrashPathEnv+"="+path, sqliteDAGCrashRootEnv+"="+root)
			output, err := command.CombinedOutput()
			var exitError *exec.ExitError
			exitCode := -1
			if errors.As(err, &exitError) {
				exitCode = exitError.ExitCode()
			}
			if exitCode != sqliteDAGCrashExit {
				t.Fatalf("crash writer exit = %v (code %d), output: %s", err, exitCode, output)
			}

			db, sessionID, enabled, err := sqliteSessionEventStore(path)
			if err != nil || !enabled {
				t.Fatalf("reopen event store after process exit: enabled=%v err=%v", enabled, err)
			}
			t.Cleanup(func() {
				if cached, ok := sessionEventStores.Load(filepath.Clean(config.DesktopSessionIdentityPath())); ok {
					_ = cached.(*sessionidentity.Store).Close()
					sessionEventStores.Delete(filepath.Clean(config.DesktopSessionIdentityPath()))
				}
			})
			status, err := db.EventStreamStatus(context.Background(), sessionID)
			if err != nil || status.LastSequence != 2 {
				t.Fatalf("committed event stream after abrupt %s exit = %#v, %v", mode, status, err)
			}
			state, err := replaySessionDAG(context.Background(), store.SessionEventLog(path), defaultSessionReplayLimits)
			if err != nil || !state.sqliteBacked || state.records != 2 {
				t.Fatalf("recover projection after abrupt %s exit: backing=%v records=%d err=%v", mode, state.sqliteBacked, state.records, err)
			}
			messages, _ := state.materialize(SessionMainHead)
			if len(messages) != 1 || messages[0].Content != "committed before crash" {
				t.Fatalf("recovered message after abrupt %s exit = %#v", mode, messages)
			}
			status, err = db.EventStreamStatus(context.Background(), sessionID)
			if err != nil || status.ProjectionSequence != status.LastSequence {
				t.Fatalf("projection watermark after abrupt %s recovery = %#v, %v", mode, status, err)
			}
		})
	}
}

func TestSQLiteSessionCheckpointProjectionRecoversAfterCrash(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	session := NewSession("system prompt")
	session.Add(dagMsg("user", "recover checkpoint", ""))
	previousCrashPoint := fileutil.CrashPoint
	t.Cleanup(func() { fileutil.CrashPoint = previousCrashPoint })
	fileutil.CrashPoint = func(op, _ string) {
		if op == "session-checkpoint" {
			panic("simulated crash before transcript checkpoint publish")
		}
	}
	crashed := false
	func() {
		defer func() { crashed = recover() != nil }()
		_ = session.Save(path)
	}()
	if !crashed {
		t.Fatal("expected injected checkpoint crash")
	}
	fileutil.CrashPoint = previousCrashPoint
	loaded, err := LoadSession(path)
	if err != nil {
		t.Fatalf("load after checkpoint crash: %v", err)
	}
	if got := dagContents(loaded.Snapshot()); len(got) != 2 || got[1] != "recover checkpoint" {
		t.Fatalf("recovered transcript = %#v", got)
	}
	checkpoint, err := os.ReadFile(path)
	if err != nil || len(checkpoint) == 0 {
		t.Fatalf("compatibility checkpoint after recovery = %q, %v", checkpoint, err)
	}
	db, id, _, err := sqliteSessionEventStore(path)
	if err != nil {
		t.Fatal(err)
	}
	status, err := db.EventStreamStatus(context.Background(), id)
	if err != nil || status.CheckpointSequence != status.LastSequence || status.CheckpointSHA256 == "" {
		t.Fatalf("checkpoint watermark after recovery = %#v, %v", status, err)
	}
}

func TestSQLiteSessionDAGShadowReplayMatchesJSONLForBranchesAndRedaction(t *testing.T) {
	path := prepareSQLiteEventTestSession(t)
	_, base := dagLinearLog(t, path)
	dagAppend(t, path,
		sessionDAGEntry{Type: sessionDAGTypeFork, Head: SessionMainHead, NewHead: "fork-shadow", From: "A1", Kind: HeadKindFork, Name: "review", At: base.Add(10 * time.Second)},
		dagMessageEntry(t, "fork-shadow", "A1", "turn-fork", dagMsg("user", "alternate", "U-alt"), base.Add(11*time.Second)),
		dagMessageEntry(t, "fork-shadow", "U-alt", "turn-fork", dagMsg("user", summaryTagOpen+"condensed history"+summaryTagClose, "U-summary"), base.Add(11*time.Second+500*time.Microsecond)),
		sessionDAGEntry{Type: sessionDAGTypeRename, Head: "fork-shadow", Name: "reviewed", At: base.Add(11*time.Second + time.Millisecond)},
		sessionDAGEntry{Type: sessionDAGTypeTurnBegin, Head: "fork-shadow", Turn: "turn-open", Leaf: "U-alt", Writer: "writer-shadow", At: base.Add(11*time.Second + 2*time.Millisecond)},
		sessionDAGEntry{Type: sessionDAGTypeTurnEnd, Head: "fork-shadow", Turn: "turn-open", Writer: "writer-shadow", At: base.Add(11*time.Second + 3*time.Millisecond)},
		sessionDAGEntry{Type: sessionDAGTypeCompaction, Head: "fork-shadow", CoveredLeaf: "U-alt", CoveredCount: 4, PrefixHash: "shadow-prefix", At: base.Add(11*time.Second + 4*time.Millisecond)},
		sessionDAGEntry{Type: sessionDAGTypeRewind, Head: SessionMainHead, To: "U1", Cause: "shadow test", At: base.Add(12 * time.Second)},
		sessionDAGEntry{Type: sessionDAGTypeSelect, Head: "fork-shadow", At: base.Add(13 * time.Second)},
		sessionDAGEntry{Type: sessionDAGTypeWriter, Head: "fork-shadow", Writer: "writer-shadow", PID: 42, Hostname: "preview-test", LeaseGeneration: 3, At: base.Add(13*time.Second + time.Millisecond)},
	)
	redacted, err := encodeSessionDAGMessage(dagMsg("assistant", "[redacted]", ""))
	if err != nil {
		t.Fatal(err)
	}
	dagAppend(t, path, sessionDAGEntry{Type: sessionDAGTypeRedact, Head: "fork-shadow", Targets: map[string]json.RawMessage{"A1": redacted}, Reason: "test secret", At: base.Add(14 * time.Second)})
	logPath := store.SessionEventLog(path)
	sqliteState, err := replaySessionDAG(context.Background(), logPath, defaultSessionReplayLimits)
	if err != nil {
		t.Fatal(err)
	}
	jsonlState, err := replaySessionDAGFile(context.Background(), logPath, defaultSessionReplayLimits)
	if err != nil {
		t.Fatal(err)
	}
	if !sessionDAGShadowSemanticsEqual(jsonlState, sqliteState) {
		t.Fatal("SQLite reducer and JSONL reducer differ in DAG metadata, heads, turns, redactions, compaction, or materialized messages")
	}
	if !sqliteState.sqliteBacked || sqliteState.records != jsonlState.records || sqliteState.selectedHead() != jsonlState.selectedHead() {
		t.Fatalf("shadow state differs: sqlite records/head %d/%q; JSONL %d/%q", sqliteState.records, sqliteState.selectedHead(), jsonlState.records, jsonlState.selectedHead())
	}
	for _, head := range []string{SessionMainHead, "fork-shadow"} {
		sqliteMessages, _ := sqliteState.materialize(head)
		jsonlMessages, _ := jsonlState.materialize(head)
		if !reflect.DeepEqual(sqliteMessages, jsonlMessages) {
			t.Fatalf("shadow messages for head %s differ:\nSQLite=%#v\nJSONL=%#v", head, sqliteMessages, jsonlMessages)
		}
	}
	destination := filepath.Join(t.TempDir(), "rollback-export.jsonl")
	if err := ExportSessionSchemaOne(path, destination); err != nil {
		t.Fatalf("export selected SQLite head for old reader: %v", err)
	}
	exported, err := LoadSession(destination)
	if err != nil {
		t.Fatalf("load schema-1 rollback export: %v", err)
	}
	wantMessages, _ := sqliteState.materialize(sqliteState.selectedHead())
	if !reflect.DeepEqual(dagContents(exported.Messages), dagContents(wantMessages)) {
		t.Fatalf("rollback export contents differ: got %#v want %#v", dagContents(exported.Messages), dagContents(wantMessages))
	}
	if !hasCompactionSummary(exported.Messages) {
		t.Fatal("rollback export dropped the selected head's compacted summary")
	}
	identityStore, _, _, err := sqliteSessionEventStore(path)
	if err != nil {
		t.Fatal(err)
	}
	reimportID := "sqlite-roundtrip"
	reimportPath := filepath.Join(filepath.Dir(path), "tauri-"+reimportID+".jsonl")
	if err := identityStore.Reserve(context.Background(), config.SessionDir(), sessionidentity.Candidate{ID: reimportID, Path: reimportPath}); err != nil {
		t.Fatalf("reserve isolated SQLite reimport target: %v", err)
	}
	projection, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.SessionEventLog(reimportPath), projection, 0o600); err != nil {
		t.Fatal(err)
	}
	reimported, err := replaySessionDAG(context.Background(), store.SessionEventLog(reimportPath), defaultSessionReplayLimits)
	if err != nil || !reimported.sqliteBacked || !sessionDAGShadowSemanticsEqual(jsonlState, reimported) {
		t.Fatalf("SQLite export/import round trip differs: state=%#v err=%v", reimported, err)
	}
}
