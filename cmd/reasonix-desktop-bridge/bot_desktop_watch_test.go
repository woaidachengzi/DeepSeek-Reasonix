package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"reasonix/internal/bot"
	appconfig "reasonix/internal/config"
)

func desktopWatchTestRoute() bot.DesktopWatchRoute {
	return bot.DesktopWatchRoute{Platform: bot.PlatformFeishu, ConnectionID: "watch-fixture", Domain: "feishu", ChatType: bot.ChatDM, ChatID: "private-chat"}
}

func desktopWatchTestConfig(t *testing.T, records ...appconfig.BotDesktopWatcherConfig) string {
	t.Helper()
	profile := t.TempDir()
	t.Setenv("REASONIX_HOME", profile)
	t.Setenv("REASONIX_STATE_HOME", profile)
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	path := filepath.Join(profile, "config.toml")
	cfg := appconfig.Default()
	cfg.Bot.DesktopWatchers = records
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPreviewDesktopWatchDeliveryLeaseCannotAdoptReenabledActor(t *testing.T) {
	store, err := newPreviewDesktopWatchStore(desktopWatchTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	route := desktopWatchTestRoute()
	if err := store.SetWatch(context.Background(), route, "owner", true); err != nil {
		t.Fatal(err)
	}
	old := store.deliveryLeases()[0]
	if same := store.deliveryLeases()[0]; same.ctx != old.ctx {
		t.Fatal("snapshot silently replaced generation")
	}
	if err := store.SetWatch(context.Background(), route, "owner", false); err != nil {
		t.Fatal(err)
	}
	if err := store.SetWatch(context.Background(), route, "owner", true); err != nil {
		t.Fatal(err)
	}
	fresh := store.deliveryLeases()[0]
	if old.ctx.Err() == nil || fresh.ctx.Err() != nil || old.ctx == fresh.ctx {
		t.Fatal("same actor off/on revived stale lease")
	}
	store.Close()
	if fresh.ctx.Err() == nil || len(store.deliveryLeases()) != 0 {
		t.Fatal("close retained active delivery")
	}
}

func TestPreviewDesktopWatchActualConfigActorRoundTripAndPerRouteEdit(t *testing.T) {
	ctx := context.Background()
	route := desktopWatchTestRoute()
	legacyRoute := route
	legacyRoute.ChatID = "legacy-chat"
	legacy := previewWatcherConfig(previewDesktopWatcher{route: legacyRoute})
	path := desktopWatchTestConfig(t, legacy)
	store, err := newPreviewDesktopWatchStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if store.Watching(legacyRoute) {
		t.Fatal("actor-less legacy record gained Preview authority")
	}
	if err := store.SetWatch(ctx, route, "authenticated-operator", true); err != nil {
		t.Fatal(err)
	}
	restarted, err := newPreviewDesktopWatchStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	got := restarted.Snapshot()
	if len(got) != 1 || got[0].route != route || got[0].actor != "authenticated-operator" {
		t.Fatalf("restart snapshot = %#v", got)
	}
	got[0].actor = "consumer mutation"
	if restarted.Snapshot()[0].actor != "authenticated-operator" {
		t.Fatal("consumer changed stored identity")
	}
	other := route
	other.ChatType, other.ChatID = bot.ChatGroup, "shared-chat"
	if err := appconfig.EditConfigFileWithoutCredentials(path, func(cfg *appconfig.Config) error {
		cfg.Bot.QueueCap = 73
		cfg.Bot.DesktopWatchers = append(cfg.Bot.DesktopWatchers, previewWatcherConfig(previewDesktopWatcher{other, "other-operator"}))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetWatch(ctx, route, "authenticated-operator", false); err != nil {
		t.Fatal(err)
	}
	cfg, err := appconfig.LoadForEditWithoutCredentialsReadOnlyStrict(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.QueueCap != 73 || len(cfg.Bot.DesktopWatchers) != 2 {
		t.Fatalf("per-route save overwrote unrelated state: %#v", cfg.Bot.DesktopWatchers)
	}
	for _, record := range cfg.Bot.DesktopWatchers {
		watcher := previewWatcherFromConfig(record)
		if watcher.route == route {
			t.Fatal("off persisted old on record")
		}
		if watcher.route == other && watcher.actor != "other-operator" {
			t.Fatal("another process's actor lost")
		}
	}
	if store.Watching(route) {
		t.Fatal("off kept active memory subscription")
	}
}

func TestPreviewDesktopWatchFailedSaveAndCancellationKeepHonestState(t *testing.T) {
	path := desktopWatchTestConfig(t)
	store, err := newPreviewDesktopWatchStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	route := desktopWatchTestRoute()
	actualPersist := store.persist
	store.persist = func(previewDesktopWatcher, bool) error { return errors.New("isolated injected disk failure") }
	if err := store.SetWatch(context.Background(), route, "actor", true); err == nil {
		t.Fatal("save failure concealed")
	}
	if !store.Watching(route) {
		t.Fatal("failed save rolled back active process state")
	}
	cfg, err := appconfig.LoadForEditWithoutCredentialsReadOnlyStrict(path)
	if err != nil || len(cfg.Bot.DesktopWatchers) != 0 {
		t.Fatal("failure unexpectedly reached disk", err)
	}
	if err := store.SetWatch(context.Background(), route, "actor", false); err == nil {
		t.Fatal("off save failure concealed")
	}
	if store.Watching(route) {
		t.Fatal("failed off restored subscription")
	}
	store.persist = actualPersist
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.SetWatch(ctx, route, "actor", true); err == nil || store.Watching(route) {
		t.Fatal("pre-cancel dispatched mutation")
	}
	ctx, cancel = context.WithCancel(context.Background())
	store.persist = func(w previewDesktopWatcher, on bool) error { err := actualPersist(w, on); cancel(); return err }
	if err := store.SetWatch(ctx, route, "actor", true); !errors.Is(err, context.Canceled) {
		t.Fatal("post-save cancellation reported success", err)
	}
	if !store.Watching(route) {
		t.Fatal("unknown saved mutation rolled back in memory")
	}
	store.Close()
	if store.Watching(route) || len(store.Snapshot()) != 0 {
		t.Fatal("close retained private audience")
	}
	if err := store.SetWatch(context.Background(), route, "actor", true); err == nil {
		t.Fatal("closed store accepted mutation")
	}
}

func TestPreviewDesktopWatchRejectsAmbiguousLegacyAndUnboundedIdentity(t *testing.T) {
	route := desktopWatchTestRoute()
	first := previewWatcherConfig(previewDesktopWatcher{route, "first"})
	second := previewWatcherConfig(previewDesktopWatcher{route, "second"})
	path := desktopWatchTestConfig(t, first, second)
	store, err := newPreviewDesktopWatchStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if store.Watching(route) {
		t.Fatal("duplicate route ownership selected by file order")
	}
	for _, watcher := range []previewDesktopWatcher{
		{route, ""}, {route, " actor "}, {route, "actor\nother"},
		{bot.DesktopWatchRoute{Platform: bot.PlatformFeishu, ConnectionID: " x ", ChatType: bot.ChatDM, ChatID: "chat"}, "actor"},
	} {
		if err := store.SetWatch(context.Background(), watcher.route, watcher.actor, true); err == nil {
			t.Fatal("non-canonical watcher accepted")
		}
	}
	// An explicit new admin subscription replaces all conflicting disk records.
	if err := store.SetWatch(context.Background(), route, "fresh-actor", true); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < previewDesktopWatchLimit; i++ {
		r := route
		r.ChatID = fmt.Sprintf("bounded-%d", i)
		// Capacity checks use in-memory state; avoid 127 unrelated disk writes.
		store.watchers[r.Key()] = previewDesktopWatcher{r, "actor"}
	}
	foreign := route
	foreign.ChatID = "overflow"
	if err := store.SetWatch(context.Background(), foreign, "actor", true); err == nil {
		t.Fatal("observer limit bypassed")
	}
}

func TestPreviewDesktopWatchSerializesOnOffWithoutStaleSnapshot(t *testing.T) {
	path := desktopWatchTestConfig(t)
	store, err := newPreviewDesktopWatchStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	route := desktopWatchTestRoute()
	actualPersist := store.persist
	entered, release := make(chan struct{}), make(chan struct{})
	store.persist = func(w previewDesktopWatcher, on bool) error {
		if on {
			close(entered)
			<-release
		}
		return actualPersist(w, on)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Add(1)
	go func() { defer wg.Done(); results <- store.SetWatch(context.Background(), route, "actor", true) }()
	<-entered
	wg.Add(1)
	go func() { defer wg.Done(); results <- store.SetWatch(context.Background(), route, "actor", false) }()
	close(release)
	wg.Wait()
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if store.Watching(route) {
		t.Fatal("delayed on overwrote off")
	}
	restarted, err := newPreviewDesktopWatchStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if restarted.Watching(route) {
		t.Fatal("disk restored stale on")
	}
}
