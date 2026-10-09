package projects

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Project struct {
	Name          string `json:"name"`
	Selector      string `json:"selector"`
	Description   string `json:"description,omitempty"`
	GitRepository bool   `json:"gitRepository"`
	Source        string `json:"source"`
}

type ListOutput struct {
	AccessRoot string    `json:"accessRoot"`
	Projects   []Project `json:"projects"`
	Total      int       `json:"total"`
	// Workspace, Selected and AwaitingSelection carry the caller's current
	// binding so a mounted setup card can render without an extra status call.
	Workspace         *WorkspaceInfo `json:"workspace,omitempty"`
	Selected          bool           `json:"selected,omitempty"`
	AwaitingSelection bool           `json:"awaitingSelection,omitempty"`
}

func (m *Manager) List(query string, limit int) (ListOutput, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	projects, err := m.discoverProjects()
	if err != nil {
		return ListOutput{}, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	filtered := make([]Project, 0, len(projects))
	for _, p := range projects {
		if query != "" {
			haystack := strings.ToLower(p.Name + "\n" + p.Selector + "\n" + p.Description)
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		filtered = append(filtered, p)
	}
	total := len(filtered)
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return ListOutput{
		AccessRoot: m.access.Path(),
		Projects:   filtered,
		Total:      total,
	}, nil
}

func (m *Manager) discoverProjects() ([]Project, error) {
	seen := map[string]Project{}

	for _, spec := range m.cfg.Projects {
		path := spec.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(m.access.Path(), path)
		}
		path, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		resolved, err := m.access.Resolve(relativeTo(m.access.Path(), path), false)
		if err != nil {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			continue
		}
		selector, err := m.access.Relative(resolved)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			name = filepath.Base(resolved)
		}
		seen[strings.ToLower(selector)] = Project{
			Name:          name,
			Selector:      selector,
			Description:   spec.Description,
			GitRepository: isGitProject(resolved),
			Source:        "config",
		}
	}

	root := m.access.Path()
	maxDepth := m.cfg.ProjectScanDepth
	if maxDepth == 0 {
		maxDepth = 2
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if rel == "." {
			if hasProjectMarker(path) {
				selector := "."
				seen[selector] = Project{
					Name:          filepath.Base(path),
					Selector:      selector,
					GitRepository: isGitProject(path),
					Source:        "discovered",
				}
			}
			return nil
		}
		depth := len(strings.Split(filepath.ToSlash(rel), "/"))
		base := filepath.Base(path)
		if shouldSkipProjectDir(base) {
			return filepath.SkipDir
		}
		if depth > maxDepth {
			return filepath.SkipDir
		}
		if hasProjectMarker(path) {
			selector := filepath.ToSlash(rel)
			key := strings.ToLower(selector)
			if _, exists := seen[key]; !exists {
				seen[key] = Project{
					Name:          base,
					Selector:      selector,
					GitRepository: isGitProject(path),
					Source:        "discovered",
				}
			}
			if isGitProject(path) {
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	projects := make([]Project, 0, len(seen))
	for _, project := range seen {
		projects = append(projects, project)
	}
	sort.Slice(projects, func(i, j int) bool {
		return strings.ToLower(projects[i].Selector) < strings.ToLower(projects[j].Selector)
	})
	return projects, nil
}

func hasProjectMarker(path string) bool {
	for _, marker := range []string{".git", "go.mod", "Cargo.toml", "package.json", "pyproject.toml", "pom.xml", "build.gradle", "build.gradle.kts"} {
		if _, err := os.Stat(filepath.Join(path, marker)); err == nil {
			return true
		}
	}
	return false
}

func isGitProject(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func shouldSkipProjectDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch strings.ToLower(name) {
	case "node_modules", "vendor", "dist", "build", "target", "__pycache__":
		return true
	default:
		return false
	}
}

func relativeTo(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
