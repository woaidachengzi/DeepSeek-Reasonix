package boot

import "reasonix/internal/config"

func appendCorePolicies(prompt string) string {
	for _, policy := range []string{config.UserDecisionPolicy, config.WorkPracticePolicy, config.LanguagePolicy, config.DesktopControlPolicy} {
		prompt += "\n\n" + policy
	}
	return prompt
}

func appendOfflineEnvironmentNote(prompt string, offline bool) string {
	if offline {
		prompt += "\n\n" + config.OfflineEnvironmentNote
	}
	return prompt
}
