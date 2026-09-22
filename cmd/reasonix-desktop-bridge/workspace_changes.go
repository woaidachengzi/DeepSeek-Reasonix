package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"reasonix/internal/desktopbridge"
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
	entries, err := bridgeGitStatus(base)
	if err != nil {
		return desktopbridge.WorkspaceChanges{Files: []desktopbridge.WorkspaceChangeView{}, GitAvailable: false, GitErr: err.Error()}
	}
	files := make([]desktopbridge.WorkspaceChangeView, 0, len(entries))
	for _, entry := range entries {
		files = append(files, desktopbridge.WorkspaceChangeView{
			Path:      entry.Path,
			OldPath:   entry.OldPath,
			Sources:   []string{"git"},
			GitStatus: entry.Status,
		})
	}
	return desktopbridge.WorkspaceChanges{
		Files:        files,
		GitAvailable: true,
		GitBranch:    bridgeGitBranch(base),
	}
}

func (r *controllerRuntime) WorkspaceChangeDetail(rel string) (desktopbridge.WorkspaceChangeDetail, error) {
	base, _, cleanRel, err := r.resolveWorkspacePath(rel)
	if err != nil {
		return desktopbridge.WorkspaceChangeDetail{}, err
	}
	if cleanRel == "" {
		return desktopbridge.WorkspaceChangeDetail{}, fmt.Errorf("%w: a changed file path is required", desktopbridge.ErrInvalidWorkspacePath)
	}
	entries, err := bridgeGitStatus(base)
	if err != nil {
		return desktopbridge.WorkspaceChangeDetail{}, err
	}
	var entry *bridgeGitStatusEntry
	for i := range entries {
		if entries[i].Path == filepath.ToSlash(cleanRel) {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return desktopbridge.WorkspaceChangeDetail{}, nil
	}

	args := []string{"-C", base, "diff", "--no-ext-diff", "--no-textconv", "--relative", "HEAD", "--", filepath.FromSlash(cleanRel)}
	allowExitOne := false
	if entry.Status == "??" {
		// Git does not include untracked files in `diff HEAD`; --no-index gives
		// the same create patch while keeping the requested path argument literal.
		// Keep the diff operands relative to -C base; using the resolved absolute
		// path would leak the user's workspace location in Git's header.
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
	if patch == "" {
		return desktopbridge.WorkspaceChangeDetail{}, nil
	}
	added, removed := bridgeTallyUnifiedPatch(patch)
	return desktopbridge.WorkspaceChangeDetail{
		Diff:    patch,
		Source:  "git",
		Added:   added,
		Removed: removed,
		Binary:  bytes.Contains(raw, []byte("Binary files ")) || bytes.Contains(raw, []byte("GIT binary patch")),
	}, nil
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
