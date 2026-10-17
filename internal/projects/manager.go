package projects

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/workspace"
)

type Manager struct {
	cfg    config.MCPConfig
	access *workspace.Root
	groups *GroupRegistry
	mu     sync.Mutex

	// Protects file reads against the atomic rename used to persist a new
	// binding or workspace-switch record. Windows can reject an overlapping
	// ReadFile with ERROR_SHARING_VIOLATION; m.mu only serializes mutations.
	// This is in-process safety, not a cross-process compare-and-swap lock.
	bindingIO sync.RWMutex

	transientMu       sync.RWMutex
	transientBindings map[string]Binding
	transientChanges  map[string]WorkspaceChange
	transientHistory  map[string][]Binding
}

type WorkspaceInfo struct {
	Mode                      string                   `json:"mode"`
	AccessRoot                string                   `json:"accessRoot"`
	SourceProjectRoot         string                   `json:"sourceProjectRoot"`
	ProjectRoot               string                   `json:"projectRoot"`
	RepositoryURL             string                   `json:"repositoryUrl,omitempty"`
	CheckoutCommit            string                   `json:"checkoutCommit,omitempty"`
	Cloned                    bool                     `json:"cloned,omitempty"`
	ManagedWorktree           bool                     `json:"managedWorktree"`
	WorktreeGitRoot           string                   `json:"worktreeGitRoot,omitempty"`
	WorktreesRoot             string                   `json:"worktreesRoot,omitempty"`
	WorktreeMode              string                   `json:"worktreeMode"`
	NewlySelected             bool                     `json:"newlySelected"`
	BindingScope              string                   `json:"bindingScope"`
	GroupName                 string                   `json:"groupName,omitempty"`
	PrimaryRepository         string                   `json:"primaryRepository,omitempty"`
	ActiveRepository          string                   `json:"activeRepository,omitempty"`
	EffectiveRepository       string                   `json:"effectiveRepository,omitempty"`
	MembershipFingerprint     string                   `json:"membershipFingerprint,omitempty"`
	Repositories              []GroupBindingRepository `json:"repositories,omitempty"`
	Generation                uint64                   `json:"generation,omitempty"`
	BindingEpoch              string                   `json:"bindingEpoch,omitempty"`
	EffectiveCheckoutRevision uint64                   `json:"effectiveCheckoutRevision,omitempty"`
}

type StatusInfo struct {
	MultiProject         bool           `json:"multiProject"`
	ProjectGroupsEnabled bool           `json:"projectGroupsEnabled,omitempty"`
	AccessRoot           string         `json:"accessRoot"`
	WorktreeMode         string         `json:"worktreeMode"`
	Selected             bool           `json:"selected"`
	AwaitingSelection    bool           `json:"awaitingSelection"`
	Workspace            *WorkspaceInfo `json:"workspace,omitempty"`
}

func New(cfg config.MCPConfig) (*Manager, error) {
	access, err := workspace.New(cfg.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	if cfg.BindingsDir == "" {
		cfg.BindingsDir = filepath.Join(access.Path(), ".codexify-go", "bindings")
	}
	if cfg.CloneDir == "" {
		cfg.CloneDir = filepath.Join(access.Path(), ".codexify-go", "clones")
	}
	if cfg.ScratchDir == "" {
		cfg.ScratchDir = filepath.Join(access.Path(), ".codexify-go", "scratch")
	}
	if cfg.Worktrees.Root == "" {
		cfg.Worktrees.Root = filepath.Join(access.Path(), ".codexify-go", "worktrees")
	} else {
		// Configuration imported from Windows may contain an extended-length
		// (\\?\C:\...) spelling. Keep filesystem identity canonical before this
		// path is handed to Git: git-for-windows does not accept that spelling
		// consistently for `git worktree add` destinations.
		cfg.Worktrees.Root = workspace.NormalizePathIdentity(cfg.Worktrees.Root)
	}
	if cfg.ProjectGroupsEnabled {
		// A configured path may be an alias (notably /var -> /private/var
		// on macOS). Group binding identity and containment checks must
		// compare canonical paths, including a not-yet-created worktrees root.
		canonical, canonicalErr := workspace.Canonical(cfg.Worktrees.Root)
		if canonicalErr != nil {
			return nil, fmt.Errorf("canonicalize group worktrees root: %w", canonicalErr)
		}
		cfg.Worktrees.Root = canonical
	}
	if strings.TrimSpace(cfg.Worktrees.Mode) == "" {
		cfg.Worktrees.Mode = "auto"
	}
	groups, err := NewGroupRegistry(access, cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize project groups: %w", err)
	}
	return &Manager{
		cfg:               cfg,
		access:            access,
		groups:            groups,
		transientBindings: make(map[string]Binding),
		transientChanges:  make(map[string]WorkspaceChange),
		transientHistory:  make(map[string][]Binding),
	}, nil
}

func (m *Manager) AccessRoot() *workspace.Root {
	return m.access
}

// ListProjectGroups is catalog-only and never changes the conversation.
func (m *Manager) ListProjectGroups() []GroupDefinition {
	return m.groups.List()
}

func (m *Manager) ResolveProjectGroupSource(groupName, alias string) (ResolvedGroupSource, error) {
	return m.groups.ResolveSource(groupName, alias)
}

func (m *Manager) Status(meta map[string]any) (StatusInfo, error) {
	status := StatusInfo{
		MultiProject:         m.cfg.MultiProject,
		ProjectGroupsEnabled: m.cfg.ProjectGroupsEnabled,
		AccessRoot:           m.access.Path(),
		WorktreeMode:         strings.ToLower(m.cfg.Worktrees.Mode),
	}
	if !m.cfg.MultiProject {
		info := WorkspaceInfo{
			Mode:              "project",
			AccessRoot:        m.access.Path(),
			SourceProjectRoot: m.access.Path(),
			ProjectRoot:       m.access.Path(),
			WorktreeMode:      strings.ToLower(m.cfg.Worktrees.Mode),
			BindingScope:      "single_project",
		}
		status.Selected = true
		status.Workspace = &info
		return status, nil
	}
	identity := IdentityFromMeta(meta)
	if identity == nil {
		return status, nil
	}
	change, err := m.readSwitch(identity)
	if err != nil {
		return StatusInfo{}, err
	}
	if change != nil && change.AwaitingSelection {
		status.AwaitingSelection = true
		return status, nil
	}
	binding, err := m.readBinding(identity)
	if err != nil {
		return StatusInfo{}, err
	}
	if binding != nil {
		info := infoFromBinding(m, *binding, false)
		if info.Mode == "group" {
			m.decorateGroupStatus(&info)
		}
		status.Selected = true
		status.Workspace = &info
	}
	return status, nil
}

func (m *Manager) Workspace(meta map[string]any) (*workspace.Root, WorkspaceInfo, error) {
	return m.WorkspaceFor(meta, "")
}

// WorkspaceFor resolves exactly one validated member before the handler
// performs any file, shell or Git operation. Explicit routing never changes
// the persisted active alias. Legacy chats accept only an empty alias.
func (m *Manager) WorkspaceFor(meta map[string]any, repository string) (*workspace.Root, WorkspaceInfo, error) {
	if !m.cfg.MultiProject {
		if repository != "" {
			return nil, WorkspaceInfo{}, errors.New("repository alias requires a project group binding")
		}
		return m.access, WorkspaceInfo{
			Mode:              "project",
			AccessRoot:        m.access.Path(),
			SourceProjectRoot: m.access.Path(),
			ProjectRoot:       m.access.Path(),
			WorktreeMode:      strings.ToLower(m.cfg.Worktrees.Mode),
			BindingScope:      "single_project",
		}, nil
	}
	identity := IdentityFromMeta(meta)
	if identity == nil {
		return nil, WorkspaceInfo{}, errors.New("no ChatGPT conversation or MCP transport-session identity is available; call set_project_root from a stateful MCP session or disable multiProject")
	}
	binding, err := m.readBinding(identity)
	if err != nil {
		return nil, WorkspaceInfo{}, err
	}
	if binding == nil {
		return nil, WorkspaceInfo{}, errors.New("no project selected for this conversation; call list_projects then set_project_root")
	}
	if binding.Mode == "group" {
		alias := binding.ActiveRepository
		if strings.TrimSpace(repository) != "" {
			alias = strings.ToLower(strings.TrimSpace(repository))
		}
		member, ok := boundGroupMember(*binding, alias)
		if !ok {
			return nil, WorkspaceInfo{}, fmt.Errorf("unknown repository alias %q", repository)
		}
		root, err := workspace.New(member.ProjectRoot)
		if err != nil {
			return nil, WorkspaceInfo{}, err
		}
		info := infoFromBinding(m, *binding, false)
		info.SourceProjectRoot = member.SourceProjectRoot
		info.ProjectRoot = member.ProjectRoot
		info.ManagedWorktree = member.ManagedWorktree
		info.WorktreeGitRoot = member.WorktreeGitRoot
		info.EffectiveRepository = member.Alias
		info.EffectiveCheckoutRevision = member.CheckoutRevision
		return root, info, nil
	}
	if repository != "" {
		return nil, WorkspaceInfo{}, errors.New("repository alias requires a project group binding")
	}
	root, err := workspace.New(binding.ProjectRoot)
	if err != nil {
		return nil, WorkspaceInfo{}, err
	}
	return root, infoFromBinding(m, *binding, false), nil
}

func (m *Manager) Select(meta map[string]any, selector string, createWorktree *bool) (WorkspaceInfo, error) {
	identity := IdentityFromMeta(meta)
	if m.cfg.MultiProject && identity == nil {
		return WorkspaceInfo{}, errors.New("set_project_root requires a ChatGPT conversation identity or stateful MCP transport session in multi-project mode")
	}
	if !m.cfg.MultiProject {
		return WorkspaceInfo{
			Mode:              "project",
			AccessRoot:        m.access.Path(),
			SourceProjectRoot: m.access.Path(),
			ProjectRoot:       m.access.Path(),
			WorktreeMode:      strings.ToLower(m.cfg.Worktrees.Mode),
			BindingScope:      "single_project",
		}, nil
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return WorkspaceInfo{}, errors.New("selector is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, err := m.readBinding(identity)
	if err != nil {
		return WorkspaceInfo{}, err
	}
	if existing != nil && existing.Mode == "group" {
		return WorkspaceInfo{}, errors.New("conversation is bound to a project group; use set_active_repository or explicitly switch the workspace")
	}

	var (
		source              string
		repositoryURL       string
		checkoutCommit      string
		cloned              bool
		targetNeedsWorktree bool
	)
	if LooksLikeRepositoryURL(selector) {
		reference, parseErr := ParseRepositoryReference(selector)
		if parseErr != nil {
			return WorkspaceInfo{}, parseErr
		}
		if existing != nil {
			if bindingMatchesReference(*existing, reference) {
				if modeErr := checkExistingWorktreePreference(*existing, createWorktree); modeErr != nil {
					return WorkspaceInfo{}, modeErr
				}
				return infoFromBinding(m, *existing, false), nil
			}
			return WorkspaceInfo{}, fmt.Errorf("this conversation is already bound to %s; use switch_project_root before selecting another workspace", existing.ProjectRoot)
		}
		resolved, resolveErr := m.materializeRepository(reference)
		if resolveErr != nil {
			return WorkspaceInfo{}, resolveErr
		}
		source = resolved.SourceProjectRoot
		repositoryURL = resolved.RepositoryURL
		checkoutCommit = resolved.CheckoutCommit
		cloned = resolved.Cloned
		targetNeedsWorktree = checkoutCommit != "" && !resolved.SourceMatches
	} else {
		var resolveErr error
		source, resolveErr = m.access.Resolve(filepath.FromSlash(selector), false)
		if resolveErr != nil {
			return WorkspaceInfo{}, fmt.Errorf("select project: %w", resolveErr)
		}
		info, statErr := os.Stat(source)
		if statErr != nil || !info.IsDir() {
			return WorkspaceInfo{}, errors.New("selected project must be an existing directory beneath the access root")
		}
		if existing != nil {
			if cleanComparable(existing.SourceProjectRoot) != cleanComparable(source) {
				return WorkspaceInfo{}, fmt.Errorf("this conversation is already bound to %s; use switch_project_root before selecting another workspace", existing.ProjectRoot)
			}
			if modeErr := checkExistingWorktreePreference(*existing, createWorktree); modeErr != nil {
				return WorkspaceInfo{}, modeErr
			}
			return infoFromBinding(m, *existing, false), nil
		}
	}

	gitRoot, gitErr := gitTopLevel(source)
	hasGit := gitErr == nil
	mode := strings.ToLower(m.cfg.Worktrees.Mode)
	wantWorktree := false
	if createWorktree != nil {
		wantWorktree = *createWorktree
	} else {
		switch mode {
		case "always":
			wantWorktree = hasGit
		case "auto":
			wantWorktree = hasGit && m.sourceInUse(source, identity)
		}
	}
	if targetNeedsWorktree {
		if createWorktree != nil && !*createWorktree {
			return WorkspaceInfo{}, errors.New("the requested Git target is not the current source checkout and createWorktree=false forbids the required isolated worktree")
		}
		if createWorktree == nil && mode == "never" {
			return WorkspaceInfo{}, errors.New("the requested Git target is not the current source checkout and worktree isolation is disabled")
		}
		wantWorktree = true
	}
	if wantWorktree && !hasGit {
		return WorkspaceInfo{}, errors.New("managed worktree requested but selected project is not inside a Git repository")
	}

	activeRoot := source
	managed := false
	worktreeGitRoot := ""
	if wantWorktree {
		target := "HEAD"
		if targetNeedsWorktree {
			target = checkoutCommit
		}
		created, err := m.createManagedWorktreeAt(identity, source, gitRoot, target)
		if err != nil {
			return WorkspaceInfo{}, err
		}
		activeRoot = created.ProjectRoot
		managed = true
		worktreeGitRoot = created.GitRoot
	}

	binding := Binding{
		Version:           bindingVersion,
		IdentityHash:      identity.Key,
		Scope:             identity.Scope,
		Mode:              "project",
		SourceProjectRoot: source,
		ProjectRoot:       activeRoot,
		RepositoryURL:     repositoryURL,
		CheckoutCommit:    checkoutCommit,
		Cloned:            cloned,
		ManagedWorktree:   managed,
		WorktreeGitRoot:   worktreeGitRoot,
		WorktreesRoot:     m.cfg.Worktrees.Root,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := m.writeBinding(identity, binding); err != nil {
		if managed {
			_ = removeManagedWorktree(gitRoot, worktreeGitRoot)
		}
		return WorkspaceInfo{}, err
	}
	m.finishSwitch(identity)
	return infoFromBinding(m, binding, true), nil
}

// A binding cannot change checkout type through an idempotent Select call.
// Users must explicitly archive it using setup_ui_switch_project first.
func checkExistingWorktreePreference(binding Binding, requested *bool) error {
	if requested == nil || binding.ManagedWorktree == *requested {
		return nil
	}
	actual := "source checkout"
	if binding.ManagedWorktree {
		actual = "managed worktree"
	}
	return fmt.Errorf("conversation is already bound to %s (%s); use setup_ui_switch_project before changing checkout type", binding.ProjectRoot, actual)
}

func infoFromBinding(m *Manager, binding Binding, newly bool) WorkspaceInfo {
	mode := binding.Mode
	if mode == "" {
		mode = "project"
	}
	scope := binding.Scope
	if scope == "" {
		scope = "chatgpt_conversation"
	}
	info := WorkspaceInfo{
		Mode:              mode,
		AccessRoot:        m.access.Path(),
		SourceProjectRoot: binding.SourceProjectRoot,
		ProjectRoot:       binding.ProjectRoot,
		RepositoryURL:     binding.RepositoryURL,
		CheckoutCommit:    binding.CheckoutCommit,
		Cloned:            binding.Cloned,
		ManagedWorktree:   binding.ManagedWorktree,
		WorktreeGitRoot:   binding.WorktreeGitRoot,
		WorktreesRoot:     binding.WorktreesRoot,
		WorktreeMode:      strings.ToLower(m.cfg.Worktrees.Mode),
		NewlySelected:     newly,
		BindingScope:      scope,
	}
	if mode == "group" {
		member, _ := boundGroupMember(binding, binding.ActiveRepository)
		info.SourceProjectRoot = member.SourceProjectRoot
		info.ProjectRoot = member.ProjectRoot
		info.ManagedWorktree = member.ManagedWorktree
		info.WorktreeGitRoot = member.WorktreeGitRoot
		info.GroupName = binding.GroupName
		info.PrimaryRepository = binding.PrimaryRepository
		info.ActiveRepository = binding.ActiveRepository
		info.EffectiveRepository = binding.ActiveRepository
		info.MembershipFingerprint = binding.MembershipFingerprint
		info.Repositories = append([]GroupBindingRepository(nil), binding.Repositories...)
		for i := range info.Repositories {
			if info.Repositories[i].CheckoutRevision == 0 {
				info.Repositories[i].CheckoutRevision = 1
			}
		}
		info.Generation = binding.Generation
		info.EffectiveCheckoutRevision = member.CheckoutRevision
	}
	info.BindingEpoch = binding.CreatedAt
	return info
}

func (m *Manager) ForgetTransportSession(sessionID string) {
	identity := IdentityFromTransportSession(sessionID)
	if identity == nil {
		return
	}
	m.transientMu.Lock()
	delete(m.transientBindings, identity.Key)
	delete(m.transientChanges, identity.Key)
	delete(m.transientHistory, identity.Key)
	m.transientMu.Unlock()
}

func bindingMatchesReference(binding Binding, reference RepositoryReference) bool {
	if binding.RepositoryURL == "" {
		return reference.CheckoutKind == CheckoutDefault && repositoryMatches(binding.SourceProjectRoot, reference)
	}
	stored, err := ParseRepositoryReference(binding.RepositoryURL)
	if err != nil {
		return false
	}
	return stored.Identity == reference.Identity && stored.CheckoutKind == reference.CheckoutKind && stored.Checkout == reference.Checkout
}
