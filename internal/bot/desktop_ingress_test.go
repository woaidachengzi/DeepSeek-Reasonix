package bot

import (
	"context"
	"testing"
)

func TestDesktopScopedIngressRejectsMessageSuppliedConnectionAuthority(t *testing.T) {
	gw, fixture := scopedDesktopGateway()
	gw.cfg.ConnectionAccess = map[string]AccessConfig{
		"restricted": {Enabled: true, Users: []string{"attacker"}, Admins: []string{"safe-admin"}},
		"privileged": {Enabled: true, Admins: []string{"attacker"}},
	}
	inbound := 0
	gw.cfg.OnInbound = func(InboundMessage) { inbound++ }
	adapter := newFakeAdapter(PlatformFeishu, "private-ingress-fixture")
	binding := AdapterBinding{ID: "restricted", Domain: "feishu", Platform: PlatformFeishu, Adapter: adapter}
	for _, text := range []string{"/desktop approve captured-ticket", "drive captured owner"} {
		fixture.active = true
		for _, tc := range []struct{ connection, domain, actor string }{
			{"privileged", "feishu", "attacker"},
			{"restricted", "other-domain", "safe-admin"},
			{"privileged", "other-domain", "attacker"},
		} {
			msg := desktopTestMessage(text)
			msg.ConnectionID, msg.Domain, msg.UserID = tc.connection, tc.domain, tc.actor
			gw.handleMessage(context.Background(), binding, msg)
		}
	}
	if len(fixture.commands) != 0 || inbound != 0 || len(adapter.sentMessages()) != 0 {
		t.Fatal("forged routing reached authority/callback/reply")
	}
	msg := desktopTestMessage("/desktop status")
	msg.ConnectionID, msg.Domain, msg.UserID = "", "", "safe-admin"
	gw.handleMessage(context.Background(), binding, msg)
	if len(fixture.commands) != 1 || inbound != 1 {
		t.Fatal("valid transport identity rejected")
	}
	got := fixture.commands[0]
	if got.Route.ConnectionID != binding.ID || got.Route.Domain != binding.Domain || got.Route.Platform != binding.Platform || got.ActorID != "safe-admin" {
		t.Fatalf("actual binding lost: %#v", got)
	}
}
