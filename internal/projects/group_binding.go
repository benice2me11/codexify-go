package projects

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/benice2me11/codexify-go/internal/workspace"
)

// SelectGroup creates one version-3 conversation binding. It does not expose a
// new MCP tool yet: discovery/schema and per-call routing must be introduced
// together in the next stage rather than advertising partially routed tools.
func (m *Manager) SelectGroup(meta map[string]any, name string) (WorkspaceInfo, error) {
	if !m.cfg.ProjectGroupsEnabled || !m.cfg.MultiProject {
		return WorkspaceInfo{}, errors.New("project groups are disabled")
	}
	identity := IdentityFromMeta(meta)
	if identity == nil {
		return WorkspaceInfo{}, errors.New("project group selection requires a conversation or transport-session identity")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	existing, err := m.readBinding(identity)
	if err != nil {
		return WorkspaceInfo{}, err
	}
	if existing != nil {
		if existing.Mode == "group" && strings.EqualFold(existing.GroupName, strings.TrimSpace(name)) {
			return infoFromBinding(m, *existing, false), nil
		}
		return WorkspaceInfo{}, errors.New("conversation already has a workspace binding; explicitly switch or resume before selecting a group")
	}

	group, ok := m.groups.groups[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return WorkspaceInfo{}, fmt.Errorf("unknown project group %q", name)
	}

	type newWorktree struct{ sourceGitRoot, activeGitRoot string }
	var created []newWorktree
	saved := false
	defer func() {
		if !saved {
			// Only freshly created, unexposed managed worktrees are removed on
			// incomplete initial binding. Never touch pre-existing checkouts.
			for i := len(created) - 1; i >= 0; i-- {
				_ = removeManagedWorktree(created[i].sourceGitRoot, created[i].activeGitRoot)
			}
		}
	}()

	binding := Binding{
		Version:               groupBindingVersion,
		Mode:                  "group",
		GroupName:             group.definition.Name,
		PrimaryRepository:     group.definition.Primary,
		ActiveRepository:      group.definition.Primary,
		MembershipFingerprint: group.definition.MembershipFingerprint,
		Generation:            1,
		WorktreesRoot:         m.cfg.Worktrees.Root,
		CreatedAt:             time.Now().UTC().Format(time.RFC3339Nano),
	}
	for _, member := range group.definition.Repositories {
		source, err := m.groups.ResolveSource(group.definition.Name, member.Alias)
		if err != nil {
			return WorkspaceInfo{}, err
		}
		row := GroupBindingRepository{
			Alias:             member.Alias,
			SourceProjectRoot: source.Member.SourceProjectRoot,
			ProjectRoot:       source.Member.SourceProjectRoot,
			Checkout:          member.Checkout,
		}
		if member.Checkout == "managed" {
			worktreesRoot, err := m.pathInsideAccess(m.cfg.Worktrees.Root)
			if err != nil {
				return WorkspaceInfo{}, fmt.Errorf("managed group worktrees root is outside access root: %w", err)
			}
			for _, groupMember := range group.definition.Repositories {
				if groupPathWithin(groupMember.SourceProjectRoot, worktreesRoot) ||
					groupPathWithin(worktreesRoot, groupMember.SourceProjectRoot) {
					return WorkspaceInfo{}, errors.New("managed group worktrees root overlaps a configured source checkout")
				}
			}
			createdWorktree, err := m.createManagedWorktreeAt(
				identity, source.Member.SourceProjectRoot, source.Member.GitRoot, "HEAD")
			if err != nil {
				return WorkspaceInfo{}, err
			}
			created = append(created, newWorktree{source.Member.GitRoot, createdWorktree.GitRoot})
			row.ProjectRoot = createdWorktree.ProjectRoot
			row.ManagedWorktree = true
			row.WorktreeGitRoot = createdWorktree.GitRoot
		}
		binding.Repositories = append(binding.Repositories, row)
	}
	primary, ok := boundGroupMember(binding, binding.PrimaryRepository)
	if !ok {
		return WorkspaceInfo{}, errors.New("group primary member missing after construction")
	}
	binding.SourceProjectRoot = primary.SourceProjectRoot
	binding.ProjectRoot = primary.ProjectRoot
	binding.ManagedWorktree = primary.ManagedWorktree
	binding.WorktreeGitRoot = primary.WorktreeGitRoot
	if err := m.validateGroupBinding(&binding); err != nil {
		return WorkspaceInfo{}, err
	}
	if err := m.writeBinding(identity, binding); err != nil {
		return WorkspaceInfo{}, err
	}
	saved = true
	m.finishSwitch(identity)
	return infoFromBinding(m, binding, true), nil
}

// SetActiveRepository persists the implicit default only. An explicit alias
// passed to WorkspaceFor never modifies this value.
func (m *Manager) SetActiveRepository(meta map[string]any, alias string) (WorkspaceInfo, error) {
	identity := IdentityFromMeta(meta)
	if identity == nil {
		return WorkspaceInfo{}, errors.New("active repository selection requires a conversation identity")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	binding, err := m.readBinding(identity)
	if err != nil {
		return WorkspaceInfo{}, err
	}
	if binding == nil || binding.Mode != "group" {
		return WorkspaceInfo{}, errors.New("active repository selection requires a project group binding")
	}
	member, ok := boundGroupMember(*binding, alias)
	if !ok {
		return WorkspaceInfo{}, fmt.Errorf("unknown repository alias %q", alias)
	}
	if member.Alias == binding.ActiveRepository {
		return infoFromBinding(m, *binding, false), nil
	}
	binding.ActiveRepository = member.Alias
	binding.Generation++
	if err := m.writeBinding(identity, *binding); err != nil {
		return WorkspaceInfo{}, err
	}
	return infoFromBinding(m, *binding, false), nil
}

func boundGroupMember(binding Binding, alias string) (GroupBindingRepository, bool) {
	alias = strings.ToLower(strings.TrimSpace(alias))
	for _, member := range binding.Repositories {
		if member.Alias == alias {
			return member, true
		}
	}
	return GroupBindingRepository{}, false
}

func (m *Manager) validateBoundWorkspace(binding *Binding) error {
	if binding.Mode == "group" {
		if !m.cfg.ProjectGroupsEnabled {
			return errors.New("project group binding exists but projectGroupsEnabled=false; binding preserved, enable the flag to resume")
		}
		return m.validateGroupBinding(binding)
	}
	if _, err := os.Stat(binding.ProjectRoot); err != nil {
		return fmt.Errorf("bound project is no longer available: %w", err)
	}
	return nil
}

func (m *Manager) validateGroupBinding(binding *Binding) error {
	if binding.Version != groupBindingVersion || binding.Mode != "group" {
		return errors.New("invalid project group binding version/mode")
	}
	if binding.Generation == 0 || !sameGroupPath(binding.WorktreesRoot, m.cfg.Worktrees.Root) {
		return errors.New("project group generation or worktree configuration changed; explicit rebind required")
	}
	group, ok := m.groups.groups[strings.ToLower(binding.GroupName)]
	if !ok || group.definition.MembershipFingerprint != binding.MembershipFingerprint ||
		group.definition.Primary != binding.PrimaryRepository ||
		len(binding.Repositories) != len(group.definition.Repositories) {
		return errors.New("project group membership changed; explicit rebind required")
	}
	if _, exists := boundGroupMember(*binding, binding.PrimaryRepository); !exists {
		return errors.New("group binding primary alias missing")
	}
	if _, exists := boundGroupMember(*binding, binding.ActiveRepository); !exists {
		return errors.New("group binding active alias missing")
	}
	seen := make(map[string]struct{}, len(binding.Repositories))
	for _, member := range binding.Repositories {
		if _, duplicated := seen[member.Alias]; duplicated {
			return fmt.Errorf("project group binding repeats alias %q", member.Alias)
		}
		seen[member.Alias] = struct{}{}
		expected, found := group.sources[member.Alias]
		if !found || expected.member.Checkout != member.Checkout ||
			!sameGroupPath(expected.member.SourceProjectRoot, member.SourceProjectRoot) ||
			member.ManagedWorktree != (member.Checkout == "managed") {
			return fmt.Errorf("project group member %q differs from permitted catalog; rebind required", member.Alias)
		}
		if _, err := m.groups.ResolveSource(group.definition.Name, member.Alias); err != nil {
			return err
		}
		active, err := workspace.New(member.ProjectRoot)
		if err != nil || !sameGroupPath(active.Path(), member.ProjectRoot) {
			return fmt.Errorf("project group member %q active checkout moved or vanished; rebind required", member.Alias)
		}
		if _, err := m.pathInsideAccess(active.Path()); err != nil {
			return fmt.Errorf("project group member %q active root outside access root: %w", member.Alias, err)
		}
		if member.ManagedWorktree {
			if member.WorktreeGitRoot == "" || !groupPathWithin(m.cfg.Worktrees.Root, active.Path()) {
				return fmt.Errorf("project group member %q managed root outside configured worktrees directory", member.Alias)
			}
			gitRoot, err := gitTopLevel(active.Path())
			if err != nil || !sameGroupPath(gitRoot, member.WorktreeGitRoot) {
				return fmt.Errorf("project group member %q managed Git root changed", member.Alias)
			}
			if !sourceOwnsWorktree(expected.member.GitRoot, gitRoot) {
				return fmt.Errorf("project group member %q managed Git root is not owned by its configured source", member.Alias)
			}
		} else if member.WorktreeGitRoot != "" || !sameGroupPath(active.Path(), member.SourceProjectRoot) {
			return fmt.Errorf("project group member %q source checkout identity changed", member.Alias)
		}
	}
	if len(seen) != len(group.sources) {
		return errors.New("project group binding has incomplete repository membership")
	}
	primary, _ := boundGroupMember(*binding, binding.PrimaryRepository)
	if !sameGroupPath(primary.ProjectRoot, binding.ProjectRoot) ||
		!sameGroupPath(primary.SourceProjectRoot, binding.SourceProjectRoot) {
		return errors.New("project group primary root changed; explicit rebind required")
	}
	return nil
}

func sourceOwnsWorktree(sourceGitRoot, activeGitRoot string) bool {
	out, err := exec.Command("git", "-C", sourceGitRoot, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		candidate, err := workspace.Canonical(strings.TrimPrefix(line, "worktree "))
		if err == nil && sameGroupPath(candidate, activeGitRoot) {
			return true
		}
	}
	return false
}
