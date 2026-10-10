package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/sessionidentity"
	"reasonix/internal/store"
)

// Historical rows are constructed independently of today's Session.Save, so
// this exercises old JSONL loading rather than a current writer round trip.
func TestSQLiteActualPackageLegacyHistory(t *testing.T) {
	binary := os.Getenv("REASONIX_SQLITE_PACKAGE_BIN")
	if binary == "" {
		t.Skip("requires explicitly selected packaged sidecar")
	}
	for _, mode := range []string{"legacy_unicode", "hidden_protocol", "newest_page", "malformed", "unsupported_event_schema", "unknown_dag_entry", "selected_dag", "corrupt_projection", "unsupported_projection_schema"} {
		t.Run(mode, func(t *testing.T) {
			profile := t.TempDir()
			t.Setenv("REASONIX_HOME", profile)
			t.Setenv("REASONIX_STATE_HOME", profile)
			// An unreachable synthetic provider permits opening, not inference.
			if err := os.WriteFile(appconfig.UserConfigPath(), []byte("default_model=\"local/alpha\"\n[desktop]\nprovider_access=[\"local\"]\n[[providers]]\nname=\"local\"\nkind=\"openai\"\nbase_url=\"http://127.0.0.1:1/v1\"\nmodels=[\"alpha\"]\ndefault=\"alpha\"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(appconfig.SessionDir(), 0700); err != nil {
				t.Fatal(err)
			}
			const id = "legacy-history-owned"
			path, err := sessionpath.TranscriptPath(appconfig.SessionDir(), id)
			if err != nil {
				t.Fatal(err)
			}
			var fixture bytes.Buffer
			appendRow := func(row map[string]any) {
				t.Helper()
				if err := json.NewEncoder(&fixture).Encode(row); err != nil {
					t.Fatal(err)
				}
			}
			appendRow(map[string]any{"role": "system", "content": "PRIVATE_SYSTEM_SENTINEL"})
			want := []string{"旧问题：你好 🌏", "旧回答：兼容读取\n第二行"}
			start, total := 0, 2
			if mode == "newest_page" {
				want = nil
				start, total = 40, 240
				for i := 0; i < total; i++ {
					role := "user"
					if i%2 != 0 {
						role = "assistant"
					}
					content := fmt.Sprintf("historical row %03d", i)
					appendRow(map[string]any{"role": role, "content": content})
					if i >= start {
						want = append(want, content)
					}
				}
			} else {
				appendRow(map[string]any{"role": "user", "content": want[0]})
				if mode == "hidden_protocol" {
					// Legacy v1 envelope predates the current section manifest.
					body := "This host-generated snapshot supersedes every earlier session-context snapshot.\n\n## Workspace\n\nPRIVATE_HOST_SENTINEL"
					envelope := fmt.Sprintf("<session-context version=\"1\">\n%s\n\nDigest: sha256:%x\n</session-context>", body, sha256.Sum256([]byte(body)))
					appendRow(map[string]any{"role": "user", "origin": "host", "content": envelope})
					appendRow(map[string]any{"role": "assistant", "tool_calls": []map[string]any{{"id": "old-call", "name": "echo", "arguments": "{}"}}})
					appendRow(map[string]any{"role": "tool", "tool_call_id": "old-call", "name": "echo", "content": "PRIVATE_TOOL_SENTINEL"})
				}
				appendRow(map[string]any{"role": "assistant", "content": want[1], "reasoning_content": "PRIVATE_REASONING_SENTINEL"})
			}
			if mode == "malformed" {
				fixture.WriteString("{broken historical row}\n")
			}
			original := append([]byte(nil), fixture.Bytes()...)
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			// A compatibility checkpoint must not conceal an authoritative event
			// log, either a selected branch or an unsupported/newer format. Construct
			// wire bytes independently of current Session.Save or DAG writers.
			var originalEvents []byte
			var wantCreatedAt []int64
			switch mode {
			case "unsupported_event_schema":
				originalEvents = []byte("{\"schema_version\":999,\"type\":\"log\",\"generation\":1,\"at\":\"2026-01-08T10:00:00Z\"}\n")
			case "unknown_dag_entry":
				originalEvents = []byte("{\"schema_version\":2,\"type\":\"log\",\"generation\":1,\"at\":\"2026-01-08T10:00:00Z\"}\n{\"schema_version\":2,\"type\":\"PRIVATE_UNSUPPORTED_ENTRY\",\"at\":\"2026-01-08T10:00:01Z\"}\n")
			case "selected_dag":
				// Independent schema-2 wire fixture, not a current writer round trip.
				// The checkpoint and the unselected main branch intentionally disagree.
				want = []string{"DAG 分支问题 🌏", "已选择分支的回答\n第二行"}
				base := time.Date(2026, 1, 8, 10, 0, 0, 0, time.UTC)
				wantCreatedAt = []int64{base.Add(time.Second).UnixMilli(), base.Add(4 * time.Second).UnixMilli()}
				var events bytes.Buffer
				appendEvent := func(kind string, offset int, fields map[string]any) {
					t.Helper()
					fields["schema_version"], fields["type"], fields["at"] = 2, kind, base.Add(time.Duration(offset)*time.Second)
					if err := json.NewEncoder(&events).Encode(fields); err != nil {
						t.Fatal(err)
					}
				}
				appendMessage := func(head, entry, parent, parentDigest, role, content string, offset int) string {
					t.Helper()
					// Historical hash identity omits local ID and timestamp. Field
					// order matches the published message wire shape, independent of
					// today's agent package encoder/identity helpers.
					identity, err := json.Marshal(struct {
						Role    string `json:"role"`
						Content string `json:"content,omitempty"`
					}{role, content})
					if err != nil {
						t.Fatal(err)
					}
					digest := fmt.Sprintf("%x", sha256.Sum256(append(append([]byte(parentDigest), 0), identity...)))
					appendEvent("message", offset, map[string]any{"head": head, "id": entry, "parent": parent,
						"digest": digest, "msgs": []map[string]any{{"id": entry, "role": role, "content": content,
							"createdAt": base.Add(time.Duration(offset) * time.Second).UnixMilli()}}})
					return digest
				}
				appendEvent("log", 0, map[string]any{"generation": 1})
				userDigest := appendMessage("main", "U", "", "", "user", want[0], 1)
				appendMessage("main", "M", "U", userDigest, "assistant", "PRIVATE_UNSELECTED_BRANCH", 2)
				appendEvent("fork", 3, map[string]any{"head": "main", "new_head": "F", "from": "U", "kind": "fork"})
				appendMessage("F", "A", "U", userDigest, "assistant", want[1], 4)
				appendEvent("select", 5, map[string]any{"head": "F"})
				originalEvents = append([]byte(nil), events.Bytes()...)
			}
			if originalEvents != nil {
				if err := os.WriteFile(store.SessionEventLog(path), originalEvents, 0600); err != nil {
					t.Fatal(err)
				}
			}
			identityStore, err := sessionidentity.Open(context.Background(), appconfig.DesktopSessionIdentityPath(), profile)
			if err != nil {
				t.Fatal(err)
			}
			importErr := identityStore.Import(context.Background(), appconfig.SessionDir(), []sessionidentity.Candidate{{ID: id, Path: path, Title: "Historical title"}})
			closeErr := identityStore.Close()
			if importErr != nil || closeErr != nil {
				t.Fatal(importErr, closeErr)
			}
			for launch := 0; launch < 2; launch++ {
				// Unlike the authoritative event log, a projection is disposable.
				// Reinstall independent bad wire bytes before each cold start so
				// both launches exercise rejection, not merely a missing cache.
				var invalidProjection []byte
				switch mode {
				case "corrupt_projection":
					invalidProjection = []byte("{broken PRIVATE_PROJECTION_SENTINEL}\n")
				case "unsupported_projection_schema":
					invalidProjection = []byte(`{"schema_version":999,"projection":{"messages":[{"role":"assistant","content":"PRIVATE_PROJECTION_SENTINEL"}]}}`)
				}
				if invalidProjection != nil {
					if err := os.WriteFile(store.SessionContext(path), invalidProjection, 0600); err != nil {
						t.Fatal(err)
					}
				}
				p := startSQLitePackagedSidecar(t, binary, profile, "")
				if status, _ := p.call(t, "GET", "/v1/sessions/"+id+"/history", nil, false); status != 401 {
					t.Fatal("history bypassed authentication", status)
				}
				status, openReply := p.call(t, "POST", "/v1/sessions:open", map[string]string{"sessionId": id}, true)
				if mode == "malformed" || mode == "unsupported_event_schema" || mode == "unknown_dag_entry" {
					if status != 500 {
						t.Fatal("malformed/unsupported authoritative history was not rejected", status)
					}
					if bytes.Contains(openReply, []byte(profile)) || bytes.Contains(openReply, []byte("PRIVATE_")) {
						t.Fatal("history rejection leaked private path or unsupported entry details")
					}
				} else {
					if status != 200 {
						t.Fatal("legacy history failed to open", status)
					}
					status, reply := p.call(t, "GET", "/v1/sessions/"+id+"/history", nil, true)
					var history historyResponse
					if status != 200 || json.Unmarshal(reply, &history) != nil || history.ProtocolVersion != 1 || history.Session.ID != id || history.StartIndex != start || history.TotalMessages != total || len(history.Messages) != len(want) {
						t.Fatal("legacy history paging/identity differs", status)
					}
					for i, message := range history.Messages {
						role := "user"
						if i%2 != 0 {
							role = "assistant"
						}
						createdAt := int64(0)
						if wantCreatedAt != nil {
							createdAt = wantCreatedAt[i]
						}
						if message.Content != want[i] || message.Role != role || message.TurnUsage != nil || message.CreatedAtMs != createdAt || message.WorkDurationMs != 0 {
							t.Fatal("legacy content/order or unknown accounting was fabricated", i)
						}
					}
					if bytes.Contains(reply, []byte("PRIVATE_")) {
						t.Fatal("history leaked hidden persisted protocol content")
					}
				}
				p.stop(t)
				if invalidProjection != nil {
					if _, err := os.Stat(store.SessionContext(path)); !os.IsNotExist(err) {
						t.Fatal("invalid disposable projection was retained or silently rewritten", err)
					}
				}
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, original) {
					t.Fatal("opening or shutdown rewrote historical JSONL", err)
				}
				if originalEvents != nil {
					gotEvents, err := os.ReadFile(store.SessionEventLog(path))
					if err != nil || !bytes.Equal(gotEvents, originalEvents) {
						t.Fatal("authoritative event log was downgraded, repaired or rewritten", err)
					}
					if _, err := os.Stat(store.SessionEventLogDamaged(path)); !os.IsNotExist(err) {
						t.Fatal("hard format rejection salvaged or truncated an authoritative log", err)
					}
				}
				entries, err := os.ReadDir(appconfig.SessionDir())
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					// Lifecycle/event sidecars share .jsonl, but are not sessions.
					if store.IsSessionTranscriptName(entry.Name()) && entry.Name() != filepath.Base(path) {
						t.Fatal("reading old history created a phantom transcript", entry.Name())
					}
				}
			}
		})
	}
}
