package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Duration time.Duration

func (d *Duration) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("duration must be a string: %w", err)
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

func (d Duration) Duration() time.Duration { return time.Duration(d) }

type Config struct {
	Version         int                   `json:"version"`
	Log             LogConfig             `json:"log"`
	Service         ServiceConfig         `json:"service"`
	MCP             MCPConfig             `json:"mcp"`
	Memory          MemoryConfig          `json:"memory"`
	Skills          SkillsConfig          `json:"skills"`
	ProjectDoc      ProjectDocConfig      `json:"projectDoc"`
	Diff            DiffConfig            `json:"diff"`
	ArtifactIngress ArtifactIngressConfig `json:"artifactIngress"`
	ArtifactEgress  ArtifactEgressConfig  `json:"artifactEgress"`
	AgentChat       AgentChatConfig       `json:"agentChat"`
	Experimental    ExperimentalConfig    `json:"experimental"`
	Tunnel          TunnelConfig          `json:"tunnel"`
	Supervisor      SupervisorConfig      `json:"supervisor"`
}

type LogConfig struct {
	File  string `json:"file"`
	Level string `json:"level"`
}

type ServiceConfig struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

type MCPConfig struct {
	WorkspaceRoot       string               `json:"workspaceRoot"`
	AuthEnabled         bool                 `json:"authEnabled"`
	MaxRequestBodyBytes int64                `json:"maxRequestBodyBytes"`
	MultiProject        bool                 `json:"multiProject"`
	BindingsDir         string               `json:"bindingsDir,omitempty"`
	CloneDir            string               `json:"cloneDir,omitempty"`
	ScratchDir          string               `json:"scratchDir,omitempty"`
	ProjectScanDepth    int                  `json:"projectScanDepth,omitempty"`
	Worktrees           WorktreeConfig       `json:"worktrees"`
	Projects            []ProjectSpec        `json:"projects,omitempty"`
	Upstreams           []UpstreamMCPConfig  `json:"upstreams,omitempty"`
	Diagnostics         MCPDiagnosticsConfig `json:"diagnostics"`
}

// MCPDiagnosticsConfig is opt-in and never enables a tunnel or changes routing.
type MCPDiagnosticsConfig struct {
	Enabled     bool     `json:"enabled"`
	Directory   string   `json:"directory,omitempty"`
	MaxEvents   int      `json:"maxEvents"`
	MaxDuration Duration `json:"maxDuration"`
}

type WorktreeConfig struct {
	Mode string `json:"mode"`
	Root string `json:"root,omitempty"`
}

type ProjectSpec struct {
	Path        string `json:"path"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type UpstreamMCPConfig struct {
	Name            string            `json:"name"`
	Transport       string            `json:"transport,omitempty"`
	Mode            string            `json:"mode,omitempty"`
	ProtocolVersion string            `json:"protocolVersion,omitempty"`
	Command         string            `json:"command,omitempty"`
	Args            []string          `json:"args,omitempty"`
	Workdir         string            `json:"workdir,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	URL             string            `json:"url,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Required        bool              `json:"required,omitempty"`
	Timeout         Duration          `json:"timeout,omitempty"`
}

type MemoryConfig struct {
	Enabled  bool   `json:"enabled"`
	Dir      string `json:"dir,omitempty"`
	MaxBytes int    `json:"maxBytes,omitempty"`
}

type SkillsConfig struct {
	Enabled        bool     `json:"enabled"`
	IncludeUser    bool     `json:"includeUser"`
	IncludePlugins *bool    `json:"includePlugins,omitempty"`
	ClaudePlugins  bool     `json:"claudePlugins,omitempty"`
	Dirs           []string `json:"dirs,omitempty"`
}

type ProjectDocConfig struct {
	MaxBytes          int      `json:"maxBytes,omitempty"`
	FallbackFilenames []string `json:"fallbackFilenames,omitempty"`
	RootMarkers       []string `json:"rootMarkers,omitempty"`
}

type DiffConfig struct {
	MaxPatchBytes int `json:"maxPatchBytes"`
}

type ArtifactIngressConfig struct {
	Enabled                bool     `json:"enabled"`
	MaxFileBytes           int64    `json:"maxFileBytes"`
	RequestTimeout         Duration `json:"requestTimeout"`
	IdleTimeout            Duration `json:"idleTimeout"`
	MaxRedirects           int      `json:"maxRedirects"`
	MaxConcurrentDownloads int      `json:"maxConcurrentDownloads"`
	AllowedHosts           []string `json:"allowedHosts"`
}

type ArtifactEgressConfig struct {
	Enabled              bool     `json:"enabled"`
	Dir                  string   `json:"dir,omitempty"`
	MaxFileBytes         int64    `json:"maxFileBytes,omitempty"`
	SnapshotMaxFileBytes int64    `json:"snapshotMaxFileBytes,omitempty"`
	MaxSnapshotBytes     int64    `json:"maxSnapshotBytes,omitempty"`
	FallbackToSource     bool     `json:"fallbackToSource"`
	MaxReferences        int      `json:"maxReferences,omitempty"`
	ReferenceTTL         Duration `json:"referenceTtl,omitempty"`
}

type AgentChatConfig struct {
	Enabled   bool   `json:"enabled"`
	Dir       string `json:"dir,omitempty"`
	MaxWaitMS int    `json:"maxWaitMs,omitempty"`
}

type ExperimentalConfig struct {
	AgentTickets bool `json:"agentTickets"`
}

type TunnelConfig struct {
	Executable          string            `json:"executable,omitempty"`
	ManagedDir          string            `json:"managedDir,omitempty"`
	TunnelID            string            `json:"tunnelId"`
	APIKeyRef           string            `json:"apiKeyRef"`
	OrganizationID      string            `json:"organizationId,omitempty"`
	MCPServerURL        string            `json:"mcpServerUrl"`
	MCPAuthorizationRef string            `json:"mcpAuthorizationRef,omitempty"`
	StartupWaitTimeout  Duration          `json:"startupWaitTimeout"`
	HealthURLFile       string            `json:"healthUrlFile"`
	Environment         map[string]string `json:"environment,omitempty"`
	ExtraArgs           []string          `json:"extraArgs,omitempty"`
}

type SupervisorConfig struct {
	MinBackoff             Duration `json:"minBackoff"`
	MaxBackoff             Duration `json:"maxBackoff"`
	StableWindow           Duration `json:"stableWindow"`
	HealthInterval         Duration `json:"healthInterval"`
	HealthFailureThreshold int      `json:"healthFailureThreshold"`
	ShutdownTimeout        Duration `json:"shutdownTimeout"`
}

func Default() Config {
	return Config{
		Version: 1,
		Log: LogConfig{
			File:  "codexify-go.log",
			Level: "info",
		},
		Service: ServiceConfig{
			Name:        "CodexifyGo",
			DisplayName: "Codexify Go",
			Description: "Native supervisor for Codexify-compatible MCP and OpenAI tunnel runtime.",
		},
		MCP: MCPConfig{
			WorkspaceRoot:       ".",
			AuthEnabled:         true,
			MaxRequestBodyBytes: 4 << 20,
			ProjectScanDepth:    2,
			Diagnostics: MCPDiagnosticsConfig{
				MaxEvents:   20000,
				MaxDuration: Duration(10 * time.Minute),
			},
			Worktrees: WorktreeConfig{
				Mode: "auto",
			},
		},
		Memory: MemoryConfig{
			Enabled:  true,
			MaxBytes: 16 * 1024,
		},
		Skills: SkillsConfig{
			Enabled:     true,
			IncludeUser: true,
		},
		ProjectDoc: ProjectDocConfig{
			MaxBytes:    32 * 1024,
			RootMarkers: []string{".git"},
		},
		Diff: DiffConfig{
			MaxPatchBytes: 4 * 1024 * 1024,
		},
		ArtifactIngress: ArtifactIngressConfig{
			Enabled:                true,
			MaxFileBytes:           100 * 1024 * 1024,
			RequestTimeout:         Duration(2 * time.Minute),
			IdleTimeout:            Duration(30 * time.Second),
			MaxRedirects:           3,
			MaxConcurrentDownloads: 2,
			AllowedHosts:           []string{"*"},
		},
		ArtifactEgress: ArtifactEgressConfig{
			Enabled:              true,
			MaxFileBytes:         100 * 1024 * 1024,
			SnapshotMaxFileBytes: 100 * 1024 * 1024,
			MaxSnapshotBytes:     5 * 1024 * 1024 * 1024,
			FallbackToSource:     true,
			MaxReferences:        64,
			ReferenceTTL:         Duration(5 * time.Minute),
		},
		AgentChat: AgentChatConfig{
			Enabled:   false,
			MaxWaitMS: 55_000,
		},
		Tunnel: TunnelConfig{
			StartupWaitTimeout: Duration(15 * time.Second),
			HealthURLFile:      filepath.Join(os.TempDir(), "codexify-go-tunnel-health.url"),
		},
		Supervisor: SupervisorConfig{
			MinBackoff:             Duration(2 * time.Second),
			MaxBackoff:             Duration(60 * time.Second),
			StableWindow:           Duration(60 * time.Second),
			HealthInterval:         Duration(5 * time.Second),
			HealthFailureThreshold: 3,
			ShutdownTimeout:        Duration(10 * time.Second),
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return Config{}, fmt.Errorf("resolve config path: %w", err)
	}
	base := filepath.Dir(absPath)
	cfg.expand(base)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) expand(base string) {
	expand := func(v string) string {
		v = os.ExpandEnv(v)
		if v == "" || filepath.IsAbs(v) {
			return v
		}
		return filepath.Clean(filepath.Join(base, v))
	}
	c.Log.File = expand(c.Log.File)
	c.MCP.Diagnostics.Directory = expand(c.MCP.Diagnostics.Directory)
	if c.Memory.Dir != "" {
		c.Memory.Dir = expand(c.Memory.Dir)
	}
	if c.ArtifactEgress.Dir != "" {
		c.ArtifactEgress.Dir = expand(c.ArtifactEgress.Dir)
	}
	if c.AgentChat.Dir == "" {
		c.AgentChat.Dir = filepath.Join(base, ".codexify-go", "chats")
	} else {
		c.AgentChat.Dir = expand(c.AgentChat.Dir)
	}
	for i, dir := range c.Skills.Dirs {
		c.Skills.Dirs[i] = expand(dir)
	}
	c.MCP.WorkspaceRoot = expand(c.MCP.WorkspaceRoot)
	if c.MCP.BindingsDir == "" {
		c.MCP.BindingsDir = filepath.Join(c.MCP.WorkspaceRoot, ".codexify-go", "bindings")
	} else {
		c.MCP.BindingsDir = expand(c.MCP.BindingsDir)
	}
	if c.MCP.CloneDir == "" {
		c.MCP.CloneDir = filepath.Join(c.MCP.WorkspaceRoot, ".codexify-go", "clones")
	} else {
		c.MCP.CloneDir = expand(c.MCP.CloneDir)
	}
	if c.MCP.ScratchDir == "" {
		c.MCP.ScratchDir = filepath.Join(c.MCP.WorkspaceRoot, ".codexify-go", "scratch")
	} else {
		c.MCP.ScratchDir = expand(c.MCP.ScratchDir)
	}
	if c.MCP.Worktrees.Root == "" {
		c.MCP.Worktrees.Root = filepath.Join(c.MCP.WorkspaceRoot, ".codexify-go", "worktrees")
	} else {
		c.MCP.Worktrees.Root = expand(c.MCP.Worktrees.Root)
	}
	for i := range c.MCP.Projects {
		c.MCP.Projects[i].Path = expand(c.MCP.Projects[i].Path)
	}
	for i := range c.MCP.Upstreams {
		u := &c.MCP.Upstreams[i]
		u.Command = os.ExpandEnv(u.Command)
		if u.Workdir != "" {
			u.Workdir = expand(u.Workdir)
		}
		u.URL = os.ExpandEnv(u.URL)
		for j, arg := range u.Args {
			u.Args[j] = os.ExpandEnv(arg)
		}
		for k, v := range u.Env {
			u.Env[k] = os.ExpandEnv(v)
		}
		for k, v := range u.Headers {
			u.Headers[k] = os.ExpandEnv(v)
		}
	}
	c.Tunnel.Executable = expand(c.Tunnel.Executable)
	if c.Tunnel.ManagedDir == "" {
		c.Tunnel.ManagedDir = filepath.Join(base, ".codexify-go", "openai-tunnel")
	} else {
		c.Tunnel.ManagedDir = expand(c.Tunnel.ManagedDir)
	}
	c.Tunnel.HealthURLFile = expand(c.Tunnel.HealthURLFile)
	c.Tunnel.APIKeyRef = expandReference(c.Tunnel.APIKeyRef, base)
	c.Tunnel.MCPAuthorizationRef = os.ExpandEnv(c.Tunnel.MCPAuthorizationRef)
	c.Tunnel.MCPServerURL = os.ExpandEnv(c.Tunnel.MCPServerURL)
	for k, v := range c.Tunnel.Environment {
		c.Tunnel.Environment[k] = os.ExpandEnv(v)
	}
}

func expandReference(v, base string) string {
	v = os.ExpandEnv(v)
	if !strings.HasPrefix(v, "file:") {
		return v
	}
	path := strings.TrimPrefix(v, "file:")
	if path == "" {
		return v
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return "file:" + filepath.Clean(path)
}

func (c Config) Validate() error {
	var errs []error
	if c.Version != 1 {
		errs = append(errs, fmt.Errorf("unsupported config version %d", c.Version))
	}
	if strings.TrimSpace(c.Service.Name) == "" {
		errs = append(errs, errors.New("service.name is required"))
	}
	if strings.TrimSpace(c.MCP.WorkspaceRoot) == "" {
		errs = append(errs, errors.New("mcp.workspaceRoot is required"))
	}
	if c.MCP.MaxRequestBodyBytes <= 0 {
		errs = append(errs, errors.New("mcp.maxRequestBodyBytes must be > 0"))
	}
	if c.MCP.Diagnostics.Enabled {
		if strings.TrimSpace(c.MCP.Diagnostics.Directory) == "" {
			errs = append(errs, errors.New("mcp.diagnostics.directory is required when enabled"))
		}
		if c.MCP.Diagnostics.MaxEvents < 4 || c.MCP.Diagnostics.MaxEvents > 100000 {
			errs = append(errs, errors.New("mcp.diagnostics.maxEvents must be between 4 and 100000"))
		}
		if d := c.MCP.Diagnostics.MaxDuration.Duration(); d <= 0 || d > time.Hour {
			errs = append(errs, errors.New("mcp.diagnostics.maxDuration must be > 0 and <= 1h"))
		}
	}
	if c.Memory.MaxBytes <= 0 {
		errs = append(errs, errors.New("memory.maxBytes must be > 0"))
	}
	if c.ProjectDoc.MaxBytes < 0 {
		errs = append(errs, errors.New("projectDoc.maxBytes must be >= 0"))
	}
	if c.Diff.MaxPatchBytes < 0 {
		errs = append(errs, errors.New("diff.maxPatchBytes must be >= 0"))
	}
	if c.ArtifactIngress.MaxFileBytes <= 0 {
		errs = append(errs, errors.New("artifactIngress.maxFileBytes must be > 0"))
	}
	if c.ArtifactIngress.RequestTimeout.Duration() <= 0 {
		errs = append(errs, errors.New("artifactIngress.requestTimeout must be > 0"))
	}
	if c.ArtifactIngress.IdleTimeout.Duration() <= 0 || c.ArtifactIngress.IdleTimeout.Duration() > c.ArtifactIngress.RequestTimeout.Duration() {
		errs = append(errs, errors.New("artifactIngress.idleTimeout must be > 0 and <= requestTimeout"))
	}
	if c.ArtifactIngress.MaxRedirects < 0 || c.ArtifactIngress.MaxRedirects > 10 {
		errs = append(errs, errors.New("artifactIngress.maxRedirects must be between 0 and 10"))
	}
	if c.ArtifactIngress.MaxConcurrentDownloads < 1 || c.ArtifactIngress.MaxConcurrentDownloads > 16 {
		errs = append(errs, errors.New("artifactIngress.maxConcurrentDownloads must be between 1 and 16"))
	}
	if len(c.ArtifactIngress.AllowedHosts) == 0 {
		errs = append(errs, errors.New("artifactIngress.allowedHosts must not be empty"))
	}
	if c.ArtifactEgress.MaxFileBytes <= 0 {
		errs = append(errs, errors.New("artifactEgress.maxFileBytes must be > 0"))
	}
	if c.ArtifactEgress.SnapshotMaxFileBytes < 0 {
		errs = append(errs, errors.New("artifactEgress.snapshotMaxFileBytes must be >= 0"))
	}
	if c.ArtifactEgress.MaxSnapshotBytes < 0 {
		errs = append(errs, errors.New("artifactEgress.maxSnapshotBytes must be >= 0"))
	}
	if c.ArtifactEgress.MaxReferences < 1 || c.ArtifactEgress.MaxReferences > 1024 {
		errs = append(errs, errors.New("artifactEgress.maxReferences must be between 1 and 1024"))
	}
	if c.ArtifactEgress.ReferenceTTL.Duration() <= 0 {
		errs = append(errs, errors.New("artifactEgress.referenceTtl must be > 0"))
	}
	if c.AgentChat.MaxWaitMS < 1_000 || c.AgentChat.MaxWaitMS > 300_000 {
		errs = append(errs, errors.New("agentChat.maxWaitMs must be between 1000 and 300000"))
	}
	if c.MCP.ProjectScanDepth < 0 || c.MCP.ProjectScanDepth > 8 {
		errs = append(errs, errors.New("mcp.projectScanDepth must be between 0 and 8"))
	}
	switch strings.ToLower(c.MCP.Worktrees.Mode) {
	case "auto", "always", "never":
	default:
		errs = append(errs, errors.New("mcp.worktrees.mode must be auto, always, or never"))
	}
	seenUpstreams := map[string]struct{}{}
	for i, upstream := range c.MCP.Upstreams {
		name := strings.TrimSpace(upstream.Name)
		if name == "" {
			errs = append(errs, fmt.Errorf("mcp.upstreams[%d].name is required", i))
			continue
		}
		if _, exists := seenUpstreams[name]; exists {
			errs = append(errs, fmt.Errorf("duplicate MCP upstream name %q", name))
		}
		seenUpstreams[name] = struct{}{}
		transport := strings.ToLower(strings.TrimSpace(upstream.Transport))
		if transport == "" {
			if upstream.URL != "" {
				transport = "streamable_http"
			} else {
				transport = "stdio"
			}
		}
		switch transport {
		case "stdio":
			if strings.TrimSpace(upstream.Command) == "" {
				errs = append(errs, fmt.Errorf("MCP upstream %q stdio transport requires command", name))
			}
		case "streamable_http", "streamable-http", "http":
			if strings.TrimSpace(upstream.URL) == "" {
				errs = append(errs, fmt.Errorf("MCP upstream %q HTTP transport requires url", name))
			}
		default:
			errs = append(errs, fmt.Errorf("MCP upstream %q has unsupported transport %q", name, transport))
		}
		mode := strings.ToLower(strings.TrimSpace(upstream.Mode))
		if mode == "" {
			mode = "catalog"
		}
		if mode != "catalog" && mode != "direct" && mode != "gateway" {
			errs = append(errs, fmt.Errorf("MCP upstream %q mode must be catalog, direct, or gateway", name))
		}
	}
	if strings.TrimSpace(c.Tunnel.Executable) == "" && strings.TrimSpace(c.Tunnel.ManagedDir) == "" {
		errs = append(errs, errors.New("tunnel.managedDir is required when tunnel.executable is omitted"))
	}
	if strings.TrimSpace(c.Tunnel.TunnelID) == "" {
		errs = append(errs, errors.New("tunnel.tunnelId is required"))
	}
	if strings.TrimSpace(c.Tunnel.APIKeyRef) == "" {
		errs = append(errs, errors.New("tunnel.apiKeyRef is required"))
	}
	if strings.TrimSpace(c.Tunnel.MCPServerURL) == "" {
		errs = append(errs, errors.New("tunnel.mcpServerUrl is required"))
	} else if err := validateMCPServerURL(c.Tunnel.MCPServerURL); err != nil {
		errs = append(errs, err)
	}
	if c.Supervisor.MinBackoff.Duration() <= 0 {
		errs = append(errs, errors.New("supervisor.minBackoff must be > 0"))
	}
	if c.Supervisor.MaxBackoff.Duration() < c.Supervisor.MinBackoff.Duration() {
		errs = append(errs, errors.New("supervisor.maxBackoff must be >= minBackoff"))
	}
	if c.Supervisor.StableWindow.Duration() <= 0 {
		errs = append(errs, errors.New("supervisor.stableWindow must be > 0"))
	}
	if c.Supervisor.HealthInterval.Duration() <= 0 {
		errs = append(errs, errors.New("supervisor.healthInterval must be > 0"))
	}
	if c.Supervisor.HealthFailureThreshold <= 0 {
		errs = append(errs, errors.New("supervisor.healthFailureThreshold must be > 0"))
	}
	if c.Supervisor.ShutdownTimeout.Duration() <= 0 {
		errs = append(errs, errors.New("supervisor.shutdownTimeout must be > 0"))
	}
	return errors.Join(errs...)
}

func validateMCPServerURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("tunnel.mcpServerUrl: %w", err)
	}
	if u.Scheme != "http" {
		return errors.New("tunnel.mcpServerUrl must use http")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("tunnel.mcpServerUrl must use a loopback host, got %q", host)
	}
	if u.Port() == "" {
		return errors.New("tunnel.mcpServerUrl must include an explicit port")
	}
	return nil
}

func (c Config) TunnelArgs() []string {
	args := []string{
		"run",
		"--control-plane.tunnel-id", c.Tunnel.TunnelID,
		"--control-plane.api-key", c.Tunnel.APIKeyRef,
		"--mcp.server-url", c.Tunnel.MCPServerURL,
	}
	if c.Tunnel.MCPAuthorizationRef != "" {
		header := "Authorization: " + c.Tunnel.MCPAuthorizationRef
		args = append(args,
			"--mcp.extra-headers", header,
			"--mcp.discovery-extra-headers", header,
		)
	}
	if c.Tunnel.OrganizationID != "" {
		args = append(args, "--control-plane.organization-id", c.Tunnel.OrganizationID)
	}
	args = append(args,
		"--mcp.startup-wait-timeout", c.Tunnel.StartupWaitTimeout.Duration().String(),
		"--health.listen-addr", "127.0.0.1:0",
		"--health.url-file", c.Tunnel.HealthURLFile,
		"--log.format", "json",
	)
	return append(args, c.Tunnel.ExtraArgs...)
}
