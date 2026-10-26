package computeruse

import (
	"encoding/json"
	"os"
	"testing"
)

func TestLinuxIdentityContract(t *testing.T) {
	data, err := os.ReadFile("testdata/linux_identity.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Stat []struct {
			Name, Text string
			Ticks      *uint64
		}
		Fingerprint struct {
			Boot, Namespace, Exe, Expected string
			UID, Dev, Ino                  uint64
		}
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	for _, item := range corpus.Stat {
		t.Run(item.Name, func(t *testing.T) {
			ticks, err := linuxStartTicks(item.Text)
			if item.Ticks == nil {
				if err == nil {
					t.Fatal("invalid or exited process accepted")
				}
				return
			}
			if err != nil || ticks != *item.Ticks {
				t.Fatalf("ticks=%d err=%v", ticks, err)
			}
		})
	}
	f := corpus.Fingerprint
	if got := linuxExecutableIdentity(f.Boot, f.Namespace, f.UID, f.Dev, f.Ino, f.Exe); got != f.Expected {
		t.Fatalf("fingerprint=%s", got)
	}
	if linuxExecutableIdentity(f.Boot, f.Namespace, f.UID, f.Dev, f.Ino+1, f.Exe) == f.Expected {
		t.Fatal("replacement executable retained grant")
	}
}
