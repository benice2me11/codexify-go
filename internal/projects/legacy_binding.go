package projects

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/benice2me11/codexify-go/internal/workspace"
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
	projectRoot, err := workspace.Canonical(legacy.ProjectRoot)
	if err != nil {
		return nil, nil
	}
	sourceProjectRoot, err := workspace.Canonical(legacy.SourceProjectRoot)
	if err != nil {
		return nil, nil
	}
	binding := Binding{
		Version: bindingVersion, Mode: "project", SourceProjectRoot: sourceProjectRoot,
		ProjectRoot: projectRoot, ManagedWorktree: legacy.ManagedWorktree,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if legacy.RepositoryURL != nil {
		binding.RepositoryURL = *legacy.RepositoryURL
	}
	if legacy.WorktreeGitRoot != nil {
		canonical, err := workspace.Canonical(*legacy.WorktreeGitRoot)
		if err != nil {
			return nil, nil
		}
		binding.WorktreeGitRoot = canonical
	}
	if legacy.WorktreesRoot != nil {
		canonical, err := workspace.Canonical(*legacy.WorktreesRoot)
		if err != nil {
			return nil, nil
		}
		binding.WorktreesRoot = canonical
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
	canonicalRoot, err := workspace.Canonical(accessRoot)
	if err != nil {
		return false
	}
	canonicalPath, err := workspace.Canonical(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(canonicalRoot, canonicalPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	info, err := os.Stat(canonicalPath)
	return err == nil && info.IsDir()
}
