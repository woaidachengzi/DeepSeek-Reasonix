package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScanImportRoutesRequireBridgeAuthentication(t *testing.T) {
	handler := newBridgeServer(testToken, "scan-import-test").handler()
	for _, path := range []string{
		"/v1/sessions/scan-import-candidates",
		"/v1/sessions/import-scan",
	} {
		request := httptest.NewRequest(http.MethodPost, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("unauthenticated %s status = %d, want %d", path, response.Code, http.StatusUnauthorized)
		}
	}
}
