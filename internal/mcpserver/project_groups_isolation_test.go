package mcpserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProjectGroupConcurrentExplicitCallsDoNotRaceActiveAlias(t *testing.T) {
	f := newPGFixture(t)
	pgCall(t, f, "set_project_group", map[string]any{"name": "job-search"})

	aliases := []string{"work", "hh", "habr"}
	var wg sync.WaitGroup
	errors := make(chan error, 32)
	for i := 0; i < 18; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			alias := aliases[i%len(aliases)]
			for _, tool := range []string{"read_file", "git_status"} {
				args := map[string]any{"repository": alias}
				if tool == "read_file" {
					args["path"] = "README.md"
				}
				result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{
					Meta: f.meta, Name: tool, Arguments: args,
				})
				if err != nil || result == nil || result.IsError {
					errors <- fmt.Errorf("%s/%s failed: %v result=%+v", alias, tool, err, result)
					return
				}
				out, ok := result.StructuredContent.(map[string]any)
				if !ok {
					errors <- fmt.Errorf("%s/%s unexpected result %T", alias, tool, result.StructuredContent)
					return
				}
				selection, ok := out["repository"].(map[string]any)
				if !ok || selection["alias"] != alias {
					errors <- fmt.Errorf("%s/%s routed incorrectly: %+v", alias, tool, out)
					return
				}
				if tool == "read_file" && !strings.Contains(fmt.Sprint(out["content"]), "project-"+alias) {
					errors <- fmt.Errorf("%s/%s saw different repository content: %+v", alias, tool, out)
					return
				}
			}
		}(i)
	}
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{
				Meta: f.meta, Name: "set_active_repository",
				Arguments: map[string]any{"repository": aliases[i%len(aliases)]},
			})
			if err != nil || result == nil || result.IsError {
				errors <- fmt.Errorf("set_active_repository %d: %v result=%+v", i, err, result)
			}
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if t.Failed() {
		return
	}
	status := pgValue(t, pgCall(t, f, "get_project_group_status", nil))
	if generation, ok := status["generation"].(float64); !ok || generation < 1 {
		t.Fatalf("active selection generation invalid after concurrent calls: %+v", status)
	}
}

func TestProjectGroupGitCommitPushIndependentDisposableFixtures(t *testing.T) {
	f := newPGFixture(t)
	pgCall(t, f, "set_project_group", map[string]any{"name": "job-search"})
	for _, alias := range []string{"hh", "habr"} {
		remote := filepath.Join(t.TempDir(), "bare-"+alias)
		if err := os.MkdirAll(remote, 0o755); err != nil {
			t.Fatal(err)
		}
		runTestGit(t, remote, "init", "--bare")
		runTestGit(t, f.paths[alias], "remote", "add", "fixture", remote)
		pgCall(t, f, "write_file", map[string]any{
			"repository": alias, "path": "push-" + alias + ".txt", "content": "push-from-" + alias + "\n",
		})

		shell := "sh"
		if runtime.GOOS == "windows" {
			shell = "cmd"
		}
		command := "git add -A && git -c user.name=PGFixture -c user.email=pg@example.invalid commit -m route-" +
			alias + " && git push fixture HEAD:refs/heads/fixture-" + alias
		started := pgValue(t, pgCall(t, f, "exec_command", map[string]any{
			"repository": alias, "cmd": command, "shell": shell, "yield_time_ms": 30000,
		}))
		for started["running"] == true {
			sessionID, _ := started["session_id"].(string)
			if sessionID == "" {
				t.Fatalf("exec session missing id for %s: %+v", alias, started)
			}
			started = pgValue(t, pgCall(t, f, "write_stdin", map[string]any{
				"session_id": sessionID, "yield_time_ms": 30000,
			}))
		}
		if started["exit_code"] != float64(0) {
			t.Fatalf("git commit/push failed for %s: %+v", alias, started)
		}
		selected, _ := started["repository"].(map[string]any)
		if selected["alias"] != alias {
			t.Fatalf("git command was routed to another checkout: %+v", started)
		}
		cmd := exec.Command("git", "--git-dir", remote, "show", "refs/heads/fixture-"+alias+":push-"+alias+".txt")
		out, err := cmd.Output()
		if err != nil || string(out) != "push-from-"+alias+"\n" {
			t.Fatalf("bare remote for %s got %q, %v", alias, out, err)
		}
	}
	if _, err := os.Stat(filepath.Join(f.paths["work"], "push-hh.txt")); !os.IsNotExist(err) {
		t.Fatalf("hh commit unexpectedly created a file in work checkout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.paths["habr"], "push-hh.txt")); !os.IsNotExist(err) {
		t.Fatalf("hh commit unexpectedly created a file in habr checkout: %v", err)
	}
	// Neither push in a disposable source changed the third repository.
	head := exec.Command("git", "-C", f.paths["work"], "log", "-1", "--pretty=%s")
	out, err := head.Output()
	if err != nil || strings.TrimSpace(string(out)) != "baseline" {
		t.Fatalf("primary checkout Git HEAD moved unexpectedly: %q, %v", out, err)
	}
}
