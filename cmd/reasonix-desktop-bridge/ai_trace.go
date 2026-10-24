package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"reasonix/internal/provider"
	"reasonix/internal/secrets"
)

const aiTraceMaxBytes = 3 << 20
const aiTraceFieldBytes = 32 << 10

var aiTraceDataURL = regexp.MustCompile(`(?i)data:[a-z0-9.+/-]+;base64,[a-z0-9+/=_-]+`)

type aiTraceCall struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ResolvedName string `json:"resolvedName,omitempty"`
	CapabilityID string `json:"capabilityId,omitempty"`
	Arguments    string `json:"arguments,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
}

type aiTraceEntry struct {
	Index              int                   `json:"index"`
	Role               provider.Role         `json:"role"`
	CreatedAt          int64                 `json:"createdAt,omitempty"`
	WorkDurationMs     int64                 `json:"workDurationMs,omitempty"`
	Content            string                `json:"content,omitempty"`
	Reasoning          string                `json:"modelReturnedReasoning,omitempty"`
	ToolCalls          []aiTraceCall         `json:"toolCalls,omitempty"`
	ToolCallID         string                `json:"toolCallId,omitempty"`
	ToolName           string                `json:"toolName,omitempty"`
	ToolRunState       provider.ToolRunState `json:"toolRunState,omitempty"`
	ModelVisibleResult string                `json:"modelVisibleResult,omitempty"`
	ComputerUse        json.RawMessage       `json:"computerUse,omitempty"`
	ImageCount         int                   `json:"imageCount,omitempty"`
	Truncated          bool                  `json:"truncated,omitempty"`
}

type aiTraceReport struct {
	SchemaVersion   int            `json:"schemaVersion"`
	ExportedAt      string         `json:"exportedAt"`
	Detailed        bool           `json:"detailed"`
	Model           string         `json:"model"`
	State           string         `json:"state"`
	Scope           string         `json:"scope"`
	Notes           []string       `json:"notes"`
	TotalMessages   int            `json:"totalMessages"`
	OmittedMessages int            `json:"omittedMessages"`
	Entries         []aiTraceEntry `json:"entries"`
}

func (r *controllerRuntime) AITrace(detailed bool) (json.RawMessage, error) {
	return projectAITrace(r.controller.History(), r.controller.ModelRef(), r.State(), detailed, time.Now())
}

func projectAITrace(messages []provider.Message, model, state string, detailed bool, now time.Time) (json.RawMessage, error) {
	report := aiTraceReport{SchemaVersion: 1, ExportedAt: now.UTC().Format(time.RFC3339Nano), Detailed: detailed,
		Model: secrets.RedactCredentials(model), State: state, Scope: "current conversation snapshot; latest bounded messages; not an atomic execution trace",
		Notes: []string{"Reasoning is only text returned by the provider, not proof of execution. Some models return none.",
			"Tool completion does not prove input dispatch or task success. Inspect computerUse.dispatched, blocked_reason and input_verification.",
			"Missing timestamps are unknown, not zero-duration actions. Running requests may not yet appear. Screenshots and system prompts are omitted.",
			"Credential patterns are masked; conversation text and personal information remain. Review before sharing."},
		TotalMessages: len(messages), Entries: []aiTraceEntry{}}
	budget := 0
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role != provider.RoleUser && m.Role != provider.RoleAssistant && m.Role != provider.RoleTool {
			report.OmittedMessages++
			continue
		}
		e := aiTraceEntry{Index: i, Role: m.Role, CreatedAt: m.CreatedAt, WorkDurationMs: m.WorkDurationMs, ToolCallID: m.ToolCallID,
			ToolName: m.Name, ToolRunState: m.ToolRunState, ImageCount: len(m.Images)}
		if detailed || m.Role != provider.RoleTool {
			e.Content = aiTraceText(m.Content, &e.Truncated)
		}
		if detailed {
			e.Reasoning = aiTraceText(m.ReasoningContent, &e.Truncated)
			if m.Role == provider.RoleTool && m.RawContent != "" {
				e.Content = aiTraceText(m.RawContent, &e.Truncated)
				e.ModelVisibleResult = aiTraceText(m.Content, &e.Truncated)
			}
		}
		for _, call := range m.ToolCalls {
			c := aiTraceCall{ID: call.ID, Name: call.Name, ResolvedName: call.ResolvedName, CapabilityID: call.CapabilityID}
			if detailed {
				c.Arguments = aiTraceText(call.Arguments, &c.Truncated)
			}
			e.ToolCalls = append(e.ToolCalls, c)
		}
		if m.Role == provider.RoleTool {
			e.ComputerUse = aiTraceComputerUse(m)
		}
		encoded, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		if len(report.Entries) >= 400 || budget+len(encoded) > aiTraceMaxBytes {
			report.OmittedMessages += i + 1
			break
		}
		budget += len(encoded)
		report.Entries = append(report.Entries, e)
	}
	for left, right := 0, len(report.Entries)-1; left < right; left, right = left+1, right-1 {
		report.Entries[left], report.Entries[right] = report.Entries[right], report.Entries[left]
	}
	return json.Marshal(report)
}

func aiTraceText(value string, truncated *bool) string {
	// Opaque image data is never useful for a text diagnostic export.
	value = aiTraceDataURL.ReplaceAllString(value, "[image data omitted]")
	value = secrets.RedactCredentials(value)
	if len(value) <= aiTraceFieldBytes {
		return value
	}
	*truncated = true
	end := aiTraceFieldBytes
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + "\n[truncated]"
}

func aiTraceComputerUse(m provider.Message) json.RawMessage {
	value := m.RawContent
	if value == "" {
		value = m.Content
	}
	start := strings.IndexByte(value, '{')
	if start < 0 {
		return nil
	}
	var result map[string]json.RawMessage
	// Image understanding may append prose after the native JSON. Only its
	// first object contains dispatch evidence; never parse facts from the suffix.
	if json.NewDecoder(strings.NewReader(value[start:])).Decode(&result) != nil || result["dispatched"] == nil {
		return nil
	}
	facts := map[string]json.RawMessage{}
	for _, key := range []string{"dispatched", "dispatch_method", "dispatch_unknown", "blocked_reason", "state_changed", "input_verification", "unchanged", "image_geometry", "pointer_preview", "observation_validation", "observation_ttl_ms", "observation_age_ms", "next_action"} {
		if value := result[key]; value != nil {
			facts[key] = value
		}
	}
	encoded, _ := json.Marshal(facts)
	var truncated bool
	safe := aiTraceText(string(encoded), &truncated)
	if truncated || !json.Valid([]byte(safe)) {
		return nil
	}
	return json.RawMessage(safe)
}

func (b *bridgeServer) sessionAITrace(w http.ResponseWriter, r *http.Request) {
	detail := r.URL.Query().Get("detail")
	if detail != "full" && detail != "compact" {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "choose full or compact AI diagnostics")
		return
	}
	report, err := b.runtimes.AITrace(r.PathValue("id"), detail == "full")
	if err != nil {
		b.writeRuntimeError(w, err, "unable to export AI diagnostics for this session")
		return
	}
	// The existing event ledger adds exact recent tool timings. It is bounded,
	// may evict events, and is reset by service restart; never invent missing data.
	var envelope map[string]any
	if json.Unmarshal(report, &envelope) != nil {
		writeProtocolError(w, http.StatusInternalServerError, "invalid_report", "unable to encode AI diagnostic report")
		return
	}
	envelope["recentToolTimings"] = b.events.AITraceTimings(r.PathValue("id"))
	envelope["timingScope"] = "recent in-memory tool-result events only; may be evicted or lost on restart; absent timing is unknown"
	writeJSON(w, http.StatusOK, envelope)
}
