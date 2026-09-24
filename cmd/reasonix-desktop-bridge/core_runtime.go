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
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"reasonix/internal/agent"
	"reasonix/internal/boot"
	appconfig "reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/desktopbridge/sessionpath"
	"reasonix/internal/event"
	"reasonix/internal/fileref"
	"reasonix/internal/pathidentity"
	"reasonix/internal/provider"
	"reasonix/internal/sessioncontext"
	"reasonix/internal/sessionidentity"
	sessionstore "reasonix/internal/store"
)

// A registered identity must never be reused as an empty session merely
// because its transcript is absent. Keep the existing conflict wire status
// until the full S2 lifecycle protocol is introduced.
var ErrKnownSessionMissing = fmt.Errorf("%w: registered transcript is missing", desktopbridge.ErrSessionConflict)

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
	opts.Sink = newBridgeLifecycleSink(opts.Sink, request.SessionID)
	if strings.TrimSpace(opts.StatsSource) == "" {
		opts.StatsSource = "desktop-tauri"
	}
	controller, err := boot.Build(ctx, opts)
	if err != nil {
		return nil, err
	}
	if err := resumeBridgeSession(ctx, controller, request.SessionID, request.WorkspaceRoot); err != nil {
		controller.Close()
		return nil, err
	}
	return &controllerRuntime{controller: controller, sessionID: request.SessionID}, nil
}

type bridgeLifecycleSink struct {
	inner        event.Sink
	sessionID    string
	sessionPath  string
	identityPath string
	mu           sync.Mutex
	markedReady  bool
}

func newBridgeLifecycleSink(inner event.Sink, sessionID string) event.Sink {
	sessionPath, err := bridgeSessionPath(appconfig.SessionDir(), sessionID)
	if err != nil || appconfig.DesktopSessionIdentityPath() == "" {
		return inner
	}
	return &bridgeLifecycleSink{
		inner: inner, sessionID: sessionID, sessionPath: sessionPath,
		identityPath: appconfig.DesktopSessionIdentityPath(),
	}
}

func (s *bridgeLifecycleSink) Emit(input event.Event) {
	if s.inner != nil {
		s.inner.Emit(input)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.markedReady {
		return
	}
	info, err := os.Lstat(s.sessionPath)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	identities, err := sessionidentity.Open(context.Background(), s.identityPath)
	if err != nil {
		return
	}
	err = identities.MarkReady(context.Background(), s.sessionID, s.sessionPath)
	_ = identities.Close()
	if err == nil {
		s.markedReady = true
	}
}

// resumeBridgeSession gives a bridge ID one deterministic transcript path.
// Existing Wails sessions are intentionally not discovered or migrated here:
// a Tauri preview creates its own explicitly named session on first open and
// can only resume the same ID it created earlier.
func resumeBridgeSession(ctx context.Context, controller *control.Controller, sessionID, workspaceRoot string) error {
	path, err := bridgeSessionPath(controller.SessionDir(), sessionID)
	if err != nil {
		return err
	}

	identityPath := appconfig.DesktopSessionIdentityPath()
	if appconfig.SessionDir() == "" || identityPath == "" ||
		pathidentity.Canonical(appconfig.SessionDir()) != pathidentity.Canonical(controller.SessionDir()) {
		return resumeUncataloguedBridgeSession(controller, path)
	}

	identities, err := sessionidentity.Open(ctx, identityPath)
	if err != nil {
		return fmt.Errorf("open session identity store: %w", err)
	}
	defer identities.Close()

	record, registered, err := identities.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("read session identity: %w", err)
	}
	if registered && pathidentity.Canonical(record.Path) != pathidentity.Canonical(path) {
		return fmt.Errorf("%w: %s", sessionidentity.ErrPathChanged, sessionID)
	}
	if registered {
		switch record.State {
		case sessionidentity.StateMissing:
			return fmt.Errorf("%w: %s", ErrKnownSessionMissing, sessionID)
		case sessionidentity.StateDeleting, sessionidentity.StateDeleted:
			return fmt.Errorf("%w: session %s is %s", desktopbridge.ErrSessionConflict, sessionID, record.State)
		case sessionidentity.StateReserved, sessionidentity.StateReady:
		default:
			return fmt.Errorf("%w: session %s has unknown state %q", desktopbridge.ErrSessionConflict, sessionID, record.State)
		}
	}

	transcriptInfo, statErr := os.Lstat(path)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect desktop bridge transcript: %w", statErr)
	}
	transcriptExists := statErr == nil
	if transcriptExists && !transcriptInfo.Mode().IsRegular() {
		return fmt.Errorf("%w: session transcript is not a regular file", desktopbridge.ErrSessionConflict)
	}
	if registered && record.State == sessionidentity.StateReady && !transcriptExists {
		if err := identities.MarkMissing(ctx, sessionID, path); err != nil {
			return err
		}
		return fmt.Errorf("%w: %s", ErrKnownSessionMissing, sessionID)
	}
	if registered && record.State == sessionidentity.StateReserved && !transcriptExists {
		controller.SetFreshSessionPath(path)
		return nil
	}
	if !registered && !transcriptExists {
		residue, err := hasBridgeSessionResidue(path)
		if err != nil {
			return err
		}
		if residue {
			return fmt.Errorf("%w: unregistered session files remain for %s", desktopbridge.ErrSessionConflict, sessionID)
		}
		if err := os.MkdirAll(controller.SessionDir(), 0o700); err != nil {
			return fmt.Errorf("create desktop bridge session directory: %w", err)
		}
		if err := identities.Reserve(ctx, controller.SessionDir(), sessionidentity.Candidate{
			ID: sessionID, Path: path, WorkspaceRoot: workspaceRoot,
		}); err != nil {
			return fmt.Errorf("reserve desktop bridge session: %w", err)
		}
		controller.SetFreshSessionPath(path)
		return nil
	}

	loaded, err := agent.LoadSession(path)
	if err != nil {
		return fmt.Errorf("load desktop bridge session: %w", err)
	}
	if registered && record.State == sessionidentity.StateReserved {
		if err := identities.MarkReady(ctx, sessionID, path); err != nil {
			return err
		}
	}
	if !registered {
		meta, _, _ := agent.LoadBranchMeta(path)
		if err := identities.Import(ctx, controller.SessionDir(), []sessionidentity.Candidate{{
			ID: sessionID, Path: path, WorkspaceRoot: workspaceRoot, Title: meta.CustomTitle,
		}}); err != nil {
			return fmt.Errorf("register existing desktop bridge session: %w", err)
		}
	}
	controller.Resume(loaded, path)
	return nil
}

func resumeUncataloguedBridgeSession(controller *control.Controller, path string) error {
	loaded, err := agent.LoadSession(path)
	if errors.Is(err, os.ErrNotExist) {
		residue, residueErr := hasBridgeSessionResidue(path)
		if residueErr != nil {
			return residueErr
		}
		if residue {
			return fmt.Errorf("%w: unregistered session files remain", desktopbridge.ErrSessionConflict)
		}
		controller.SetFreshSessionPath(path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("load desktop bridge session: %w", err)
	}
	controller.Resume(loaded, path)
	return nil
}

// hasBridgeSessionResidue detects durable sidecars left behind after a
// transcript is removed. They prevent an unknown ID from being reused fresh.
func hasBridgeSessionResidue(transcriptPath string) (bool, error) {
	for _, residuePath := range []string{
		sessionstore.SessionMeta(transcriptPath),
		sessionstore.SessionInboxDir(transcriptPath),
	} {
		_, err := os.Lstat(residuePath)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("inspect desktop bridge session residue: %w", err)
		}
	}
	return false, nil
}

// bridgeSessionPath keeps the bridge on the shared session-path rule. The
// workspace is deliberately absent: it is UI metadata and never selects the
// transcript directory.
func bridgeSessionPath(sessionDir, sessionID string) (string, error) {
	return sessionpath.TranscriptPath(sessionDir, sessionID)
}

// controllerRuntime adapts the established controller to the bridge's minimal
// Runtime surface.
type controllerRuntime struct {
	controller      *control.Controller
	sessionID       string
	deleting        atomic.Bool
	deleted         atomic.Bool
	removeArtifacts func(string) error
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
	if err := agent.RenameSession(r.SessionPath(), title); err != nil {
		return err
	}
	if r.sessionID == "" || appconfig.DesktopSessionIdentityPath() == "" {
		return nil
	}
	identities, err := sessionidentity.Open(context.Background(), appconfig.DesktopSessionIdentityPath())
	if err != nil {
		return fmt.Errorf("open session identity for title update: %w", err)
	}
	defer func() { _ = identities.Close() }()
	record, exists, err := identities.Get(context.Background(), r.sessionID)
	if err != nil {
		return fmt.Errorf("read session identity for title update: %w", err)
	}
	if !exists {
		return fmt.Errorf("session identity for title update: %w", sessionidentity.ErrSessionNotFound)
	}
	if err := identities.SetTitle(context.Background(), r.sessionID, record.TitleRevision, title, sessionidentity.TitleManualRename); err != nil {
		return fmt.Errorf("update session identity title: %w", err)
	}
	return nil
}

// Delete sweeps the session's durable artifacts through the controller's own
// removal path, so the bridge cannot drift from what the core considers a
// session to be, then releases the controller without a shutdown snapshot.
func (r *controllerRuntime) Delete() error {
	if r.deleted.Load() {
		return nil
	}
	path := r.SessionPath()
	remove := r.removeArtifacts
	if remove == nil {
		remove = control.RemoveSessionArtifacts
	}
	identityPath := appconfig.DesktopSessionIdentityPath()
	managed := r.sessionID != "" && identityPath != "" && appconfig.SessionDir() != "" &&
		pathidentity.Canonical(appconfig.SessionDir()) == pathidentity.Canonical(r.controller.SessionDir())
	if !managed {
		if err := remove(path); err != nil {
			return err
		}
		r.controller.Close()
		r.deleted.Store(true)
		return nil
	}
	identities, err := sessionidentity.Open(context.Background(), identityPath)
	if err != nil {
		return fmt.Errorf("open session identity for deletion: %w", err)
	}
	defer identities.Close()
	if err := identities.BeginDelete(context.Background(), r.sessionID, path); err != nil {
		return fmt.Errorf("fence desktop bridge session deletion: %w", err)
	}
	r.deleting.Store(true)
	if err := remove(path); err != nil {
		return err
	}
	if err := identities.FinishDelete(context.Background(), r.sessionID, path); err != nil {
		return fmt.Errorf("finalize desktop bridge session deletion: %w", err)
	}
	r.controller.Close()
	r.deleted.Store(true)
	return nil
}

func (r *controllerRuntime) State() string {
	if r.deleted.Load() {
		return "deleted"
	}
	if r.deleting.Load() {
		return "deleting"
	}
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
	if r.deleting.Load() {
		// A failed or interrupted sweep has already fenced this identity.
		// Snapshotting here would recreate a transcript during deletion.
		r.controller.Close()
		return nil
	}
	err := r.controller.SnapshotForShutdown()
	r.controller.Close()
	return err
}
