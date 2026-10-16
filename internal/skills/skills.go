package skills

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/workspace"
	"gopkg.in/yaml.v3"
)

const (
	FileName          = "SKILL.md"
	maxSkillNameBytes = 64
	maxPackageFiles   = 50
)

type Skill struct {
	Name                    string `json:"name"`
	Description             string `json:"description"`
	AllowImplicitInvocation bool   `json:"allow_implicit_invocation"`
	Scope                   string `json:"scope"`
	Path                    string `json:"path"`

	dir string
}

type Catalog struct {
	Skills   []Skill  `json:"skills"`
	Content  string   `json:"content"`
	Warnings []string `json:"warnings,omitempty"`
}

type Reader struct {
	cfg   config.SkillsConfig
	mu    sync.RWMutex
	extra []root
}

func New(cfg config.SkillsConfig) *Reader {
	return &Reader{cfg: cfg}
}

func (r *Reader) AddRoot(path, scope string) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.extra {
		if strings.EqualFold(existing.path, path) {
			return
		}
	}
	r.extra = append(r.extra, root{path: path, scope: scope})
}

func (r *Reader) List(workDir string) (Catalog, error) {
	return r.list(workDir, nil)
}

// ListWithin enforces member-root containment on repository-local skill
// packages. Explicitly configured global user/plugin roots retain their
// shared scopes, so callers can distinguish them from repo instructions.
func (r *Reader) ListWithin(root *workspace.Root) (Catalog, error) {
	if root == nil {
		return Catalog{}, errors.New("skill catalog requires an active checkout")
	}
	return r.list(root.Path(), root)
}

func (r *Reader) list(workDir string, confinement *workspace.Root) (Catalog, error) {
	if !r.cfg.Enabled {
		return Catalog{Content: "Skills are disabled by the server configuration. Nothing was searched."}, nil
	}
	roots := r.roots(workDir)
	seen := map[string]struct{}{}
	var skills []Skill
	var warnings []string

	for _, root := range roots {
		entries, err := os.ReadDir(root.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", root.path, err))
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(root.path, entry.Name())
			path := filepath.Join(dir, FileName)
			if confinement != nil && root.scope == "repo" {
				rel, relErr := filepath.Rel(confinement.Path(), path)
				if relErr != nil {
					continue
				}
				resolved, resolveErr := confinement.Resolve(rel, false)
				if resolveErr != nil {
					continue
				}
				path = resolved
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			name, description, err := parseFrontmatter(data, entry.Name())
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
				continue
			}
			key := strings.ToLower(name)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			skills = append(skills, Skill{
				Name:                    name,
				Description:             description,
				AllowImplicitInvocation: implicitAllowed(dir),
				Scope:                   root.scope,
				Path:                    root.scope + ":" + filepath.ToSlash(filepath.Join(entry.Name(), FileName)),
				dir:                     dir,
			})
		}
	}

	pluginSkills, pluginWarnings := r.discoverPluginSkills(workDir)
	for _, skill := range pluginSkills {
		key := strings.ToLower(skill.Name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		skills = append(skills, skill)
	}
	warnings = append(warnings, pluginWarnings...)

	sort.Slice(skills, func(i, j int) bool {
		return strings.ToLower(skills[i].Name) < strings.ToLower(skills[j].Name)
	})
	catalog := Catalog{Skills: skills, Warnings: warnings}
	catalog.Content = renderCatalog(catalog, roots)
	return catalog, nil
}

type ReadInput struct {
	Repository string `json:"repository,omitempty"`
	Name       string `json:"name"`
	Resource   string `json:"resource,omitempty"`
	Offset     int    `json:"offset,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type ReadOutput struct {
	Content    string `json:"content"`
	Truncated  bool   `json:"truncated"`
	NextOffset *int   `json:"nextOffset,omitempty"`
}

func (r *Reader) Read(workDir string, in ReadInput) (ReadOutput, error) {
	return r.read(workDir, in, nil)
}

func (r *Reader) ReadWithin(root *workspace.Root, in ReadInput) (ReadOutput, error) {
	if root == nil {
		return ReadOutput{}, errors.New("skill read requires an active checkout")
	}
	return r.read(root.Path(), in, root)
}

func (r *Reader) read(workDir string, in ReadInput, confinement *workspace.Root) (ReadOutput, error) {
	if !r.cfg.Enabled {
		return ReadOutput{}, errors.New("skills are disabled by the server configuration")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return ReadOutput{}, errors.New("a skill name is required")
	}
	catalog, err := r.list(workDir, confinement)
	if err != nil {
		return ReadOutput{}, err
	}
	var skill *Skill
	for i := range catalog.Skills {
		if strings.EqualFold(catalog.Skills[i].Name, name) {
			copySkill := catalog.Skills[i]
			skill = &copySkill
			break
		}
	}
	if skill == nil {
		var names []string
		for _, item := range catalog.Skills {
			names = append(names, item.Name)
		}
		if len(names) == 0 {
			return ReadOutput{}, fmt.Errorf("no skill named %s; no skills are installed", name)
		}
		return ReadOutput{}, fmt.Errorf("no skill named %s; available: %s", name, strings.Join(names, ", "))
	}

	resource := strings.TrimSpace(in.Resource)
	if resource == "" {
		resource = FileName
	}
	path, err := resolveResource(skill.dir, resource)
	if err != nil {
		return ReadOutput{}, err
	}
	if confinement != nil && skill.Scope == "repo" {
		if _, err := confinement.Relative(path); err != nil {
			return ReadOutput{}, errors.New("repository-local skill resource escapes the selected checkout")
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ReadOutput{}, fmt.Errorf("%s has no file at %s", skill.Name, resource)
	}
	if in.Offset < 0 {
		return ReadOutput{}, errors.New("offset must be >= 0")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 400
	}
	if limit > 2000 {
		limit = 2000
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if in.Offset > len(lines) {
		in.Offset = len(lines)
	}
	end := in.Offset + limit
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	displayPath := skill.Path
	if resource != FileName {
		displayPath = skill.Scope + ":" + filepath.ToSlash(filepath.Join(skill.Name, resource))
	}
	fmt.Fprintf(&b, "%s - %s\n\n", skill.Name, displayPath)
	if !skill.AllowImplicitInvocation {
		b.WriteString("Policy: explicit invocation only; use only when the user requests this skill.\n\n")
	}
	b.WriteString(strings.Join(lines[in.Offset:end], "\n"))
	truncated := end < len(lines)
	out := ReadOutput{Content: b.String(), Truncated: truncated}
	if truncated {
		next := end
		out.NextOffset = &next
		fmt.Fprintf(&b, "\n\n[truncated; continue with offset=%d]", next)
		out.Content = b.String()
	} else if resource == FileName {
		files := packageFiles(skill.dir)
		if len(files) > 0 {
			fmt.Fprintf(&b, "\n\nOther files in this skill, readable with resource=<path>: %s", strings.Join(files, ", "))
			out.Content = b.String()
		}
	}
	return out, nil
}

type root struct {
	path  string
	scope string
}

func (r *Reader) roots(workDir string) []root {
	var roots []root
	add := func(path, scope string) {
		path = filepath.Clean(path)
		for _, existing := range roots {
			if strings.EqualFold(existing.path, path) {
				return
			}
		}
		roots = append(roots, root{path: path, scope: scope})
	}

	projectRoots := []string{workDir}
	if gitRoot, err := gitTopLevel(workDir); err == nil && !strings.EqualFold(gitRoot, workDir) {
		projectRoots = append([]string{gitRoot}, projectRoots...)
	}
	for _, base := range projectRoots {
		add(filepath.Join(base, ".agents", "skills"), "repo")
		add(filepath.Join(base, ".codex", "skills"), "repo")
		if r.cfg.ClaudePlugins {
			add(filepath.Join(base, ".claude", "skills"), "repo")
		}
	}

	for _, dir := range r.cfg.Dirs {
		if strings.TrimSpace(dir) != "" {
			add(dir, "user")
		}
	}
	if r.cfg.IncludeUser && len(r.cfg.Dirs) == 0 {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			add(filepath.Join(home, ".agents", "skills"), "user")
			add(filepath.Join(home, ".codex", "skills"), "user")
			if r.cfg.ClaudePlugins {
				add(filepath.Join(home, ".claude", "skills"), "user")
			}
		}
	}
	r.mu.RLock()
	extra := append([]root(nil), r.extra...)
	r.mu.RUnlock()
	for _, item := range extra {
		add(item.path, item.scope)
	}
	return roots
}

func parseFrontmatter(data []byte, fallback string) (string, string, error) {
	text := strings.TrimPrefix(string(data), "\uFEFF")
	scanner := bufio.NewScanner(strings.NewReader(text))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return "", "", errors.New("missing YAML frontmatter delimited by ---")
	}
	var lines []string
	foundEnd := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			foundEnd = true
			break
		}
		lines = append(lines, line)
	}
	if !foundEnd {
		return "", "", errors.New("missing closing YAML frontmatter delimiter")
	}
	var front struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &front); err != nil {
		return "", "", fmt.Errorf("invalid YAML: %w", err)
	}
	name := strings.Join(strings.Fields(front.Name), " ")
	if name == "" {
		name = fallback
	}
	description := strings.Join(strings.Fields(front.Description), " ")
	if len(name) > maxSkillNameBytes {
		return "", "", fmt.Errorf("invalid name: longer than %d bytes", maxSkillNameBytes)
	}
	if description == "" {
		return "", "", errors.New("missing field description")
	}
	return name, description, nil
}

func implicitAllowed(dir string) bool {
	path := filepath.Join(dir, "agents", "openai.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	var meta struct {
		Policy struct {
			Allow *bool `yaml:"allow_implicit_invocation"`
		} `yaml:"policy"`
	}
	if yaml.Unmarshal(data, &meta) != nil || meta.Policy.Allow == nil {
		return true
	}
	return *meta.Policy.Allow
}

func resolveResource(dir, resource string) (string, error) {
	if filepath.IsAbs(resource) {
		return "", errors.New("skill resource must be package-relative")
	}
	clean := filepath.Clean(resource)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("skill resource escapes the skill package")
	}
	base, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(base, clean)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("skill resource escapes the skill package")
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return "", errors.New("skill resource is not a readable file")
	}
	return resolved, nil
}

func packageFiles(dir string) []string {
	var files []string
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == dir {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == FileName {
			return nil
		}
		files = append(files, rel)
		if len(files) >= maxPackageFiles {
			return errors.New("enough")
		}
		return nil
	})
	sort.Strings(files)
	if len(files) > maxPackageFiles {
		files = files[:maxPackageFiles]
	}
	return files
}

func renderCatalog(catalog Catalog, roots []root) string {
	var b strings.Builder
	if len(catalog.Skills) == 0 {
		b.WriteString("No skills found. A skill is a directory holding a SKILL.md whose frontmatter carries a name and a description.")
		if len(roots) > 0 {
			b.WriteString("\n\nSearched:\n")
			for _, root := range roots {
				fmt.Fprintf(&b, "- %s (%s)\n", root.path, root.scope)
			}
		}
	} else {
		fmt.Fprintf(&b, "%d skill", len(catalog.Skills))
		if len(catalog.Skills) != 1 {
			b.WriteByte('s')
		}
		b.WriteString(" available. Read one with skills_read before acting on it.\n\n")
		for _, skill := range catalog.Skills {
			fmt.Fprintf(&b, "- %s (%s) - %s\n  %s\n", skill.Name, skill.Scope, skill.Description, skill.Path)
			if !skill.AllowImplicitInvocation {
				b.WriteString("  Policy: explicit invocation only.\n")
			}
		}
	}
	if len(catalog.Warnings) > 0 {
		b.WriteString("\nDiscovery warnings:\n")
		for _, warning := range catalog.Warnings {
			fmt.Fprintf(&b, "- %s\n", warning)
		}
	}
	return strings.TrimSpace(b.String())
}

func gitTopLevel(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return filepath.Clean(strings.TrimSpace(string(out))), nil
}
