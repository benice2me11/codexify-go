package projects

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	openAISessionMetaKey    = "openai/session"
	transportSessionMetaKey = "codexify-go/transport-session"
)

type Identity struct {
	Key        string
	LegacyKey  string
	Scope      string
	Persistent bool
}

func IdentityFromMeta(meta map[string]any) *Identity {
	if meta == nil {
		return nil
	}
	if raw, ok := meta[openAISessionMetaKey].(string); ok {
		raw = strings.TrimSpace(raw)
		if raw != "" {
			identity := hashedIdentity("codexify-go/openai-session/v1\x00", raw, "chatgpt_conversation", true)
			identity.LegacyKey = hashedIdentity("codexify/openai-session/v1\x00", raw, "chatgpt_conversation", true).Key
			return identity
		}
	}
	if raw, ok := meta[transportSessionMetaKey].(string); ok {
		raw = strings.TrimSpace(raw)
		if raw != "" {
			return hashedIdentity("codexify-go/transport-session/v1\x00", raw, "transport_session", false)
		}
	}
	return nil
}

func IdentityFromTransportSession(sessionID string) *Identity {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	return hashedIdentity("codexify-go/transport-session/v1\x00", sessionID, "transport_session", false)
}

func WithTransportSession(meta map[string]any, sessionID string) map[string]any {
	if strings.TrimSpace(sessionID) == "" {
		return meta
	}
	out := make(map[string]any, len(meta)+1)
	for key, value := range meta {
		out[key] = value
	}
	if raw, ok := out[openAISessionMetaKey].(string); ok && strings.TrimSpace(raw) != "" {
		return out
	}
	out[transportSessionMetaKey] = sessionID
	return out
}

func hashedIdentity(prefix, raw, scope string, persistent bool) *Identity {
	sum := sha256.Sum256([]byte(prefix + raw))
	return &Identity{
		Key:        hex.EncodeToString(sum[:]),
		Scope:      scope,
		Persistent: persistent,
	}
}

func (i *Identity) Short() string {
	if i == nil || len(i.Key) < 12 {
		return ""
	}
	return i.Key[:12]
}
