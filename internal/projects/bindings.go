package projects

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/benice2me11/codexify-go/internal/workspace"
)

const (
	bindingVersion       = 2
	groupBindingVersion  = 3
	legacyBindingVersion = 1
)

type GroupBindingRepository struct {
	Alias                       string `json:"alias"`
	SourceProjectRoot           string `json:"sourceProjectRoot"`
	ProjectRoot                 string `json:"projectRoot"`
	Checkout                    string `json:"checkout"`
	ManagedWorktree             bool   `json:"managedWorktree"`
	WorktreeGitRoot             string `json:"worktreeGitRoot,omitempty"`
	CheckoutRevision            uint64 `json:"checkoutRevision,omitempty"`
	PreservedManagedProjectRoot string `json:"preservedManagedProjectRoot,omitempty"`
	PreservedManagedGitRoot     string `json:"preservedManagedGitRoot,omitempty"`
}

type Binding struct {
	Version               int                      `json:"version"`
	IdentityHash          string                   `json:"identityHash"`
	Scope                 string                   `json:"bindingScope,omitempty"`
	Mode                  string                   `json:"mode,omitempty"`
	SourceProjectRoot     string                   `json:"sourceProjectRoot"`
	ProjectRoot           string                   `json:"projectRoot"`
	RepositoryURL         string                   `json:"repositoryUrl,omitempty"`
	CheckoutCommit        string                   `json:"checkoutCommit,omitempty"`
	Cloned                bool                     `json:"cloned,omitempty"`
	ManagedWorktree       bool                     `json:"managedWorktree"`
	WorktreeGitRoot       string                   `json:"worktreeGitRoot,omitempty"`
	WorktreesRoot         string                   `json:"worktreesRoot,omitempty"`
	GroupName             string                   `json:"groupName,omitempty"`
	PrimaryRepository     string                   `json:"primaryRepository,omitempty"`
	ActiveRepository      string                   `json:"activeRepository,omitempty"`
	MembershipFingerprint string                   `json:"membershipFingerprint,omitempty"`
	Repositories          []GroupBindingRepository `json:"repositories,omitempty"`
	Generation            uint64                   `json:"generation,omitempty"`
	CreatedAt             string                   `json:"createdAt"`
}

func (m *Manager) readBinding(identity *Identity) (*Binding, error) {
	if identity == nil {
		return nil, nil
	}
	if !identity.Persistent {
		m.transientMu.RLock()
		binding, ok := m.transientBindings[identity.Key]
		m.transientMu.RUnlock()
		if !ok {
			return nil, nil
		}
		if err := m.validateBoundWorkspace(&binding); err != nil {
			return nil, err
		}
		copyBinding := binding
		return &copyBinding, nil
	}
	path := m.bindingPath(identity)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return m.importLegacyRustBinding(identity)
	}
	if err != nil {
		return nil, err
	}
	binding, err := decodeBinding(data)
	if err != nil {
		return nil, err
	}
	if binding.IdentityHash != identity.Key {
		return nil, errors.New("project binding identity/version mismatch")
	}
	if err := m.validateBoundWorkspace(binding); err != nil {
		return nil, err
	}
	return binding, nil
}

func (m *Manager) writeBinding(identity *Identity, binding Binding) error {
	if identity == nil {
		return errors.New("binding identity is required")
	}
	binding.IdentityHash = identity.Key
	binding.Scope = identity.Scope
	if !identity.Persistent {
		m.transientMu.Lock()
		m.transientBindings[identity.Key] = binding
		m.transientMu.Unlock()
		return nil
	}
	if err := os.MkdirAll(m.cfg.BindingsDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(m.cfg.BindingsDir, binding.IdentityHash+".json")
	return writeBindingFile(path, binding)
}

func writeBindingFile(path string, binding Binding) error {
	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp-" + fmt.Sprint(time.Now().UnixNano())
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func decodeBinding(data []byte) (*Binding, error) {
	var binding Binding
	if err := json.Unmarshal(data, &binding); err != nil {
		return nil, fmt.Errorf("decode project binding: %w", err)
	}
	if binding.Version != bindingVersion && binding.Version != legacyBindingVersion && binding.Version != groupBindingVersion {
		return nil, fmt.Errorf("unsupported project binding version %d", binding.Version)
	}
	if binding.Mode == "" {
		binding.Mode = "project"
	}
	if binding.Scope == "" {
		binding.Scope = "chatgpt_conversation"
	}
	if binding.ProjectRoot == "" {
		return nil, errors.New("project binding is missing projectRoot")
	}
	if binding.Version == groupBindingVersion {
		if binding.Mode != "group" || binding.GroupName == "" || binding.PrimaryRepository == "" ||
			binding.ActiveRepository == "" || binding.MembershipFingerprint == "" || len(binding.Repositories) == 0 {
			return nil, errors.New("invalid group binding v3: missing group identity or members")
		}
	} else if binding.Mode == "group" {
		return nil, errors.New("group mode requires binding version 3")
	}
	return &binding, nil
}

func (m *Manager) bindingPath(identity *Identity) string {
	return filepath.Join(m.cfg.BindingsDir, identity.Key+".json")
}

func (m *Manager) sourceInUse(source string, identity *Identity) bool {
	source = cleanComparable(source)
	if entries, err := os.ReadDir(m.cfg.BindingsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(m.cfg.BindingsDir, entry.Name()))
			if err != nil {
				continue
			}
			var binding Binding
			if json.Unmarshal(data, &binding) != nil {
				continue
			}
			if identity != nil && binding.IdentityHash == identity.Key {
				continue
			}
			if binding.Mode == "group" && !m.cfg.ProjectGroupsEnabled {
				continue
			}
			if bindingUsesSource(binding, source) {
				return true
			}
		}
	}
	m.transientMu.RLock()
	defer m.transientMu.RUnlock()
	for key, binding := range m.transientBindings {
		if identity != nil && key == identity.Key {
			continue
		}
		if binding.Mode == "group" && !m.cfg.ProjectGroupsEnabled {
			continue
		}
		if bindingUsesSource(binding, source) {
			return true
		}
	}
	return false
}

func bindingUsesSource(binding Binding, comparableSource string) bool {
	if binding.Mode == "scratch" {
		return false
	}
	if binding.Mode == "group" {
		for _, member := range binding.Repositories {
			if cleanComparable(member.SourceProjectRoot) == comparableSource {
				if _, err := os.Stat(member.ProjectRoot); err == nil {
					return true
				}
			}
		}
		return false
	}
	if cleanComparable(binding.SourceProjectRoot) == comparableSource {
		_, err := os.Stat(binding.ProjectRoot)
		return err == nil
	}
	return false
}

func cleanComparable(path string) string {
	canonical, err := workspace.Canonical(path)
	if err == nil {
		return strings.ToLower(canonical)
	}
	return strings.ToLower(filepath.Clean(path))
}
