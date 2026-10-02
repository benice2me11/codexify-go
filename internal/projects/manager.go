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
	mu     sync.Mutex

	transientMu       sync.RWMutex
	transientBindings map[string]Binding
	transientChanges  map[string]WorkspaceChange
	transientHistory  map[string][]Binding
}

type WorkspaceInfo struct {
	Mode              string `json:"mode"`
	AccessRoot        string `json:"accessRoot"`
	SourceProjectRoot string `json:"sourceProjectRoot"`
	ProjectRoot       string `json:"projectRoot"`
	RepositoryURL     string `json:"repositoryUrl,omitempty"`
	CheckoutCommit    string `json:"checkoutCommit,omitempty"`
	Cloned            bool   `json:"cloned,omitempty"`
	ManagedWorktree   bool   `json:"managedWorktree"`
	WorktreeGitRoot   string `json:"worktreeGitRoot,omitempty"`
	WorktreesRoot     string `json:"worktreesRoot,omitempty"`
	WorktreeMode      string `json:"worktreeMode"`
	NewlySelected     bool   `json:"newlySelected"`
	BindingScope      string `json:"bindingScope"`
}

type StatusInfo struct {
	MultiProject      bool           `json:"multiProject"`
	AccessRoot        string         `json:"accessRoot"`
	WorktreeMode      string         `json:"worktreeMode"`
	Selected          bool           `json:"selected"`
	AwaitingSelection bool           `json:"awaitingSelection"`
	Workspace         *WorkspaceInfo `json:"workspace,omitempty"`
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
	if strings.TrimSpace(cfg.Worktrees.Mode) == "" {
		cfg.Worktrees.Mode = "auto"
	}
	return &Manager{
		cfg:               cfg,
		access:            access,
		transientBindings: make(map[string]Binding),
		transientChanges:  make(map[string]WorkspaceChange),
		transientHistory:  make(map[string][]Binding),
	}, nil
}

func (m *Manager) AccessRoot() *workspace.Root {
	return m.access
}

func (m *Manager) Status(meta map[string]any) (StatusInfo, error) {
	status := StatusInfo{
		MultiProject: m.cfg.MultiProject,
		AccessRoot:   m.access.Path(),
		WorktreeMode: strings.ToLower(m.cfg.Worktrees.Mode),
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
		status.Selected = true
		status.Workspace = &info
	}
	return status, nil
}

func (m *Manager) Workspace(meta map[string]any) (*workspace.Root, WorkspaceInfo, error) {
	if !m.cfg.MultiProject {
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

func infoFromBinding(m *Manager, binding Binding, newly bool) WorkspaceInfo {
	mode := binding.Mode
	if mode == "" {
		mode = "project"
	}
	scope := binding.Scope
	if scope == "" {
		scope = "chatgpt_conversation"
	}
	return WorkspaceInfo{
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
