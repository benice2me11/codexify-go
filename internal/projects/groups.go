package projects

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/workspace"
)

// GroupMember is a source-checkout catalog record, not a conversation binding.
// An explicit managed checkout has no active root until the v3 lifecycle
// materializes and persists one. No member is implicitly given the access root.
type GroupMember struct {
	Alias             string `json:"alias"`
	Project           string `json:"project"`
	Checkout          string `json:"checkout"`
	SourceProjectRoot string `json:"sourceProjectRoot"`
	GitRoot           string `json:"gitRoot"`
}

type GroupDefinition struct {
	Name                  string        `json:"name"`
	Primary               string        `json:"primary"`
	MembershipFingerprint string        `json:"membershipFingerprint"`
	Repositories          []GroupMember `json:"repositories"`
}

// ResolvedGroupSource represents a point-in-time validated source checkout.
// Future conversation routing must also verify the bound v3 fingerprint and
// resolve each member's immutable active checkout before using its Root.
type ResolvedGroupSource struct {
	GroupName             string
	MembershipFingerprint string
	Member                GroupMember
	Root                  *workspace.Root
}

type groupSource struct {
	member       GroupMember
	declaredPath string
	fileIdentity os.FileInfo
}

type groupRecord struct {
	definition GroupDefinition
	sources    map[string]groupSource
}

type GroupRegistry struct {
	enabled bool
	access  *workspace.Root
	groups  map[string]groupRecord
}

// NewGroupRegistry performs no filesystem discovery, worktree creation or
// persistence. Only the explicit configured project catalog may grant a
// group member; every enabled member is checked for canonical Git identity.
func NewGroupRegistry(access *workspace.Root, cfg config.MCPConfig) (*GroupRegistry, error) {
	if access == nil {
		return nil, errors.New("project group registry requires a workspace access root")
	}
	if err := config.ValidateProjectGroups(cfg); err != nil {
		return nil, err
	}
	registry := &GroupRegistry{
		enabled: cfg.ProjectGroupsEnabled,
		access:  access,
		groups:  make(map[string]groupRecord),
	}
	if !cfg.ProjectGroupsEnabled {
		return registry, nil
	}

	catalog := make(map[string]config.ProjectSpec, len(cfg.Projects))
	for _, project := range cfg.Projects {
		name := strings.TrimSpace(project.Name)
		if name == "" {
			name = filepath.Base(filepath.Clean(filepath.FromSlash(project.Path)))
		}
		catalog[strings.ToLower(name)] = project
	}

	for _, spec := range cfg.ProjectGroups {
		group := groupRecord{
			definition: GroupDefinition{
				Name:    strings.TrimSpace(spec.Name),
				Primary: strings.ToLower(strings.TrimSpace(spec.Primary)),
			},
			sources: make(map[string]groupSource, len(spec.Repositories)),
		}
		for _, repo := range spec.Repositories {
			key := strings.ToLower(strings.TrimSpace(repo.Alias))
			project := catalog[strings.ToLower(strings.TrimSpace(repo.Project))]
			source, declared, err := canonicalGroupProject(access, project.Path)
			if err != nil {
				return nil, fmt.Errorf("group %q repository %q: %w", spec.Name, repo.Alias, err)
			}
			gitRoot, err := gitTopLevel(source)
			if err != nil || !sameGroupPath(gitRoot, source) {
				return nil, fmt.Errorf("group %q repository %q must point to an independent Git checkout root", spec.Name, repo.Alias)
			}
			identity, err := os.Stat(source)
			if err != nil {
				return nil, fmt.Errorf("group %q repository %q source unavailable: %w", spec.Name, repo.Alias, err)
			}
			// Windows os.FileInfo lazily reads file IDs. Force that read while
			// this is still the original checkout; otherwise a later SameFile
			// after rename/replacement could reopen the *new* path as both IDs.
			_ = os.SameFile(identity, identity)
			member := GroupMember{
				Alias:             key,
				Project:           strings.ToLower(strings.TrimSpace(repo.Project)),
				Checkout:          strings.ToLower(strings.TrimSpace(repo.Checkout)),
				SourceProjectRoot: source,
				GitRoot:           gitRoot,
			}
			for _, prior := range group.definition.Repositories {
				if groupPathWithin(prior.SourceProjectRoot, source) || groupPathWithin(source, prior.SourceProjectRoot) {
					return nil, fmt.Errorf("group %q has duplicate or overlapping repository roots (%q and %q)", spec.Name, prior.Alias, repo.Alias)
				}
			}
			group.sources[key] = groupSource{member: member, declaredPath: declared, fileIdentity: identity}
			group.definition.Repositories = append(group.definition.Repositories, member)
		}
		sort.Slice(group.definition.Repositories, func(i, j int) bool {
			return group.definition.Repositories[i].Alias < group.definition.Repositories[j].Alias
		})
		group.definition.MembershipFingerprint = groupFingerprint(group.definition)
		registry.groups[strings.ToLower(group.definition.Name)] = group
	}
	return registry, nil
}

func canonicalGroupProject(access *workspace.Root, configured string) (canonical string, declared string, err error) {
	declared = filepath.FromSlash(strings.TrimSpace(configured))
	if !filepath.IsAbs(declared) {
		declared = filepath.Join(access.Path(), declared)
	}
	declared, err = filepath.Abs(declared)
	if err != nil {
		return "", "", err
	}
	declared = workspace.NormalizePathIdentity(declared)
	actual, err := workspace.Canonical(declared)
	if err != nil {
		return "", "", fmt.Errorf("canonicalize configured checkout: %w", err)
	}
	// A relative selector is used for access.Resolve's canonical containment
	// checks, including Windows junction/reparse-point and 8.3 path aliases.
	rel, err := filepath.Rel(access.Path(), actual)
	if err != nil {
		return "", "", err
	}
	canonical, err = access.Resolve(rel, false)
	if err != nil {
		return "", "", fmt.Errorf("project path outside allowed access root or unavailable: %w", err)
	}
	if sameGroupPath(canonical, access.Path()) {
		return "", "", errors.New("a group member cannot be the entire workspace access root")
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return "", "", errors.New("group source checkout must be an existing directory")
	}
	return canonical, declared, nil
}

func sameGroupPath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func groupPathWithin(parent, child string) bool {
	if runtime.GOOS == "windows" {
		parent = strings.ToLower(filepath.Clean(parent))
		child = strings.ToLower(filepath.Clean(child))
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func groupFingerprint(group GroupDefinition) string {
	// Repositories have already been sorted by normalized alias; JSON
	// field-order is stable, so this is reproducible across restarts.
	payload, _ := json.Marshal(struct {
		Version      int           `json:"version"`
		Name         string        `json:"name"`
		Primary      string        `json:"primary"`
		Repositories []GroupMember `json:"repositories"`
	}{1, strings.ToLower(group.Name), group.Primary, group.Repositories})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// List yields detached snapshots. The feature flag disables enumeration even
// if dormant group declarations exist in config.json.
func (r *GroupRegistry) List() []GroupDefinition {
	if r == nil || !r.enabled {
		return nil
	}
	out := make([]GroupDefinition, 0, len(r.groups))
	for _, group := range r.groups {
		snapshot := group.definition
		snapshot.Repositories = append([]GroupMember(nil), snapshot.Repositories...)
		out = append(out, snapshot)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// ResolveSource never changes a conversation's active repository. It validates
// the same configured member again at the point of use, rejecting stale paths
// or reparse-point swaps rather than following an old trusted path elsewhere.
func (r *GroupRegistry) ResolveSource(groupName, alias string) (ResolvedGroupSource, error) {
	if r == nil || !r.enabled {
		return ResolvedGroupSource{}, errors.New("project groups are disabled")
	}
	group, exists := r.groups[strings.ToLower(strings.TrimSpace(groupName))]
	if !exists {
		return ResolvedGroupSource{}, fmt.Errorf("unknown project group %q", groupName)
	}
	if strings.TrimSpace(alias) == "" {
		alias = group.definition.Primary
	}
	member, exists := group.sources[strings.ToLower(strings.TrimSpace(alias))]
	if !exists {
		return ResolvedGroupSource{}, fmt.Errorf("group %q has no repository alias %q", groupName, alias)
	}
	canonical, _, err := canonicalGroupProject(r.access, member.declaredPath)
	if err != nil || !sameGroupPath(canonical, member.member.SourceProjectRoot) {
		return ResolvedGroupSource{}, fmt.Errorf("group %q alias %q source path changed or became unavailable; explicit rebind required", groupName, alias)
	}
	info, err := os.Stat(canonical)
	if err != nil || !os.SameFile(member.fileIdentity, info) {
		return ResolvedGroupSource{}, fmt.Errorf("group %q alias %q source checkout was replaced; explicit rebind required", groupName, alias)
	}
	gitRoot, err := gitTopLevel(canonical)
	if err != nil || !sameGroupPath(gitRoot, member.member.GitRoot) {
		return ResolvedGroupSource{}, fmt.Errorf("group %q alias %q Git root changed; explicit rebind required", groupName, alias)
	}
	root, err := workspace.New(canonical)
	if err != nil {
		return ResolvedGroupSource{}, err
	}
	return ResolvedGroupSource{
		GroupName:             group.definition.Name,
		MembershipFingerprint: group.definition.MembershipFingerprint,
		Member:                member.member,
		Root:                  root,
	}, nil
}
