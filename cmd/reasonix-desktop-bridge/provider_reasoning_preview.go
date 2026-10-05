package main

import (
	"net/http"
	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
)

// Resolve unsaved model/protocol choices using the runtime adapter, without
// sending a provider request, resolving credentials, or writing configuration.
func previewProviderReasoning(input saveProviderConfigRequest) ([]providerModelReasoning, error) {
	if !previewProviderName.MatchString(input.Name) {
		input.Name = "draft"
	}
	if err := validateProviderConfigInput(&input); err != nil {
		return nil, err
	}
	cfg, err := configpkg.LoadUserConfigReadOnly()
	if err != nil {
		return nil, err
	}
	entry := configpkg.ProviderEntry{}
	if saved, ok := cfg.Provider(input.Name); ok {
		entry = *saved
	}
	entry.Name, entry.Kind = input.Name, input.Kind
	if input.BaseURL != "" {
		entry.BaseURL, entry.RequestURL, entry.ChatURL = input.BaseURL, "", ""
	}
	entry.Model, entry.Models, entry.Default = input.Models[0], input.Models, input.Default
	overrides := make(map[string]configpkg.ProviderModelOverride, len(entry.ModelOverrides))
	for model, value := range entry.ModelOverrides {
		overrides[model] = value
	}
	entry.ModelOverrides = overrides
	if input.ModelReasoning != nil {
		for _, item := range *input.ModelReasoning {
			ov := overrides[item.Model]
			ov.ReasoningProtocol, ov.SupportedEfforts, ov.DefaultEffort = item.ReasoningProtocol, append([]string(nil), item.SupportedEfforts...), item.DefaultEffort
			overrides[item.Model] = ov
		}
	}
	return providerReasoningForConfig(entry), nil
}

func (b *bridgeServer) previewProviderReasoning(w http.ResponseWriter, r *http.Request) {
	var input saveProviderConfigRequest
	if err := decodeJSONBody(w, r, 64<<10, &input); err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "invalid reasoning preview request")
		return
	}
	// Endpoint errors are already validated; avoid returning load diagnostics.
	result, err := previewProviderReasoning(input)
	if err != nil {
		writeProtocolError(w, http.StatusBadRequest, "invalid_request", "unable to read reasoning levels; check the model and reasoning settings")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		ProtocolVersion int                      `json:"protocolVersion"`
		ModelReasoning  []providerModelReasoning `json:"modelReasoning"`
	}{desktopbridge.ProtocolVersion, result})
}
