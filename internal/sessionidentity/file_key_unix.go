//go:build darwin || linux

package sessionidentity

import (
	"os"
	"syscall"
)

type physicalFileKey struct {
	device uint64
	inode  uint64
}

func fileKey(info os.FileInfo) (physicalFileKey, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return physicalFileKey{}, false
	}
	return physicalFileKey{device: uint64(stat.Dev), inode: uint64(stat.Ino)}, true
}
