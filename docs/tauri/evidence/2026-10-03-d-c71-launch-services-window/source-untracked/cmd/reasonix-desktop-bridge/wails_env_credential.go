package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/joho/godotenv"
	appconfig "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	fileencoding "reasonix/internal/fileutil/encoding"
)

// This is a native-host-only authenticated transport. It never resolves shell
// or project env, never accepts a source path/account, and never edits originals.
func (b *bridgeServer) wailsEnvCredential(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	provider := r.URL.Query().Get("provider")
	value, err := readWailsEnvCredential(provider)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "Wails global credential is unavailable; check the original provider configuration")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		ProtocolVersion int     `json:"protocolVersion"`
		Value           *string `json:"value"`
	}{desktopbridge.ProtocolVersion, value})
}

func readWailsEnvCredential(name string) (*string, error) {
	unavailable := errors.New("Wails global credential unavailable")
	if name == "" || len(name) > 256 || strings.TrimSpace(name) != name {
		return nil, unavailable
	}
	current, err := appconfig.LoadUserConfigReadOnly()
	if err != nil {
		return nil, unavailable
	}
	var account string
	var selected *appconfig.ProviderEntry
	for _, provider := range current.Providers {
		if provider.Name == name && provider.RequiresAPIKey() && appconfig.IsValidCredentialKey(provider.APIKeyEnv) {
			account = provider.APIKeyEnv
			selected = &provider
			break
		}
	}
	if account == "" {
		return nil, unavailable
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return nil, unavailable
	}
	root := filepath.Join(home, ".reasonix")
	raw, err := readBoundedCredentialSource(filepath.Join(root, "config.toml"), 16<<20)
	if err != nil {
		return nil, unavailable
	}
	original, err := appconfig.LoadUserConfigBytesReadOnly(raw)
	if err != nil {
		return nil, unavailable
	}
	matched := false
	for _, provider := range original.Providers {
		if provider.Name == name && provider.RequiresAPIKey() && provider.APIKeyEnv == account && sameCredentialDestination(selected, &provider) {
			matched = true
			break
		}
	}
	if !matched {
		return nil, unavailable
	}
	raw, err = readBoundedCredentialSource(filepath.Join(root, ".env"), 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, unavailable
	}
	values, err := godotenv.Unmarshal(string(fileencoding.DecodeToUTF8(raw)))
	if err != nil {
		return nil, unavailable
	}
	value := values[account]
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	if len(value) > 32768 || strings.ContainsAny(value, "\x00\r\n") {
		return nil, unavailable
	}
	return &value, nil
}

// Inspect the opened handle before reading: a path swap cannot substitute a
// different file between lstat/open. No symlink, special file or unbounded read.
func readBoundedCredentialSource(path string, limit int64) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > limit {
		return nil, errors.New("invalid credential source")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, errors.New("credential source changed")
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, errors.New("invalid credential source")
	}
	return raw, nil
}

// Do not carry an original key into a provider whose network/auth destination
// was changed. Model lists and presentation labels may evolve independently.
func sameCredentialDestination(a, b *appconfig.ProviderEntry) bool {
	return a.Kind == b.Kind && strings.TrimRight(a.BaseURL, "/") == strings.TrimRight(b.BaseURL, "/") &&
		a.ChatURL == b.ChatURL && a.RequestURL == b.RequestURL && a.ModelsURL == b.ModelsURL &&
		a.BalanceURL == b.BalanceURL && a.AuthHeader == b.AuthHeader && reflect.DeepEqual(a.Headers, b.Headers)
}
