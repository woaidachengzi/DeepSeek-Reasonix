package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/desktopbridge"
)

func TestBridgeServerConversationRewindRoutesAreScopedAndDeduplicated(t *testing.T) {
	runtime := &bridgeTestRuntime{path: "/tmp/reasonix-session", state: "idle"}
	manager := desktopbridge.NewRuntimeManager(desktopbridge.RuntimeFactoryFunc(func(context.Context, desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		return runtime, nil
	}))
	handler := newBridgeServer(testToken, "instance", manager).handler()
	call := func(path, body, requestID string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+testToken)
		request.Header.Set("Content-Type", "application/json")
		if requestID != "" {
			request.Header.Set(requestIDHeader, requestID)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	if response := call("/v1/sessions:open", `{"sessionId":"tab-conversation"}`, ""); response.Code != http.StatusOK {
		t.Fatalf("open status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call("/v1/sessions/other:conversation-rewind-preview", `{"turn":1}`, ""); response.Code != http.StatusNotFound {
		t.Fatalf("other session preview status=%d", response.Code)
	}
	if response := call("/v1/sessions/tab-conversation:conversation-rewind-preview", `{"turn":1}`, ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"planId":"plan-conversation"`) {
		t.Fatalf("preview status=%d body=%s", response.Code, response.Body.String())
	}
	commitPath := "/v1/sessions/tab-conversation:conversation-rewind-commit"
	if response := call(commitPath, `{"planId":"plan-conversation"}`, ""); response.Code != http.StatusBadRequest || runtime.conversationRewindCommits != 0 {
		t.Fatalf("commit without request ID: status=%d calls=%d", response.Code, runtime.conversationRewindCommits)
	}
	first, second := call(commitPath, `{"planId":"plan-conversation"}`, "conversation-1"), call(commitPath, `{"planId":"plan-conversation"}`, "conversation-1")
	if first.Code != http.StatusOK || second.Code != http.StatusOK || first.Body.String() != second.Body.String() || runtime.conversationRewindCommits != 1 {
		t.Fatalf("idempotent conversation commit: first=%d second=%d calls=%d", first.Code, second.Code, runtime.conversationRewindCommits)
	}
	undoPath := "/v1/sessions/tab-conversation:conversation-rewind-undo"
	first, second = call(undoPath, `{"headId":"rewind-head"}`, "conversation-undo-1"), call(undoPath, `{"headId":"rewind-head"}`, "conversation-undo-1")
	if first.Code != http.StatusOK || second.Code != http.StatusOK || first.Body.String() != second.Body.String() || runtime.conversationRewindUndos != 1 {
		t.Fatalf("idempotent conversation undo: first=%d second=%d calls=%d", first.Code, second.Code, runtime.conversationRewindUndos)
	}
	list := httptest.NewRequest(http.MethodPost, "/v1/sessions/tab-conversation:session-heads", nil)
	list.Header.Set("Authorization", "Bearer "+testToken)
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"id":"rewind-head"`) {
		t.Fatalf("head list status=%d body=%s", listed.Code, listed.Body.String())
	}
	switchPath := "/v1/sessions/tab-conversation:session-head-switch"
	if response := call(switchPath, `{"headId":"rewind-head"}`, ""); response.Code != http.StatusBadRequest {
		t.Fatalf("switch without request ID: status=%d", response.Code)
	}
	first, second = call(switchPath, `{"headId":"rewind-head"}`, "head-switch-1"), call(switchPath, `{"headId":"rewind-head"}`, "head-switch-1")
	if first.Code != http.StatusOK || second.Code != http.StatusOK || runtime.sessionHeadSwitches != 1 {
		t.Fatalf("idempotent head switch: first=%d second=%d calls=%d", first.Code, second.Code, runtime.sessionHeadSwitches)
	}
	if response := call("/v1/sessions/other:session-head-switch", `{"headId":"main"}`, "head-switch-2"); response.Code != http.StatusNotFound {
		t.Fatalf("other session switch status=%d", response.Code)
	}
	if response := call("/v1/sessions/other:combined-rewind-preview", `{"turn":1}`, ""); response.Code != http.StatusNotFound {
		t.Fatalf("other session combined preview status=%d", response.Code)
	}
	if response := call("/v1/sessions/tab-conversation:combined-rewind-preview", `{"turn":1}`, ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"planId":"plan-both"`) {
		t.Fatalf("combined preview status=%d body=%s", response.Code, response.Body.String())
	}
	combinedPath := "/v1/sessions/tab-conversation:combined-rewind-commit"
	if response := call(combinedPath, `{"planId":"plan-both","confirmPartialCoverage":true}`, ""); response.Code != http.StatusBadRequest {
		t.Fatalf("combined commit without request ID status=%d", response.Code)
	}
	first, second = call(combinedPath, `{"planId":"plan-both","confirmPartialCoverage":true}`, "both-1"), call(combinedPath, `{"planId":"plan-both","confirmPartialCoverage":true}`, "both-1")
	if first.Code != http.StatusOK || second.Code != http.StatusOK || first.Body.String() != second.Body.String() || runtime.combinedRewindCommits != 1 || !runtime.combinedConfirmed {
		t.Fatalf("combined commit dedupe: first=%d second=%d calls=%d confirmed=%v", first.Code, second.Code, runtime.combinedRewindCommits, runtime.combinedConfirmed)
	}
}
