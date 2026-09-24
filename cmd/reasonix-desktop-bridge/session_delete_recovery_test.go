package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/sessionidentity"
)

func TestBridgeRetriesInterruptedDeleteAfterRestart(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	factory := desktopbridge.RuntimeFactoryFunc(func(ctx context.Context, request desktopbridge.OpenRequest) (desktopbridge.Runtime, error) {
		runtime, err := newControllerFactory(nil).Open(ctx, request)
		if err != nil {
			return nil, err
		}
		runtime.(*controllerRuntime).removeArtifacts = func(string) error { return errors.New("interrupted sweep") }
		return runtime, nil
	})
	manager := desktopbridge.NewRuntimeManager(factory)
	view, err := manager.Open(ctx, desktopbridge.OpenRequest{SessionID: "retry-after-restart", WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(view.Path, []byte("transcript left by interrupted deletion\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.DeleteSession(view.ID); err == nil {
		t.Fatal("injected cleanup failure was ignored")
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}

	restarted := newBridgeServer(testToken, "restart", desktopbridge.NewRuntimeManager(newControllerFactory(nil)))
	request := httptest.NewRequest(http.MethodDelete, "/v1/sessions/"+view.ID, nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set(requestIDHeader, "retry-after-restart-delete")
	response := httptest.NewRecorder()
	restarted.handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("restarted delete status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, err := os.Lstat(view.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("interrupted transcript survived recovery: %v", err)
	}
	identities, err := sessionidentity.OpenReadOnly(ctx, appconfig.DesktopSessionIdentityPath())
	if err != nil {
		t.Fatal(err)
	}
	defer identities.Close()
	record, exists, err := identities.Get(ctx, view.ID)
	if err != nil || !exists || record.State != sessionidentity.StateDeleted {
		t.Fatalf("recovered identity = %#v, %v, %v", record, exists, err)
	}
}

func TestBridgeRecoveryCannotDeleteUnownedNormalSession(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REASONIX_HOME", root)
	t.Setenv("REASONIX_STATE_HOME", root)
	ctx := context.Background()
	manager := desktopbridge.NewRuntimeManager(newControllerFactory(nil))
	view, err := manager.Open(ctx, desktopbridge.OpenRequest{SessionID: "normal-unowned", WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(view.Path, []byte("must survive\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted := newBridgeServer(testToken, "restart", desktopbridge.NewRuntimeManager(newControllerFactory(nil)))
	request := httptest.NewRequest(http.MethodDelete, "/v1/sessions/"+view.ID, nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set(requestIDHeader, "normal-unowned-delete")
	response := httptest.NewRecorder()
	restarted.handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unowned delete status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(view.Path); err != nil {
		t.Fatalf("unowned transcript was removed: %v", err)
	}
}
