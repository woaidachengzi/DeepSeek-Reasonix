package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"reasonix/internal/desktopbridge"
	"reasonix/internal/diff"
	"reasonix/internal/gitcmd"
)

const bridgeWorkspaceChangeLimit = 2 << 20

type bridgeGitStatusEntry struct {
	Path    string
	OldPath string
	Status  string
}

func (r *controllerRuntime) WorkspaceChanges() desktopbridge.WorkspaceChanges {
	base, _, _, err := r.resolveWorkspacePath("")
	if err != nil {
		return desktopbridge.WorkspaceChanges{Files: []desktopbridge.WorkspaceChangeView{}, GitAvailable: false, GitErr: err.Error()}
	}

	type accumulator struct {
		view       desktopbridge.WorkspaceChangeView
		hasSession bool
		hasGit     bool
	}
	changes := map[string]*accumulator{}
	add := func(path string) *accumulator {
		rel := bridgeWorkspaceRel(base, path)
		if rel == "" {
			return nil
		}
		if changes[rel] == nil {
			changes[rel] = &accumulator{view: desktopbridge.WorkspaceChangeView{Path: rel}}
		}
		return changes[rel]
	}

	for _, meta := range r.controller.Checkpoints() {
		for _, path := range meta.Paths {
			acc := add(path)
			if acc == nil {
				continue
			}
			acc.hasSession = true
			if len(acc.view.Turns) == 0 || acc.view.Turns[len(acc.view.Turns)-1] != meta.Turn {
				acc.view.Turns = append(acc.view.Turns, meta.Turn)
			}
			if meta.Time.UnixMilli() >= acc.view.LatestTime {
				acc.view.LatestPrompt = meta.Prompt
				acc.view.LatestTime = meta.Time.UnixMilli()
			}
		}
	}

	out := desktopbridge.WorkspaceChanges{Files: []desktopbridge.WorkspaceChangeView{}, GitAvailable: true}
	entries, gitErr := bridgeGitStatus(base)
	if gitErr != nil {
		out.GitAvailable = false
		out.GitErr = gitErr.Error()
	} else {
		out.GitBranch = bridgeGitBranch(base)
	}
	for _, entry := range entries {
		acc := add(entry.Path)
		if acc == nil {
			continue
		}
		acc.hasGit = true
		acc.view.GitStatus = entry.Status
		acc.view.OldPath = bridgeWorkspaceRel(base, entry.OldPath)
	}

	for _, acc := range changes {
		if acc.hasSession {
			acc.view.Sources = append(acc.view.Sources, "session")
			if state, ok := r.controller.CheckpointFileState(acc.view.Path); ok && state.Owned {
				acc.view.CanSessionRevert = true
			}
		}
		if acc.hasGit {
			acc.view.Sources = append(acc.view.Sources, "git")
		}
		out.Files = append(out.Files, acc.view)
	}
	sort.Slice(out.Files, func(i, j int) bool {
		if len(out.Files[i].Sources) != len(out.Files[j].Sources) {
			return len(out.Files[i].Sources) > len(out.Files[j].Sources)
		}
		return strings.ToLower(out.Files[i].Path) < strings.ToLower(out.Files[j].Path)
	})
	return out
}

func (r *controllerRuntime) WorkspaceChangeDetail(rel string) (desktopbridge.WorkspaceChangeDetail, error) {
	base, cleanRel, err := r.resolveWorkspaceChangePath(rel)
	if err != nil {
		return desktopbridge.WorkspaceChangeDetail{}, err
	}
	if cleanRel == "" {
		return desktopbridge.WorkspaceChangeDetail{}, fmt.Errorf("%w: a changed file path is required", desktopbridge.ErrInvalidWorkspacePath)
	}
	entries, gitErr := bridgeGitStatus(base)
	if gitErr == nil {
		var entry *bridgeGitStatusEntry
		for i := range entries {
			if entries[i].Path == filepath.ToSlash(cleanRel) {
				entry = &entries[i]
				break
			}
		}
		if entry != nil {
			args := []string{"-C", base, "diff", "--no-ext-diff", "--no-textconv", "--relative", "HEAD", "--", filepath.FromSlash(cleanRel)}
			allowExitOne := false
			if entry.Status == "??" {
				// Keep the diff operands relative to -C base so the user's absolute
				// workspace path never crosses the bridge in a patch header.
				args = []string{"-C", base, "diff", "--no-ext-diff", "--no-textconv", "--no-index", "--", "/dev/null", filepath.FromSlash(cleanRel)}
				allowExitOne = true
			}
			raw, truncated, err := bridgeGitDiff(args, allowExitOne)
			if err != nil {
				return desktopbridge.WorkspaceChangeDetail{}, err
			}
			if truncated {
				return desktopbridge.WorkspaceChangeDetail{Source: "git", Truncated: true}, nil
			}
			patch := strings.TrimSpace(string(raw))
			if patch != "" {
				added, removed := bridgeTallyUnifiedPatch(patch)
				return desktopbridge.WorkspaceChangeDetail{
					Diff:    patch,
					Source:  "git",
					Added:   added,
					Removed: removed,
					Binary:  bytes.Contains(raw, []byte("Binary files ")) || bytes.Contains(raw, []byte("GIT binary patch")),
				}, nil
			}
		}
	}
	if state, ok := r.controller.CheckpointFileState(cleanRel); ok {
		return bridgeSessionChangeDetail(base, cleanRel, state.Content)
	}
	if gitErr != nil {
		return desktopbridge.WorkspaceChangeDetail{}, gitErr
	}
	return desktopbridge.WorkspaceChangeDetail{}, nil
}

// resolveWorkspaceChangePath is like resolveWorkspacePath but also accepts a
// deleted checkpoint file. It keeps the lexical root guard and evaluates the
// parent directory when the target no longer exists.
func (r *controllerRuntime) resolveWorkspaceChangePath(rel string) (string, string, error) {
	root := r.controller.WorkspaceRoot()
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	raw := strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
	if raw == "" || strings.HasPrefix(raw, "/") || filepath.VolumeName(raw) != "" {
		return "", "", fmt.Errorf("%w: relative file path is required", desktopbridge.ErrInvalidWorkspacePath)
	}
	clean := filepath.Clean(filepath.FromSlash(raw))
	relative, err := filepath.Rel(base, filepath.Join(base, clean))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || relative == "." {
		return "", "", fmt.Errorf("%w: path escapes root", desktopbridge.ErrInvalidWorkspacePath)
	}
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", "", err
	}
	candidate := filepath.Join(base, clean)
	check := candidate
	if _, statErr := os.Lstat(candidate); os.IsNotExist(statErr) {
		check = filepath.Dir(candidate)
	}
	resolved, err := filepath.EvalSymlinks(check)
	if err != nil {
		return "", "", err
	}
	inside, err := filepath.Rel(resolvedBase, resolved)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("%w: path escapes root", desktopbridge.ErrInvalidWorkspacePath)
	}
	return base, filepath.ToSlash(relative), nil
}

func bridgeWorkspaceRel(base, path string) string {
	raw := strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if raw == "" {
		return ""
	}
	var candidate string
	if filepath.IsAbs(filepath.FromSlash(raw)) || filepath.VolumeName(raw) != "" {
		candidate = filepath.FromSlash(raw)
	} else {
		candidate = filepath.Join(base, filepath.FromSlash(raw))
	}
	rel, err := filepath.Rel(base, candidate)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

func bridgeSessionChangeDetail(base, rel string, old *string) (desktopbridge.WorkspaceChangeDetail, error) {
	path := filepath.Join(base, filepath.FromSlash(rel))
	oldText := ""
	if old != nil {
		if len(*old) > bridgeWorkspaceChangeLimit {
			return desktopbridge.WorkspaceChangeDetail{Source: "session", Truncated: true}, nil
		}
		oldText = *old
	}
	newText, exists, truncated, err := bridgeCurrentWorkspaceText(path)
	if err != nil {
		return desktopbridge.WorkspaceChangeDetail{}, err
	}
	if truncated {
		return desktopbridge.WorkspaceChangeDetail{Source: "session", Truncated: true}, nil
	}
	kind := diff.Modify
	if old == nil {
		kind = diff.Create
	} else if !exists {
		kind = diff.Delete
	}
	change := diff.Build(rel, oldText, newText, kind)
	if len(change.Diff) > bridgeWorkspaceChangeLimit {
		return desktopbridge.WorkspaceChangeDetail{Source: "session", Truncated: true}, nil
	}
	return desktopbridge.WorkspaceChangeDetail{Diff: change.Diff, Source: "session", Added: change.Added, Removed: change.Removed, Binary: change.Binary}, nil
}

func bridgeCurrentWorkspaceText(path string) (string, bool, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", false, false, nil
	}
	if err != nil {
		return "", false, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		return target, true, false, err
	}
	if !info.Mode().IsRegular() {
		return "", true, false, fmt.Errorf("workspace change path %q is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false, false, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, bridgeWorkspaceChangeLimit+1))
	if err != nil {
		return "", false, false, err
	}
	if len(raw) > bridgeWorkspaceChangeLimit {
		return "", true, true, nil
	}
	if !utf8.Valid(raw) {
		return "\x00", true, false, nil
	}
	return string(raw), true, false, nil
}

func bridgeGitStatus(base string) ([]bridgeGitStatusEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := bridgeGitOutput(ctx, []string{"-C", base, "status", "--porcelain=v1", "-z", "--untracked-files=all"}, false)
	if err != nil {
		return nil, fmt.Errorf("git status unavailable: %w", err)
	}
	parts := bytes.Split(raw, []byte{0})
	entries := make([]bridgeGitStatusEntry, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		part := string(parts[i])
		if part == "" || len(part) < 4 {
			continue
		}
		status := part[:2]
		path := filepath.ToSlash(strings.TrimSpace(part[3:]))
		entry := bridgeGitStatusEntry{Path: path, Status: status}
		if strings.Contains(status, "R") || strings.Contains(status, "C") {
			if i+1 < len(parts) && len(parts[i+1]) > 0 {
				entry.OldPath = filepath.ToSlash(string(parts[i+1]))
				i++
			}
		}
		if path != "" {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func bridgeGitBranch(base string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := bridgeGitOutput(ctx, []string{"-C", base, "symbolic-ref", "--short", "-q", "HEAD"}, false)
	if err == nil && strings.TrimSpace(string(raw)) != "" {
		return strings.TrimSpace(string(raw))
	}
	return "HEAD"
}

func bridgeGitDiff(args []string, allowExitOne bool) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return bridgeGitOutputLimited(ctx, args, allowExitOne)
}

func bridgeGitOutput(ctx context.Context, args []string, allowExitOne bool) ([]byte, error) {
	raw, _, err := bridgeGitOutputLimited(ctx, args, allowExitOne)
	return raw, err
}

func bridgeGitOutputLimited(ctx context.Context, args []string, allowExitOne bool) ([]byte, bool, error) {
	cmd := gitcmd.Command(ctx, "", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return nil, false, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(stdout, bridgeWorkspaceChangeLimit+1))
	_ = stdout.Close()
	if readErr != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return nil, false, readErr
	}
	truncated := len(raw) > bridgeWorkspaceChangeLimit
	if truncated {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, true, nil
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		var exitErr *exec.ExitError
		if !(allowExitOne && errors.As(waitErr, &exitErr) && exitErr.ExitCode() == 1) {
			return nil, false, waitErr
		}
	}
	return raw, false, nil
}

func bridgeTallyUnifiedPatch(patch string) (added, removed int) {
	inHunk := false
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		case strings.HasPrefix(line, "diff --git "):
			inHunk = false
		case inHunk && strings.HasPrefix(line, "+"):
			added++
		case inHunk && strings.HasPrefix(line, "-"):
			removed++
		}
	}
	return added, removed
}
