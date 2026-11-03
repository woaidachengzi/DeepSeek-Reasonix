package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"

	"reasonix/internal/provider"
	"reasonix/internal/sessioncontext"
)

// ConversationPrefixAccumulator incrementally computes the same opaque prefix
// identity as ConversationPrefixTokens, without retaining messages or a token
// per message. It is owned by one reader and must not be copied after use.
// A serialization error invalidates the entire prefix until the owner resets it.
type ConversationPrefixAccumulator struct {
	hash hash.Hash
	err  error
}

// Append reports whether this message participates in conversation identity.
// The caller may request Token only for the display rows that need one.
func (p *ConversationPrefixAccumulator) Append(message provider.Message) (bool, error) {
	if p.err != nil {
		return false, p.err
	}
	if message.Role == provider.RoleSystem || IsPinnedContextRevision(message) ||
		message.Role == provider.RoleUser && sessioncontext.IsContent(message.Content) {
		return false, nil
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		p.err = err
		return false, err
	}
	if p.hash == nil {
		p.hash = sha256.New()
	}
	p.hash.Write(encoded)
	p.hash.Write([]byte{'\n'})
	return true, nil
}

func (p *ConversationPrefixAccumulator) Token() string {
	if p.err != nil || p.hash == nil {
		return ""
	}
	return hex.EncodeToString(p.hash.Sum(nil))
}
