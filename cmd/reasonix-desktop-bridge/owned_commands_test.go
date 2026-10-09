package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/control"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/event"
	"reasonix/internal/provider"
)

type ownedCommandCall struct {
	ctx     context.Context
	request provider.Request
}
type ownedCommandProvider struct {
	calls  chan ownedCommandCall
	finish chan struct{}
}

var ownedCommandProviderOnce sync.Once
var ownedCommandProviders sync.Map
var ownedCommandProviderSequence atomic.Uint64

func (*ownedCommandProvider) Name() string { return "preview-owned-command-test" }
func (p *ownedCommandProvider) Stream(ctx context.Context, request provider.Request) (<-chan provider.Chunk, error) {
	select {
	case p.calls <- ownedCommandCall{ctx, request}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	out := make(chan provider.Chunk, 2)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			return
		case <-p.finish:
		}
		out <- provider.Chunk{Type: provider.ChunkText, Text: "owned result"}
		out <- provider.Chunk{Type: provider.ChunkDone}
	}()
	return out, nil
}

func awaitOwnedCommand[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("owned command fixture timed out")
	}
	var empty T
	return empty
}

func newOwnedCommandFixtureProvider(t *testing.T) (*ownedCommandProvider, string) {
	t.Helper()
	p := &ownedCommandProvider{calls: make(chan ownedCommandCall, 4), finish: make(chan struct{})}
	ownedCommandProviderOnce.Do(func() {
		provider.Register(p.Name(), func(cfg provider.Config) (provider.Provider, error) {
			value, ok := ownedCommandProviders.Load(cfg.BaseURL)
			if !ok {
				return nil, errors.New("owned fixture provider unavailable")
			}
			return value.(*ownedCommandProvider), nil
		})
	})
	endpoint := fmt.Sprintf("http://127.0.0.1:1/owned-%d", ownedCommandProviderSequence.Add(1))
	ownedCommandProviders.Store(endpoint, p)
	t.Cleanup(func() { ownedCommandProviders.Delete(endpoint) })
	return p, endpoint
}

func TestOwnedCommandsActualControllerAdmissionPromptAndReplacement(t *testing.T) {
	profile := t.TempDir()
	t.Setenv("REASONIX_HOME", profile)
	t.Setenv("REASONIX_STATE_HOME", profile)
	t.Setenv("REASONIX_CACHE_HOME", t.TempDir())
	t.Setenv("REASONIX_CREDENTIALS_STORE", "file")
	p, endpoint := newOwnedCommandFixtureProvider(t)
	config := fmt.Sprintf(`default_model="local/alpha"
[desktop]
provider_access=["local"]
[[providers]]
name="local"
kind="preview-owned-command-test"
base_url=%q
models=["alpha","beta"]
default="alpha"
`, endpoint)
	if err := os.WriteFile(filepath.Join(profile, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	prompts := make(chan event.Event, 4)
	answered := make(chan event.Event, 4)
	factory := &settingsControllerFactory{controllerFactory: newControllerFactory(nil)}
	observations := desktopbridge.NewOwnedEventStream()
	defer observations.Close()
	factory.ownedEvents = observations
	factory.base.Sink = event.FuncSink(func(e event.Event) {
		switch e.Kind {
		case event.AskRequest:
			prompts <- e
		case event.PromptAnswered:
			answered <- e
		}
	})
	manager := desktopbridge.NewRuntimeManager(factory)
	t.Cleanup(func() { _ = manager.Shutdown() })
	if _, err := manager.Open(context.Background(), desktopbridge.OpenRequest{SessionID: "owned-command", WorkspaceRoot: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	owned := factory.latest
	view, ok := manager.CommandSnapshot()
	if !ok || view.State.Revision == 0 || view.Scope.RuntimeEpoch == "" {
		t.Fatal("actual command snapshot missing")
	}
	initialRuntime := owned
	observer, err := observations.Subscribe(view.Scope)
	if err != nil {
		t.Fatal(err)
	}
	observeCtx, stopObserve := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopObserve()
	observed := make(chan desktopbridge.OwnedObservedEvent, 4)
	observerDone := make(chan error, 1)
	go func() {
		for {
			frame, err := observer.Read(observeCtx)
			if err != nil {
				observerDone <- err
				return
			}
			if frame.Kind == "ask_request" || frame.Kind == "prompt_answered" {
				observed <- frame
			}
		}
	}()
	wrong := view
	wrong.Scope.RuntimeEpoch += "-wrong"
	if err := manager.SubmitOwned(context.Background(), wrong, "wrong"); !errors.Is(err, desktopbridge.ErrOwnedRuntimeChanged) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.SubmitOwned(ctx, view, "revoked"); !errors.Is(err, desktopbridge.ErrOwnedRuntimeChanged) {
		t.Fatal(err)
	}
	if err := manager.SubmitOwned(context.Background(), view, "/clear"); err != nil {
		t.Fatal(err)
	}
	call := awaitOwnedCommand(t, p.calls)
	userText := false
	for _, message := range call.request.Messages {
		if message.Role == provider.RoleUser && strings.Contains(message.Content, "/clear") {
			userText = true
		}
	}
	if !userText {
		t.Fatal("scoped text was interpreted as a management command")
	}
	if err := manager.SubmitOwned(context.Background(), view, "duplicate"); !errors.Is(err, control.ErrTurnSubmitScope) {
		t.Fatal(err)
	}
	answers := make(chan []event.AskAnswer, 1)
	go func() {
		value, _ := owned.controller.Ask(call.ctx, []event.AskQuestion{{ID: "q", Prompt: "Choose?", Options: []event.AskOption{{Label: "A"}}}})
		answers <- value
	}()
	awaitOwnedCommand(t, prompts)
	observedAsk := awaitOwnedCommand(t, observed)
	if observedAsk.Scope != view.Scope || observedAsk.Kind != "ask_request" {
		t.Fatal("Ask observation lost published owner")
	}
	pending, ok := manager.CommandSnapshot()
	if !ok || len(pending.State.Pending) != 1 {
		t.Fatalf("actual pending identity missing: %+v", pending)
	}
	prompt := pending.State.Pending[0]
	raw := json.RawMessage(`{"questions":[{"questionId":"q","selected":["A"]}]}`)
	stale := prompt
	stale.RuntimeEpoch += "-wrong"
	if err := manager.ResolveOwnedPrompt(context.Background(), pending.Scope, stale, raw); err == nil {
		t.Fatal("wrong routing stamp resolved Ask")
	}
	if err := manager.ResolveOwnedPrompt(context.Background(), pending.Scope, prompt, json.RawMessage(`{"questions":[{"questionId":"q","selected":[null]}]}`)); err == nil {
		t.Fatal("invalid Ask union accepted")
	}
	select {
	case <-answers:
		t.Fatal("rejected command woke Ask")
	default:
	}
	if err := manager.ResolveOwnedPrompt(context.Background(), pending.Scope, prompt, raw); err != nil {
		t.Fatal(err)
	}
	result := awaitOwnedCommand(t, answers)
	if len(result) != 1 || result[0].Selected[0] != "A" {
		t.Fatal(result)
	}
	decision := awaitOwnedCommand(t, answered)
	observedDecision := awaitOwnedCommand(t, observed)
	if observedDecision.Scope != view.Scope || observedDecision.Kind != "prompt_answered" {
		t.Fatal("durable decision observation lost owner")
	}
	if decision.TurnID != prompt.TurnID || decision.ItemID != prompt.ID {
		t.Fatal("wrong durable decision transition")
	}
	if err := manager.ResolveOwnedPrompt(context.Background(), pending.Scope, prompt, raw); err == nil {
		t.Fatal("duplicate decision accepted")
	}
	close(p.finish)
	waitForBridgeIdle(t, manager, view.Scope.SessionID)
	if err := manager.SubmitOwned(context.Background(), view, "old idle"); !errors.Is(err, control.ErrTurnSubmitScope) {
		t.Fatal(err)
	}
	if _, err := manager.SetSessionModel(context.Background(), view.Scope.SessionID, "local/beta"); err != nil {
		t.Fatal(err)
	}
	owned = factory.latest
	replaced, ok := manager.CommandSnapshot()
	if !ok || replaced.Scope.SessionPath != view.Scope.SessionPath || replaced.Scope.OwnerEpoch == view.Scope.OwnerEpoch || replaced.Scope.RuntimeEpoch == view.Scope.RuntimeEpoch {
		t.Fatal("same-session replacement retained old command ownership")
	}
	if err := awaitOwnedCommand(t, observerDone); !errors.Is(err, desktopbridge.ErrOwnedRuntimeChanged) {
		t.Fatal("old observation did not retire", err)
	}
	if _, err := observations.Subscribe(view.Scope); !errors.Is(err, desktopbridge.ErrOwnedRuntimeChanged) {
		t.Fatal("old source still registered")
	}
	newObserver, err := observations.Subscribe(replaced.Scope)
	if err != nil {
		t.Fatal(err)
	}
	defer newObserver.Close()
	initialRuntime.lifecycleSink.Emit(event.Event{Kind: event.Notice, Text: "late old source"})
	owned.lifecycleSink.Emit(event.Event{Kind: event.Notice, Text: "published new source"})
	readCtx, stopRead := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopRead()
	frame, err := newObserver.Read(readCtx)
	if err != nil || frame.Scope != replaced.Scope || !strings.Contains(string(frame.Payload), "published new source") {
		t.Fatal("source crossed replacement ownership", err)
	}
	if err := manager.SubmitOwned(context.Background(), view, "old owner"); !errors.Is(err, desktopbridge.ErrOwnedRuntimeChanged) {
		t.Fatal(err)
	}
	if err := manager.ResolveOwnedPrompt(context.Background(), pending.Scope, prompt, raw); !errors.Is(err, desktopbridge.ErrOwnedRuntimeChanged) {
		t.Fatal(err)
	}
	select {
	case <-p.calls:
		t.Fatal("invalid command reached provider")
	default:
	}
	// A rejected settings candidate was never published and must not retire the
	// current observation. A successful explicit refresh binds a new exact source.
	betaRuntime := owned
	configPath := filepath.Join(profile, "config.toml")
	if err := os.WriteFile(configPath, []byte(strings.ReplaceAll(config, endpoint, endpoint+"-unavailable")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RebuildSettings(context.Background(), view.Scope.SessionID); !errors.Is(err, desktopbridge.ErrSessionSettingsApply) {
		t.Fatal("invalid candidate unexpectedly published", err)
	}
	stillOwned, ok := manager.CommandSnapshot()
	if !ok || stillOwned.Scope != replaced.Scope || factory.latest != betaRuntime {
		t.Fatal("failed candidate retired current owner")
	}
	betaRuntime.lifecycleSink.Emit(event.Event{Kind: event.Notice, Text: "old source survived rejected candidate"})
	frame, err = newObserver.Read(readCtx)
	if err != nil || frame.Scope != replaced.Scope {
		t.Fatal("failed candidate retired current observation", err)
	}
	newEndpoint := endpoint + "-refreshed"
	ownedCommandProviders.Store(newEndpoint, p)
	defer ownedCommandProviders.Delete(newEndpoint)
	if err := os.WriteFile(configPath, []byte(strings.ReplaceAll(config, endpoint, newEndpoint)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.SubmitOwned(context.Background(), stillOwned, "must not implicitly rebuild"); !errors.Is(err, desktopbridge.ErrOwnedRuntimeChanged) {
		t.Fatal(err)
	}
	if _, err := manager.RebuildSettings(context.Background(), view.Scope.SessionID); err != nil {
		t.Fatal(err)
	}
	refreshed, ok := manager.CommandSnapshot()
	if !ok || refreshed.Scope.OwnerEpoch == replaced.Scope.OwnerEpoch {
		t.Fatal("settings publication retained manager owner")
	}
	if _, err := newObserver.Read(readCtx); !errors.Is(err, desktopbridge.ErrOwnedRuntimeChanged) {
		t.Fatal("settings publication kept stale observer", err)
	}
	refreshedObserver, err := observations.Subscribe(refreshed.Scope)
	if err != nil {
		t.Fatal("settings publication omitted observer activation", err)
	}
	defer refreshedObserver.Close()
	betaRuntime.lifecycleSink.Emit(event.Event{Kind: event.Notice, Text: "late settings source"})
	factory.latest.lifecycleSink.Emit(event.Event{Kind: event.Notice, Text: "settings source published"})
	frame, err = refreshedObserver.Read(readCtx)
	if err != nil || frame.Scope != refreshed.Scope || !strings.Contains(string(frame.Payload), "settings source published") {
		t.Fatal("settings source crossed ownership", err)
	}
}
