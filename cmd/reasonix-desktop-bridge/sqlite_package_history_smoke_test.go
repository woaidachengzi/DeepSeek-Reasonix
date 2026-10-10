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
	for _, mode := range []string{"legacy_unicode", "hidden_protocol", "newest_page", "malformed"} {
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
				p := startSQLitePackagedSidecar(t, binary, profile, "")
				if status, _ := p.call(t, "GET", "/v1/sessions/"+id+"/history", nil, false); status != 401 {
					t.Fatal("history bypassed authentication", status)
				}
				status, _ := p.call(t, "POST", "/v1/sessions:open", map[string]string{"sessionId": id}, true)
				if mode == "malformed" {
					if status != 500 {
						t.Fatal("malformed legacy history was not rejected", status)
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
						if message.Content != want[i] || message.Role != role || message.TurnUsage != nil || message.CreatedAtMs != 0 || message.WorkDurationMs != 0 {
							t.Fatal("legacy content/order or unknown accounting was fabricated", i)
						}
					}
					if bytes.Contains(reply, []byte("PRIVATE_")) {
						t.Fatal("history leaked hidden persisted protocol content")
					}
				}
				p.stop(t)
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, original) {
					t.Fatal("opening or shutdown rewrote historical JSONL", err)
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
