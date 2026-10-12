package sessionpath

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

const LegacyPrefix = "wails-"

func IsLegacyID(id string) bool {
	if !strings.HasPrefix(id, LegacyPrefix) || len(id) != len(LegacyPrefix)+sha256.Size*2 {
		return false
	}
	for _, ch := range id[len(LegacyPrefix):] {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}

// LegacyID binds an imported identity to its original profile-relative path.
// It does not rename/copy transcripts, and survives moving a complete profile.
// Physical confinement and uniqueness remain the identity store's responsibility.
func LegacyID(profileRoot, transcriptPath string) (string, error) {
	if !filepath.IsAbs(profileRoot) || !filepath.IsAbs(transcriptPath) || filepath.Ext(transcriptPath) != ".jsonl" {
		return "", fmt.Errorf("legacy transcript requires an absolute profile and JSONL path")
	}
	relative, err := filepath.Rel(filepath.Clean(profileRoot), filepath.Clean(transcriptPath))
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("legacy transcript escapes its profile")
	}
	digest := sha256.Sum256([]byte(filepath.ToSlash(relative)))
	return LegacyPrefix + hex.EncodeToString(digest[:]), nil
}
