package serve

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/provider"
)

const maxHistoryPageMessages = 500

type historyPageQuery struct {
	limit  int
	before *int
	cursor *historyPageCursor
}

type historyPageCursor struct {
	Source string `json:"source"`
	Before int    `json:"before"`
	Anchor string `json:"anchor"`
}

// Indices count stored messages, before display filtering. A cursor anchors the
// boundary entry, so appends do not shift earlier pages and a rewritten or
// replaced session cannot silently splice a different transcript into the view.
type historyPage struct {
	Messages      []historyMessage `json:"messages"`
	SessionKey    string           `json:"sessionKey"`
	StartIndex    int              `json:"startIndex"`
	EndIndex      int              `json:"endIndex"`
	TotalMessages int              `json:"totalMessages"`
	HasMoreBefore bool             `json:"hasMoreBefore"`
	BeforeCursor  string           `json:"beforeCursor,omitempty"`
}

func parseHistoryPageQuery(query url.Values) (historyPageQuery, bool, error) {
	_, limited := query["limit"]
	_, before := query["before"]
	_, cursor := query["cursor"]
	if !limited && !before && !cursor {
		return historyPageQuery{}, false, nil
	}
	p := historyPageQuery{}
	if !limited || len(query["limit"]) != 1 || before && cursor {
		return p, true, errors.New("use one limit and either before or cursor")
	}
	value, err := strconv.Atoi(query.Get("limit"))
	if err != nil || value < 1 || value > maxHistoryPageMessages {
		return p, true, fmt.Errorf("limit must be between 1 and %d", maxHistoryPageMessages)
	}
	p.limit = value
	if before {
		value, err = strconv.Atoi(query.Get("before"))
		if len(query["before"]) != 1 || err != nil || value < 0 {
			return p, true, errors.New("before must be one non-negative message index")
		}
		p.before = &value
	}
	if cursor {
		if len(query["cursor"]) != 1 || len(query.Get("cursor")) > 512 {
			return p, true, errors.New("invalid history cursor")
		}
		data, decodeErr := base64.RawURLEncoding.DecodeString(query.Get("cursor"))
		p.cursor = &historyPageCursor{}
		if decodeErr != nil || json.Unmarshal(data, p.cursor) != nil || p.cursor.Before < 1 || len(p.cursor.Source) != 64 || len(p.cursor.Anchor) != 64 {
			return p, true, errors.New("invalid history cursor")
		}
	}
	return p, true, nil
}

func historySourceKey(path string) string {
	hash := sha256.Sum256([]byte(path))
	return hex.EncodeToString(hash[:])
}

func historyBoundaryAnchor(message provider.Message) string {
	// Include content as well as ID: an edited boundary may retain its original
	// ID, and must not silently validate an older cursor.
	encoded, err := json.Marshal(message)
	if err != nil {
		encoded = []byte(fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s", message.ID, message.Role, message.CreatedAt, message.ToolCallID, message.Content))
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func historyPageEnd(messages []provider.Message, key string, query historyPageQuery) (int, error) {
	end := len(messages)
	if query.before != nil {
		end = *query.before
	}
	if cursor := query.cursor; cursor != nil {
		if cursor.Source != key || cursor.Before >= len(messages) || historyBoundaryAnchor(messages[cursor.Before]) != cursor.Anchor {
			return 0, errors.New("history changed; reload the newest page")
		}
		end = cursor.Before
	}
	if end > len(messages) {
		return 0, errors.New("before exceeds the history length")
	}
	// Only manually supplied offsets need this conservative boundary check.
	// Generated cursors may legitimately point at display-hidden metadata; their
	// boundary was already selected with the tool batch and anchored above.
	if query.before != nil && end < len(messages) && (messages[end].Role == provider.RoleTool || messages[end].LocalOnly || agent.IsPinnedContextRevision(messages[end])) {
		return 0, errors.New("before splits a tool batch; use the returned cursor")
	}
	return end, nil
}

// Tool results stay with their preceding call batch, including local metadata
// between the call and result. Only a bounded window is inspected; a batch
// extending beyond it refuses rather than bypassing the page bound.
func historyPageStart(messages []provider.Message, end, limit int) (int, error) {
	start, floor := max(0, end-limit), max(0, end-maxHistoryPageMessages)
	hasTools := false
	for _, message := range messages[start:end] {
		hasTools = hasTools || message.Role == provider.RoleTool
	}
	if !hasTools {
		return start, nil
	}
	owners := make(map[string]int)
	type pair struct{ result, call int }
	var pairs []pair
	for i := floor; i < end; i++ {
		message := messages[i]
		for _, call := range message.ToolCalls {
			owners[call.ID] = i
		}
		if message.Role == provider.RoleTool && message.ToolCallID != "" {
			owner, ok := owners[message.ToolCallID]
			if !ok {
				owner = -1
			}
			pairs = append(pairs, pair{i, owner})
		}
	}
	for i := len(pairs) - 1; i >= 0; i-- {
		pair := pairs[i]
		if pair.result < start {
			continue
		}
		if pair.call < 0 {
			if floor > 0 {
				return 0, errors.New("tool batch exceeds history page limit")
			}
			continue // Preserve an orphan already present at the stored root.
		}
		start = min(start, pair.call)
	}
	return start, nil
}

func projectHistoryPage(messages []provider.Message, source string, query historyPageQuery) (historyPage, error) {
	key := historySourceKey(source)
	end, err := historyPageEnd(messages, key, query)
	if err != nil {
		return historyPage{}, err
	}
	start, err := historyPageStart(messages, end, query.limit)
	if err != nil {
		return historyPage{}, err
	}
	page := historyPage{
		Messages: historyMessages(messages[start:end]), SessionKey: key,
		StartIndex: start, EndIndex: end, TotalMessages: len(messages), HasMoreBefore: start > 0,
	}
	if start > 0 {
		cursor, _ := json.Marshal(historyPageCursor{Source: key, Before: start, Anchor: historyBoundaryAnchor(messages[start])})
		page.BeforeCursor = base64.RawURLEncoding.EncodeToString(cursor)
	}
	return page, nil
}

func (s *Server) historyPageSource(selector string) (string, []provider.Message, bool) {
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	ctrl := s.ctl()
	path := agent.CanonicalSessionPath(ctrl.SessionPath())
	if selector != "" {
		selected, err := s.resolveSessionPath(selector)
		if err != nil {
			return "", nil, false
		}
		if external, messages, ok := s.externalReadView(selector); ok {
			return external, messages, true
		}
		if agent.CanonicalSessionPath(selected) != path {
			return "", nil, false
		}
	}
	if s.sessionMirrored(path) || leaseHeldByForeignRuntime(path) {
		messages, ok := s.mirroredHistory(path)
		return path, messages, ok
	}
	if path == "" {
		path = fmt.Sprintf("controller:%p", ctrl)
	}
	return path, ctrl.History(), true
}

func (s *Server) pagedHistory(w http.ResponseWriter, r *http.Request, query historyPageQuery) {
	values := r.URL.Query()
	if len(values["session"]) > 1 || len(values["session"]) == 1 && strings.TrimSpace(values.Get("session")) == "" {
		http.Error(w, "select one session", http.StatusBadRequest)
		return
	}
	source, messages, ok := s.historyPageSource(values.Get("session"))
	if !ok {
		http.Error(w, "history session is unavailable", http.StatusNotFound)
		return
	}
	page, err := projectHistoryPage(messages, source, query)
	if err != nil {
		status := http.StatusBadRequest
		if query.cursor != nil {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSONCached(w, r, page)
}

// writeJSONCached encodes v as JSON, computes an ETag from the body, and
// returns 304 Not Modified if the client's If-None-Match matches. This avoids
// re-sending unchanged history/context payloads on every reconnect.
func writeJSONCached(w http.ResponseWriter, r *http.Request, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		slog.Warn("serve: writeJSONCached marshal failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(body))
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}
