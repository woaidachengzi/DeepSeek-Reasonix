package computeruse

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Linux uses kernel start ticks as an exact incarnation token, not a wall clock.
// The executable fingerprint also detects exec(2) without a PID change. Keep
// this wire representation identical to cua-runtime/src/identity/linux.rs.
func linuxStartTicks(stat string) (uint64, error) {
	end := strings.LastIndexByte(stat, ')')
	if end < 0 {
		return 0, errors.New("invalid Linux process stat")
	}
	fields := strings.Fields(stat[end+1:])
	if len(fields) < 20 || fields[0] == "Z" || fields[0] == "X" || fields[0] == "x" {
		return 0, errors.New("Linux process unavailable")
	}
	ticks, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || ticks == 0 || ticks > 1<<53-1 {
		return 0, errors.New("invalid Linux process start time")
	}
	return ticks, nil
}

func linuxExecutableIdentity(boot, namespace string, uid, dev, ino uint64, executable string) string {
	canonical := fmt.Sprintf("reasonix-linux-process-v1\n%s\n%s\n%d\n%d\n%d\n%s\n", strings.TrimSpace(boot), namespace, uid, dev, ino, hex.EncodeToString([]byte(executable)))
	digest := sha256.Sum256([]byte(canonical))
	return "linux:" + hex.EncodeToString(digest[:])
}
