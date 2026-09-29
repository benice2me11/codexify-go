package connectorschema

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/benice2me11/codexify-go/internal/buildinfo"
	"github.com/benice2me11/codexify-go/internal/config"
)

var BaseVersion = buildinfo.Version

type Store struct {
	dir string
	mu  sync.Mutex
}

type HistoryEntry struct {
	Version    string `json:"version"`
	ObservedAt string `json:"observedAt"`
	Source     string `json:"source"`
}

func Version(cfg config.Config) string {
	version := BaseVersion
	if cfg.AgentChat.Enabled {
		version += "+markdown-chat-v5"
	}
	if cfg.Experimental.AgentTickets {
		version += "+tickets-v1"
	}
	if cfg.MCP.MultiProject {
		version += "+workspace-v1"
	}
	if cfg.ArtifactIngress.Enabled {
		version += "+artifact-ingress-v1"
	}
	if cfg.MCP.Upstreams != nil {
		for _, upstream := range cfg.MCP.Upstreams {
			if strings.EqualFold(strings.TrimSpace(upstream.Mode), "gateway") {
				version += "+gateway-v1"
				break
			}
		}
	}
	if len(version) > 64 {
		sum := sha256.Sum256([]byte(version))
		version = BaseVersion + "+" + hex.EncodeToString(sum[:6])
	}
	return version
}

func NewForTunnel(tunnelID string) (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, errors.New("cannot resolve user home for connector schema state")
	}
	sum := sha256.Sum256([]byte("codexify-go/connector-schema/tunnel/v1\x00" + tunnelID))
	scope := hex.EncodeToString(sum[:16])
	return &Store{dir: filepath.Join(home, ".codexify-go", "connector-schemas", scope)}, nil
}

func (s *Store) RecordConnector(version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeWithHistoryUnlocked("connector", version, "runtime", false)
}

func (s *Store) ConnectorVersion() string {
	return s.read("connector")
}

func (s *Store) ConversationVersion(identityHash string) string {
	if strings.TrimSpace(identityHash) == "" {
		return ""
	}
	return s.read("conversation-" + identityHash)
}

func (s *Store) RememberConversationVersion(identityHash, version string) error {
	if strings.TrimSpace(identityHash) == "" || strings.TrimSpace(version) == "" {
		return nil
	}
	key := "conversation-" + identityHash
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.readUnlocked(key); existing != "" {
		_ = s.ensureLegacyHistoryUnlocked(key, existing)
		return nil
	}
	return s.writeWithHistoryUnlocked(key, version, "conversation", true)
}

func (s *Store) History(key string) []HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.readUnlocked(key)
	if current != "" {
		_ = s.ensureLegacyHistoryUnlocked(key, current)
	}
	return s.readHistoryUnlocked(key)
}

func (s *Store) write(key, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeUnlocked(key, version)
}

func (s *Store) writeUnlocked(key, version string) error {
	if strings.TrimSpace(version) == "" || len(version) > 64 {
		return errors.New("connector schema version is empty or too long")
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, key+".tmp")
	if err := os.WriteFile(tmp, []byte(version), 0o600); err != nil {
		return err
	}
	target := filepath.Join(s.dir, key)
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (s *Store) writeWithHistoryUnlocked(key, version, source string, firstOnly bool) error {
	version = strings.TrimSpace(version)
	if version == "" || len(version) > 64 {
		return errors.New("connector schema version is empty or too long")
	}
	existing := s.readUnlocked(key)
	if existing != "" {
		if err := s.ensureLegacyHistoryUnlocked(key, existing); err != nil {
			return err
		}
		if firstOnly || existing == version {
			return nil
		}
	}
	if err := s.writeUnlocked(key, version); err != nil {
		return err
	}
	return s.appendHistoryUnlocked(key, HistoryEntry{Version: version, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Source: source})
}

func (s *Store) ensureLegacyHistoryUnlocked(key, version string) error {
	if len(s.readHistoryUnlocked(key)) > 0 {
		return nil
	}
	return s.appendHistoryUnlocked(key, HistoryEntry{Version: version, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Source: "legacy_plain"})
}

func (s *Store) historyPath(key string) string {
	return filepath.Join(s.dir, key+".history.jsonl")
}

func (s *Store) readHistoryUnlocked(key string) []HistoryEntry {
	data, err := os.ReadFile(s.historyPath(key))
	if err != nil {
		return nil
	}
	var entries []HistoryEntry
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry HistoryEntry
		if json.Unmarshal([]byte(line), &entry) == nil && entry.Version != "" {
			entries = append(entries, entry)
		}
	}
	if len(entries) > 256 {
		entries = entries[len(entries)-256:]
	}
	return entries
}

func (s *Store) appendHistoryUnlocked(key string, entry HistoryEntry) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	entries := s.readHistoryUnlocked(key)
	if len(entries) > 0 && entries[len(entries)-1].Version == entry.Version {
		return nil
	}
	entries = append(entries, entry)
	if len(entries) > 256 {
		entries = entries[len(entries)-256:]
	}
	var b strings.Builder
	for _, item := range entries {
		line, err := json.Marshal(item)
		if err != nil {
			return err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	path := s.historyPath(key)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (s *Store) read(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readUnlocked(key)
}

func (s *Store) readUnlocked(key string) string {
	data, err := os.ReadFile(filepath.Join(s.dir, key))
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(data))
	if value == "" || len(value) > 64 {
		return ""
	}
	return value
}
