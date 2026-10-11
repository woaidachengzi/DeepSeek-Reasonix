package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"reasonix/internal/provider"
	"reasonix/internal/sessioncontext"
)

// ConversationPrefixTokens identifies each exact selected transcript prefix
// without exposing provider-facing messages to the desktop. Appending a turn
// preserves earlier tokens; changing an earlier message invalidates them.
func ConversationPrefixTokens(messages []provider.Message) ([]string, error) {
	hash := sha256.New()
	tokens := make([]string, len(messages))
	for index, message := range messages {
		// Host prompts can be refreshed by model/effort settings without changing
		// conversation identity. They remain in the copied prefix but do not
		// invalidate actions on already completed answers.
		if message.Role == provider.RoleSystem || IsPinnedContextRevision(message) ||
			message.Role == provider.RoleUser && sessioncontext.IsContent(message.Content) {
			tokens[index] = ""
			continue
		}
		encoded, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}
		hash.Write(encoded)
		hash.Write([]byte{'\n'})
		tokens[index] = hex.EncodeToString(hash.Sum(nil))
	}
	return tokens, nil
}
