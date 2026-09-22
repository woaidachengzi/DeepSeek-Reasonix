package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"reasonix/internal/agent"
	"reasonix/internal/boot"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/fileref"
	"reasonix/internal/provider"
	"reasonix/internal/sessioncontext"
)

// controllerFactory builds the established Go core only after a bridge client
// opens a session. It lives with the host because the layering rule keeps
// internal/desktopbridge free of boot and control imports.
type controllerFactory struct {
	base   boot.Options
	events *desktopbridge.EventStream
}

func newControllerFactory(events *desktopbridge.EventStream) *controllerFactory {
	return &controllerFactory{events: events}
}

func (f *controllerFactory) Open(ctx context.Context, request desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
	opts := f.base
	opts.WorkspaceRoot = request.WorkspaceRoot
	if f.events != nil {
		opts.Sink = f.events.Sink(request.SessionID)
	} else if opts.Sink == nil {
		opts.Sink = event.Discard
	}
	if strings.TrimSpace(opts.StatsSource) == "" {
		opts.StatsSource = "desktop-tauri"
	}
	controller, err := boot.Build(ctx, opts)
	if err != nil {
		return nil, err
	}
	if err := resumeBridgeSession(controller, request.SessionID); err != nil {
		controller.Close()
		return nil, err
	}
	return &controllerRuntime{controller: controller}, nil
}

// resumeBridgeSession gives a bridge ID one deterministic transcript path.
// Existing Wails sessions are intentionally not discovered or migrated here:
// a Tauri preview creates its own explicitly named session on first open and
// can only resume the same ID it created earlier.
func resumeBridgeSession(controller *control.Controller, sessionID string) error {
	path, err := bridgeSessionPath(controller.SessionDir(), sessionID)
	if err != nil {
		return err
	}
	loaded, err := agent.LoadSession(path)
	if errors.Is(err, os.ErrNotExist) {
		controller.SetFreshSessionPath(path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("load desktop bridge session: %w", err)
	}
	controller.Resume(loaded, path)
	return nil
}

func bridgeSessionPath(sessionDir, sessionID string) (string, error) {
	if strings.TrimSpace(sessionDir) == "" {
		return "", fmt.Errorf("desktop bridge session directory is unavailable")
	}
	if sessionID == "" || len(sessionID) > 128 {
		return "", fmt.Errorf("desktop bridge session identifier is invalid")
	}
	for _, byte := range []byte(sessionID) {
		if !(byte >= 'a' && byte <= 'z') && !(byte >= 'A' && byte <= 'Z') && !(byte >= '0' && byte <= '9') && byte != '-' && byte != '_' {
			return "", fmt.Errorf("desktop bridge session identifier is invalid")
		}
	}
	return filepath.Join(sessionDir, "tauri-"+sessionID+".jsonl"), nil
}

// controllerRuntime adapts the established controller to the bridge's minimal
// Runtime surface.
type controllerRuntime struct {
	controller *control.Controller
}

func (r *controllerRuntime) SessionPath() string { return r.controller.SessionPath() }

func (r *controllerRuntime) Title() string {
	meta, ok, err := agent.LoadBranchMeta(r.SessionPath())
	if err != nil || !ok {
		return ""
	}
	return meta.CustomTitle
}

func (r *controllerRuntime) Rename(title string) error {
	return agent.RenameSession(r.SessionPath(), title)
}

// Delete sweeps the session's durable artifacts through the controller's own
// removal path, so the bridge cannot drift from what the core considers a
// session to be, then releases the controller without a shutdown snapshot.
func (r *controllerRuntime) Delete() error {
	path := r.SessionPath()
	if err := control.RemoveSessionArtifacts(path); err != nil {
		return err
	}
	r.controller.Close()
	return nil
}

func (r *controllerRuntime) State() string {
	status := r.controller.RuntimeStatus()
	switch {
	case status.Running:
		return "running"
	case status.PendingPrompt:
		return "paused"
	default:
		return "idle"
	}
}

const bridgeHistoryMaxContentRunes = 16_000

func bridgeHistoryMessageVisible(message provider.Message) bool {
	return message.Role == provider.RoleUser && !sessioncontext.IsContent(message.Content) ||
		message.Role == provider.RoleAssistant
}

func bridgeHistoryMessageContent(message provider.Message) string {
	if message.Role == provider.RoleUser {
		return agent.UserMessageText(message)
	}
	return message.Content
}

// History makes only user- and assistant-visible text available to the host.
// The controller transcript also contains system prompts, provider reasoning,
// tool requests/results, image references, and local execution metadata; none
// of those are a safe or stable first bridge contract.
func (r *controllerRuntime) History() []desktopbridge.HistoryMessage {
	history := r.controller.History()
	messages := make([]desktopbridge.HistoryMessage, 0, len(history))
	for _, message := range history {
		// Session-context is provider-facing runtime metadata, not a user
		// question. It is persisted as a host-authored user-role message so the
		// model can see it, but must never be projected into the chat transcript.
		if !bridgeHistoryMessageVisible(message) {
			continue
		}
		var role string
		switch message.Role {
		case provider.RoleUser:
			role = "user"
		case provider.RoleAssistant:
			role = "assistant"
		default:
			continue
		}
		content := bridgeHistoryMessageContent(message)
		if strings.TrimSpace(content) == "" {
			continue
		}
		content, truncated := truncateBridgeHistoryContent(content)
		messages = append(messages, desktopbridge.HistoryMessage{
			Role:      role,
			Content:   content,
			Truncated: truncated,
		})
	}
	return messages
}

func (r *controllerRuntime) AttachFile(path string) (desktopbridge.AttachmentView, error) {
	if !filepath.IsAbs(path) {
		return desktopbridge.AttachmentView{}, desktopbridge.ErrInvalidAttachment
	}
	name := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))
	isImage := ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp"
	var (
		rel string
		err error
	)
	if isImage {
		rel, err = control.SaveImageFileInRoot(r.controller.WorkspaceRoot(), path)
	} else {
		rel, err = control.SaveAttachmentFileInRoot(r.controller.WorkspaceRoot(), path)
	}
	if err != nil {
		return desktopbridge.AttachmentView{}, fmt.Errorf("%w: copy selected file", desktopbridge.ErrInvalidAttachment)
	}
	info, err := os.Stat(filepath.Join(r.controller.WorkspaceRoot(), filepath.FromSlash(rel)))
	if err != nil {
		return desktopbridge.AttachmentView{}, fmt.Errorf("read copied attachment metadata: %w", err)
	}
	return desktopbridge.AttachmentView{Path: rel, Name: name, Size: info.Size(), IsImage: isImage}, nil
}

const bridgeWorkspaceEntryLimit = 200

const bridgeWorkspacePreviewLimit = 512 << 10

// resolveWorkspacePath validates a renderer-supplied relative path and
// resolves symlinks before any filesystem access. Returning the normalized
// relative spelling separately keeps responses stable even when a path points
// through a symlink that remains inside the workspace.
func (r *controllerRuntime) resolveWorkspacePath(rel string) (string, string, string, error) {
	root := strings.TrimSpace(r.controller.WorkspaceRoot())
	if root == "" {
		root = "."
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", "", "", err
	}
	rawRel := strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
	if strings.HasPrefix(rawRel, "/") || filepath.VolumeName(rawRel) != "" {
		return "", "", "", fmt.Errorf("%w: absolute paths are not allowed", desktopbridge.ErrInvalidWorkspacePath)
	}
	cleanRel := strings.Trim(rawRel, "/")
	if len(cleanRel) > 1024 || strings.ContainsRune(cleanRel, '\x00') {
		return "", "", "", desktopbridge.ErrInvalidWorkspacePath
	}
	if cleanRel == "." {
		cleanRel = ""
	}
	candidate := base
	if cleanRel != "" {
		candidate = filepath.Join(base, filepath.FromSlash(cleanRel))
		relative, relErr := filepath.Rel(base, candidate)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return "", "", "", fmt.Errorf("%w: path escapes root", desktopbridge.ErrInvalidWorkspacePath)
		}
	}
	resolvedBase, resolveErr := filepath.EvalSymlinks(base)
	if resolveErr != nil {
		return "", "", "", resolveErr
	}
	resolvedPath, resolveErr := filepath.EvalSymlinks(candidate)
	if resolveErr != nil {
		return "", "", "", resolveErr
	}
	resolvedRelative, resolveErr := filepath.Rel(resolvedBase, resolvedPath)
	if resolveErr != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(os.PathSeparator)) {
		return "", "", "", fmt.Errorf("%w: symlink escapes root", desktopbridge.ErrInvalidWorkspacePath)
	}
	return base, resolvedPath, cleanRel, nil
}

// ListWorkspace exposes the same bounded, one-level view used by the stable
// desktop file-reference picker. Paths are always relative to the active
// workspace and generated/vendor directories stay hidden.
func (r *controllerRuntime) ListWorkspace(rel string) (desktopbridge.WorkspaceList, error) {
	_, dir, cleanRel, err := r.resolveWorkspacePath(rel)
	if err != nil {
		return desktopbridge.WorkspaceList{}, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return desktopbridge.WorkspaceList{}, err
	}
	result := make([]desktopbridge.WorkspaceEntry, 0, minInt(len(entries), bridgeWorkspaceEntryLimit))
	for _, entry := range entries {
		name := entry.Name()
		isDir := entry.IsDir()
		entryPath := name
		if cleanRel != "" {
			entryPath = cleanRel + "/" + name
		}
		if fileref.SkipEntry(entryPath, name, isDir) {
			continue
		}
		if !isDir {
			info, infoErr := entry.Info()
			if infoErr != nil || !info.Mode().IsRegular() {
				continue
			}
		}
		result = append(result, desktopbridge.WorkspaceEntry{Name: name, Path: filepath.ToSlash(entryPath), IsDir: isDir})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		left, right := strings.ToLower(result[i].Name), strings.ToLower(result[j].Name)
		if left == right {
			return result[i].Name < result[j].Name
		}
		return left < right
	})
	truncated := len(result) > bridgeWorkspaceEntryLimit
	if truncated {
		result = result[:bridgeWorkspaceEntryLimit]
	}
	return desktopbridge.WorkspaceList{Path: cleanRel, Entries: result, Truncated: truncated}, nil
}

// ReadWorkspaceFile returns at most bridgeWorkspacePreviewLimit bytes of a
// regular text file. Binary and invalid-UTF-8 files are identified without
// returning their contents to the renderer.
func (r *controllerRuntime) ReadWorkspaceFile(rel string) (desktopbridge.WorkspaceFilePreview, error) {
	_, resolvedPath, cleanRel, err := r.resolveWorkspacePath(rel)
	if err != nil {
		return desktopbridge.WorkspaceFilePreview{}, err
	}
	if cleanRel == "" {
		return desktopbridge.WorkspaceFilePreview{}, fmt.Errorf("%w: a file path is required", desktopbridge.ErrInvalidWorkspacePath)
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return desktopbridge.WorkspaceFilePreview{}, fmt.Errorf("%w: file is unavailable", desktopbridge.ErrInvalidWorkspacePath)
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return desktopbridge.WorkspaceFilePreview{}, fmt.Errorf("%w: path is not a regular file", desktopbridge.ErrInvalidWorkspacePath)
	}
	file, err := os.Open(resolvedPath)
	if err != nil {
		return desktopbridge.WorkspaceFilePreview{}, fmt.Errorf("%w: file cannot be opened", desktopbridge.ErrInvalidWorkspacePath)
	}
	defer file.Close()
	buffer := make([]byte, bridgeWorkspacePreviewLimit+1)
	read, readErr := io.ReadFull(file, buffer)
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return desktopbridge.WorkspaceFilePreview{}, fmt.Errorf("read workspace file: %w", readErr)
	}
	truncated := read > bridgeWorkspacePreviewLimit
	if truncated {
		buffer = buffer[:bridgeWorkspacePreviewLimit]
	} else {
		buffer = buffer[:read]
	}
	preview := desktopbridge.WorkspaceFilePreview{Path: filepath.ToSlash(cleanRel), Size: info.Size(), Truncated: truncated}
	if bytes.IndexByte(buffer, 0) != -1 || !utf8.Valid(buffer) {
		preview.Binary = true
		return preview, nil
	}
	preview.Body = string(buffer)
	return preview, nil
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func truncateBridgeHistoryContent(content string) (string, bool) {
	runes := []rune(content)
	if len(runes) <= bridgeHistoryMaxContentRunes {
		return content, false
	}
	return string(runes[:bridgeHistoryMaxContentRunes]) + "\n\n[Preview truncated this message]", true
}

func (r *controllerRuntime) Submit(input string) { r.controller.SubmitHTTP(input) }

func (r *controllerRuntime) Cancel() { r.controller.Cancel() }

func (r *controllerRuntime) Approve(promptID string, allow bool) {
	r.controller.Approve(promptID, allow, false, false)
}

func (r *controllerRuntime) AnswerQuestion(promptID string, answers []desktopbridge.AskAnswer) error {
	selected := make([]event.AskAnswer, len(answers))
	for i, answer := range answers {
		selected[i] = event.AskAnswer{QuestionID: answer.QuestionID, Selected: append([]string(nil), answer.Selected...)}
	}
	return r.controller.AnswerQuestionChecked(promptID, selected)
}

func (r *controllerRuntime) AnswerMCPInteraction(promptID, action string, content map[string]any) error {
	return r.controller.AnswerMCPInteractionChecked(promptID, action, content)
}

func (r *controllerRuntime) ReplayPendingPrompts() { r.controller.ReplayPendingPrompts() }

func (r *controllerRuntime) Shutdown() error {
	err := r.controller.SnapshotForShutdown()
	r.controller.Close()
	return err
}
