package mcpserver

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/benice2me11/codexify-go/internal/projects"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const setupContextMetaKey = "io.github.devnoname120/codexify/setup-context"
const setupContextLifetime = 30 * time.Minute
const setupContextLimit = 1024

var errSetupContext = errors.New("This workspace card has no valid conversation context. Open a fresh project list in this conversation.")

type setupContextEntry struct {
	session     string
	identityKey string
	expires     time.Time
}

// Contexts are opaque routing capabilities on the authenticated MCP connection,
// not authentication credentials. They never leave memory except in widget-only
// result metadata. They cannot be used with file or execution tools.
type setupContextStore struct {
	mu         sync.Mutex
	entries    map[string]setupContextEntry
	byIdentity map[string]string
	now        func() time.Time
}

func (s *setupContextStore) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *setupContextStore) prune(now time.Time) {
	for token, entry := range s.entries {
		if !now.Before(entry.expires) {
			delete(s.entries, token)
			delete(s.byIdentity, entry.identityKey)
		}
	}
}

func (s *setupContextStore) issue(meta map[string]any) (string, error) {
	identity := projects.IdentityFromMeta(meta)
	if identity == nil || !identity.Persistent {
		return "", nil
	}
	session, _ := meta["openai/session"].(string)
	session = strings.TrimSpace(session)
	if session == "" {
		return "", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock()
	s.prune(now)
	if token := s.byIdentity[identity.Key]; token != "" {
		return token, nil
	}
	if len(s.entries) >= setupContextLimit {
		return "", errors.New("Too many active workspace cards; wait for an older card to expire.")
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", errors.New("Could not create workspace card context.")
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	if s.entries == nil {
		s.entries = make(map[string]setupContextEntry)
		s.byIdentity = make(map[string]string)
	}
	s.entries[token] = setupContextEntry{session: session, identityKey: identity.Key, expires: now.Add(setupContextLifetime)}
	s.byIdentity[identity.Key] = token
	return token, nil
}

func (s *setupContextStore) resolve(token string, observed map[string]any) (map[string]any, error) {
	if len(token) != 43 {
		return nil, errSetupContext
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[token]
	if !ok || !s.clock().Before(entry.expires) {
		return nil, errSetupContext
	}
	if current := projects.IdentityFromMeta(observed); current != nil && current.Persistent && current.Key != entry.identityKey {
		return nil, errSetupContext
	}
	// This is the original server-observed identity, never a client-provided
	// thread UUID or a guessed conversation identifier.
	return map[string]any{"openai/session": entry.session}, nil
}

func setupContextTool(name string) bool {
	switch name {
	case "list_projects", "setup_status", "set_project_root", "setup_ui_switch_project":
		return true
	default:
		return false
	}
}

func setupContextError(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}
}

func (r *Runtime) setupContextMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			call, ok := request.(*mcp.CallToolRequest)
			if method != "tools/call" || !ok || call.Params == nil {
				return next(ctx, method, request)
			}
			var args map[string]json.RawMessage
			if err := json.Unmarshal(call.Params.Arguments, &args); err != nil {
				return next(ctx, method, request)
			}
			raw, present := args["uiContext"]
			allowed := setupContextTool(call.Params.Name)
			if present && !allowed {
				return setupContextError(errSetupContext), nil
			}
			if !allowed {
				return next(ctx, method, request)
			}
			if present {
				var token string
				if json.Unmarshal(raw, &token) != nil || token == "" {
					return setupContextError(errSetupContext), nil
				}
				verified, err := r.setupContexts.resolve(token, r.requestMeta(call))
				if err != nil {
					return setupContextError(err), nil
				}
				// No wire metadata is changed. Only this request can see the verified
				// routing context, including its existing diff/workspace ownership checks.
				r.setupRequests.Store(call, verified)
				defer r.setupRequests.Delete(call)
			}
			token, err := r.setupContexts.issue(r.requestMeta(call))
			if err != nil {
				return setupContextError(err), nil
			}
			result, err := next(ctx, method, request)
			if err == nil && token != "" {
				if output, ok := result.(*mcp.CallToolResult); ok && output != nil {
					if output.Meta == nil {
						output.Meta = mcp.Meta{}
					}
					output.Meta[setupContextMetaKey] = token
				}
			}
			return result, err
		}
	}
}
