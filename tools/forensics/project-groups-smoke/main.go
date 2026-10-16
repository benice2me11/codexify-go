// Command project-groups-smoke tests a compiled codexify-go worker without
// installing a service, launching a tunnel, or using real user repositories.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/ui"
	"github.com/benice2me11/codexify-go/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const smokeConversation = "project-groups-compiled-worker-fixture"

type fixture struct {
	dir       string
	repos     map[string]string
	cfg       config.Config
	cfgPath   string
	candidate string
	marker    string
	managed   string
	artifact  string
	chatPath  string
}

type runningWorker struct {
	cmd     *exec.Cmd
	output  *bytes.Buffer
	session *mcp.ClientSession
}

func main() {
	if len(os.Args) != 2 {
		fatal(errors.New("usage: go run ./tools/forensics/project-groups-smoke <compiled-codexify-go-binary>"))
	}
	if err := run(os.Args[1]); err != nil {
		fatal(err)
	}
	fmt.Println("PROJECT_GROUPS_COMPILED_WORKER_SMOKE=PASS")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "PROJECT_GROUPS_SMOKE_FAIL:", err)
	os.Exit(1)
}

func mustGit(path string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", path}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s in fixture: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func setup(candidate string) (_ *fixture, err error) {
	bin, err := filepath.Abs(candidate)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("compiled candidate not found: %w", err)
	}
	dir, err := os.MkdirTemp("", "pg-compiled-worker-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()
	f := &fixture{
		dir: dir, candidate: bin, cfgPath: filepath.Join(dir, "config.json"),
		repos: map[string]string{
			"work": filepath.Join(dir, "work-search"),
			"hh":   filepath.Join(dir, "hh-mcp-server"),
			"habr": filepath.Join(dir, "habr-career-mcp"),
		},
		marker: "compiled-worker-owner-fixture",
	}
	for _, alias := range []string{"work", "hh", "habr"} {
		repo := f.repos[alias]
		if err := os.MkdirAll(repo, 0o755); err != nil {
			return nil, err
		}
		if err := mustGit(repo, "init"); err != nil {
			return nil, err
		}
		for name, value := range map[string]string{
			"README.md": "compiled-" + alias + "\n",
			"AGENTS.md": "instructions-for-" + alias + "\n",
		} {
			if err := os.WriteFile(filepath.Join(repo, name), []byte(value), 0o644); err != nil {
				return nil, err
			}
		}
		if err := mustGit(repo, "add", "README.md", "AGENTS.md"); err != nil {
			return nil, err
		}
		if err := mustGit(repo, "-c", "user.name=PG Fixture", "-c", "user.email=pg@invalid.example", "commit", "-m", "fixture"); err != nil {
			return nil, err
		}
	}

	cfg := config.Default()
	cfg.Service.Name = "CodexifyProjectGroupsSmoke"
	cfg.Log.File = filepath.Join(dir, "logs", "worker.jsonl")
	cfg.MCP.WorkspaceRoot = dir
	cfg.MCP.MultiProject = true
	cfg.MCP.ProjectGroupsEnabled = true
	cfg.MCP.AuthEnabled = false
	cfg.MCP.BindingsDir = filepath.Join(dir, "state", "bindings")
	cfg.MCP.CloneDir = filepath.Join(dir, "state", "clones")
	cfg.MCP.ScratchDir = filepath.Join(dir, "state", "scratch")
	cfg.MCP.Worktrees.Mode = "never"
	cfg.MCP.Worktrees.Root = filepath.Join(dir, "state", "managed")
	cfg.MCP.Projects = []config.ProjectSpec{
		{Name: "work-search", Path: f.repos["work"]},
		{Name: "hh-mcp-server", Path: f.repos["hh"]},
		{Name: "habr-career-mcp", Path: f.repos["habr"]},
	}
	cfg.MCP.ProjectGroups = []config.ProjectGroupSpec{{
		Name: "job-search", Primary: "work",
		Repositories: []config.ProjectGroupRepository{
			{Alias: "work", Project: "work-search", Checkout: "source"},
			{Alias: "hh", Project: "hh-mcp-server", Checkout: "source"},
			{Alias: "habr", Project: "habr-career-mcp", Checkout: "source"},
		},
	}}
	cfg.MCP.Upstreams = nil // Fixture never reaches real HH/Habr/upstream MCP.
	cfg.Memory.Dir = filepath.Join(dir, "state", "memory")
	cfg.Skills.IncludeUser = false
	cfg.AgentChat.Enabled = true
	cfg.AgentChat.Dir = filepath.Join(dir, "state", "chat")
	cfg.AgentChat.MaxWaitMS = 1500
	cfg.ArtifactEgress.Dir = filepath.Join(dir, "state", "artifacts")
	cfg.ArtifactEgress.MaxFileBytes = 1 << 20
	cfg.ArtifactEgress.SnapshotMaxFileBytes = 1 << 20
	cfg.ArtifactEgress.MaxSnapshotBytes = 4 << 20
	cfg.Tunnel.TunnelID = "tunnel_pg_worker_isolated"
	cfg.Tunnel.APIKeyRef = "env:PG_SMOKE_NEVER_READ"
	cfg.Tunnel.Executable = filepath.Join(dir, "fake-tunnel-never-executed")
	cfg.Tunnel.ManagedDir = filepath.Join(dir, "state", "unused-tunnel")
	cfg.Tunnel.HealthURLFile = filepath.Join(dir, "state", "unused-tunnel-health.url")
	cfg.Tunnel.Environment = nil
	f.cfg = cfg
	if err := os.MkdirAll(filepath.Join(dir, "home"), 0o700); err != nil {
		return nil, err
	}
	return f, nil
}

// writeConfig writes only local fixture paths and intentionally leaves every
// tunnel key as an inert placeholder. Config.Duration has a numeric MarshalJSON
// representation, while Load expects strings; convert before worker startup.
func (f *fixture) writeConfig(port int, enabled bool) error {
	cfg := f.cfg
	cfg.MCP.ProjectGroupsEnabled = enabled
	cfg.Tunnel.MCPServerURL = fmt.Sprintf("http://127.0.0.1:%d/mcp", port)
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(encoded, &doc); err != nil {
		return err
	}
	durations := []struct {
		section, field string
		value          time.Duration
	}{
		{"supervisor", "minBackoff", cfg.Supervisor.MinBackoff.Duration()},
		{"supervisor", "maxBackoff", cfg.Supervisor.MaxBackoff.Duration()},
		{"supervisor", "stableWindow", cfg.Supervisor.StableWindow.Duration()},
		{"supervisor", "healthInterval", cfg.Supervisor.HealthInterval.Duration()},
		{"supervisor", "shutdownTimeout", cfg.Supervisor.ShutdownTimeout.Duration()},
		{"tunnel", "startupWaitTimeout", cfg.Tunnel.StartupWaitTimeout.Duration()},
		{"tunnel", "pollRestartCooldown", cfg.Tunnel.PollRestartCooldown.Duration()},
		{"artifactIngress", "requestTimeout", cfg.ArtifactIngress.RequestTimeout.Duration()},
		{"artifactIngress", "idleTimeout", cfg.ArtifactIngress.IdleTimeout.Duration()},
		{"artifactEgress", "referenceTtl", cfg.ArtifactEgress.ReferenceTTL.Duration()},
	}
	for _, v := range durations {
		doc[v.section].(map[string]any)[v.field] = v.value.String()
	}
	doc["mcp"].(map[string]any)["diagnostics"].(map[string]any)["maxDuration"] = cfg.MCP.Diagnostics.MaxDuration.Duration().String()
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.cfgPath, append(data, '\n'), 0o600)
}

func availablePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port, nil
}

func (f *fixture) start(ctx context.Context, enabled bool) (_ *runningWorker, err error) {
	port, err := availablePort()
	if err != nil {
		return nil, err
	}
	if err := f.writeConfig(port, enabled); err != nil {
		return nil, err
	}
	w := &runningWorker{output: &bytes.Buffer{}}
	w.cmd = exec.CommandContext(ctx, f.candidate, "worker", "run", "--config", f.cfgPath)
	w.cmd.Env = append(os.Environ(),
		"USERPROFILE="+filepath.Join(f.dir, "home"),
		"HOME="+filepath.Join(f.dir, "home"),
	)
	w.cmd.Stdout, w.cmd.Stderr = w.output, w.output
	if err := w.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start compiled worker: %w", err)
	}
	defer func() {
		if err != nil {
			w.close()
		}
	}()
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	ready := false
	client := &http.Client{Timeout: 300 * time.Millisecond}
	for i := 0; i < 150; i++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if reqErr != nil {
			return nil, reqErr
		}
		resp, doErr := client.Do(req)
		if doErr == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(80 * time.Millisecond)
	}
	if !ready {
		logText, _ := os.ReadFile(f.cfg.Log.File)
		return nil, fmt.Errorf("compiled worker health failed: output=%q log_tail=%q",
			tail(w.output.String(), 2200), tail(string(logText), 2200))
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint:             fmt.Sprintf("http://127.0.0.1:%d/mcp", port),
		DisableStandaloneSSE: true, MaxRetries: -1,
	}
	sdk := mcp.NewClient(&mcp.Implementation{Name: "pg-compiled-smoke", Version: "1"}, nil)
	session, err := sdk.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	w.session = session
	return w, nil
}

func (w *runningWorker) close() {
	if w == nil {
		return
	}
	if w.session != nil {
		_ = w.session.Close()
	}
	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
		_, _ = w.cmd.Process.Wait()
	}
}

func tail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

func (w *runningWorker) call(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	out, err := w.session.CallTool(ctx, &mcp.CallToolParams{
		Meta: mcp.Meta{"openai/session": smokeConversation},
		Name: name, Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if out == nil {
		return nil, fmt.Errorf("%s: empty result", name)
	}
	if out.IsError {
		var content []string
		for _, item := range out.Content {
			if text, ok := item.(*mcp.TextContent); ok {
				content = append(content, text.Text)
			}
		}
		return nil, fmt.Errorf("%s: %s", name, strings.Join(content, " | "))
	}
	obj, ok := out.StructuredContent.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s unexpected output: %T", name, out.StructuredContent)
	}
	return obj, nil
}

func (w *runningWorker) expectDenied(ctx context.Context, name string, args map[string]any) error {
	out, err := w.session.CallTool(ctx, &mcp.CallToolParams{
		Meta: mcp.Meta{"openai/session": smokeConversation},
		Name: name, Arguments: args,
	})
	if err == nil && (out == nil || !out.IsError) {
		return fmt.Errorf("%s unexpectedly permitted %+v", name, args)
	}
	return nil
}
func check(condition bool, format string, args ...any) error {
	if condition {
		return nil
	}
	return fmt.Errorf(format, args...)
}
func canonicalEqual(a, b string) bool {
	x, err := workspace.Canonical(a)
	if err != nil {
		return false
	}
	y, err := workspace.Canonical(b)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(x, y)
	}
	return x == y
}
func member(records any, alias string) map[string]any {
	members, ok := records.([]any)
	if !ok {
		return nil
	}
	for _, entry := range members {
		row, ok := entry.(map[string]any)
		if ok && row["alias"] == alias {
			return row
		}
	}
	return nil
}
func (f *fixture) first(ctx context.Context, w *runningWorker) error {
	tools, err := w.session.ListTools(ctx, nil)
	if err != nil {
		return err
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
		if tool.Name == "setup" {
			if tool.Meta["ui/resourceUri"] != ui.GroupSetupURI {
				return fmt.Errorf("Setup v4 resource not advertised: %v", tool.Meta)
			}
		}
	}
	for _, name := range []string{
		"set_project_group", "set_active_repository", "set_repository_checkout",
		"read_file", "git_status", "show_diff", "chat_write", "export_host_file",
	} {
		if !names[name] {
			return fmt.Errorf("missing compiled MCP tool %s", name)
		}
	}
	if _, err := w.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: ui.GroupSetupURI}); err != nil {
		return fmt.Errorf("Setup v4 resource read: %w", err)
	}
	if _, err := w.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: ui.SetupURI}); err != nil {
		return fmt.Errorf("legacy Setup v3 resource read: %w", err)
	}
	initial, err := w.call(ctx, "set_project_group", map[string]any{
		"name":      "job-search",
		"checkouts": map[string]string{"hh": "managed", "work": "source", "habr": "source"},
	})
	if err != nil {
		return err
	}
	if err := check(initial["groupName"] == "job-search" && initial["activeRepository"] == "work",
		"initial group binding invalid: %+v", initial); err != nil {
		return err
	}
	hh := member(initial["repositories"], "hh")
	if hh == nil || hh["checkout"] != "managed" || hh["managedWorktree"] != true {
		return fmt.Errorf("hh managed selection missing: %+v", hh)
	}
	f.managed, _ = hh["projectRoot"].(string)
	if f.managed == "" || canonicalEqual(f.managed, f.repos["hh"]) {
		return fmt.Errorf("managed checkout incorrectly points to source: %q", f.managed)
	}
	f.marker = "uncommitted-from-managed-fixture\n"
	if _, err := w.call(ctx, "write_file", map[string]any{
		"repository": "hh", "path": "fixture-uncommitted.txt", "content": f.marker,
	}); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(f.repos["hh"], "fixture-uncommitted.txt")); !os.IsNotExist(err) {
		return fmt.Errorf("fixture managed write leaked to source: %v", err)
	}
	if _, err := w.call(ctx, "set_active_repository", map[string]any{"repository": "hh"}); err != nil {
		return err
	}
	read, err := w.call(ctx, "read_file", map[string]any{"path": "README.md"})
	if err != nil {
		return err
	}
	if err := check(strings.Contains(fmt.Sprint(read["content"]), "compiled-hh"), "implicit member not hh: %+v", read); err != nil {
		return err
	}
	brief, err := w.call(ctx, "get_agent_brief", map[string]any{"repository": "habr"})
	if err != nil {
		return err
	}
	if err := check(strings.Contains(fmt.Sprint(brief), "instructions-for-habr") && !strings.Contains(fmt.Sprint(brief), "instructions-for-hh"), "cross-member AGENTS: %+v", brief); err != nil {
		return err
	}
	chat, err := w.call(ctx, "chat_write", map[string]any{"message": "persisted group chat"})
	if err != nil {
		return err
	}
	f.chatPath, _ = chat["chat_file"].(string)
	if f.chatPath == "" {
		return fmt.Errorf("missing primary CHAT.md path %+v", chat)
	}
	if _, err := w.call(ctx, "show_diff", map[string]any{
		"repository": "hh", "since": "project_open", "advance": true,
	}); err != nil {
		return err
	}
	artifact, err := w.session.CallTool(ctx, &mcp.CallToolParams{
		Meta: mcp.Meta{"openai/session": smokeConversation},
		Name: "export_host_file", Arguments: map[string]any{"repository": "hh", "path": "README.md"},
	})
	if err != nil || artifact == nil || artifact.IsError {
		return fmt.Errorf("artifact export failed: %v %+v", err, artifact)
	}
	for _, item := range artifact.Content {
		if link, ok := item.(*mcp.ResourceLink); ok {
			f.artifact = link.URI
			break
		}
	}
	if f.artifact == "" {
		return errors.New("group export produced no artifact resource")
	}
	if _, err := w.session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: f.artifact, Meta: mcp.Meta{"openai/session": smokeConversation},
	}); err != nil {
		return fmt.Errorf("own scoped artifact read: %w", err)
	}
	if _, err := w.session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: f.artifact, Meta: mcp.Meta{"openai/session": "foreign-smoke-conversation"},
	}); err == nil {
		return errors.New("foreign conversation could read scoped artifact")
	}
	if err := w.expectDenied(ctx, "read_file", map[string]any{
		"repository": "hh", "path": "../habr-career-mcp/README.md",
	}); err != nil {
		return err
	}
	fmt.Println("PG_SMOKE_STAGE_INITIAL=PASS")
	return nil
}

func (f *fixture) second(ctx context.Context, w *runningWorker) error {
	status, err := w.call(ctx, "get_project_group_status", nil)
	if err != nil {
		return err
	}
	if err := check(status["groupName"] == "job-search" && status["activeRepository"] == "hh",
		"group state lost across worker restart: %+v", status); err != nil {
		return err
	}
	hh := member(status["repositories"], "hh")
	if hh == nil || !canonicalEqual(fmt.Sprint(hh["projectRoot"]), f.managed) {
		return fmt.Errorf("managed checkout identity lost after restart: %+v", hh)
	}
	content, err := w.call(ctx, "read_file", map[string]any{"repository": "hh", "path": "fixture-uncommitted.txt"})
	if err != nil {
		return err
	}
	if err := check(strings.Contains(fmt.Sprint(content["content"]), f.marker), "dirty fixture missing after worker restart: %+v", content); err != nil {
		return err
	}
	unchanged, err := w.call(ctx, "show_diff", map[string]any{"repository": "hh", "since": "last_diff", "advance": false})
	if err != nil {
		return err
	}
	if err := check(strings.Contains(fmt.Sprint(unchanged), "No changes since last diff"), "diff cursor moved across restart: %+v", unchanged); err != nil {
		return err
	}
	if _, err := w.session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: f.artifact, Meta: mcp.Meta{"openai/session": smokeConversation},
	}); err != nil {
		return fmt.Errorf("owned artifact did not survive worker restart: %w", err)
	}
	if _, err := w.call(ctx, "set_active_repository", map[string]any{"repository": "habr"}); err != nil {
		return err
	}
	implicit, err := w.call(ctx, "read_file", map[string]any{"path": "README.md"})
	if err != nil {
		return err
	}
	if err := check(strings.Contains(fmt.Sprint(implicit["content"]), "compiled-habr"), "active alias did not switch to habr: %+v", implicit); err != nil {
		return err
	}
	if _, err := w.call(ctx, "set_repository_checkout", map[string]any{"repository": "hh", "checkout": "source"}); err != nil {
		return err
	}
	if _, err := w.session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: f.artifact, Meta: mcp.Meta{"openai/session": smokeConversation},
	}); err == nil {
		return errors.New("scoped artifact survived checkout revision unexpectedly")
	}
	if _, err := os.Stat(filepath.Join(f.managed, "fixture-uncommitted.txt")); err != nil {
		return fmt.Errorf("managed dirty file removed by mode switch: %w", err)
	}
	if _, err := w.call(ctx, "set_repository_checkout", map[string]any{"repository": "hh", "checkout": "managed"}); err != nil {
		return err
	}
	restored, err := w.call(ctx, "get_environment", map[string]any{"repository": "hh"})
	if err != nil {
		return err
	}
	if err := check(canonicalEqual(fmt.Sprint(restored["workspaceRoot"]), f.managed),
		"saved managed checkout was not reused: %+v", restored); err != nil {
		return err
	}
	chat, err := w.call(ctx, "chat_write", map[string]any{"message": "after first worker restart"})
	if err != nil {
		return err
	}
	if err := check(chat["chat_file"] == f.chatPath, "primary group chat moved: %+v", chat); err != nil {
		return err
	}
	fmt.Println("PG_SMOKE_STAGE_RESTART_1=PASS")
	return nil
}

func (f *fixture) third(ctx context.Context, w *runningWorker) error {
	status, err := w.call(ctx, "get_project_group_status", nil)
	if err != nil {
		return err
	}
	if status["activeRepository"] != "habr" || status["groupName"] != "job-search" {
		return fmt.Errorf("second restart did not persist active group state: %+v", status)
	}
	hh := member(status["repositories"], "hh")
	if hh == nil || hh["checkout"] != "managed" || !canonicalEqual(fmt.Sprint(hh["projectRoot"]), f.managed) {
		return fmt.Errorf("second restart lost managed checkout: %+v", hh)
	}
	content, err := w.call(ctx, "read_file", map[string]any{"repository": "hh", "path": "fixture-uncommitted.txt"})
	if err != nil {
		return err
	}
	if !strings.Contains(fmt.Sprint(content["content"]), f.marker) {
		return fmt.Errorf("managed dirty content missing after second restart: %+v", content)
	}
	work, err := w.call(ctx, "read_file", map[string]any{"repository": "work", "path": "README.md"})
	if err != nil {
		return err
	}
	if !strings.Contains(fmt.Sprint(work["content"]), "compiled-work") {
		return fmt.Errorf("primary repo routing lost: %+v", work)
	}
	chat, err := w.call(ctx, "chat_write", map[string]any{"message": "after second worker restart"})
	if err != nil {
		return err
	}
	if chat["chat_file"] != f.chatPath {
		return fmt.Errorf("primary conversation chat moved on second restart: %+v", chat)
	}
	if _, err := w.session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: f.artifact, Meta: mcp.Meta{"openai/session": smokeConversation},
	}); err == nil {
		return errors.New("old artifact revived after checkout roundtrip on second restart")
	}
	fmt.Println("PG_SMOKE_STAGE_RESTART_2=PASS")
	return nil
}
func (f *fixture) disabled(ctx context.Context, w *runningWorker) error {
	tools, err := w.session.ListTools(ctx, nil)
	if err != nil {
		return err
	}
	for _, tool := range tools.Tools {
		if tool.Name == "set_project_group" || tool.Name == "set_repository_checkout" {
			return fmt.Errorf("disabled feature advertised %s", tool.Name)
		}
	}
	if err := w.expectDenied(ctx, "get_environment", nil); err != nil {
		return err
	}
	// Feature-off must fail closed on a v3 chat while retaining the binding
	// bytes unchanged for an explicit future re-enable.
	fmt.Println("PG_SMOKE_STAGE_FLAG_OFF=PASS")
	return nil
}
func (f *fixture) reenabled(ctx context.Context, w *runningWorker) error {
	status, err := w.call(ctx, "get_project_group_status", nil)
	if err != nil {
		return fmt.Errorf("v3 group binding was not preserved after feature-off restart: %w", err)
	}
	if status["activeRepository"] != "habr" {
		return fmt.Errorf("reenabled group lost persisted active alias: %+v", status)
	}
	hh := member(status["repositories"], "hh")
	if hh == nil || hh["checkout"] != "managed" || !canonicalEqual(fmt.Sprint(hh["projectRoot"]), f.managed) {
		return fmt.Errorf("reenabled group lost managed checkout: %+v", hh)
	}
	if _, err := w.call(ctx, "read_file", map[string]any{
		"repository": "hh", "path": "fixture-uncommitted.txt",
	}); err != nil {
		return fmt.Errorf("v3 group managed checkout inaccessible after reenable: %w", err)
	}
	fmt.Println("PG_SMOKE_STAGE_REENABLE=PASS")
	return nil
}
func run(candidate string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	f, err := setup(candidate)
	if err != nil {
		return err
	}
	defer os.RemoveAll(f.dir)
	for step := 0; step < 5; step++ {
		w, err := f.start(ctx, step != 3)
		if err != nil {
			return fmt.Errorf("worker stage %d startup: %w", step, err)
		}
		var stepErr error
		switch step {
		case 0:
			stepErr = f.first(ctx, w)
		case 1:
			stepErr = f.second(ctx, w)
		case 2:
			stepErr = f.third(ctx, w)
		case 3:
			stepErr = f.disabled(ctx, w)
		case 4:
			stepErr = f.reenabled(ctx, w)
		}
		w.close()
		if stepErr != nil {
			return fmt.Errorf("worker stage %d: %w", step, stepErr)
		}
	}
	fmt.Println("PG_SMOKE_RESTARTS=4;FEATURE_DISABLE_REENABLE=PASS;REAL_TUNNEL=OFF;SCM=UNTOUCHED")
	return nil
}
