package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteHostNameBoundary(t *testing.T) {
	for _, name := range []string{"172.21.0.5", "dev.example.com", "gpu-1_dev", "开发机", strings.Repeat("a", 64)} {
		if !validRemoteHostName(name) {
			t.Errorf("valid saved SSH name rejected: %q", name)
		}
	}
	for _, name := range []string{"", "host name", "host\tname", "host\nname", "host\x00name", "host\x7fname", "user@host", "host/name", "host:22", "host\u00a0name", strings.Repeat("a", 65), string([]byte{0xff})} {
		if validRemoteHostName(name) {
			t.Errorf("invalid SSH name accepted: %q", name)
		}
	}
}

func TestDottedRemoteNamePassesAllOperationGuards(t *testing.T) {
	bridge := newBridgeServer(testToken, "dotted-host-test")
	t.Cleanup(bridge.remoteSessions.closeAll)
	for _, endpoint := range []string{"browse", "preview", "save", "paths", "forwards", "serve", "controller"} {
		t.Run(endpoint, func(t *testing.T) {
			// No live connection: a valid request must reach connection lookup
			// (409), rather than reject the saved IP-shaped identifier (400).
			input := map[string]any{"name": "172.21.0.5"}
			switch endpoint {
			case "browse", "preview":
				input["path"] = "/disk/mdm"
			case "save":
				input["path"], input["revision"], input["content"] = "/disk/mdm/a.txt", strings.Repeat("a", 64), "updated"
			case "paths":
				input["action"], input["path"] = "mkdir", "/disk/mdm/new"
			case "forwards":
				input["action"] = "list"
			case "serve", "controller":
				input["workspace"] = "/disk/mdm"
			}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("POST", "/v1/settings/remote/"+endpoint, strings.NewReader(string(body)))
			res := httptest.NewRecorder()
			switch endpoint {
			case "browse":
				bridge.browseRemoteHost(res, req)
			case "preview":
				bridge.previewRemoteFile(res, req)
			case "save":
				bridge.saveRemoteFile(res, req)
			case "paths":
				bridge.changeRemotePath(res, req)
			case "forwards":
				bridge.remoteForwards(res, req)
			case "serve":
				bridge.remoteServeConnectedClient(res, req)
			case "controller":
				bridge.attachRemoteController(res, req)
			}
			if res.Code != 409 {
				t.Fatalf("dotted host %s = %d, want connection lookup conflict: %s", endpoint, res.Code, res.Body.String())
			}
		})
	}
}
