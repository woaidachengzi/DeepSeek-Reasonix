package desktopbridge

import (
	"fmt"
	"strings"
)

type WorkspaceCombinedRewindPlan struct {
	PlanID                       string   `json:"planId,omitempty"`
	Turn                         int      `json:"turn"`
	CanFiles                     bool     `json:"canFiles"`
	CanConversation              bool     `json:"canConversation"`
	FileCount                    int      `json:"fileCount"`
	Files                        []string `json:"files"`
	FilesTruncated               bool     `json:"filesTruncated"`
	CoverageGaps                 []string `json:"coverageGaps"`
	RequiresCoverageConfirmation bool     `json:"requiresCoverageConfirmation"`
	Conflicts                    []string `json:"conflicts"`
	DisabledReason               string   `json:"disabledReason,omitempty"`
}

type WorkspaceCombinedRewindResult struct {
	OK                 bool     `json:"ok"`
	Partial            bool     `json:"partial"`
	ConversationForked bool     `json:"conversationForked"`
	HeadID             string   `json:"headId,omitempty"`
	FilesRestored      bool     `json:"filesRestored"`
	TransactionID      string   `json:"transactionId,omitempty"`
	UndoAvailable      bool     `json:"undoAvailable"`
	WrittenCount       int      `json:"writtenCount"`
	DeletedCount       int      `json:"deletedCount"`
	Conflicts          []string `json:"conflicts"`
	Error              string   `json:"error,omitempty"`
}

type RuntimeCombinedRewindProvider interface {
	PrepareCombinedRewind(turn int) (WorkspaceCombinedRewindPlan, error)
	CommitCombinedRewind(planID string, confirmPartialCoverage bool) (WorkspaceCombinedRewindResult, error)
}

func (m *RuntimeManager) PrepareCombinedRewind(sessionID string, turn int) (WorkspaceCombinedRewindPlan, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceCombinedRewindPlan{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceCombinedRewindPlan{}, ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeCombinedRewindProvider)
	if !ok {
		return WorkspaceCombinedRewindPlan{}, fmt.Errorf("%w: combined rewind is unavailable", ErrSessionConflict)
	}
	return provider.PrepareCombinedRewind(turn)
}

func (m *RuntimeManager) CommitCombinedRewind(sessionID, planID string, confirmed bool) (WorkspaceCombinedRewindResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return WorkspaceCombinedRewindResult{}, ErrClosed
	}
	if m.runtime == nil || sessionID == "" || m.view.ID != sessionID {
		return WorkspaceCombinedRewindResult{}, ErrSessionNotFound
	}
	provider, ok := m.runtime.(RuntimeCombinedRewindProvider)
	if !ok {
		return WorkspaceCombinedRewindResult{}, fmt.Errorf("%w: combined rewind is unavailable", ErrSessionConflict)
	}
	result, err := provider.CommitCombinedRewind(planID, confirmed)
	if m.runtime.SessionPath() != m.view.Path {
		return WorkspaceCombinedRewindResult{}, fmt.Errorf("%w: combined rewind changed the transcript path", ErrSessionConflict)
	}
	return result, err
}
