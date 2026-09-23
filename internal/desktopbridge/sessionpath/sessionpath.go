// Package sessionpath owns the one rule that maps a desktop session ID to its
// transcript file. It imports nothing from the kernel so both the bridge (which
// writes transcripts) and the identity importer (which only inspects them) can
// share it without either side re-deriving the layout.
package sessionpath

import (
	"fmt"
	"path/filepath"
	"strings"
)

// TranscriptPath returns the transcript file a desktop session ID owns.
//
// It is the single authority for that mapping: the bridge writes sessions
// through it and the identity store verifies them through it, so the two can
// never drift. The shape is deliberately flat — a session's workspace is UI
// metadata and never selects the directory.
func TranscriptPath(sessionDir, sessionID string) (string, error) {
	if strings.TrimSpace(sessionDir) == "" {
		return "", fmt.Errorf("desktop session directory is unavailable")
	}
	if !ValidID(sessionID) {
		return "", fmt.Errorf("desktop session identifier is invalid")
	}
	return filepath.Join(sessionDir, "tauri-"+sessionID+".jsonl"), nil
}

// ValidID reports whether a session ID may be used as a path component.
// It accepts a bare UUID, with or without the tauri- prefix.
func ValidID(sessionID string) bool {
	if sessionID == "" || len(sessionID) > 128 {
		return false
	}
	for _, b := range []byte(sessionID) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_' {
			continue
		}
		return false
	}
	return true
}
