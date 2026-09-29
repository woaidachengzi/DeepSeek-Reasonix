package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"reasonix/internal/boot"
	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/netclient"
	"reasonix/internal/provider"
)

type providerProbeRequest struct {
	Name   string `json:"name"`
	Model  string `json:"model"`
	APIKey string `json:"apiKey,omitempty"`
}

type providerProbeResponse struct {
	ProtocolVersion int   `json:"protocolVersion"`
	LatencyMillis   int64 `json:"latencyMillis"`
}

var errProviderProbeUnavailable = errors.New("provider or model is unavailable")
var errProviderProbeFailed = errors.New("provider probe failed; check the saved endpoint, model, and credentials")

func (b *bridgeServer) testProviderModel(w http.ResponseWriter, r *http.Request) {
	var input providerProbeRequest
	if err := decodeJSONBody(w, r, 64<<10, &input); err != nil {
		writeProtocolError(w, 400, "invalid_request", "invalid provider probe request")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Model = strings.TrimSpace(input.Model)
	if !previewProviderName.MatchString(input.Name) || input.Model == "" || len(input.Model) > 256 || len(input.APIKey) > 16<<10 {
		writeProtocolError(w, 400, "invalid_request", "invalid provider probe request")
		return
	}

	unlock := configpkg.LockUserConfigEdits()
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		unlock()
		writeProtocolError(w, 500, "internal", "unable to load provider settings")
		return
	}
	entry, found := cfg.Provider(input.Name)
	if !found || !providerAccessAllowed(cfg.Desktop.ProviderAccess, input.Name) {
		unlock()
		writeProtocolError(w, 400, "invalid_request", errProviderProbeUnavailable.Error())
		return
	}
	proxy := cfg.NetworkProxySpec()
	unlock()

	started := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := runProviderProbeWithProxy(ctx, entry, input.Model, input.APIKey, proxy); err != nil {
		if errors.Is(err, errProviderProbeUnavailable) {
			writeProtocolError(w, 400, "invalid_request", errProviderProbeUnavailable.Error())
		} else {
			writeProtocolError(w, 502, "provider_probe_failed", errProviderProbeFailed.Error())
		}
		return
	}
	writeJSON(w, 200, providerProbeResponse{ProtocolVersion: desktopbridge.ProtocolVersion, LatencyMillis: time.Since(started).Milliseconds()})
}

func runProviderProbeWithProxy(ctx context.Context, entry *configpkg.ProviderEntry, model, transientKey string, proxy netclient.ProxySpec) error {
	if entry == nil || strings.TrimSpace(model) == "" {
		return errProviderProbeUnavailable
	}
	listed := false
	for _, candidate := range entry.ModelList() {
		if candidate == strings.TrimSpace(model) {
			listed = true
			break
		}
	}
	if !listed {
		return errProviderProbeUnavailable
	}
	resolved := *entry
	resolved.Model = strings.TrimSpace(model)
	resolved.ResolveAPIKeyForRoot(".")
	if strings.TrimSpace(transientKey) != "" {
		resolved = resolved.WithAPIKeyForProbe(transientKey)
	}
	if !resolved.Configured() {
		return errProviderProbeUnavailable
	}
	client, err := boot.NewProviderWithProxy(&resolved, proxy)
	if err != nil {
		return errProviderProbeFailed
	}
	chunks, err := client.Stream(ctx, provider.Request{Messages: []provider.Message{{Role: "user", Content: "Reply with OK."}}, MaxTokens: 16})
	if err != nil {
		return errProviderProbeFailed
	}
	for {
		select {
		case <-ctx.Done():
			return errProviderProbeFailed
		case chunk, open := <-chunks:
			if !open || chunk.Err != nil {
				return errProviderProbeFailed
			}
			if chunk.Type == provider.ChunkText && strings.TrimSpace(chunk.Text) != "" || chunk.Type == provider.ChunkDone {
				return nil
			}
		}
	}
}
