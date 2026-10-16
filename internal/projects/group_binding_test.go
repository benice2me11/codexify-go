package projects

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/benice2me11/codexify-go/internal/workspace"
)

func TestGroupBindingV3PersistsActiveAndExplicitRouting(t *testing.T) {
	_, cfg := projectGroupFixture(t)
	meta := map[string]any{"openai/session": "group-v3-source-conversation"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := m.SelectGroup(meta, "job-search")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Mode != "group" || selected.ActiveRepository != "work" ||
		selected.EffectiveRepository != "work" || selected.Generation != 1 {
		t.Fatalf("new v3 group binding incorrect: %+v", selected)
	}
	if selected.ManagedWorktree || len(selected.Repositories) != 3 {
		t.Fatalf("source group should not create managed worktrees: %+v", selected)
	}
	stored, err := os.ReadFile(m.bindingPath(IdentityFromMeta(meta)))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(stored, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["version"] != float64(groupBindingVersion) || raw["mode"] != "group" {
		t.Fatalf("binding on disk is not tagged v3: %s", stored)
	}

	root, info, err := m.WorkspaceFor(meta, "HH")
	if err != nil || info.EffectiveRepository != "hh" || !strings.Contains(root.Path(), "hh-mcp-server") {
		t.Fatalf("explicit hh route failed: %+v, %v", info, err)
	}
	status, err := m.Status(meta)
	if err != nil || !status.Selected || status.Workspace.ActiveRepository != "work" {
		t.Fatalf("explicit routing changed active: %+v, %v", status, err)
	}
	afterSwitch, err := m.SetActiveRepository(meta, "HaBr")
	if err != nil || afterSwitch.ActiveRepository != "habr" || afterSwitch.Generation != 2 {
		t.Fatalf("persist active alias failed: %+v, %v", afterSwitch, err)
	}
	_, implicit, err := m.Workspace(meta)
	if err != nil || implicit.EffectiveRepository != "habr" {
		t.Fatalf("implicit routing did not follow persisted active: %+v, %v", implicit, err)
	}
	_, explicit, err := m.WorkspaceFor(meta, "work")
	if err != nil || explicit.EffectiveRepository != "work" || explicit.ActiveRepository != "habr" {
		t.Fatalf("explicit routing corrupted active: %+v, %v", explicit, err)
	}
	if _, _, err := m.WorkspaceFor(meta, "missing"); err == nil {
		t.Fatal("unknown repository alias accepted")
	}
	// Two independent Manager instances model worker restarts without touching
	// the live Windows SCM service.
	for i := 0; i < 2; i++ {
		m, err = New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		_, restored, err := m.Workspace(meta)
		if err != nil || restored.ActiveRepository != "habr" ||
			restored.GroupName != "job-search" || restored.Generation != 2 {
			t.Fatalf("restart %d lost group binding: %+v, %v", i+1, restored, err)
		}
	}
	again, err := m.SelectGroup(meta, "job-search")
	if err != nil || again.NewlySelected {
		t.Fatalf("idempotent group select changed binding: %+v, %v", again, err)
	}
}

func TestGroupBindingV3FeatureOffFailsClosedButLegacyWorks(t *testing.T) {
	_, cfg := projectGroupFixture(t)
	groupMeta := map[string]any{"openai/session": "feature-flag-v3"}
	legacyMeta := map[string]any{"openai/session": "feature-flag-legacy"}
	enabled, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := enabled.SelectGroup(groupMeta, "job-search"); err != nil {
		t.Fatal(err)
	}
	off := false
	if _, err := enabled.Select(legacyMeta, "work-search", &off); err != nil {
		t.Fatal(err)
	}
	cfg.ProjectGroupsEnabled = false
	disabled, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := disabled.Workspace(groupMeta); err == nil || !strings.Contains(err.Error(), "binding preserved") {
		t.Fatalf("disabled v3 binding should fail closed and remain on disk: %v", err)
	}
	if _, _, err := disabled.Workspace(legacyMeta); err != nil {
		t.Fatalf("disabled feature regressed v2 binding: %v", err)
	}
	if _, err := disabled.SelectGroup(map[string]any{"openai/session": "another-group"}, "job-search"); err == nil {
		t.Fatal("disabled config allowed group selection")
	}
	cfg.ProjectGroupsEnabled = true
	restored, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := restored.Workspace(groupMeta); err != nil {
		t.Fatalf("reenabling feature could not read preserved group: %v", err)
	}
}

func TestGroupBindingStaleMembershipRejected(t *testing.T) {
	_, cfg := projectGroupFixture(t)
	meta := map[string]any{"openai/session": "stale-group-binding"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SelectGroup(meta, "job-search"); err != nil {
		t.Fatal(err)
	}
	cfg.ProjectGroups[0].Repositories[1].Checkout = "managed"
	changed, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := changed.Workspace(meta); err == nil || !strings.Contains(err.Error(), "membership changed") {
		t.Fatalf("stale group unexpectedly accepted: %v", err)
	}
}

func TestGroupBindingManagedCheckoutDoesNotChangeSources(t *testing.T) {
	root, cfg := projectGroupFixture(t)
	cfg.ProjectGroups[0].Repositories[1].Checkout = "managed"
	meta := map[string]any{"openai/session": "group-managed-member"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := m.SelectGroup(meta, "job-search")
	if err != nil {
		t.Fatal(err)
	}
	hh, ok := boundGroupMember(Binding{Repositories: selected.Repositories}, "hh")
	if !ok || !hh.ManagedWorktree || hh.WorktreeGitRoot == "" ||
		hh.ProjectRoot == hh.SourceProjectRoot {
		t.Fatalf("explicit managed member was not isolated: %+v", hh)
	}
	work, _ := boundGroupMember(Binding{Repositories: selected.Repositories}, "work")
	wantSourceRoot, err := workspace.Canonical(filepath.Join(root, "work-search"))
	if err != nil {
		t.Fatal(err)
	}
	if work.ManagedWorktree || !sameGroupPath(work.ProjectRoot, wantSourceRoot) {
		t.Fatalf("source member unexpectedly moved: %+v", work)
	}
	if _, err := os.Stat(filepath.Join(hh.SourceProjectRoot, "README.md")); err != nil {
		t.Fatalf("original source checkout missing: %v", err)
	}
	_, info, err := m.WorkspaceFor(meta, "hh")
	if err != nil || !info.ManagedWorktree || !sameGroupPath(info.ProjectRoot, hh.ProjectRoot) {
		t.Fatalf("managed member resolution failed: %+v, %v", info, err)
	}
	m, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, restarted, err := m.WorkspaceFor(meta, "hh")
	if err != nil || !restarted.ManagedWorktree || !sameGroupPath(restarted.ProjectRoot, hh.ProjectRoot) {
		t.Fatalf("managed worktree identity lost across Manager restart: %+v, %v", restarted, err)
	}
	// v3 membership counts as a source in use for unrelated single-project
	// conversations, preserving legacy auto-worktree safety.
	other := map[string]any{"openai/session": "other-legacy-chat"}
	legacy, err := m.Select(other, "hh-mcp-server", nil)
	if err != nil || !legacy.ManagedWorktree {
		t.Fatalf("group member source-in-use was ignored: %+v, %v", legacy, err)
	}
}

func TestGroupBindingManagedSelectionRollsBackNewWorktreeOnPersistFailure(t *testing.T) {
	root, cfg := projectGroupFixture(t)
	cfg.ProjectGroups[0].Repositories[0].Checkout = "managed"
	blocker := filepath.Join(root, "file-not-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.BindingsDir = blocker // Forces writeBinding failure *after* worktree creation.
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.SelectGroup(map[string]any{"openai/session": "rollback-on-write-failure"}, "job-search")
	if err == nil {
		t.Fatal("expected binding persistence failure")
	}
	// The worktree directory tree may remain, but Git must not have a live
	// registered disposable worktree or branch from the aborted selection.
	worktrees, err := os.ReadDir(filepath.Join(root, ".codexify-go", "worktrees", "work-search"))
	if err == nil {
		for _, row := range worktrees {
			if _, statErr := os.Stat(filepath.Join(root, ".codexify-go", "worktrees", "work-search", row.Name())); statErr == nil {
				t.Fatalf("aborted selection left a managed worktree at %s", row.Name())
			}
		}
	}
}

func TestGroupBindingExplicitSelectionConcurrentWithActiveSwitch(t *testing.T) {
	_, cfg := projectGroupFixture(t)
	meta := map[string]any{"openai/session": "concurrent-group-routing"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SelectGroup(meta, "job-search"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			alias := "hh"
			if i%2 == 0 {
				alias = "habr"
			}
			_, err := m.SetActiveRepository(meta, alias)
			if err != nil {
				errs <- err
			}
		}(i)
		go func() {
			defer wg.Done()
			_, info, err := m.WorkspaceFor(meta, "work")
			if err != nil {
				errs <- err
			} else if info.EffectiveRepository != "work" {
				errs <- os.ErrInvalid
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent group routing failed: %v", err)
	}
}

func TestGroupBindingV3DoesNotResumeAsLegacySingleProject(t *testing.T) {
	_, cfg := projectGroupFixture(t)
	meta := map[string]any{"openai/session": "group-resume-guard"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := m.SelectGroup(meta, "job-search")
	if err != nil {
		t.Fatal(err)
	}
	other := map[string]any{"openai/session": "different-chat"}
	if _, err := m.Resume(other, selected.ProjectRoot); err == nil || !strings.Contains(err.Error(), "no valid saved workspace") {
		t.Fatalf("group binding was treated as a single project: %v", err)
	}
}

func TestGroupBindingRejectsDuplicateOrForeignManagedMembership(t *testing.T) {
	_, cfg := projectGroupFixture(t)
	cfg.ProjectGroups[0].Repositories[0].Checkout = "managed"
	cfg.ProjectGroups[0].Repositories[1].Checkout = "managed"
	meta := map[string]any{"openai/session": "tamper-v3"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SelectGroup(meta, "job-search"); err != nil {
		t.Fatal(err)
	}
	path := m.bindingPath(IdentityFromMeta(meta))
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var binding Binding
	if err := json.Unmarshal(original, &binding); err != nil {
		t.Fatal(err)
	}
	binding.Repositories[1] = binding.Repositories[0]
	if err := writeBindingFile(path, binding); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Workspace(meta); err == nil || !strings.Contains(err.Error(), "repeats alias") {
		t.Fatalf("duplicate alias in persisted v3 binding accepted: %v", err)
	}
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(original, &binding); err != nil {
		t.Fatal(err)
	}
	work, _ := boundGroupMember(binding, "work")
	for i := range binding.Repositories {
		if binding.Repositories[i].Alias == "hh" {
			binding.Repositories[i].ProjectRoot = work.ProjectRoot
			binding.Repositories[i].WorktreeGitRoot = work.WorktreeGitRoot
		}
	}
	if err := writeBindingFile(path, binding); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Workspace(meta); err == nil || !strings.Contains(err.Error(), "not owned by its configured source") {
		t.Fatalf("foreign managed checkout accepted: %v", err)
	}
}

func TestGroupBindingSwitchAcceptsVisibleActiveRoot(t *testing.T) {
	_, cfg := projectGroupFixture(t)
	meta := map[string]any{"openai/session": "group-switch-visible-active"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SelectGroup(meta, "job-search"); err != nil {
		t.Fatal(err)
	}
	active, err := m.SetActiveRepository(meta, "hh")
	if err != nil {
		t.Fatal(err)
	}
	change, err := m.Switch(meta, active.ProjectRoot)
	if err != nil || !change.AwaitingSelection {
		t.Fatalf("active member should allow explicit archived switch: %+v, %v", change, err)
	}
	if _, _, err := m.Workspace(meta); err == nil {
		t.Fatal("switched group binding should no longer resolve until selected")
	}
	if _, err := m.SelectGroup(meta, "job-search"); err != nil {
		t.Fatalf("group should allow reselection after explicit switch: %v", err)
	}
}

func TestProjectGroupCheckoutModesPersistAndPreserveDirtyWorktree(t *testing.T) {
	_, cfg := projectGroupFixture(t)
	meta := map[string]any{"openai/session": "per-member-checkout-mode"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SelectGroupWithModes(meta, "job-search", map[string]string{
		"HH": "managed", "habr": "source",
	}); err != nil {
		t.Fatalf("initial explicit modes failed: %v", err)
	}
	_, first, err := m.WorkspaceFor(meta, "hh")
	if err != nil || !first.ManagedWorktree || first.EffectiveCheckoutRevision != 1 {
		t.Fatalf("first HH managed selection incorrect: %+v, %v", first, err)
	}
	initialManaged := first.ProjectRoot
	dirty := filepath.Join(initialManaged, "uncommitted-checkout-note.txt")
	if err := os.WriteFile(dirty, []byte("keep this working tree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetActiveRepository(meta, "habr"); err != nil {
		t.Fatal(err)
	}
	selected, err := m.SetRepositoryCheckout(meta, "hh", "source")
	if err != nil || selected.ActiveRepository != "habr" {
		t.Fatalf("source switch changed active/default: %+v %v", selected, err)
	}
	_, source, err := m.WorkspaceFor(meta, "hh")
	if err != nil || source.ManagedWorktree || source.EffectiveCheckoutRevision != 2 ||
		!sameGroupPath(source.ProjectRoot, source.SourceProjectRoot) {
		t.Fatalf("HH source checkout incorrect: %+v %v", source, err)
	}
	row, ok := boundGroupMember(Binding{Repositories: selected.Repositories}, "hh")
	if !ok || !sameGroupPath(row.PreservedManagedProjectRoot, initialManaged) {
		t.Fatalf("managed checkout was not preserved: %+v", row)
	}
	if data, err := os.ReadFile(dirty); err != nil || string(data) != "keep this working tree\n" {
		t.Fatalf("dirty data lost on managed-to-source: %q %v", data, err)
	}
	for i := 0; i < 2; i++ {
		m, err = New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		_, restored, err := m.WorkspaceFor(meta, "hh")
		if err != nil || restored.ManagedWorktree || restored.EffectiveCheckoutRevision != 2 {
			t.Fatalf("manager recreation %d lost mode: %+v, %v", i+1, restored, err)
		}
	}
	selected, err = m.SetRepositoryCheckout(meta, "hh", "managed")
	if err != nil {
		t.Fatal(err)
	}
	_, reopened, err := m.WorkspaceFor(meta, "hh")
	if err != nil || !reopened.ManagedWorktree || reopened.EffectiveCheckoutRevision != 3 ||
		!sameGroupPath(reopened.ProjectRoot, initialManaged) {
		t.Fatalf("managed checkout should be reused: %+v %v", reopened, err)
	}
	if data, err := os.ReadFile(dirty); err != nil || string(data) != "keep this working tree\n" {
		t.Fatalf("dirty data lost after source-to-managed: %q %v", data, err)
	}
	_, work, err := m.WorkspaceFor(meta, "work")
	if err != nil || work.ManagedWorktree {
		t.Fatalf("unrelated work checkout changed: %+v %v", work, err)
	}
	_, habr, err := m.WorkspaceFor(meta, "habr")
	if err != nil || habr.ManagedWorktree {
		t.Fatalf("unrelated habr checkout changed: %+v %v", habr, err)
	}
	if selected.ActiveRepository != "habr" || selected.Generation < 4 {
		t.Fatalf("checkout switch reset active/default or revision: %+v", selected)
	}
}

func TestProjectGroupInitialCheckoutOverridesRejectInvalidValues(t *testing.T) {
	_, cfg := projectGroupFixture(t)
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i, invalid := range []map[string]string{
		{"missing": "managed"},
		{"hh": "auto"},
		{"HH": "source", "hh": "managed"},
	} {
		meta := map[string]any{"openai/session": fmt.Sprintf("initial-checkout-%d", i)}
		if _, err := m.SelectGroupWithModes(meta, "job-search", invalid); err == nil {
			t.Fatalf("invalid initial checkout overrides accepted: %+v", invalid)
		}
		if _, _, err := m.Workspace(meta); err == nil {
			t.Fatal("rejected initial checkout persisted a binding")
		}
	}
	meta := map[string]any{"openai/session": "initial-valid"}
	if _, err := m.SelectGroupWithModes(meta, "job-search", map[string]string{"hh": "managed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SelectGroupWithModes(meta, "job-search", map[string]string{"hh": "source"}); err == nil {
		t.Fatal("idempotent group select silently changed checkout mode")
	}
	if _, err := m.SetRepositoryCheckout(meta, "missing", "managed"); err == nil {
		t.Fatal("unknown member checkout was accepted")
	}
	if _, err := m.SetRepositoryCheckout(meta, "hh", "auto"); err == nil {
		t.Fatal("auto mode should not be a member checkout")
	}
}

func TestProjectGroupPrimaryCheckoutSwitchUpdatesBindingRoot(t *testing.T) {
	_, cfg := projectGroupFixture(t)
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	meta := map[string]any{"openai/session": "primary-switch"}
	if _, err := m.SelectGroup(meta, "job-search"); err != nil {
		t.Fatal(err)
	}
	managed, err := m.SetRepositoryCheckout(meta, "work", "managed")
	if err != nil || !managed.ManagedWorktree {
		t.Fatalf("primary switch didn't select managed: %+v, %v", managed, err)
	}
	primaryManaged := managed.ProjectRoot
	m, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, primary, err := m.Workspace(meta)
	if err != nil || !sameGroupPath(primary.ProjectRoot, primaryManaged) {
		t.Fatalf("primary active root changed on restart: %+v, %v", primary, err)
	}
	source, err := m.SetRepositoryCheckout(meta, "work", "source")
	if err != nil || source.ManagedWorktree || sameGroupPath(source.ProjectRoot, primaryManaged) {
		t.Fatalf("primary did not return to source: %+v, %v", source, err)
	}
	if _, err := os.Stat(primaryManaged); err != nil {
		t.Fatalf("managed tree was removed on primary source switch: %v", err)
	}
	if resumed, err := m.SetRepositoryCheckout(meta, "work", "managed"); err != nil || !sameGroupPath(resumed.ProjectRoot, primaryManaged) {
		t.Fatalf("primary managed checkout was not preserved: %+v, %v", resumed, err)
	}
}

func TestProjectGroupLegacyV3CheckoutRevisionUpgradeIsSafe(t *testing.T) {
	_, cfg := projectGroupFixture(t)
	meta := map[string]any{"openai/session": "legacy-pg-i2-v3-checkout"}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SelectGroup(meta, "job-search"); err != nil {
		t.Fatal(err)
	}
	identity := IdentityFromMeta(meta)
	binding, err := m.readBinding(identity)
	if err != nil {
		t.Fatal(err)
	}
	for i := range binding.Repositories {
		binding.Repositories[i].CheckoutRevision = 0 // Old PG-I2 on-disk v3 record.
	}
	if err := writeBindingFile(m.bindingPath(identity), *binding); err != nil {
		t.Fatal(err)
	}
	m, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, effective, err := m.WorkspaceFor(meta, "hh")
	if err != nil || effective.EffectiveCheckoutRevision != 1 {
		t.Fatalf("old group binding should normalize implicit checkout revision 1: %+v %v", effective, err)
	}
	changed, err := m.SetRepositoryCheckout(meta, "hh", "managed")
	if err != nil {
		t.Fatalf("old group binding cannot change member mode safely: %v", err)
	}
	member, exists := boundGroupMember(Binding{Repositories: changed.Repositories}, "hh")
	if !exists || member.CheckoutRevision != 2 {
		t.Fatalf("mode switch did not advance old v3 revision: %+v", member)
	}
	_, effective, err = m.WorkspaceFor(meta, "hh")
	if err != nil || effective.EffectiveCheckoutRevision != 2 {
		t.Fatalf("upgraded revision was not persisted: %+v %v", effective, err)
	}
}
