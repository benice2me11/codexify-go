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
	"strings"
	"time"

	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/benice2me11/codexify-go/internal/ui"
	"github.com/benice2me11/codexify-go/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if len(os.Args) != 2 {
		fail(fmt.Errorf("usage: go run setup_v3_smoke.go <staged-exe>"))
	}
	if err := run(os.Args[1]); err != nil {
		fail(err)
	}
	fmt.Println("SETUP_V3_ISOLATED_BINARY_SMOKE=PASS")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "SMOKE_FAILURE:", err)
	os.Exit(1)
}

func mustGit(dir string, args ...string) error {
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
func checkResponse(out *mcp.CallToolResult, err error, name string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if out == nil || out.IsError {
		return fmt.Errorf("%s returned an error: %+v", name, out)
	}
	return nil
}
func object(out *mcp.CallToolResult) (map[string]any, error) {
	raw, err := json.Marshal(out.StructuredContent)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	err = json.Unmarshal(raw, &data)
	return data, err
}

// Windows runners can spell one directory using both RUNNER~1 and its long
// name. Compare canonical on-disk identity rather than cleaned path strings.
func sameRoot(a, b string) bool {
	left, err := workspace.Canonical(a)
	if err != nil {
		return false
	}
	right, err := workspace.Canonical(b)
	return err == nil && strings.EqualFold(left, right)
}

func run(candidate string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "codexify-setup-v3-smoke-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return err
	}
	if err := mustGit(repo, "init"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(repo, "README.txt"), []byte("isolated source checkout\n"), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"add", "README.txt"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "fixture"}} {
		if err := mustGit(repo, args...); err != nil {
			return err
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	cfg := config.Default()
	cfg.Service.Name = "CodexifySmokeFixture"
	cfg.Log.File = filepath.Join(dir, "worker.jsonl")
	cfg.MCP.WorkspaceRoot = dir
	cfg.MCP.MultiProject = true
	cfg.MCP.AuthEnabled = false
	cfg.MCP.BindingsDir = filepath.Join(dir, ".state", "bindings")
	cfg.MCP.CloneDir = filepath.Join(dir, ".state", "clones")
	cfg.MCP.ScratchDir = filepath.Join(dir, ".state", "scratch")
	cfg.MCP.Worktrees.Root = filepath.Join(dir, ".state", "worktrees")
	cfg.MCP.Worktrees.Mode = "never"
	cfg.MCP.Projects = []config.ProjectSpec{{Name: "repo", Path: repo}}
	cfg.MCP.Upstreams = nil
	cfg.AgentChat.Enabled = false
	cfg.Tunnel.TunnelID = "tunnel_local_setup_v3_fixture"
	cfg.Tunnel.APIKeyRef = "env:UNUSED_OFFLINE_FIXTURE"
	cfg.Tunnel.MCPServerURL = fmt.Sprintf("http://127.0.0.1:%d/mcp", port)
	cfg.Tunnel.HealthURLFile = filepath.Join(dir, "never-used-tunnel-health.url")
	cfg.Tunnel.ManagedDir = filepath.Join(dir, "never-used-tunnel")
	cfg.Tunnel.Environment = nil
	configPath := filepath.Join(dir, "config.json")
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	// config.Load expects duration strings in JSON, whereas config.Duration
	// is a numeric Go alias when directly marshaled from config.Default().
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return err
	}
	setDuration := func(section, field string, value time.Duration) {
		document[section].(map[string]any)[field] = value.String()
	}
	setDuration("supervisor", "minBackoff", cfg.Supervisor.MinBackoff.Duration())
	setDuration("supervisor", "maxBackoff", cfg.Supervisor.MaxBackoff.Duration())
	setDuration("supervisor", "stableWindow", cfg.Supervisor.StableWindow.Duration())
	setDuration("supervisor", "healthInterval", cfg.Supervisor.HealthInterval.Duration())
	setDuration("supervisor", "shutdownTimeout", cfg.Supervisor.ShutdownTimeout.Duration())
	setDuration("tunnel", "startupWaitTimeout", cfg.Tunnel.StartupWaitTimeout.Duration())
	setDuration("tunnel", "pollRestartCooldown", cfg.Tunnel.PollRestartCooldown.Duration())
	setDuration("artifactIngress", "requestTimeout", cfg.ArtifactIngress.RequestTimeout.Duration())
	setDuration("artifactIngress", "idleTimeout", cfg.ArtifactIngress.IdleTimeout.Duration())
	setDuration("artifactEgress", "referenceTtl", cfg.ArtifactEgress.ReferenceTTL.Duration())
	document["mcp"].(map[string]any)["diagnostics"].(map[string]any)["maxDuration"] = cfg.MCP.Diagnostics.MaxDuration.Duration().String()
	raw, err = json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		return err
	}

	worker := exec.CommandContext(ctx, candidate, "worker", "run", "--config", configPath)
	worker.Env = append(os.Environ(), "USERPROFILE="+filepath.Join(dir, "home"))
	var workerOutput bytes.Buffer
	worker.Stdout = &workerOutput
	worker.Stderr = &workerOutput
	if err := os.MkdirAll(filepath.Join(dir, "home"), 0o700); err != nil {
		return err
	}
	if err := worker.Start(); err != nil {
		return err
	}
	defer func() { _ = worker.Process.Kill(); _, _ = worker.Process.Wait() }()
	endpoint := cfg.Tunnel.MCPServerURL
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	ready := false
	for i := 0; i < 100; i++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		hc := &http.Client{Timeout: 300 * time.Millisecond}
		resp, err := hc.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		workerLog, _ := os.ReadFile(cfg.Log.File)
		return fmt.Errorf("isolated worker never reached localhost health; output=%q; log=%q", workerOutput.String(), string(workerLog))
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "setup-v3-isolated-smoke", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: endpoint, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		return err
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		return err
	}
	setupFound := false
	for _, t := range tools.Tools {
		if t.Name == "setup" {
			setupFound = fmt.Sprint(t.Meta["ui/resourceUri"]) == ui.SetupURI
		}
	}
	if !setupFound || !strings.Contains(ui.SetupURI, "/setup/v3/") {
		return fmt.Errorf("setup tool did not advertise v3 UI resource: %s", ui.SetupURI)
	}
	resource, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: ui.SetupURI})
	if err != nil {
		return err
	}
	if len(resource.Contents) != 1 || !strings.Contains(resource.Contents[0].Text, "Source checkout") || !strings.Contains(resource.Contents[0].Text, "Managed worktree") {
		return errors.New("setup v3 resource did not contain checkout modes")
	}
	meta := mcp.Meta{"openai/session": "isolated-fixture-conversation"}
	call := func(name string, args map[string]any) (*mcp.CallToolResult, error) {
		out, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Meta: meta, Arguments: args})
		if err := checkResponse(out, err, name); err != nil {
			return nil, err
		}
		return out, nil
	}

	opened, err := call("setup", map[string]any{})
	if err != nil {
		return err
	}
	openInfo, err := object(opened)
	if err != nil || !strings.Contains(fmt.Sprint(openInfo["projects"]), "repo") {
		return errors.New("setup did not embed discovered fixture project")
	}
	source, err := call("set_project_root", map[string]any{"path": "repo", "createWorktree": false})
	if err != nil {
		return err
	}
	sourceInfo, _ := object(source)
	sourceRoot, _ := sourceInfo["projectRoot"].(string)
	if !sameRoot(sourceRoot, repo) || sourceInfo["managedWorktree"] != false {
		return fmt.Errorf("source selection mismatch: %#v", sourceInfo)
	}
	if _, err := call("read_file", map[string]any{"path": "README.txt"}); err != nil {
		return err
	}
	escape, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "read_file", Meta: meta, Arguments: map[string]any{"path": "../unrelated.txt"}})
	if err != nil || escape == nil || !escape.IsError {
		return fmt.Errorf("parent traversal must be rejected: err=%v", err)
	}
	if _, err := call("setup_ui_switch_project", map[string]any{"expectedPath": sourceRoot}); err != nil {
		return err
	}
	managed, err := call("set_project_root", map[string]any{"path": "repo", "createWorktree": true})
	if err != nil {
		return err
	}
	managedInfo, _ := object(managed)
	managedRoot, _ := managedInfo["projectRoot"].(string)
	if managedInfo["managedWorktree"] != true || managedRoot == "" || sameRoot(managedRoot, sourceRoot) {
		return fmt.Errorf("managed selection mismatch: %#v", managedInfo)
	}
	if _, err := call("setup_ui_switch_project", map[string]any{"expectedPath": managedRoot}); err != nil {
		return err
	}
	back, err := call("set_project_root", map[string]any{"path": "repo", "createWorktree": false})
	if err != nil {
		return err
	}
	backInfo, _ := object(back)
	if !sameRoot(fmt.Sprint(backInfo["projectRoot"]), sourceRoot) || backInfo["managedWorktree"] != false {
		return fmt.Errorf("same chat did not return to source: %#v", backInfo)
	}
	if _, err := os.Stat(managedRoot); err != nil {
		return fmt.Errorf("previous managed checkout unexpectedly lost: %w", err)
	}
	return nil
}
