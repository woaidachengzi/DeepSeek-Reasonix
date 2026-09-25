//go:build !darwin && !linux

package sessionidentity

import "os"

// The fallback keeps os.SameFile's platform semantics when no portable
// device/inode key is available for grouping hard-linked transcripts.
type physicalFileKey struct{}

func fileKey(os.FileInfo) (physicalFileKey, bool) {
	return physicalFileKey{}, false
}
