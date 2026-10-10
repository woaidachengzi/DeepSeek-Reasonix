package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"reasonix/internal/bot"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

func combinedDrivingTestFixture(t *testing.T) (*remoteDrivingFixture, *previewDesktopCommands, *ownedCommandProvider, previewDesktopCatalogueEntry) {
	t.Helper()
	f := remoteDrivingTestFixture(t)
	t.Setenv("REASONIX_STATE_HOME", t.TempDir())
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	p, endpoint := newOwnedCommandFixtureProvider(t)
	if err := appconfig.Default().SaveTo(appconfig.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf("default_model=\"local/alpha\"\n[desktop]\nprovider_access=[\"local\"]\n[[providers]]\nname=\"local\"\nkind=\"preview-owned-command-test\"\nbase_url=%q\nmodels=[\"alpha\",\"beta\"]\ndefault=\"alpha\"\n", endpoint)
	if err := os.WriteFile(appconfig.UserConfigPath(), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	manager := desktopbridge.NewRuntimeManager(newControllerFactory(nil))
	t.Cleanup(func() { _ = manager.Shutdown() })
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "combined-private-local", WorkspaceRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	catalogue := newPreviewDesktopCatalogue(manager, f.catalogue.remotes)
	t.Cleanup(catalogue.Close)
	commands := newPreviewDesktopCommands(catalogue)
	t.Cleanup(commands.Close)
	entries, err := catalogue.Refresh(context.Background())
	if err != nil || len(entries) != 2 {
		t.Fatal(entries, err)
	}
	var local previewDesktopCatalogueEntry
	for _, entry := range entries {
		if entry.local != nil {
			local = entry
		} else {
			f.entry = entry
		}
	}
	if local.local == nil {
		t.Fatal("actual local owner missing")
	}
	return f, commands, p, local
}

func TestPreviewCombinedDrivingOpaqueLocalRemoteSingleRouteAndLocalReclaim(t *testing.T) {
	f, commands, p, local := combinedDrivingTestFixture(t)
	defer func() {
		select {
		case <-p.finish:
		default:
			close(p.finish)
		}
	}()
	command := f.command("takeover")
	command.TargetID = local.handle
	out, err := commands.ExecuteDesktopCommand(context.Background(), command)
	if err != nil || strings.Contains(out, "combined-private-local") || !commands.DesktopTakeoverActive(f.route, "operator") {
		t.Fatal(out, err)
	}
	if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("takeover")); err == nil {
		t.Fatal("one chat acquired local and remote simultaneously")
	}
	wrong := f.command("release")
	wrong.ActorID = "another-admin"
	if _, err := commands.ExecuteDesktopCommand(context.Background(), wrong); err == nil || !commands.DesktopTakeoverActive(f.route, "operator") {
		t.Fatal("another actor released local route")
	}
	drive := f.command("drive")
	drive.AnswerText = "/clear literal question"
	if _, err := commands.ExecuteDesktopCommand(context.Background(), drive); err != nil {
		t.Fatal(err)
	}
	if call := awaitOwnedCommand(t, p.calls); !desktopDriverCallContains(call, drive.AnswerText) {
		t.Fatal("combined local command interpreted literal text")
	}
	close(p.finish)
	awaitDesktopDriverIdle(t, commands.catalogue.manager)
	if _, err := commands.catalogue.manager.Submit(context.Background(), local.local.Scope.SessionID, "real local input reclaims"); err != nil {
		t.Fatal(err)
	}
	awaitOwnedCommand(t, p.calls)
	awaitDesktopDriverIdle(t, commands.catalogue.manager)
	if commands.DesktopTakeoverActive(f.route, "operator") {
		t.Fatal("combined route survived local reclaim")
	}
	if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("takeover")); err != nil {
		t.Fatal("reclaimed local route blocked explicit remote takeover", err)
	}
	notificationWait(t, f.requests)
	acquire := notificationWait(t, f.requests)
	if _, err := commands.ExecuteDesktopCommand(context.Background(), command); err == nil {
		t.Fatal("remote route acquired a second local holder")
	}
	if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("drive")); err != nil {
		t.Fatal(err)
	}
	notificationWait(t, f.requests)
	notificationWait(t, f.requests)
	input := notificationWait(t, f.requests)
	if input.Key != acquire.Key {
		t.Fatal("combined remote route adopted another grant")
	}
	if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("release")); err != nil {
		t.Fatal(err)
	}
	if request := notificationWait(t, f.requests); request.Action != "release" {
		t.Fatal("combined release selected wrong backend")
	}
}

func TestPreviewCombinedDrivingUnknownRemoteCannotFallBackToLocal(t *testing.T) {
	f, commands, _, local := combinedDrivingTestFixture(t)
	f.mu.Lock()
	f.fail = "acquire"
	f.mu.Unlock()
	if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("takeover")); err != errPreviewDesktopBinding {
		t.Fatal(err)
	}
	notificationWait(t, f.requests)
	notificationWait(t, f.requests)
	if !commands.DesktopTakeoverActive(f.route, "operator") {
		t.Fatal("unknown remote lost continuing route")
	}
	command := f.command("takeover")
	command.TargetID = local.handle
	if _, err := commands.ExecuteDesktopCommand(context.Background(), command); err == nil {
		t.Fatal("unknown remote retried as local")
	}
	if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("drive")); err == nil {
		t.Fatal("unknown remote input retried")
	}
	select {
	case request := <-f.requests:
		t.Fatal("unknown rearmed remote", request.Action)
	default:
	}
	if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("release")); err != nil {
		t.Fatal(err)
	}
	notificationWait(t, f.requests)
	if _, err := commands.ExecuteDesktopCommand(context.Background(), command); err != nil {
		t.Fatal("explicit confirmed release did not free route", err)
	}
}

func TestPreviewCombinedDrivingReleaseCancelsInitialRemoteReservation(t *testing.T) {
	f, commands, _, _ := combinedDrivingTestFixture(t)
	f.mu.Lock()
	f.block = "acquire"
	f.mu.Unlock()
	finished := make(chan error, 1)
	go func() {
		_, err := commands.ExecuteDesktopCommand(context.Background(), f.command("takeover"))
		finished <- err
	}()
	notificationWait(t, f.requests)
	notificationWait(t, f.requests)
	if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("release")); err != nil {
		t.Fatal("combined release did not await original reservation", err)
	}
	if err := notificationWait(t, finished); err != errPreviewDesktopBinding {
		t.Fatal("cancelled acquire reported success", err)
	}
	if request := notificationWait(t, f.requests); request.Action != "release" {
		t.Fatal("release selected new owner")
	}
	if commands.DesktopTakeoverActive(f.route, "operator") {
		t.Fatal("released coordinator route survived")
	}
}

func TestPreviewCombinedDrivingConcurrentLocalRemoteTakeoverHasOneWinner(t *testing.T) {
	f, commands, _, local := combinedDrivingTestFixture(t)
	result := make(chan error, 2)
	for _, handle := range []string{local.handle, f.entry.handle} {
		go func(handle string) {
			command := f.command("takeover")
			command.TargetID = handle
			_, err := commands.ExecuteDesktopCommand(context.Background(), command)
			result <- err
		}(handle)
	}
	winners := 0
	for range 2 {
		if err := notificationWait(t, result); err == nil {
			winners++
		}
	}
	if winners != 1 || !commands.DesktopTakeoverActive(f.route, "operator") {
		t.Fatal("route acquired both kinds or lost sole winner", winners)
	}
	if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("release")); err != nil {
		t.Fatal("winner could not release", err)
	}
	if commands.DesktopTakeoverActive(f.route, "operator") {
		t.Fatal("concurrent route retained winner after release")
	}
}

func TestPreviewCombinedDrivingActualGatewayLocalAndRemoteIngress(t *testing.T) {
	f, commands, p, local := combinedDrivingTestFixture(t)
	defer func() {
		select {
		case <-p.finish:
		default:
			close(p.finish)
		}
	}()
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 32)}
	gw := bot.NewGatewayWithAdapterBindings(bot.GatewayConfig{
		Desktop: &desktopCommandsGatewayFixture{commands: commands}, Enabled: map[bot.Platform]bool{f.route.Platform: true},
		ConnectionAccess: map[string]bot.AccessConfig{f.route.ConnectionID: {Enabled: true, Users: []string{"member"}, Admins: []string{"operator"}}},
	}, []bot.AdapterBinding{{ID: f.route.ConnectionID, Domain: f.route.Domain, Platform: f.route.Platform, Adapter: adapter}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := gw.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer gw.Stop()
	send := func(actor, text string) string {
		t.Helper()
		adapter.messages <- bot.InboundMessage{Platform: f.route.Platform, ConnectionID: f.route.ConnectionID, Domain: f.route.Domain, ChatID: f.route.ChatID, ChatType: f.route.ChatType, UserID: actor, Text: text}
		return notificationWait(t, adapter.sent).Text
	}
	if out := send("member", "/desktop takeover "+local.handle); !strings.Contains(out, "权限") {
		t.Fatal(out)
	}
	if out := send("operator", "/desktop takeover "+local.handle); !strings.Contains(out, "已接管") || strings.Contains(out, "combined-private-local") {
		t.Fatal(out)
	}
	text := "actual gateway  local\ninput"
	if out := send("operator", text); !strings.Contains(out, "已接纳") {
		t.Fatal(out)
	}
	if call := awaitOwnedCommand(t, p.calls); !desktopDriverCallContains(call, text) {
		t.Fatal("gateway local input lost literal text")
	}
	close(p.finish)
	awaitDesktopDriverIdle(t, commands.catalogue.manager)
	if out := send("operator", "/desktop release"); !strings.Contains(out, "已解除") {
		t.Fatal(out)
	}
	if out := send("operator", "/desktop takeover "+f.entry.handle); !strings.Contains(out, "已接管") {
		t.Fatal(out)
	}
	notificationWait(t, f.requests)
	notificationWait(t, f.requests)
	if out := send("operator", "actual gateway remote input"); !strings.Contains(out, "已接纳") {
		t.Fatal(out)
	}
	notificationWait(t, f.requests)
	notificationWait(t, f.requests)
	if input := notificationWait(t, f.requests); input.Text != "actual gateway remote input" {
		t.Fatal("remote gateway input changed")
	}
	if out := send("operator", "/desktop release"); !strings.Contains(out, "已解除") {
		t.Fatal(out)
	}
	notificationWait(t, f.requests)
}
