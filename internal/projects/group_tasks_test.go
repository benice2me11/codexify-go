package projects

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareRepositoryTaskStartsAtFreshRemoteWithoutTouchingDirtySource(t *testing.T) {
	root, cfg := projectGroupFixture(t)
	source := filepath.Join(root, "work-search")
	runGit(t, source, "branch", "-M", "main")
	remote := filepath.Join(root, "upstream.git")
	runGit(t, source, "init", "--bare", remote)
	runGit(t, source, "remote", "add", "origin", remote)
	runGit(t, source, "push", "-u", "origin", "main")

	publisher := filepath.Join(root, "publisher")
	runGit(t, source, "clone", "--branch", "main", remote, publisher)
	runGit(t, publisher, "-c", "user.name=Codexify Test", "-c", "user.email=test@example.invalid",
		"commit", "--allow-empty", "-m", "remote ahead")
	runGit(t, publisher, "push", "origin", "main")
	runGit(t, source, "fetch", "origin")

	sourceREADME := filepath.Join(source, "README.md")
	dirty := []byte("# source dirty, must stay untouched\n")
	if err := os.WriteFile(sourceREADME, dirty, 0o644); err != nil {
		t.Fatal(err)
	}
	meta := map[string]any{"openai/session": "model-first-prepare"}
	manager, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SelectGroup(meta, "job-search"); err != nil {
		t.Fatal(err)
	}
	remoteCommit, err := gitTaskCommand(source, "rev-parse", "refs/remotes/origin/main")
	if err != nil {
		t.Fatal(err)
	}
	sourceCommit, err := gitTaskCommand(source, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if sourceCommit == remoteCommit {
		t.Fatal("test fixture did not create a behind source checkout")
	}

	created, err := manager.PrepareRepositoryTask(meta, "work", "feat/model-first-work", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if created.BaseRef != "refs/remotes/origin/main" || created.BaseCommit != remoteCommit ||
		created.Branch != "feat/model-first-work" || created.SourceBranch != "main" ||
		created.SourceBehind == nil || *created.SourceBehind != 1 || !created.SourceDirty {
		t.Fatalf("task did not report source/upstream state: %+v", created)
	}
	head, err := gitTaskCommand(created.WorktreeRoot, "rev-parse", "HEAD")
	if err != nil || head != remoteCommit || readGitBranch(created.WorktreeRoot) != created.Branch {
		t.Fatalf("new managed worktree not based on fresh remote: HEAD=%q branch=%q err=%v",
			head, readGitBranch(created.WorktreeRoot), err)
	}
	if content, err := os.ReadFile(sourceREADME); err != nil || string(content) != string(dirty) {
		t.Fatalf("source dirty files overwritten: %q %v", content, err)
	}
	if readGitBranch(source) != "main" {
		t.Fatal("source branch was switched")
	}
	sourceHEADAfter, err := gitTaskCommand(source, "rev-parse", "HEAD")
	if err != nil || sourceHEADAfter != sourceCommit {
		t.Fatalf("source HEAD was modified: %q %v", sourceHEADAfter, err)
	}
	if created.Workspace.ActiveRepository != "work" || created.Workspace.ManagedWorktree != true {
		t.Fatalf("binding not updated for task checkout: %+v", created.Workspace)
	}
	status, err := manager.Status(meta)
	if err != nil || status.Workspace == nil {
		t.Fatal(err)
	}
	member, found := boundGroupMember(Binding{Repositories: status.Workspace.Repositories}, "work")
	if !found || member.Branch != created.Branch || member.SourceBranch != "main" {
		t.Fatalf("UI status did not expose branch identities: %+v", member)
	}
	// A second task must never replace an existing managed worktree.
	if _, err := manager.PrepareRepositoryTask(meta, "work", "feat/another-task", "origin/main"); err == nil ||
		!strings.Contains(err.Error(), "already has a managed checkout") {
		t.Fatalf("existing managed checkout overwritten: %v", err)
	}
	if _, err := manager.SetRepositoryCheckout(meta, "work", "source"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PrepareRepositoryTask(meta, "work", "feat/another-task", "origin/main"); err == nil {
		t.Fatal("preserved worktree was replaced with a new task")
	}
	if _, err := os.Stat(created.WorktreeRoot); err != nil {
		t.Fatalf("preserved feature worktree was deleted: %v", err)
	}
	restarted, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, selected, err := restarted.WorkspaceFor(meta, "work"); err != nil || selected.ManagedWorktree {
		t.Fatalf("restart lost protected source selection: %+v %v", selected, err)
	}
	if _, err := restarted.SetRepositoryCheckout(meta, "work", "managed"); err != nil {
		t.Fatalf("cannot return to preserved managed task: %v", err)
	}
	if _, selected, err := restarted.WorkspaceFor(meta, "work"); err != nil || selected.ProjectRoot != created.WorktreeRoot {
		t.Fatalf("managed task was not resumable: %+v %v", selected, err)
	}
}

func TestPrepareRepositoryTaskGuardsBranchBaseAndAliasAndKeepsOtherRepos(t *testing.T) {
	root, cfg := projectGroupFixture(t)
	meta := map[string]any{"openai/session": "model-first-routing"}
	manager, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SelectGroup(meta, "job-search"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		alias, branch, base string
	}{
		{"outside", "feat/wrong-alias", "origin/main"},
		{"hh", "main", "origin/main"},
		{"hh", "feat/unsafe-base", "HEAD"},
		{"hh", "feat/missing-base", "origin/main"},
		{"hh", "../invalid", "origin/main"},
	} {
		if _, err := manager.PrepareRepositoryTask(meta, tc.alias, tc.branch, tc.base); err == nil {
			t.Fatalf("invalid task unexpectedly accepted: %+v", tc)
		}
	}
	for _, alias := range []string{"hh", "habr"} {
		path := filepath.Join(root, map[string]string{"hh": "hh-mcp-server", "habr": "habr-career-mcp"}[alias])
		runGit(t, path, "update-ref", "refs/remotes/origin/master", "HEAD")
	}
	hh, err := manager.PrepareRepositoryTask(meta, "hh", "fix/hh-integration", "origin/master")
	if err != nil {
		t.Fatal(err)
	}
	habr, err := manager.PrepareRepositoryTask(meta, "habr", "test/habr-compat", "origin/master")
	if err != nil {
		t.Fatal(err)
	}
	if hh.WorktreeRoot == habr.WorktreeRoot || hh.Branch == habr.Branch ||
		hh.Workspace.ActiveRepository != "work" || habr.Workspace.ActiveRepository != "work" {
		t.Fatalf("tasks were not independent: hh=%+v habr=%+v", hh, habr)
	}
	work, info, err := manager.WorkspaceFor(meta, "work")
	if err != nil || info.ManagedWorktree || filepath.Base(work.Path()) != "work-search" {
		t.Fatalf("implicit primary source changed while working on other members: %+v %v", info, err)
	}
	for alias, branch := range map[string]string{"hh": hh.Branch, "habr": habr.Branch} {
		_, info, err = manager.WorkspaceFor(meta, alias)
		if err != nil || !info.ManagedWorktree || info.EffectiveRepository != alias ||
			readGitBranch(info.ProjectRoot) != branch {
			t.Fatalf("route %s was not independent: %+v %v", alias, info, err)
		}
	}
}

func TestFinishRepositoryTaskRequiresCleanAndMergedThenPreservesSource(t *testing.T) {
	root, cfg := projectGroupFixture(t)
	source := filepath.Join(root, "hh-mcp-server")
	runGit(t, source, "branch", "-M", "main")
	runGit(t, source, "update-ref", "refs/remotes/origin/main", "HEAD")
	manager, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	meta := map[string]any{"openai/session": "task-finish-safety"}
	if _, err := manager.SelectGroup(meta, "job-search"); err != nil {
		t.Fatal(err)
	}
	task, err := manager.PrepareRepositoryTask(meta, "hh", "fix/finish-guard", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.FinishRepositoryTask(meta, "hh", "origin/main"); err == nil {
		t.Fatal("finish removed an active managed checkout without source selection")
	}
	file := filepath.Join(task.WorktreeRoot, "feature.txt")
	if err := os.WriteFile(file, []byte("feature commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(task.WorktreeRoot, ".gitignore"), []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, task.WorktreeRoot, "add", "feature.txt", ".gitignore")
	runGit(t, task.WorktreeRoot, "-c", "user.name=Codexify Test",
		"-c", "user.email=test@example.invalid", "commit", "-m", "feature")
	featureHead, err := gitTaskCommand(task.WorktreeRoot, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetRepositoryCheckout(meta, "hh", "source"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.FinishRepositoryTask(meta, "hh", "origin/main"); err == nil ||
		!strings.Contains(err.Error(), "not an ancestor") {
		t.Fatalf("unmerged task unexpectedly removed: %v", err)
	}
	untracked := filepath.Join(task.WorktreeRoot, "uncommitted.txt")
	if err := os.WriteFile(untracked, []byte("keep this"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "update-ref", "refs/remotes/origin/main", featureHead)
	if _, err := manager.FinishRepositoryTask(meta, "hh", "origin/main"); err == nil ||
		!strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("dirty merged task unexpectedly removed: %v", err)
	}
	if err := os.Remove(untracked); err != nil {
		t.Fatal(err)
	}
	ignored := filepath.Join(task.WorktreeRoot, "build.log")
	if err := os.WriteFile(ignored, []byte("preserve ignored data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.FinishRepositoryTask(meta, "hh", "origin/main"); err == nil ||
		!strings.Contains(err.Error(), "ignored files") {
		t.Fatalf("ignored task data unexpectedly removed: %v", err)
	}
	if body, err := os.ReadFile(ignored); err != nil || string(body) != "preserve ignored data" {
		t.Fatalf("ignored data lost on rejected cleanup: %q %v", body, err)
	}
	if err := os.Remove(ignored); err != nil {
		t.Fatal(err)
	}
	cleaned, err := manager.FinishRepositoryTask(meta, "hh", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if !cleaned.Released || !cleaned.Removed || cleaned.Repository != "hh" ||
		cleaned.Workspace.ActiveRepository != "work" {
		t.Fatalf("clean merged task was not safely released: %+v", cleaned)
	}
	if _, err := os.Stat(task.WorktreeRoot); !os.IsNotExist(err) {
		t.Fatalf("clean task worktree not removed: %v", err)
	}
	if _, err := gitTaskCommand(source, "show-ref", "--verify", "refs/heads/fix/finish-guard"); err != nil {
		t.Fatalf("feature branch was deleted: %v", err)
	}
	if readGitBranch(source) != "main" {
		t.Fatal("source branch changed while finishing task")
	}
	next, err := manager.PrepareRepositoryTask(meta, "hh", "fix/next-task", "origin/main")
	if err != nil || readGitBranch(next.WorktreeRoot) != "fix/next-task" {
		t.Fatalf("clean finished repository cannot start next task: %+v %v", next, err)
	}
}
