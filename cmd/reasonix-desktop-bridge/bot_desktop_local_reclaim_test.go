package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"reasonix/internal/bot"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

func localReclaimManager(t *testing.T) (*desktopbridge.RuntimeManager, *ownedCommandProvider) {
	t.Helper()
	p, endpoint := newOwnedCommandFixtureProvider(t)
	cfg := fmt.Sprintf("default_model=\"local/alpha\"\n[desktop]\nprovider_access=[\"local\"]\n[[providers]]\nname=\"local\"\nkind=\"preview-owned-command-test\"\nbase_url=%q\nmodels=[\"alpha\",\"beta\"]\ndefault=\"alpha\"\n", endpoint)
	if err := os.WriteFile(appconfig.UserConfigPath(), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	m := desktopbridge.NewRuntimeManager(newControllerFactory(nil))
	t.Cleanup(func() {
		select {
		case <-p.finish:
		default:
			close(p.finish)
		}
		awaitDesktopDriverIdle(t, m)
		_ = m.Shutdown()
	})
	if _, err := m.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "local-reclaim-owner", WorkspaceRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	return m, p
}

func TestLocalReclaimActualAgentAndGatewayNoticeWithoutWatch(t *testing.T) {
	adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 8)}
	store, sender, _, _, _ := notificationTestSetup(t, adapter)
	m, p := localReclaimManager(t)
	d := newPreviewDesktopDriver(m)
	if err := d.ConfigureReclaimNotifications(context.Background(), sender); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		d.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := d.AwaitReclaimNotifications(ctx); err != nil {
			t.Error(err)
		}
	})
	route := desktopWatchTestRoute()
	command := bot.DesktopCommand{Route: route, ActorID: "owner", Action: "takeover", TargetID: "local-reclaim-owner"}
	if _, err := d.ExecuteDesktopCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	command.Action, command.TargetID, command.AnswerText = "drive", "", "private remote task"
	if _, err := d.ExecuteDesktopCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	call := notificationWait(t, p.calls)
	if len(adapter.sent) != 0 {
		t.Fatal("remote input reported local reclaim")
	}
	if _, err := m.Submit(context.Background(), "local-reclaim-owner", "private local steering input"); err != nil {
		t.Fatal(err)
	}
	msg := notificationWait(t, adapter.sent)
	if msg.Text != previewLocalReclaimSummary || msg.ChatID != route.ChatID || store.Watching(route) {
		t.Fatal("wrong control notice audience or watch dependency", msg)
	}
	if call.ctx.Err() != nil {
		t.Fatal("local reclaim cancelled accepted remote task")
	}
	if d.DesktopTakeoverActive(route, "owner") {
		t.Fatal("local fence did not revoke driving")
	}
	if delivery := notificationWait(t, sender.calls); delivery.err != nil {
		t.Fatal(delivery.err)
	}
	d.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.AwaitReclaimNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	if len(adapter.sent) != 0 {
		t.Fatal("reclaim notice retried")
	}
	close(p.finish)
	// controllerRuntime.Submit uses SubmitHTTP: running plain input is
	// steering, not a promised second provider request. Await the actual turn
	// finishing/idle commit rather than inventing another admission.
	awaitDesktopDriverIdle(t, m)
}

func TestLocalReclaimRetirementOrReleaseCancelsSDKAndAwaits(t *testing.T) {
	for _, action := range []string{"release", "model_replace", "model_replace_during_send", "close"} {
		t.Run(action, func(t *testing.T) {
			cancelled := make(chan struct{})
			adapter := &notificationAdapterFixture{messages: make(chan bot.InboundMessage), sent: make(chan bot.OutboundMessage, 8), hook: func(ctx context.Context) error { <-ctx.Done(); close(cancelled); return ctx.Err() }}
			_, sender, _, _, _ := notificationTestSetup(t, adapter)
			m, p := localReclaimManager(t)
			d := newPreviewDesktopDriver(m)
			t.Cleanup(func() {
				d.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := d.AwaitReclaimNotifications(ctx); err != nil {
					t.Error(err)
				}
			})
			if err := d.ConfigureReclaimNotifications(context.Background(), sender); err != nil {
				t.Fatal(err)
			}
			command := bot.DesktopCommand{Route: desktopWatchTestRoute(), ActorID: "owner", Action: "takeover", TargetID: "local-reclaim-owner"}
			if _, err := d.ExecuteDesktopCommand(context.Background(), command); err != nil {
				t.Fatal(err)
			}
			// Close before any input must not produce a local-reclaim notice.
			if action == "model_replace" {
				if _, err := m.SetSessionModel(context.Background(), command.TargetID, "local/beta"); err != nil {
					t.Fatal(err)
				}
				d.Close()
			} else {
				if _, err := m.Submit(context.Background(), command.TargetID, "private local"); err != nil {
					t.Fatal(err)
				}
				notificationWait(t, adapter.sent)
				if action == "release" {
					command.Action, command.TargetID = "release", ""
					if _, err := d.ExecuteDesktopCommand(context.Background(), command); err != nil {
						t.Fatal(err)
					}
				} else if action == "model_replace_during_send" {
					// Finish the accepted task before replacing its actual owner;
					// the in-flight SDK must still observe the retired source.
					close(p.finish)
					notificationWait(t, p.calls)
					awaitDesktopDriverIdle(t, m)
					if _, err := m.SetSessionModel(context.Background(), command.TargetID, "local/beta"); err != nil {
						t.Fatal(err)
					}
				} else {
					d.Close()
				}
				notificationWait(t, cancelled)
				notificationWait(t, sender.calls)
				if action != "model_replace_during_send" {
					close(p.finish)
					notificationWait(t, p.calls)
					awaitDesktopDriverIdle(t, m)
				}
			}
			d.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := d.AwaitReclaimNotifications(ctx); err != nil {
				t.Fatal(err)
			}
			want := 0
			if len(adapter.sent) != want {
				t.Fatal("retired source produced extra notice")
			}
		})
	}
}
