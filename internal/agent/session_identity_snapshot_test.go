package agent

import (
	"fmt"
	"reflect"
	"testing"

	"reasonix/internal/provider"
)

func TestMessageIdentitySnapshotBoundedAndIndependent(t *testing.T) {
	s := &Session{}
	ids, ok := s.MessageIdentitySnapshot(0)
	if !ok || ids == nil || len(ids) != 0 {
		t.Fatal("empty identity fence")
	}
	s.Add(provider.Message{ID: "first", Role: provider.RoleUser, Content: "owned"})
	s.Add(provider.Message{ID: "second", Role: provider.RoleAssistant, Content: "owned answer"})
	if ids, ok := s.MessageIdentitySnapshot(1); ok || ids != nil {
		t.Fatal("over-budget identities must fail before allocation")
	}
	ids, ok = s.MessageIdentitySnapshot(2)
	if !ok || !reflect.DeepEqual(ids, []string{"first", "second"}) {
		t.Fatal("identity snapshot drift")
	}
	ids[0] = "changed by reader"
	if s.Snapshot()[0].ID != "first" {
		t.Fatal("reader changed canonical identity")
	}
	s.Replace([]provider.Message{{ID: "duplicate"}, {ID: "duplicate"}})
	if ids, ok := s.MessageIdentitySnapshot(2); ok || ids != nil {
		t.Fatal("duplicate fence")
	}
	invalid := &Session{Messages: []provider.Message{{ID: ""}}}
	if ids, ok := invalid.MessageIdentitySnapshot(1); ok || ids != nil {
		t.Fatal("missing identity fence")
	}
	if invalid.Messages[0].ID != "" {
		t.Fatal("spectator fence must not repair identities")
	}
	var absent *Session
	if ids, ok := absent.MessageIdentitySnapshot(2); ok || ids != nil {
		t.Fatal("absent identity fence")
	}
}

func TestMessageIdentitySnapshotConcurrentAppendHasOnePrefix(t *testing.T) {
	s := &Session{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			s.Add(provider.Message{ID: fmt.Sprintf("owned-%d", i), Role: provider.RoleUser})
		}
	}()
	for {
		ids, ok := s.MessageIdentitySnapshot(100)
		if !ok {
			t.Fatal("bounded append prefix became unavailable")
		}
		for i, id := range ids {
			if id != fmt.Sprintf("owned-%d", i) {
				t.Fatal("mixed append prefix")
			}
		}
		select {
		case <-done:
			return
		default:
		}
	}
}

func TestProjectionBaseSnapshotFencesContentWithStableIdentity(t *testing.T) {
	s := &Session{}
	s.Add(provider.Message{ID: "owned", Role: provider.RoleUser, Content: "old"})
	ids, before, ok := s.MessageProjectionBaseSnapshot(1)
	if !ok || before == "" || len(ids) != 1 || ids[0] != "owned" {
		t.Fatal("missing atomic base")
	}
	s.Replace([]provider.Message{{ID: "owned", Role: provider.RoleUser, Content: "edited"}})
	_, after, ok := s.MessageProjectionBaseSnapshot(1)
	if !ok || after == before {
		t.Fatal("same-ID content edit escaped fence")
	}
	if ids, digest, ok := s.MessageProjectionBaseSnapshot(0); ok || ids != nil || digest != "" {
		t.Fatal("display count budget was bypassed")
	}
	var absent *Session
	if _, _, ok := absent.MessageProjectionBaseSnapshot(1); ok {
		t.Fatal("nil base accepted")
	}
	s.Replace([]provider.Message{{ID: "duplicate"}, {ID: "duplicate"}})
	if _, _, ok := s.MessageProjectionBaseSnapshot(2); ok {
		t.Fatal("duplicate base accepted")
	}
	invalid := &Session{Messages: []provider.Message{{ID: ""}}}
	if _, _, ok := invalid.MessageProjectionBaseSnapshot(1); ok {
		t.Fatal("missing identity repaired by base read")
	}
}
