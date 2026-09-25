package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSessionCatalogEndpointsReturnProtocolErrorForMalformedJSON(t *testing.T) {
	for _, endpoint := range []struct {
		name string
		path string
	}{
		{name: "import", path: "/v1/sessions/import-catalog"},
		{name: "sync", path: "/v1/sessions/sync-catalog"},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			handler := newBridgeServer(testToken, "instance", nil).handler()
			request := httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(`{"sessions":[`))
			request.Header.Set("Authorization", "Bearer "+testToken)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			var body struct {
				ProtocolVersion int `json:"protocolVersion"`
				Error           struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if response.Code != http.StatusBadRequest || json.Unmarshal(response.Body.Bytes(), &body) != nil ||
				body.ProtocolVersion != 1 || body.Error.Code != "invalid_request" {
				t.Fatalf("malformed request response: status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
