package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"weak"

	"reasonix/internal/agent"
	"reasonix/internal/agent/testutil"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	remotecontroller "reasonix/internal/remote/controller"
	"reasonix/internal/tool"
)

type projectionGateProvider struct {
	ready, later, laterReady, release chan struct{}
}

// The first real engine turn creates a protocol-recovery checkpoint. The
// second uses the public Controller recovery API, not a relabelled user input
// or a fabricated admission event.
type admissionRecoveryProvider struct {
	first *testutil.MockProvider
	gate  *projectionGateProvider
	calls atomic.Int32
}

func (p *admissionRecoveryProvider) Name() string                     { return "owned-admission-recovery" }
func (p *admissionRecoveryProvider) RequiresAssistantReasoning() bool { return true }
func (p *admissionRecoveryProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	if p.calls.Add(1) == 1 {
		return p.first.Stream(ctx, req)
	}
	return p.gate.Stream(ctx, req)
}

func TestDesktopSessionAdmissionActualEngineServeRecovery(t *testing.T) {
	f, client := desktopViewFixture(t)
	f.server.ctl().Close()
	session := agent.NewSession("PRIVATE system")
	session.Add(provider.Message{ID: "prior-answer", Role: provider.RoleAssistant, Content: "earlier", ReasoningContent: "earlier reasoning"})
	f.active = filepath.Join(f.dir, "admission-recovery.jsonl")
	if err := session.Save(f.active); err != nil {
		t.Fatal(err)
	}
	if err := f.leases.Rebind(f.active); err != nil {
		t.Fatal(err)
	}
	f.server.bc.SetCurrentSession(f.active)
	gate := &projectionGateProvider{ready: make(chan struct{}), later: make(chan struct{}), laterReady: make(chan struct{}), release: make(chan struct{})}
	p := &admissionRecoveryProvider{first: testutil.NewMock("strict", testutil.ErrorTurn(&provider.APIError{Status: 400, Body: `{"model":"deepseek"}`})), gate: gate}
	executor := agent.New(p, tool.NewRegistry(), session, agent.Options{}, event.Discard)
	tag := newSessionTagSink(f.server.bc)
	tag.SetPath(f.active)
	done := make(chan event.Event, 2)
	ctrl := control.New(control.Options{Runner: executor, Executor: executor, Sink: event.FuncSink(func(e event.Event) {
		tag.Emit(e)
		if e.Kind == event.TurnDone {
			done <- e
		}
	}), SessionDir: f.dir, SessionPath: f.active})
	f.server.mu.Lock()
	f.server.ctrl = ctrl
	f.server.mu.Unlock()
	f.server.RegisterSessionTag(ctrl, tag)
	t.Cleanup(ctrl.Close)
	path := agent.CanonicalSessionPath(f.active)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stream, err := client.SessionEvents(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	ctrl.Submit("actual user question")
	select {
	case finished := <-done:
		if finished.Err == nil {
			t.Fatal("fixture must create a real protocol-recovery checkpoint")
		}
	case <-ctx.Done():
		t.Fatal("owned user turn never settled")
	}
	var userID string
	for userID == "" {
		frame, err := stream.Next()
		if err != nil {
			t.Fatal(err)
		}
		if frame.Kind == "user_message_admitted" {
			if frame.MessageID == "" || frame.TurnID == "" || frame.Sequence == 0 || frame.SessionPath != path || !frame.SessionCurrent || frame.Text != "" || frame.ItemID != "" {
				t.Fatalf("actual user SSE identity/privacy: %+v", frame)
			}
			userID = frame.MessageID
		}
	}
	action := ctrl.PendingProtocolRecovery()
	if action == nil || userID == "" {
		t.Fatal("missing canonical user identity or recovery token")
	}
	beforeRecovery := session.Snapshot()
	ctrl.SubmitProtocolRecovery(action.ID, "PRIVATE host guidance")
	settled := false
	t.Cleanup(func() {
		cancel()
		ctrl.Cancel()
		if settled {
			return
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("owned recovery did not terminate")
		}
	})
	var hostID, hostTurn string
	var hostSequence uint64
	for hostID == "" {
		frame, err := stream.Next()
		if err != nil {
			t.Fatal(err)
		}
		if frame.Kind != "host_input_admitted" {
			continue
		}
		if frame.MessageID == "" || frame.MessageID == userID || frame.TurnID == "" || frame.Sequence == 0 || frame.SessionPath != path || !frame.SessionCurrent || frame.Text != "" || frame.ItemID != "" {
			t.Fatalf("actual host SSE identity/privacy: %+v", frame)
		}
		hostID, hostTurn, hostSequence = frame.MessageID, frame.TurnID, frame.Sequence
	}
	select {
	case <-gate.ready:
	case <-ctx.Done():
		t.Fatal("recovery never reached its active capture")
	}
	// The exact engine-authored hidden metadata consumption can now advance
	// the fence without blessing unrelated edits or exposing a new question.
	cut, err := client.SessionProjection(ctx, path)
	if err != nil || cut.ActiveTurnID != hostTurn || len(cut.UserSuffix) != 0 || !cut.Replay.HasMore {
		t.Fatalf("actual active recovery cut did not reconcile: %v", err)
	}
	encoded, err := json.Marshal(cut)
	if err != nil || strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("private host/system input escaped active display cut")
	}
	page, err := client.SessionProjectionPage(ctx, path, cut)
	if err != nil || page.Replay.HasMore || page.Replay.NextAfterSequence != cut.Replay.LatestSequence {
		t.Fatalf("actual recovery pagination: %v", err)
	}
	original := session.Snapshot()
	edited := session.Snapshot()
	for i := range edited {
		if edited[i].ID == userID {
			edited[i].Content = "unattested same-ID rewrite"
		}
	}
	session.Replace(edited)
	_, rewriteErr := client.SessionProjection(ctx, path)
	session.Replace(original)
	if !errors.Is(rewriteErr, remotecontroller.ErrProjectionReconcile) {
		t.Fatalf("unattested history edit bypassed reconciliation: %v", rewriteErr)
	}
	replay, err := ctrl.TurnProjectionReplay()
	if err != nil || replay.ActiveTurnID != hostTurn {
		t.Fatalf("durable recovery replay: %v", err)
	}
	foundHost := false
	for _, envelope := range replay.Events {
		frame := envelope.Event
		if frame.Kind == "host_input_admitted" {
			foundHost = frame.MessageID == hostID && envelope.Sequence == hostSequence && envelope.TurnID == hostTurn && frame.Text == ""
		}
	}
	userCount, hostCount := 0, 0
	recordConsumed := false
	for _, message := range session.Snapshot() {
		if message.ID == userID && message.Origin == provider.MessageOriginUser {
			userCount++
		}
		if message.ID == hostID && message.Origin == provider.MessageOriginHost {
			hostCount++
		}
		if record, ok := provider.DecodeProtocolRecovery(message.ProtocolRecovery); ok && record.ID == action.ID && record.State == "consumed" {
			for _, prior := range beforeRecovery {
				if previous, ok := provider.DecodeProtocolRecovery(prior.ProtocolRecovery); ok && prior.ID == message.ID && previous.ID == action.ID && previous.State == "pending" {
					recordConsumed = true
				}
			}
		}
	}
	if !foundHost || !recordConsumed || userCount != 1 || hostCount != 1 || p.calls.Load() != 2 {
		t.Fatalf("canonical provenance was lost: users=%d host=%v/%d consumed=%v calls=%d", userCount, foundHost, hostCount, recordConsumed, p.calls.Load())
	}
	ctrl.Cancel()
	select {
	case <-done:
		settled = true
	case <-ctx.Done():
		t.Fatal("owned recovery cancellation never settled")
	}
	cut, err = client.SessionProjection(ctx, path)
	if err != nil || cut.ActiveTurnID != "" || len(cut.UserSuffix) != 0 {
		t.Fatalf("settled recovery cut did not reconcile: %v", err)
	}
	encoded, err = json.Marshal(cut)
	if err != nil || strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("private host/system input escaped terminal display cut")
	}
	foundUser := false
	for _, row := range cut.History {
		foundUser = foundUser || row.ID == userID && row.Content == "actual user question"
		if row.ID == hostID {
			t.Fatal("synthetic continuation became a visible question")
		}
	}
	if !foundUser {
		t.Fatal("settled recovery lost the original canonical question")
	}
}

func (p *projectionGateProvider) Name() string { return "owned-projection-fixture" }
func (p *projectionGateProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	chunks := make(chan provider.Chunk)
	go func() {
		defer close(chunks)
		send := func(chunk provider.Chunk) bool {
			select {
			case chunks <- chunk:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if !send(provider.Chunk{Type: provider.ChunkText, Text: "owned partial"}) {
			return
		}
		// Fixture search updates emit distinct ordered result
		// events without real search/model I/O or an unbounded saved payload.
		for i := 0; i < 530; i++ {
			if !send(provider.Chunk{Type: provider.ChunkServerSearch, ServerSearch: &provider.ServerSearchCall{ID: "owned-search", Query: "owned query", Results: []provider.ServerSearchHit{{Title: "owned source", URL: "https://example.invalid"}}}}) {
				return
			}
		}
		close(p.ready)
		select {
		case <-p.later:
		case <-ctx.Done():
			return
		}
		if !send(provider.Chunk{Type: provider.ChunkText, Text: "LATER outside cut"}) {
			return
		}
		if !send(provider.Chunk{Type: provider.ChunkServerSearch, ServerSearch: &provider.ServerSearchCall{ID: "later-search", Query: "later query", Results: []provider.ServerSearchHit{{Title: "owned late source", URL: "https://example.invalid/late"}}}}) {
			return
		}
		close(p.laterReady)
		select {
		case <-p.release:
		case <-ctx.Done():
			return
		}
		send(provider.Chunk{Type: provider.ChunkDone})
	}()
	return chunks, nil
}

func TestDesktopSessionProjectionOwnedHTTPFixedPagesAndPrivacy(t *testing.T) {
	f := newOwnershipFixture(t)
	f.server.ctl().Close()
	p := &projectionGateProvider{ready: make(chan struct{}), later: make(chan struct{}), laterReady: make(chan struct{}), release: make(chan struct{})}
	session := agent.NewSession("PRIVATE system prompt")
	session.Add(provider.Message{ID: "old-user", Role: provider.RoleUser, Content: "old question"})
	session.Add(provider.Message{ID: "old-answer", Role: provider.RoleAssistant, Content: "old answer", ServerSearch: []provider.ServerSearchCall{{ID: "old-search", Raw: json.RawMessage(`{"private":"PRIVATE encrypted"}`)}}})
	f.active = filepath.Join(f.dir, "projection.jsonl")
	if err := session.Save(f.active); err != nil {
		t.Fatal(err)
	}
	if err := f.leases.Rebind(f.active); err != nil {
		t.Fatal(err)
	}
	executor := agent.New(p, tool.NewRegistry(), session, agent.Options{}, event.Discard)
	done := make(chan struct{}, 1)
	ctrl := control.New(control.Options{Runner: executor, Executor: executor, SessionDir: f.dir, SessionPath: f.active, Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnDone {
			done <- struct{}{}
		}
	})})
	f.server.mu.Lock()
	f.server.ctrl = ctrl
	f.server.mu.Unlock()
	f.server.titleProv = nil
	var laterOnce, releaseOnce sync.Once
	t.Cleanup(func() {
		laterOnce.Do(func() { close(p.later) })
		releaseOnce.Do(func() { close(p.release) })
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("owned run did not settle")
		}
		ctrl.Close()
	})
	ctrl.Submit("owned current question")
	select {
	case <-p.ready:
	case <-time.After(30 * time.Second):
		t.Fatal("owned provider did not reach capture")
	}
	path := agent.CanonicalSessionPath(f.active)
	route := "/desktop/session-projection?session=" + url.QueryEscape(path)
	status, body := f.get(t, route)
	if status != 200 {
		t.Fatalf("initial %d %s", status, body)
	}
	var first desktopSessionProjection
	if err := json.Unmarshal([]byte(body), &first); err != nil {
		t.Fatal(err)
	}
	if !first.Initial || !first.ReadOnly || first.SessionPath != path || len(first.History) != 2 || first.History[1].ID != "old-answer" || len(first.UserSuffix) != 1 || first.UserSuffix[0].Content != "owned current question" || !first.Replay.HasMore || len(first.Replay.Events) != 512 || len(first.PageToken) != 32 || first.ActiveTurnID == "" {
		t.Fatalf("initial projection: history=%d users=%d events=%d more=%v token=%d turn=%q", len(first.History), len(first.UserSuffix), len(first.Replay.Events), first.Replay.HasMore, len(first.PageToken), first.ActiveTurnID)
	}
	if strings.Contains(body, "PRIVATE") || strings.Contains(body, `"raw"`) {
		t.Fatal("provider/system metadata escaped projection")
	}
	if len(first.Replay.Events) == 0 || first.Replay.Events[0].Sequence != first.ReplayAfterSequence+1 {
		t.Fatal("initial replay baseline is missing or mixed")
	}
	// Exercise the shared client against the actual Serve token gate and
	// controller, not just a compatible JSON fixture.
	f.server.auth = newAuthGate(config.ServeConfig{AuthMode: "token", Token: "owned-projection-client"})
	authenticated := httptest.NewServer(f.server.Handler())
	t.Cleanup(authenticated.Close)
	client, err := remotecontroller.Connect(context.Background(), context.Background(), authenticated.URL, "owned-projection-client")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	clientFirst, err := client.SessionProjection(context.Background(), path)
	if err != nil || !clientFirst.Initial || !clientFirst.Replay.HasMore || len(clientFirst.History) != 2 || clientFirst.History[1].ID != "old-answer" {
		t.Fatalf("actual shared client capture: %v", err)
	}
	laterOnce.Do(func() { close(p.later) })
	select {
	case <-p.laterReady:
	case <-time.After(5 * time.Second):
		t.Fatal("owned late delta did not arrive")
	}
	pageRoute := fmt.Sprintf("%s&page=%s&after=%d", route, first.PageToken, first.Replay.NextAfterSequence)
	status, body = f.get(t, pageRoute)
	if status != 200 {
		t.Fatalf("page %d %s", status, body)
	}
	var page desktopSessionProjection
	if err := json.Unmarshal([]byte(body), &page); err != nil {
		t.Fatal(err)
	}
	if page.Initial || !page.ReadOnly || page.Replay.HasMore || page.PageToken != "" || len(page.History) != 0 || len(page.UserSuffix) != 0 || len(page.Replay.Events) == 0 || page.Replay.LatestSequence != first.Replay.LatestSequence || page.Replay.NextAfterSequence != first.Replay.LatestSequence || page.ActiveTurnID != first.ActiveTurnID {
		t.Fatalf("fixed page: %+v", page)
	}
	if page.ReplayAfterSequence != first.ReplayAfterSequence {
		t.Fatal("continuation changed initial replay baseline")
	}
	for _, frame := range page.Replay.Events {
		if frame.Sequence > first.Replay.LatestSequence || frame.SessionPath != path || frame.TurnID != first.ActiveTurnID || strings.Contains(frame.Text, "LATER") || (frame.Tool != nil && frame.Tool.ID == "later-search") {
			t.Fatalf("late/foreign event: %+v", frame)
		}
	}
	if repeatedStatus, repeated := f.get(t, pageRoute); repeatedStatus != 200 || repeated != body {
		t.Fatal("lost final response cannot be retried at the same cut")
	}
	clientPage, err := client.SessionProjectionPage(context.Background(), path, clientFirst)
	if err != nil || clientPage.Replay.HasMore || clientPage.Replay.LatestSequence != clientFirst.Replay.LatestSequence || clientPage.Replay.NextAfterSequence != clientFirst.Replay.LatestSequence {
		t.Fatalf("actual shared client page: %v", err)
	}
	for _, frame := range clientPage.Replay.Events {
		if frame.Sequence > clientFirst.Replay.LatestSequence || strings.Contains(frame.Text, "LATER") || (frame.Tool != nil && frame.Tool.ID == "later-search") {
			t.Fatal("actual shared client crossed initial cut")
		}
	}
	if !ctrl.Running() || ctrl.SessionPath() != f.active {
		t.Fatal("snapshot took ownership or stopped turn")
	}
	// Continuations are scoped to both selected path and exact controller.
	f.server.bindMu.Lock()
	entry := f.server.desktopProjectionPages[first.PageToken]
	foreign := entry
	foreign.path = path + "-foreign"
	f.server.desktopProjectionPages[first.PageToken] = foreign
	f.server.bindMu.Unlock()
	if status, _ := f.get(t, pageRoute); status != 409 {
		t.Fatal("foreign path continuation accepted")
	}
	f.server.bindMu.Lock()
	foreign = entry
	foreign.owner = weak.Pointer[control.Controller]{}
	f.server.desktopProjectionPages[first.PageToken] = foreign
	f.server.bindMu.Unlock()
	if status, _ := f.get(t, pageRoute); status != 409 {
		t.Fatal("foreign controller continuation accepted")
	}
	f.server.bindMu.Lock()
	f.server.desktopProjectionPages = make(map[string]desktopProjectionPage)
	for i := 0; i < desktopProjectionPageLimit; i++ {
		f.server.desktopProjectionPages[fmt.Sprintf("%032x", i)] = entry
	}
	f.server.bindMu.Unlock()
	if status, _ := f.get(t, route); status != 429 {
		t.Fatal("continuation bound ignored")
	}
	f.server.bindMu.Lock()
	f.server.desktopProjectionPages = map[string]desktopProjectionPage{first.PageToken: entry}
	f.server.bindMu.Unlock()
	f.server.bindMu.Lock()
	entry = f.server.desktopProjectionPages[first.PageToken]
	entry.expires = time.Now().Add(-time.Second)
	f.server.desktopProjectionPages[first.PageToken] = entry
	f.server.bindMu.Unlock()
	if status, _ := f.get(t, pageRoute); status != 409 {
		t.Fatal("expired continuation accepted")
	}
	if status, _ := f.get(t, route+"&page="+strings.Repeat("0", 32)+"&after=0"); status != 409 {
		t.Fatal("unknown continuation accepted")
	}
}

func TestDesktopSessionProjectionQueryAndTokenGate(t *testing.T) {
	for _, query := range []string{"", "session=x&session=y", "session=x&url=https://example.invalid", "session=x&page=", "session=x&page=" + strings.Repeat("0", 32) + "&after=-1", "session=x&page=" + strings.Repeat("0", 32) + "&after=01", "session=x&page=" + strings.Repeat("0", 32) + "&after=9007199254740992", "session=x&page=" + strings.Repeat("0", 32) + "&after=1&after=2", "session=%ZZ"} {
		if _, err := parseProjectionQuery(httptest.NewRequest("GET", "/desktop/session-projection?"+query, nil)); err == nil {
			t.Fatalf("accepted query %q", query)
		}
	}
	f := newOwnershipFixture(t)
	f.server.auth = newAuthGate(config.ServeConfig{AuthMode: "token", Token: "owned-projection-token"})
	authenticated := httptest.NewServer(f.server.Handler())
	t.Cleanup(authenticated.Close)
	response, err := http.Get(authenticated.URL + "/desktop/session-projection?session=" + url.QueryEscape(agent.CanonicalSessionPath(f.active)))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("projection bypassed auth gate")
	}
}
