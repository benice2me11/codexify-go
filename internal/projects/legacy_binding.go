package projects

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type legacyRustBinding struct {
	Version           int     `json:"version"`
	AccessRoot        string  `json:"accessRoot"`
	ProjectRoot       string  `json:"projectRoot"`
	SourceProjectRoot string  `json:"sourceProjectRoot"`
	RepositoryURL     *string `json:"repositoryUrl"`
	ManagedWorktree   bool    `json:"managedWorktree"`
	WorktreeGitRoot   *string `json:"worktreeGitRoot"`
	WorktreesRoot     *string `json:"worktreesRoot"`
}

func (m *Manager) importLegacyRustBinding(identity *Identity) (*Binding, error) {
	if identity == nil || !identity.Persistent || identity.LegacyKey == "" {
		return nil, nil
	}
	legacyRoot := filepath.Join(m.access.Path(), ".codexify", "conversation-projects")
	entries, err := os.ReadDir(legacyRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, nil
	}
	var match string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(legacyRoot, entry.Name(), identity.LegacyKey+".json")
		if _, err := os.Stat(candidate); err == nil {
			if match != "" {
				return nil, nil
			}
			match = candidate
		}
	}
	if match == "" {
		return nil, nil
	}
	data, err := os.ReadFile(match)
	if err != nil {
		return nil, nil
	}
	var legacy legacyRustBinding
	if json.Unmarshal(data, &legacy) != nil || legacy.Version != 2 {
		return nil, nil
	}
	access, err := filepath.Abs(legacy.AccessRoot)
	if err != nil || cleanComparable(access) != cleanComparable(m.access.Path()) {
		return nil, nil
	}
	for _, path := range []string{legacy.ProjectRoot, legacy.SourceProjectRoot} {
		if !safeLegacyPath(m.access.Path(), path) {
			return nil, nil
		}
	}
	if legacy.ManagedWorktree {
		if legacy.WorktreeGitRoot == nil || legacy.WorktreesRoot == nil || !safeLegacyPath(m.access.Path(), *legacy.WorktreeGitRoot) || !safeLegacyPath(m.access.Path(), *legacy.WorktreesRoot) {
			return nil, nil
		}
	}
	binding := Binding{
		Version: bindingVersion, Mode: "project", SourceProjectRoot: legacy.SourceProjectRoot,
		ProjectRoot: legacy.ProjectRoot, ManagedWorktree: legacy.ManagedWorktree,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if legacy.RepositoryURL != nil {
		binding.RepositoryURL = *legacy.RepositoryURL
	}
	if legacy.WorktreeGitRoot != nil {
		binding.WorktreeGitRoot = *legacy.WorktreeGitRoot
	}
	if legacy.WorktreesRoot != nil {
		binding.WorktreesRoot = *legacy.WorktreesRoot
	}
	if err := m.writeBinding(identity, binding); err != nil {
		return nil, err
	}
	return &binding, nil
}

func safeLegacyPath(accessRoot, path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(accessRoot, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	info, err := os.Stat(abs)
	return err == nil && info.IsDir()
}
