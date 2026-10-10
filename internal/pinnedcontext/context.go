package pinnedcontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/fileutil"
	"reasonix/internal/store"
)

const (
	maxPinnedFileCount   = agent.MaxPinnedContextFiles
	maxPinnedFileSize    = agent.MaxPinnedContextFileBytes
	maxPinnedContextSize = agent.MaxPinnedContextRevisionBytes
)

var (
	ErrNotRegular   = errors.New("only regular files can be pinned")
	ErrFileTooLarge = errors.New("pinned file exceeds the size limit")
)

// Info holds metadata about one pinned context file.
type Info struct {
	Path          string `json:"path"`
	SizeBytes     int64  `json:"sizeBytes"`
	TokenEstimate int    `json:"tokenEstimate"`
	Error         string `json:"error,omitempty"`
}

type BuildResult struct {
	Snapshot agent.PinnedContextSnapshot
	Infos    []Info
}

func NormalizePath(relPath string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(relPath)))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "" || clean == "." || filepath.IsAbs(relPath) || strings.HasPrefix(clean, "/") {
		return "", errors.New("invalid empty or absolute path")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("path traversal outside workspace is forbidden")
	}
	if !utf8.ValidString(clean) {
		return "", errors.New("pinned path is not valid UTF-8")
	}
	return clean, nil
}

func ReadFile(root, relPath string, afterStat func()) (string, []byte, int64, error) {
	clean, err := NormalizePath(relPath)
	if err != nil {
		return "", nil, 0, err
	}
	if strings.TrimSpace(root) == "" {
		return clean, nil, 0, errors.New("tab has no workspace root")
	}
	file, err := fileutil.OpenFileBeneath(root, filepath.FromSlash(clean))
	if err != nil {
		return clean, nil, 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return clean, nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return clean, nil, info.Size(), ErrNotRegular
	}
	if afterStat != nil {
		afterStat()
	}
	if info.Size() > maxPinnedFileSize {
		return clean, nil, info.Size(), fmt.Errorf("%w: file size (%d bytes) exceeds the %d-byte limit", ErrFileTooLarge, info.Size(), maxPinnedFileSize)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxPinnedFileSize+1))
	if err != nil {
		return clean, nil, info.Size(), err
	}
	if len(data) > maxPinnedFileSize {
		return clean, nil, int64(len(data)), fmt.Errorf("%w: file grew beyond the %d-byte limit while reading", ErrFileTooLarge, maxPinnedFileSize)
	}
	return clean, data, int64(len(data)), nil
}

func Build(root string, files []string, afterStat func()) BuildResult {
	result := BuildResult{
		Snapshot: agent.PinnedContextSnapshot{
			Files:  make([]agent.PinnedContextFile, 0, len(files)),
			Issues: make([]agent.PinnedContextIssue, 0, len(files)),
		},
		Infos: make([]Info, 0, len(files)),
	}
	if len(files) == 0 || strings.TrimSpace(root) == "" {
		return result
	}
	for _, rel := range files {
		clean, data, size, err := ReadFile(root, rel, afterStat)
		info := Info{Path: rel, SizeBytes: size, TokenEstimate: estimateTokensFromBytes(size)}
		if clean != "" {
			info.Path = clean
		}
		if err != nil {
			info.Error = err.Error()
			result.Snapshot.Issues = append(result.Snapshot.Issues, agent.PinnedContextIssue{
				Path: info.Path, Reason: IssueReason(err),
			})
			result.Infos = append(result.Infos, info)
			continue
		}
		content := agent.SanitizePinnedContextContent(string(data))
		if len(content) > maxPinnedFileSize {
			info.Error = fmt.Sprintf("pinned file exceeds the %d-byte limit after XML normalization", maxPinnedFileSize)
			result.Snapshot.Issues = append(result.Snapshot.Issues, agent.PinnedContextIssue{
				Path: clean, Reason: agent.PinnedContextIssueFileTooLarge,
			})
			result.Infos = append(result.Infos, info)
			continue
		}
		candidate, err := agent.NormalizePinnedContextFile(agent.PinnedContextFile{Path: clean, Content: content})
		if err != nil {
			info.Error = err.Error()
			result.Snapshot.Issues = append(result.Snapshot.Issues, agent.PinnedContextIssue{
				Path: clean, Reason: agent.PinnedContextIssueReadFailed,
			})
			result.Infos = append(result.Infos, info)
			continue
		}
		result.Snapshot.Files = append(result.Snapshot.Files, candidate)
		if err := agent.ValidatePinnedContextSnapshot(result.Snapshot); err != nil {
			result.Snapshot.Files = result.Snapshot.Files[:len(result.Snapshot.Files)-1]
			result.Snapshot.Issues = append(result.Snapshot.Issues, agent.PinnedContextIssue{
				Path: clean, Reason: agent.PinnedContextIssueTotalLimit,
			})
			info.Error = fmt.Sprintf("pinned context would exceed the %d-byte total limit", maxPinnedContextSize)
			result.Infos = append(result.Infos, info)
			continue
		}
		result.Infos = append(result.Infos, info)
	}
	return result
}

func IssueReason(err error) agent.PinnedContextIssueReason {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return agent.PinnedContextIssueNotFound
	case errors.Is(err, ErrNotRegular):
		return agent.PinnedContextIssueNotRegular
	case errors.Is(err, ErrFileTooLarge):
		return agent.PinnedContextIssueFileTooLarge
	default:
		return agent.PinnedContextIssueReadFailed
	}
}

func Loader(root string, afterStat func()) control.PinnedContextLoader {
	return func(ctx context.Context, sessionPath string) (agent.PinnedContextSnapshot, error) {
		if err := ctx.Err(); err != nil {
			return agent.PinnedContextSnapshot{}, err
		}
		state, err := LoadState(sessionPath)
		if err != nil {
			return agent.PinnedContextSnapshot{}, err
		}
		build := Build(root, state.Files, afterStat)
		if err := ctx.Err(); err != nil {
			return agent.PinnedContextSnapshot{}, err
		}
		return build.Snapshot, nil
	}
}

const (
	pinnedContextSchemaVersion = 1
	maxPinnedContextStateBytes = 64 * 1024
)

type State struct {
	SchemaVersion int      `json:"schemaVersion"`
	SessionID     string   `json:"sessionId"`
	Files         []string `json:"files"`
}

func EmptyState(sessionPath string) State {
	return State{
		SchemaVersion: pinnedContextSchemaVersion,
		SessionID:     agent.BranchID(sessionPath),
		Files:         []string{},
	}
}

func NormalizeFiles(files []string) ([]string, error) {
	out := make([]string, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, path := range files {
		clean, err := NormalizePath(path)
		if err != nil {
			return nil, fmt.Errorf("invalid pinned path %q: %w", path, err)
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	if len(out) > maxPinnedFileCount {
		return nil, fmt.Errorf("at most %d files can be pinned", maxPinnedFileCount)
	}
	sort.Strings(out)
	return out, nil
}

func LoadState(sessionPath string) (State, error) {
	sessionPath = strings.TrimSpace(sessionPath)
	state := EmptyState(sessionPath)
	if sessionPath == "" {
		return state, nil
	}
	path := store.SessionPinnedContext(sessionPath)
	path, err := filepath.Abs(path)
	if err != nil {
		return state, fmt.Errorf("resolve pinned context state: %w", err)
	}
	// Use the descriptor-confined, nonblocking reader for manifests too:
	// a replaced sidecar must not follow an outside symlink or block on a FIFO.
	file, err := fileutil.OpenFileBeneath(filepath.Dir(path), filepath.Base(path))
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read pinned context state: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return state, fmt.Errorf("stat pinned context state: %w", err)
	}
	if !info.Mode().IsRegular() {
		return state, fmt.Errorf("pinned context state: %w", ErrNotRegular)
	}
	if info.Size() > maxPinnedContextStateBytes {
		return state, fmt.Errorf("pinned context state exceeds %d bytes", maxPinnedContextStateBytes)
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxPinnedContextStateBytes+1))
	if err != nil {
		return state, fmt.Errorf("read pinned context state: %w", err)
	}
	if len(raw) > maxPinnedContextStateBytes {
		return state, fmt.Errorf("pinned context state exceeds %d bytes", maxPinnedContextStateBytes)
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return EmptyState(sessionPath), fmt.Errorf("decode pinned context state: %w", err)
	}
	if state.SchemaVersion != pinnedContextSchemaVersion {
		return EmptyState(sessionPath), fmt.Errorf("unsupported pinned context schema version %d", state.SchemaVersion)
	}
	wantID := agent.BranchID(sessionPath)
	if state.SessionID != wantID {
		return EmptyState(sessionPath), fmt.Errorf("pinned context belongs to session %q, not %q", state.SessionID, wantID)
	}
	files, err := NormalizeFiles(state.Files)
	if err != nil {
		return EmptyState(sessionPath), err
	}
	state.Files = files
	return state, nil
}

func estimateTokensFromBytes(bytes int64) int {
	if bytes <= 0 {
		return 0
	}
	tok := int(bytes / 4)
	if tok == 0 {
		return 1
	}
	return tok
}
