package config

import "strings"

// Add newly published MiMo models to an unchanged installed preset catalog.
// Reading old configuration must not replace a user-curated list, route,
// credential, or selected default. The current preset seeds new installations.
func normalizeLegacyMimoV26Catalog(c *Config) bool {
	if c == nil {
		return false
	}
	changed := false
	for i := range c.Providers {
		p := &c.Providers[i]
		id := strings.TrimSpace(p.PresetID)
		if id == "" {
			id = strings.TrimSpace(p.Name)
		}
		if id != "mimo-api" && id != "mimo-anthropic" && !strings.HasPrefix(id, "mimo-token-plan-") {
			continue
		}
		preset, ok := CuratedProviderPreset(id)
		if !ok || len(preset.Entries) != 1 {
			continue
		}
		canonical := preset.Entries[0]
		if !strings.EqualFold(strings.TrimSpace(p.Kind), canonical.Kind) ||
			normalizedBaseURLForMigration(p.BaseURL) != canonical.BaseURL ||
			strings.TrimSpace(p.RequestURL) != "" || strings.TrimSpace(p.ChatURL) != "" ||
			strings.TrimSpace(p.Model) != "" || !stringSlicesEqual(p.Models, legacyMimoV25Models) {
			continue
		}
		p.Models = append([]string(nil), mimoModels...)
		if p.VisionModels == nil || stringSlicesEqual(p.VisionModels, legacyMimoV25VisionModels) {
			p.VisionModels = append([]string(nil), mimoCatalogVisionModels...)
		}
		// A provider-wide custom price remains the fallback for new models.
		if p.Price == nil {
			if p.Prices == nil {
				p.Prices = mimoDomesticPrices(mimoModels)
			} else {
				for model, price := range mimoDomesticPrices(mimoModels[:2]) {
					if _, exists := p.Prices[model]; !exists {
						p.Prices[model] = price
					}
				}
			}
		}
		changed = true
	}
	return changed
}
