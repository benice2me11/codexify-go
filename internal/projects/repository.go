package projects

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/benice2me11/codexify-go/internal/workspace"
)

type CheckoutKind string

const (
	CheckoutDefault CheckoutKind = "default"
	CheckoutBranch  CheckoutKind = "branch"
	CheckoutPR      CheckoutKind = "pull_request"
	CheckoutCommit  CheckoutKind = "commit"
)

type RepositoryReference struct {
	Name         string
	Identity     string
	WebURL       string
	CloneURL     string
	CheckoutKind CheckoutKind
	Checkout     string
}

type ResolvedRepository struct {
	SourceProjectRoot string
	RepositoryURL     string
	CheckoutCommit    string
	SourceMatches     bool
	Cloned            bool
	Reference         RepositoryReference
}

func ParseRepositoryReference(input string) (RepositoryReference, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return RepositoryReference{}, errors.New("repository URL must be non-empty")
	}
	if strings.IndexFunc(input, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return RepositoryReference{}, errors.New("Git repository URL contains control characters")
	}
	if strings.ContainsAny(input, "?#") {
		return RepositoryReference{}, errors.New("Git repository URL must not contain a query or fragment")
	}
	lower := strings.ToLower(input)
	if strings.HasPrefix(lower, "git@github.com:") {
		path := input[len("git@github.com:"):]
		return githubReference(path, "git@github.com:", false)
	}
	if strings.Contains(input, "://") {
		u, err := url.Parse(input)
		if err != nil {
			return RepositoryReference{}, fmt.Errorf("invalid Git repository URL: %w", err)
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "https" && scheme != "ssh" {
			return RepositoryReference{}, errors.New("only HTTPS or SSH Git repository URLs are supported")
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return RepositoryReference{}, errors.New("Git repository URL must not contain a query or fragment")
		}
		if scheme == "https" && u.User != nil {
			return RepositoryReference{}, errors.New("credential-bearing HTTPS Git URLs are rejected; use a credential helper or SSH URL")
		}
		host := strings.ToLower(u.Hostname())
		if host == "" {
			return RepositoryReference{}, errors.New("Git repository URL is missing a host")
		}
		if host == "github.com" || host == "www.github.com" || host == "ssh.github.com" {
			if scheme == "https" {
				if u.Port() != "" && u.Port() != "443" {
					return RepositoryReference{}, errors.New("GitHub HTTPS URL has an unsupported port")
				}
				path := strings.Trim(u.Path, "/")
				return githubReference(path, "https://github.com/", true)
			}
			user := ""
			if u.User != nil {
				user = u.User.Username()
			}
			if user != "git" {
				return RepositoryReference{}, errors.New("GitHub SSH URLs must use the git user")
			}
			if host == "ssh.github.com" && u.Port() != "443" {
				return RepositoryReference{}, errors.New("ssh.github.com must use port 443")
			}
			path := strings.Trim(u.Path, "/")
			prefix := "git@github.com:"
			if host == "ssh.github.com" {
				prefix = "ssh://git@ssh.github.com:443/"
			}
			return githubReference(path, prefix, false)
		}
		path := strings.Trim(u.Path, "/")
		return genericReference(scheme, u.Host, path, input)
	}

	at, rest, ok := strings.Cut(input, ":")
	if !ok || !strings.Contains(at, "@") || strings.ContainsAny(at, "/\\") {
		return RepositoryReference{}, errors.New("SSH Git repository URLs without a scheme must use user@host:path.git syntax")
	}
	user, host, ok := strings.Cut(at, "@")
	if !ok || !validSSHUser(user) || !validHost(host) {
		return RepositoryReference{}, errors.New("invalid SSH Git repository authority")
	}
	path := strings.Trim(rest, "/")
	return genericReference("ssh", at, path, input)
}

func githubReference(path, clonePrefix string, allowTarget bool) (RepositoryReference, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return RepositoryReference{}, errors.New("GitHub repository URL must identify an owner and repository")
	}
	owner := parts[0]
	name := strings.TrimSuffix(parts[1], ".git")
	if !validGitHubComponent(owner) || !validGitHubComponent(name) {
		return RepositoryReference{}, errors.New("GitHub owner or repository name is invalid")
	}
	ref := RepositoryReference{
		Name:         name,
		Identity:     strings.ToLower(owner + "/" + name),
		WebURL:       "https://github.com/" + owner + "/" + name,
		CheckoutKind: CheckoutDefault,
	}
	if strings.HasPrefix(clonePrefix, "https://") {
		ref.CloneURL = "https://github.com/" + owner + "/" + name + ".git"
	} else if strings.HasPrefix(clonePrefix, "ssh://") {
		ref.CloneURL = clonePrefix + owner + "/" + name + ".git"
	} else {
		ref.CloneURL = "git@github.com:" + owner + "/" + name + ".git"
	}
	tail := parts[2:]
	if len(tail) == 0 {
		return ref, nil
	}
	if !allowTarget {
		return RepositoryReference{}, errors.New("targeted GitHub branch/PR/commit selection requires an HTTPS GitHub URL")
	}
	switch {
	case len(tail) >= 2 && tail[0] == "tree":
		branch, err := url.PathUnescape(strings.Join(tail[1:], "/"))
		if err != nil || !validBranchName(branch) {
			return RepositoryReference{}, errors.New("GitHub branch URL contains an invalid Git branch name")
		}
		ref.CheckoutKind = CheckoutBranch
		ref.Checkout = branch
		ref.WebURL += "/tree/" + escapeBranch(branch)
	case len(tail) == 2 && tail[0] == "pull":
		n, err := strconv.ParseUint(tail[1], 10, 64)
		if err != nil || n == 0 {
			return RepositoryReference{}, errors.New("GitHub pull-request URL has an invalid PR number")
		}
		ref.CheckoutKind = CheckoutPR
		ref.Checkout = tail[1]
		ref.WebURL += "/pull/" + tail[1]
	case len(tail) == 2 && tail[0] == "commit":
		commit := strings.ToLower(tail[1])
		if len(commit) != 40 || strings.IndexFunc(commit, func(r rune) bool {
			return !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f'))
		}) >= 0 {
			return RepositoryReference{}, errors.New("GitHub commit URL must contain a full 40-character hexadecimal commit ID")
		}
		ref.CheckoutKind = CheckoutCommit
		ref.Checkout = commit
		ref.WebURL += "/commit/" + commit
	default:
		return RepositoryReference{}, errors.New("supported GitHub URLs identify a repository root, /tree/<branch>, /pull/<number>, or /commit/<sha>")
	}
	return ref, nil
}

func genericReference(scheme, authority, path, cloneURL string) (RepositoryReference, error) {
	if path == "" || strings.Contains(path, "\\") {
		return RepositoryReference{}, errors.New("Git repository URL has an invalid repository path")
	}
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return RepositoryReference{}, errors.New("Git repository URL has an invalid repository path")
		}
	}
	last := parts[len(parts)-1]
	if !strings.HasSuffix(strings.ToLower(last), ".git") {
		return RepositoryReference{}, errors.New("non-GitHub repository URLs must end in .git")
	}
	name := last[:len(last)-4]
	if name == "" || strings.ContainsAny(name, "/\\") {
		return RepositoryReference{}, errors.New("Git repository URL has an invalid repository name")
	}
	if scheme == "https" && strings.Contains(authority, "@") {
		return RepositoryReference{}, errors.New("credential-bearing HTTPS Git URLs are rejected")
	}
	identityAuthority := strings.ToLower(authority)
	if scheme == "ssh" {
		if user, host, ok := strings.Cut(authority, "@"); ok && strings.EqualFold(user, "git") {
			identityAuthority = strings.ToLower(host)
		}
	}
	identityPath := strings.Join(append(parts[:len(parts)-1], name), "/")
	return RepositoryReference{
		Name:         name,
		Identity:     identityAuthority + "/" + strings.ToLower(identityPath),
		WebURL:       cloneURL,
		CloneURL:     cloneURL,
		CheckoutKind: CheckoutDefault,
	}, nil
}

func LooksLikeRepositoryURL(input string) bool {
	value := strings.TrimSpace(strings.ToLower(input))
	return strings.HasPrefix(value, "https://") ||
		strings.HasPrefix(value, "ssh://") ||
		strings.HasPrefix(value, "git@") ||
		(strings.Contains(value, "@") && strings.Contains(value, ":"))
}

func (m *Manager) materializeRepository(ref RepositoryReference) (ResolvedRepository, error) {
	cloneDir, err := m.pathInsideAccess(m.cfg.CloneDir)
	if err != nil {
		return ResolvedRepository{}, fmt.Errorf("cloneDir: %w", err)
	}
	if err := os.MkdirAll(cloneDir, 0o700); err != nil {
		return ResolvedRepository{}, err
	}

	candidates, err := m.repositoryCandidates(ref, cloneDir)
	if err != nil {
		return ResolvedRepository{}, err
	}
	if len(candidates) > 1 {
		return ResolvedRepository{}, fmt.Errorf("multiple local checkouts match %s; select an explicit project path instead", ref.WebURL)
	}

	source := ""
	cloned := false
	if len(candidates) == 1 {
		source = candidates[0]
	} else {
		target := filepath.Join(cloneDir, ref.Name)
		if _, err := os.Stat(target); err == nil {
			return ResolvedRepository{}, fmt.Errorf("cannot clone %s because destination %q already exists and is not a matching checkout", ref.WebURL, target)
		}
		source, err = cloneRepository(ref, cloneDir, target)
		if err != nil {
			return ResolvedRepository{}, err
		}
		cloned = true
	}

	commit := ""
	sourceMatches := true
	if ref.CheckoutKind != CheckoutDefault {
		commit, err = fetchCheckoutCommit(source, ref)
		if err != nil {
			return ResolvedRepository{}, err
		}
		head, headErr := gitOutput(source, "rev-parse", "HEAD")
		sourceMatches = headErr == nil && strings.EqualFold(strings.TrimSpace(head), commit)
	}

	return ResolvedRepository{
		SourceProjectRoot: source,
		RepositoryURL:     ref.WebURL,
		CheckoutCommit:    commit,
		SourceMatches:     sourceMatches,
		Cloned:            cloned,
		Reference:         ref,
	}, nil
}

func (m *Manager) repositoryCandidates(ref RepositoryReference, cloneDir string) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	add := func(path string) {
		path = filepath.Clean(path)
		key := strings.ToLower(path)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		if repositoryMatches(path, ref) {
			out = append(out, path)
		}
	}

	target := filepath.Join(cloneDir, ref.Name)
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		add(target)
	}
	projects, err := m.discoverProjects()
	if err == nil {
		for _, project := range projects {
			path, resolveErr := m.access.Resolve(filepath.FromSlash(project.Selector), false)
			if resolveErr == nil {
				add(path)
			}
		}
	}
	return out, nil
}

func repositoryMatches(path string, ref RepositoryReference) bool {
	gitRoot, err := gitTopLevel(path)
	if err != nil {
		return false
	}
	cmd := exec.Command("git", "-C", gitRoot, "remote", "-v")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		candidate, err := ParseRepositoryReference(fields[1])
		if err == nil && candidate.Identity == ref.Identity {
			return true
		}
	}
	return false
}

func cloneRepository(ref RepositoryReference, cloneDir, target string) (string, error) {
	staging := filepath.Join(cloneDir, ".codexify-go-clone-"+randomSuffix())
	repo := filepath.Join(staging, "repository")
	if err := os.MkdirAll(staging, 0o700); err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)

	args := []string{"clone", "--no-recurse-submodules"}
	if ref.CheckoutKind == CheckoutBranch {
		args = append(args, "--branch", ref.Checkout)
	}
	args = append(args, "--", ref.CloneURL, repo)
	cmd := exec.Command("git", args...)
	cmd.Dir = cloneDir
	cmd.Env = append(os.Environ(),
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=Never",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git clone for %s failed: %w: %s", ref.WebURL, err, strings.TrimSpace(stderr.String()))
	}
	if err := os.Rename(repo, target); err != nil {
		return "", fmt.Errorf("publish cloned repository: %w", err)
	}
	return target, nil
}

func fetchCheckoutCommit(source string, ref RepositoryReference) (string, error) {
	var fetchRef string
	switch ref.CheckoutKind {
	case CheckoutBranch:
		fetchRef = "refs/heads/" + ref.Checkout
	case CheckoutPR:
		fetchRef = "refs/pull/" + ref.Checkout + "/head"
	case CheckoutCommit:
		fetchRef = ref.Checkout
	default:
		return "", nil
	}
	cmd := exec.Command("git", "-C", source, "fetch", "--no-tags", "origin", fetchRef)
	cmd.Env = append(os.Environ(),
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=Never",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("fetch target for %s failed: %w: %s", ref.WebURL, err, strings.TrimSpace(stderr.String()))
	}
	out, err := gitOutput(source, "rev-parse", "FETCH_HEAD^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (m *Manager) pathInsideAccess(path string) (string, error) {
	if path == "" {
		return "", errors.New("path is empty")
	}
	abs, err := workspace.Canonical(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(m.access.Path(), abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("path resolves outside access root")
	}
	resolved, err := m.access.Resolve(rel, true)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func validGitHubComponent(value string) bool {
	if value == "" || len(value) > 100 || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func validSSHUser(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func validHost(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func validBranchName(branch string) bool {
	if branch == "" || len(branch) > 1024 || branch == "@" || strings.HasPrefix(branch, "-") || strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") || strings.HasSuffix(branch, ".") || strings.Contains(branch, "//") || strings.Contains(branch, "..") || strings.Contains(branch, "@{") {
		return false
	}
	for _, component := range strings.Split(branch, "/") {
		if strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".lock") {
			return false
		}
	}
	for _, r := range branch {
		if r < 0x20 || r == 0x7f || r == ' ' || strings.ContainsRune("~^:?*[\\", r) {
			return false
		}
	}
	return true
}

func escapeBranch(branch string) string {
	segments := strings.Split(branch, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
