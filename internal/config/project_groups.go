package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateProjectGroups validates the static, permission-bearing catalog.
// Filesystem identity is checked separately by projects.NewGroupRegistry on
// the host OS, after config-relative project paths have been expanded.
func ValidateProjectGroups(m MCPConfig) error {
	if len(m.ProjectGroups) == 0 && !m.ProjectGroupsEnabled {
		return nil
	}
	if !m.MultiProject {
		return errors.New("mcp.projectGroups requires mcp.multiProject=true")
	}
	if m.ProjectGroupsEnabled && len(m.ProjectGroups) == 0 {
		return errors.New("mcp.projectGroupsEnabled requires at least one configured group")
	}

	projects := make(map[string]struct{}, len(m.Projects))
	for i, project := range m.Projects {
		path := strings.TrimSpace(project.Path)
		if path == "" {
			return fmt.Errorf("mcp.projects[%d].path is required for project groups", i)
		}
		name := strings.TrimSpace(project.Name)
		if name == "" {
			name = filepath.Base(filepath.Clean(filepath.FromSlash(path)))
		}
		if !validGroupIdentifier(name) {
			return fmt.Errorf("mcp.projects[%d] has invalid project identifier %q", i, name)
		}
		key := strings.ToLower(name)
		if _, exists := projects[key]; exists {
			return fmt.Errorf("duplicate configured project name %q", name)
		}
		projects[key] = struct{}{}
	}

	groups := make(map[string]struct{}, len(m.ProjectGroups))
	for i, group := range m.ProjectGroups {
		name := strings.TrimSpace(group.Name)
		if !validGroupIdentifier(name) {
			return fmt.Errorf("mcp.projectGroups[%d] has invalid group name %q", i, group.Name)
		}
		key := strings.ToLower(name)
		if _, exists := groups[key]; exists {
			return fmt.Errorf("duplicate project group name %q", name)
		}
		groups[key] = struct{}{}
		if len(group.Repositories) == 0 {
			return fmt.Errorf("project group %q must contain repositories", name)
		}
		aliases := map[string]struct{}{}
		memberProjects := map[string]struct{}{}
		for j, repository := range group.Repositories {
			alias := strings.TrimSpace(repository.Alias)
			if !validGroupIdentifier(alias) {
				return fmt.Errorf("project group %q repository[%d] has invalid alias %q", name, j, repository.Alias)
			}
			aliasKey := strings.ToLower(alias)
			if _, exists := aliases[aliasKey]; exists {
				return fmt.Errorf("project group %q has duplicate alias %q", name, alias)
			}
			aliases[aliasKey] = struct{}{}

			project := strings.ToLower(strings.TrimSpace(repository.Project))
			if _, exists := projects[project]; !exists {
				return fmt.Errorf("project group %q alias %q references unknown configured project %q", name, alias, repository.Project)
			}
			if _, exists := memberProjects[project]; exists {
				return fmt.Errorf("project group %q repeats configured project %q", name, repository.Project)
			}
			memberProjects[project] = struct{}{}
			switch strings.ToLower(strings.TrimSpace(repository.Checkout)) {
			case "source", "managed":
			default:
				return fmt.Errorf("project group %q alias %q checkout must be source or managed", name, alias)
			}
		}
		primary := strings.ToLower(strings.TrimSpace(group.Primary))
		if _, exists := aliases[primary]; !exists {
			return fmt.Errorf("project group %q primary %q is not a member alias", name, group.Primary)
		}
	}
	return nil
}

// Identifiers have ASCII, case-insensitive identity across Windows, macOS and
// Linux. Reject Unicode lookalikes, path separators and whitespace.
func validGroupIdentifier(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		switch {
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z':
		case i > 0 && b >= '0' && b <= '9':
		case i > 0 && (b == '-' || b == '_'):
		default:
			return false
		}
	}
	return true
}
