package provider

import "strings"

const DeepSeekFlashModel = "deepseek-flash"

// IsDeepSeekFlashModel lists the official Flash name and its retained aliases.
// Gateway-specific IDs are deliberately excluded from the direct API catalog.
func IsDeepSeekFlashModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case DeepSeekFlashModel, "deepseek-v4-flash", "deepseek-v4-flash-vision-exp":
		return true
	}
	return false
}

func IsDeepSeekReasoningModel(model string) bool {
	return IsDeepSeekFlashModel(model) || strings.EqualFold(strings.TrimSpace(model), "deepseek-v4-pro") ||
		strings.EqualFold(strings.TrimSpace(model), "deepseek-v4.1-flash")
}
