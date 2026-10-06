package bot

import (
	"os"
	"reasonix/internal/config"
	"testing"
)

func TestPairingApprovalCorruptConfigurationRetainsRequest(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg := config.Default()
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	req, _, err := CreateOrRefreshPairingRequest(InboundMessage{Platform: PlatformFeishu, ChatType: ChatDM, ChatID: "chat", UserID: "user"}, PairingConfig{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	const corrupt = "[bad = TOML"
	if err := os.WriteFile(config.UserConfigPath(), []byte(corrupt), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApprovePairingCode(req.Code); err == nil {
		t.Fatal("corrupt config accepted")
	}
	data, _ := os.ReadFile(config.UserConfigPath())
	if string(data) != corrupt {
		t.Fatal("corrupt configuration overwritten")
	}
	pending, err := ListPairingRequests()
	if err != nil || len(pending) != 1 || pending[0].Code != req.Code {
		t.Fatal("failed approval consumed request")
	}
}

func TestPairingApprovalRemovedConnectionDoesNotGrantGlobalAccess(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg := config.Default()
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	req, _, err := CreateOrRefreshPairingRequest(InboundMessage{Platform: PlatformFeishu, ConnectionID: "removed-account", Domain: "feishu", ChatType: ChatDM, ChatID: "chat", UserID: "user"}, PairingConfig{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApprovePairingCode(req.Code); err == nil {
		t.Fatal("removed connection approved")
	}
	next, err := config.LoadForEditReadOnlyStrict(config.UserConfigPath())
	if err != nil || len(next.Bot.Allowlist.FeishuUsers) > 0 || len(next.Bot.Allowlist.FeishuAdmins) > 0 {
		t.Fatal("orphan request granted global access")
	}
	pending, err := ListPairingRequests()
	if err != nil || len(pending) != 1 || pending[0].Code != req.Code {
		t.Fatal("request consumed on refused grant")
	}
}
