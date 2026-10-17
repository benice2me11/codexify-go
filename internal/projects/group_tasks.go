package projects

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// RepositoryTaskOutput describes the new isolated checkout and the source
// checkout's state at preparation time. A dirty or outdated source is never
// reset, checked out or merged by this operation.
type RepositoryTaskOutput struct {
	Repository   string        `json:"repository"`
	Branch       string        `json:"branch"`
	BaseRef      string        `json:"baseRef"`
	BaseCommit   string        `json:"baseCommit"`
	WorktreeRoot string        `json:"worktreeRoot"`
	SourceBranch string        `json:"sourceBranch,omitempty"`
	SourceDirty  bool          `json:"sourceDirty"`
	SourceBehind *int          `json:"sourceBehind,omitempty"`
	Workspace    WorkspaceInfo `json:"workspace"`
}

// RepositoryTaskFinishOutput reports whether a previously preserved managed
// worktree was actually removed. The original feature branch is retained.
type RepositoryTaskFinishOutput struct {
	Repository string        `json:"repository"`
	Branch     string        `json:"branch"`
	BaseRef    string        `json:"baseRef"`
	Released   bool          `json:"released"`
	Removed    bool          `json:"removed"`
	Warning    string        `json:"warning,omitempty"`
	Workspace  WorkspaceInfo `json:"workspace"`
}

// PrepareRepositoryTask creates a named feature branch and managed worktree
// starting from an explicitly chosen remote-tracking ref. It does not fetch,
// switch, reset, rebase or write into any existing checkout. The caller should
// fetch the chosen remote first to ensure the ref is current.
func (m *Manager) PrepareRepositoryTask(meta map[string]any, alias, branch, baseRef string) (RepositoryTaskOutput, error) {
	if !m.cfg.ProjectGroupsEnabled || !m.cfg.MultiProject {
		return RepositoryTaskOutput{}, errors.New("repository tasks require enabled project groups")
	}
	identity := IdentityFromMeta(meta)
	if identity == nil {
		return RepositoryTaskOutput{}, errors.New("repository task requires a conversation identity")
	}
	alias = strings.ToLower(strings.TrimSpace(alias))
	branch = strings.TrimSpace(branch)
	if branch == "" || strings.EqualFold(branch, "main") || strings.EqualFold(branch, "master") {
		return RepositoryTaskOutput{}, errors.New("provide a new feature branch name, not main or master")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	binding, err := m.readBinding(identity)
	if err != nil {
		return RepositoryTaskOutput{}, err
	}
	if binding == nil || binding.Mode != "group" {
		return RepositoryTaskOutput{}, errors.New("repository task requires an active project group")
	}
	index := -1
	for i, member := range binding.Repositories {
		if member.Alias == alias {
			index = i
			break
		}
	}
	if index < 0 {
		return RepositoryTaskOutput{}, fmt.Errorf("unknown repository alias %q", alias)
	}
	row := binding.Repositories[index]
	if row.Checkout != "source" || row.PreservedManagedProjectRoot != "" ||
		row.PreservedManagedGitRoot != "" {
		return RepositoryTaskOutput{}, fmt.Errorf("repository %q already has a managed checkout; finish or explicitly return to the existing worktree before starting another task", alias)
	}
	source, err := m.groups.ResolveSource(binding.GroupName, alias)
	if err != nil {
		return RepositoryTaskOutput{}, err
	}
	if err := m.groupWorktreeDirectorySafe(binding.GroupName); err != nil {
		return RepositoryTaskOutput{}, err
	}
	if _, err := gitTaskCommand(source.Member.GitRoot, "check-ref-format", "--branch", branch); err != nil {
		return RepositoryTaskOutput{}, fmt.Errorf("invalid feature branch %q: %w", branch, err)
	}
	ref, err := normalizeTaskBaseRef(baseRef)
	if err != nil {
		return RepositoryTaskOutput{}, err
	}
	// Only remote-tracking refs are accepted, never the possibly dirty or
	// behind local main/master/HEAD of an existing source checkout.
	if _, err := gitTaskCommand(source.Member.GitRoot, "show-ref", "--verify", "--quiet", ref); err != nil {
		return RepositoryTaskOutput{}, fmt.Errorf("base %q is unavailable; fetch the remote and choose an existing remote-tracking branch: %w", ref, err)
	}
	baseCommit, err := gitTaskCommand(source.Member.GitRoot, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return RepositoryTaskOutput{}, fmt.Errorf("cannot resolve base %q to a commit: %w", ref, err)
	}
	if len(baseCommit) < 40 {
		return RepositoryTaskOutput{}, fmt.Errorf("cannot resolve base %q to a full commit", ref)
	}
	dirty, err := gitTaskCommand(source.Member.GitRoot, "status", "--porcelain")
	if err != nil {
		return RepositoryTaskOutput{}, fmt.Errorf("inspect source Git status: %w", err)
	}
	sourceBranch := readGitBranch(source.Member.GitRoot)
	var behind *int
	if count, countErr := gitTaskCommand(source.Member.GitRoot, "rev-list", "--count", "HEAD..@{upstream}"); countErr == nil {
		if parsed, parseErr := strconv.Atoi(count); parseErr == nil {
			behind = &parsed
		}
	}
	created, err := m.createManagedWorktreeOnBranch(identity,
		source.Member.SourceProjectRoot, source.Member.GitRoot, baseCommit, branch)
	if err != nil {
		return RepositoryTaskOutput{}, err
	}
	// readBinding can hold a shared slice for transient sessions. The new
	// binding must not alter it before the durable write succeeds.
	binding.Repositories = append([]GroupBindingRepository(nil), binding.Repositories...)
	current := &binding.Repositories[index]
	current.ProjectRoot = created.ProjectRoot
	current.WorktreeGitRoot = created.GitRoot
	current.Checkout = "managed"
	current.ManagedWorktree = true
	current.CheckoutRevision++
	if current.CheckoutRevision < 2 {
		current.CheckoutRevision = 2
	}
	binding.Generation++
	primary, _ := boundGroupMember(*binding, binding.PrimaryRepository)
	binding.SourceProjectRoot = primary.SourceProjectRoot
	binding.ProjectRoot = primary.ProjectRoot
	binding.ManagedWorktree = primary.ManagedWorktree
	binding.WorktreeGitRoot = primary.WorktreeGitRoot
	if err := m.validateGroupBinding(binding); err != nil {
		_ = removeManagedWorktree(source.Member.GitRoot, created.GitRoot)
		return RepositoryTaskOutput{}, err
	}
	if err := m.writeBinding(identity, *binding); err != nil {
		_ = removeManagedWorktree(source.Member.GitRoot, created.GitRoot)
		return RepositoryTaskOutput{}, err
	}
	return RepositoryTaskOutput{
		Repository: alias, Branch: created.Branch, BaseRef: ref,
		BaseCommit: baseCommit, WorktreeRoot: created.ProjectRoot,
		SourceBranch: sourceBranch, SourceDirty: dirty != "",
		SourceBehind: behind, Workspace: infoFromBinding(m, *binding, false),
	}, nil
}

// FinishRepositoryTask releases and removes a preserved managed worktree.
// This is intentionally a second step after an explicit managed->source
// checkout switch, not an automatic side effect of merging or selecting a
// different task. Only clean and fully merged worktrees qualify. Squash merges
// require manual review because Git ancestry cannot prove their integration.
func (m *Manager) FinishRepositoryTask(meta map[string]any, alias, baseRef string) (RepositoryTaskFinishOutput, error) {
	if !m.cfg.ProjectGroupsEnabled || !m.cfg.MultiProject {
		return RepositoryTaskFinishOutput{}, errors.New("repository tasks require enabled project groups")
	}
	identity := IdentityFromMeta(meta)
	if identity == nil {
		return RepositoryTaskFinishOutput{}, errors.New("repository task requires a conversation identity")
	}
	alias = strings.ToLower(strings.TrimSpace(alias))
	m.mu.Lock()
	defer m.mu.Unlock()
	binding, err := m.readBinding(identity)
	if err != nil {
		return RepositoryTaskFinishOutput{}, err
	}
	if binding == nil || binding.Mode != "group" {
		return RepositoryTaskFinishOutput{}, errors.New("repository task requires an active project group")
	}
	index := -1
	for i, row := range binding.Repositories {
		if row.Alias == alias {
			index = i
			break
		}
	}
	if index < 0 {
		return RepositoryTaskFinishOutput{}, fmt.Errorf("unknown repository alias %q", alias)
	}
	member := binding.Repositories[index]
	if member.Checkout != "source" || member.PreservedManagedGitRoot == "" ||
		member.PreservedManagedProjectRoot == "" {
		return RepositoryTaskFinishOutput{}, errors.New("first select the source checkout to preserve the managed task before finishing it")
	}
	source, err := m.groups.ResolveSource(binding.GroupName, alias)
	if err != nil {
		return RepositoryTaskFinishOutput{}, err
	}
	if err := m.validateManagedCheckout(member.PreservedManagedProjectRoot,
		member.PreservedManagedGitRoot, source.Member.GitRoot); err != nil {
		return RepositoryTaskFinishOutput{}, err
	}
	ref, err := normalizeTaskBaseRef(baseRef)
	if err != nil {
		return RepositoryTaskFinishOutput{}, err
	}
	if _, err := gitTaskCommand(source.Member.GitRoot, "show-ref", "--verify", "--quiet", ref); err != nil {
		return RepositoryTaskFinishOutput{}, fmt.Errorf("target base %q is unavailable; fetch first: %w", ref, err)
	}
	preservedRoot := member.PreservedManagedGitRoot
	branch := readGitBranch(preservedRoot)
	changes, err := gitTaskCommand(preservedRoot, "status", "--porcelain")
	if err != nil {
		return RepositoryTaskFinishOutput{}, err
	}
	if changes != "" {
		return RepositoryTaskFinishOutput{}, errors.New("managed worktree has uncommitted changes; nothing was deleted")
	}
	ignored, err := gitTaskCommand(preservedRoot, "ls-files", "--others", "--ignored", "--exclude-standard")
	if err != nil {
		return RepositoryTaskFinishOutput{}, fmt.Errorf("inspect ignored worktree files: %w", err)
	}
	if ignored != "" {
		return RepositoryTaskFinishOutput{}, errors.New("managed worktree contains ignored files; review and remove them explicitly before cleanup")
	}
	head, err := gitTaskCommand(preservedRoot, "rev-parse", "HEAD")
	if err != nil {
		return RepositoryTaskFinishOutput{}, err
	}
	if _, err := gitTaskCommand(preservedRoot, "merge-base", "--is-ancestor", head, ref); err != nil {
		return RepositoryTaskFinishOutput{}, fmt.Errorf("managed task HEAD is not an ancestor of %s; merge and fetch before cleanup, or review squash merges manually", ref)
	}
	binding.Repositories = append([]GroupBindingRepository(nil), binding.Repositories...)
	row := &binding.Repositories[index]
	row.PreservedManagedProjectRoot = ""
	row.PreservedManagedGitRoot = ""
	row.CheckoutRevision++
	binding.Generation++
	if err := m.validateGroupBinding(binding); err != nil {
		return RepositoryTaskFinishOutput{}, err
	}
	if err := m.writeBinding(identity, *binding); err != nil {
		return RepositoryTaskFinishOutput{}, err
	}
	// Persist the safe source binding BEFORE removing the clean worktree so a
	// filesystem/permission failure cannot leave a binding pointing to a
	// deleted worktree. A Git removal failure leaves an orphan for operator
	// inspection; it must never trigger a forced removal or branch deletion.
	out := RepositoryTaskFinishOutput{
		Repository: alias, Branch: branch, BaseRef: ref, Released: true,
		Workspace: infoFromBinding(m, *binding, false),
	}
	command := exec.Command("git", "-C", source.Member.GitRoot, "worktree", "remove", preservedRoot)
	if message, removeErr := command.CombinedOutput(); removeErr != nil {
		detail := strings.TrimSpace(string(message))
		if len(detail) > 512 {
			detail = detail[:512]
		}
		out.Warning = "checkout released, but clean worktree removal failed; inspect git worktree list: " +
			detail
		return out, nil
	}
	out.Removed = true
	return out, nil
}

// normalizeTaskBaseRef requires an explicit remote-tracking branch. It does
// not guess main/master from the source checkout's possibly outdated HEAD.
func normalizeTaskBaseRef(value string) (string, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "refs/remotes/")
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" ||
		strings.HasPrefix(parts[0], "-") || strings.Contains(value, "..") ||
		strings.ContainsAny(value, " \\~^:?*[]") || strings.Contains(value, "@{") {
		return "", errors.New("baseRef must identify a remote-tracking branch, e.g. origin/main or origin/master")
	}
	return "refs/remotes/" + value, nil
}

func gitTaskCommand(root string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		// Limit the diagnostic: Git output may include server and remote data.
		message := strings.TrimSpace(string(out))
		if len(message) > 512 {
			message = message[:512]
		}
		return "", fmt.Errorf("%w: %s", err, message)
	}
	return strings.TrimSpace(string(out)), nil
}

func readGitBranch(root string) string {
	if branch, err := gitTaskCommand(root, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
		return branch
	}
	if commit, err := gitTaskCommand(root, "rev-parse", "--short", "HEAD"); err == nil {
		return commit + " (detached)"
	}
	return ""
}

// Decorate only UI/status responses. WorkspaceFor (called on every file/exec
// tool) must not start extra Git processes for every member.
func (m *Manager) decorateGroupStatus(info *WorkspaceInfo) {
	for i := range info.Repositories {
		member := &info.Repositories[i]
		member.SourceBranch = readGitBranch(member.SourceProjectRoot)
		if member.ManagedWorktree {
			member.Branch = readGitBranch(member.ProjectRoot)
		} else {
			member.Branch = member.SourceBranch
		}
	}
}
