package main

import (
	"context"
	"testing"

	"reasonix/internal/bot"
)

func TestPreviewDrivingShutdownReleasesOriginalGrantOnceAndWaits(t *testing.T) {
	f, commands, _, _ := combinedDrivingTestFixture(t)
	if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("takeover")); err != nil {
		t.Fatal(err)
	}
	notificationWait(t, f.requests)
	acquire := notificationWait(t, f.requests)
	if err := commands.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if release := notificationWait(t, f.requests); release.Action != "release" || release.Key != acquire.Key || release.Scope != acquire.Scope {
		t.Fatal("shutdown adopted a new grant")
	}
	if commands.DesktopTakeoverActive(f.route, "operator") {
		t.Fatal("shutdown still routed input")
	}
	if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("takeover")); err == nil {
		t.Fatal("closed driving accepted a new reservation")
	}
	status := f.command("status")
	status.TargetID = ""
	if _, err := commands.ExecuteDesktopCommand(context.Background(), status); err == nil {
		t.Fatal("orderly command shutdown still accepted ingress")
	}
	if err := commands.driving.Shutdown(context.Background()); err != nil {
		t.Fatal("confirmed shutdown not idempotent", err)
	}
	select {
	case r := <-f.requests:
		t.Fatal("repeat shutdown retried IO", r.Action)
	default:
	}
}

func TestPreviewDrivingShutdownUnknownReleaseSharedAndNeverRetried(t *testing.T) {
	for _, mode := range []string{"failure", "cancel", "emergency"} {
		t.Run(mode, func(t *testing.T) {
			f, commands, _, _ := combinedDrivingTestFixture(t)
			if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("takeover")); err != nil {
				t.Fatal(err)
			}
			notificationWait(t, f.requests)
			notificationWait(t, f.requests)
			f.mu.Lock()
			if mode == "failure" {
				f.fail = "release"
			} else {
				f.block = "release"
			}
			f.mu.Unlock()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			first := make(chan error, 1)
			go func() { first <- commands.driving.Shutdown(ctx) }()
			if request := notificationWait(t, f.requests); request.Action != "release" {
				t.Fatal("shutdown changed operation")
			}
			second := make(chan error, 1)
			go func() { second <- commands.driving.Shutdown(context.Background()) }()
			if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("drive")); err == nil {
				t.Fatal("draining host dispatched input")
			}
			if mode == "cancel" {
				cancel()
			}
			if mode == "emergency" {
				commands.driving.Close()
			}
			if err := notificationWait(t, first); err != errPreviewDesktopBinding {
				t.Fatal("unknown revoke reported complete", err)
			}
			if err := notificationWait(t, second); err != errPreviewDesktopBinding {
				t.Fatal("concurrent shutdown lost original unknown outcome", err)
			}
			if err := commands.driving.Shutdown(context.Background()); err != errPreviewDesktopBinding {
				t.Fatal("repeat hid first unknown outcome", err)
			}
			select {
			case request := <-f.requests:
				t.Fatal("unknown release automatically retried", request.Action)
			default:
			}
		})
	}
}

func TestPreviewDrivingShutdownCancelsAndAwaitsAcquiringReservation(t *testing.T) {
	f, commands, _, _ := combinedDrivingTestFixture(t)
	f.mu.Lock()
	f.block = "acquire"
	f.mu.Unlock()
	operation := make(chan error, 1)
	go func() {
		_, err := commands.ExecuteDesktopCommand(context.Background(), f.command("takeover"))
		operation <- err
	}()
	notificationWait(t, f.requests)
	acquire := notificationWait(t, f.requests)
	if err := commands.driving.Shutdown(context.Background()); err != nil {
		t.Fatal("shutdown did not settle cancelled acquire before release", err)
	}
	if err := notificationWait(t, operation); err != errPreviewDesktopBinding {
		t.Fatal("cancelled acquire acknowledged")
	}
	if release := notificationWait(t, f.requests); release.Action != "release" || release.Key != acquire.Key {
		t.Fatal("shutdown used replacement holder")
	}
}

func TestPreviewDrivingShutdownLocalReleaseDoesNotCancelAcceptedAgentTurn(t *testing.T) {
	f, commands, p, local := combinedDrivingTestFixture(t)
	defer func() { close(p.finish); awaitDesktopDriverIdle(t, commands.catalogue.manager) }()
	command := f.command("takeover")
	command.TargetID = local.handle
	if _, err := commands.ExecuteDesktopCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("drive")); err != nil {
		t.Fatal(err)
	}
	call := awaitOwnedCommand(t, p.calls)
	if err := commands.driving.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	view, ok := commands.catalogue.manager.CommandSnapshot()
	if !ok || !view.State.Running || call.ctx.Err() != nil {
		t.Fatal("driver shutdown canceled accepted Agent turn")
	}
	select {
	case r := <-f.requests:
		t.Fatal("local shutdown touched remote grant", r.Action)
	default:
	}
}

func TestPreviewDrivingShutdownEmptyAndEmergencyCloseAreDistinct(t *testing.T) {
	f := remoteDrivingTestFixture(t)
	d := newPreviewDesktopDriving(f.catalogue)
	if err := d.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	other := newPreviewDesktopDriving(f.catalogue)
	other.Close()
	if err := other.Shutdown(context.Background()); err != errPreviewDesktopBinding {
		t.Fatal("emergency Close became confirmed release")
	}
	if _, err := other.ExecuteDesktopCommand(context.Background(), bot.DesktopCommand{Route: f.route, ActorID: "operator", Action: "release"}); err != errPreviewDesktopBinding {
		t.Fatal("closed host accepted release")
	}
}

func TestPreviewDrivingShutdownRetiredParentCannotConfirmOriginalGrant(t *testing.T) {
	f, commands, _, _ := combinedDrivingTestFixture(t)
	if _, err := commands.ExecuteDesktopCommand(context.Background(), f.command("takeover")); err != nil {
		t.Fatal(err)
	}
	notificationWait(t, f.requests)
	notificationWait(t, f.requests)
	commands.catalogue.Close()
	if err := commands.Shutdown(context.Background()); err != errPreviewDesktopBinding {
		t.Fatal("parent retirement became confirmed remote release", err)
	}
	select {
	case r := <-f.requests:
		t.Fatal("retired parent reconnected for revoke", r.Action)
	default:
	}
}
