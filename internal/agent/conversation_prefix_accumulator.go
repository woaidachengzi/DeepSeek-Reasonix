package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"

	"reasonix/internal/provider"
	"reasonix/internal/sessioncontext"
)

// ConversationPrefixAccumulator incrementally computes the same opaque prefix
// identity as ConversationPrefixTokens, without retaining messages or a token
// per message. It is owned by one reader and must not be copied after use.
// A serialization error invalidates the entire prefix until the owner resets it.
type ConversationPrefixAccumulator struct {
	hash    hash.Hash
	encoder *json.Encoder
	err     error
}

var errConversationPrefixEncoding = errors.New("conversation prefix serialization failed")

// Append reports whether this message participates in conversation identity.
// The caller may request Token only for the display rows that need one.
// It borrows message only until return; callers must not mutate it concurrently.
func (p *ConversationPrefixAccumulator) Append(message *provider.Message) (bool, error) {
	if p.err != nil {
		return false, p.err
	}
	if message == nil {
		p.err = errConversationPrefixEncoding
		p.encoder = nil
		p.hash = nil
		return false, p.err
	}
	if message.Role == provider.RoleSystem || IsPinnedContextRevision(*message) ||
		message.Role == provider.RoleUser && sessioncontext.IsContent(message.Content) {
		return false, nil
	}
	if p.hash == nil {
		p.hash = sha256.New()
		p.encoder = json.NewEncoder(p.hash)
	}
	// Default Encode preserves Marshal's escaping and appends the same newline.
	// Borrowing an addressable message avoids boxing a full Message value, and
	// writing to the hash avoids Marshal's returned byte-slice copy.
	if err := p.encoder.Encode(message); err != nil {
		// An encoder error may retain a reflect.Value into the caller's data.
		// Keep only a static failure marker in this long-lived accumulator.
		p.err = errConversationPrefixEncoding
		p.encoder = nil
		p.hash = nil
		return false, err
	}
	return true, nil
}

func (p *ConversationPrefixAccumulator) Token() string {
	if p.err != nil || p.hash == nil {
		return ""
	}
	return hex.EncodeToString(p.hash.Sum(nil))
}
