package upstream

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/benice2me11/codexify-go/internal/buildinfo"
	"github.com/benice2me11/codexify-go/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

type Bridge struct {
	mu               sync.RWMutex
	sources          map[string]*source
	resources        map[string]resourceRef
	sessions         []*mcp.ClientSession
	report           []string
	log              *slog.Logger
	maxResourceBytes int64
}

const resourcePrefix = "codexify-go://upstream-resource/"

type resourceRef struct {
	source *source
	uri    string
}

type source struct {
	name      string
	mode      string
	transport string
	timeout   time.Duration
	session   *mcp.ClientSession
	info      *mcp.Implementation
	tools     map[string]*mcp.Tool
}

type SourceInfo struct {
	Name       string `json:"name"`
	Mode       string `json:"mode"`
	Transport  string `json:"transport"`
	ServerName string `json:"serverName,omitempty"`
	Version    string `json:"version,omitempty"`
	ToolCount  int    `json:"toolCount"`
}

type ListSourcesInput struct{}

type ListSourcesOutput struct {
	Sources []SourceInfo `json:"sources"`
}

type SearchToolsInput struct {
	Query  string `json:"query,omitempty" jsonschema:"case-insensitive substring over source, tool name, title, and description"`
	Source string `json:"source,omitempty" jsonschema:"optional upstream source name"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum results; default 50 and maximum 200"`
}

type ToolInfo struct {
	Source       string `json:"source"`
	Name         string `json:"name"`
	Title        string `json:"title,omitempty"`
	Description  string `json:"description,omitempty"`
	InputSchema  any    `json:"inputSchema,omitempty"`
	OutputSchema any    `json:"outputSchema,omitempty"`
}

type SearchToolsOutput struct {
	Tools []ToolInfo `json:"tools"`
	Total int        `json:"total"`
}

type GetToolInput struct {
	Source string `json:"source"`
	Name   string `json:"name"`
}

type CallToolInput struct {
	Source    string         `json:"source"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

func ConnectAndRegister(ctx context.Context, specs []config.UpstreamMCPConfig, server *mcp.Server, logger *slog.Logger, used map[string]struct{}, maxResourceBytes int64, generatedSkillsDir string) (*Bridge, error) {
	if logger == nil {
		logger = slog.Default()
	}
	b := &Bridge{
		sources:          make(map[string]*source),
		resources:        make(map[string]resourceRef),
		log:              logger,
		maxResourceBytes: maxResourceBytes,
	}
	if b.maxResourceBytes <= 0 {
		b.maxResourceBytes = 100 * 1024 * 1024
	}
	if len(specs) > 0 {
		server.AddResourceTemplate(&mcp.ResourceTemplate{
			URITemplate: resourcePrefix + "{token}",
			Name:        "codexify-go-upstream-resource",
			Title:       "Bridged upstream MCP resource",
			Description: "Opaque capability URI for resources returned by an upstream MCP tool.",
		}, b.readResource)
	}
	if generatedSkillsDir != "" {
		_ = os.RemoveAll(generatedSkillsDir)
	}
	for _, spec := range specs {
		src, err := connectSource(ctx, spec, logger)
		if err != nil {
			line := fmt.Sprintf("%s -> FAILED: %v", spec.Name, err)
			b.report = append(b.report, line)
			if spec.Required {
				b.Close()
				return nil, errors.New(line)
			}
			logger.Warn("upstream MCP unavailable", "source", spec.Name, "error", err)
			continue
		}
		b.sources[src.name] = src
		b.sessions = append(b.sessions, src.session)
		b.report = append(b.report, fmt.Sprintf("%s -> %s (%d tool(s))", src.name, src.mode, len(src.tools)))
		switch src.mode {
		case "direct":
			b.registerDirect(server, src, used)
		case "gateway":
			b.registerGateway(server, src, used, generatedSkillsDir)
		}
	}

	if b.hasCatalogSources() {
		b.registerCatalog(server, used)
	}
	return b, nil
}

func (b *Bridge) registerGateway(server *mcp.Server, src *source, used map[string]struct{}, generatedSkillsDir string) {
	name := uniqueToolName(sanitizeGatewayName(src.name), used)
	functionNames := make([]string, 0, len(src.tools))
	for function := range src.tools {
		functionNames = append(functionNames, function)
	}
	sort.Strings(functionNames)
	description := gatewayDescription(src, functionNames)
	server.AddTool(&mcp.Tool{
		Name:        name,
		Title:       "Call " + src.name + " MCP tool",
		Description: description,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"function": map[string]any{
					"type":        "string",
					"enum":        functionNames,
					"description": "The upstream MCP function to call.",
				},
				"arguments": map[string]any{
					"type":        "object",
					"description": "Arguments for the selected function; see the generated skill for the exact schema.",
				},
			},
			"required":             []string{"function"},
			"additionalProperties": false,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var input struct {
			Function  string         `json:"function"`
			Arguments map[string]any `json:"arguments,omitempty"`
		}
		if req == nil || req.Params == nil {
			return nil, errors.New("gateway arguments are required")
		}
		if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
			return nil, fmt.Errorf("decode gateway arguments: %w", err)
		}
		tool := src.tools[input.Function]
		if tool == nil {
			return nil, fmt.Errorf("unknown %s function %q; read the %q skill for the function list", src.name, input.Function, src.name)
		}
		callCtx, cancel := context.WithTimeout(ctx, src.timeout)
		defer cancel()
		result, err := src.session.CallTool(callCtx, &mcp.CallToolParams{Name: input.Function, Arguments: input.Arguments})
		if err != nil {
			return nil, err
		}
		return b.rewriteResourceLinks(src, result)
	})
	if generatedSkillsDir != "" {
		if err := writeGatewaySkill(generatedSkillsDir, src, name, functionNames); err != nil {
			b.log.Warn("could not write gateway skill", "source", src.name, "error", err)
		}
	}
}

func gatewayDescription(src *source, functions []string) string {
	var lines []string
	for _, name := range functions {
		tool := src.tools[name]
		summary := ""
		if tool != nil {
			summary = strings.TrimSpace(strings.Split(tool.Description, "\n")[0])
			if len(summary) > 100 {
				summary = summary[:100]
			}
		}
		if summary == "" {
			lines = append(lines, "- "+name)
		} else {
			lines = append(lines, "- "+name+": "+summary)
		}
	}
	return fmt.Sprintf("Gateway to the %q MCP server - call any of its %d functions through this one tool. Call it as {\"function\":\"<name>\",\"arguments\":{...}}. For exact argument schemas read the %q skill with skills_read.\n\nFunctions:\n%s", src.name, len(functions), src.name, strings.Join(lines, "\n"))
}

func writeGatewaySkill(base string, src *source, gatewayName string, functions []string) error {
	dir := filepath.Join(base, sanitizeGatewayName(src.name))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	frontmatter, err := yaml.Marshal(map[string]any{
		"name":        src.name,
		"description": fmt.Sprintf("Call the %s MCP server's %d functions through the %s gateway tool. Use when a task needs %s operations.", src.name, len(functions), gatewayName, src.name),
	})
	if err != nil {
		return err
	}
	var body strings.Builder
	body.WriteString("---\n")
	body.Write(frontmatter)
	body.WriteString("---\n\n# ")
	body.WriteString(src.name)
	body.WriteString(" gateway\n\nEvery function is invoked through the single `")
	body.WriteString(gatewayName)
	body.WriteString("` tool with `{\"function\":\"<name>\",\"arguments\":{...}}`.\n\n")
	for _, name := range functions {
		tool := src.tools[name]
		body.WriteString("## ")
		body.WriteString(name)
		body.WriteString("\n\n")
		if tool != nil && strings.TrimSpace(tool.Description) != "" {
			body.WriteString(tool.Description)
			body.WriteString("\n\n")
		}
		body.WriteString("Arguments:\n\n```json\n")
		schema := any(map[string]any{"type": "object"})
		if tool != nil && tool.InputSchema != nil {
			schema = tool.InputSchema
		}
		encoded, _ := json.MarshalIndent(schema, "", "  ")
		body.Write(encoded)
		body.WriteString("\n```\n\n")
	}
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body.String()), 0o600)
}

func sanitizeGatewayName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		out = "mcp_gateway"
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

func (b *Bridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, session := range b.sessions {
		_ = session.Close()
	}
	b.sessions = nil
}

func (b *Bridge) Report() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]string(nil), b.report...)
}

func connectSource(ctx context.Context, spec config.UpstreamMCPConfig, logger *slog.Logger) (*source, error) {
	transportKind := normalizedTransport(spec)
	mode := strings.ToLower(strings.TrimSpace(spec.Mode))
	if mode == "" {
		mode = "catalog"
	}
	timeout := spec.Timeout.Duration()
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "codexify-go-bridge", Version: buildinfo.Version}, nil)
	var transport mcp.Transport
	switch transportKind {
	case "stdio":
		cmd := exec.Command(spec.Command, spec.Args...)
		if spec.Workdir != "" {
			cmd.Dir = spec.Workdir
		}
		cmd.Env = append([]string{}, os.Environ()...)
		for key, value := range spec.Env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
		transport = &mcp.CommandTransport{
			Command:           cmd,
			TerminateDuration: 3 * time.Second,
		}
	case "streamable_http":
		transport = &mcp.StreamableClientTransport{
			Endpoint:             spec.URL,
			DisableStandaloneSSE: true,
			MaxRetries:           -1,
			HTTPClient: &http.Client{
				Transport: &headerRoundTripper{
					base:    http.DefaultTransport,
					headers: spec.Headers,
				},
			},
		}
	default:
		return nil, fmt.Errorf("unsupported transport %q", transportKind)
	}

	var sessionOpts *mcp.ClientSessionOptions
	if version := strings.TrimSpace(spec.ProtocolVersion); version != "" {
		sessionOpts = &mcp.ClientSessionOptions{ProtocolVersion: version}
	}
	session, err := client.Connect(connectCtx, transport, sessionOpts)
	if err != nil {
		return nil, err
	}
	tools, err := listAllTools(connectCtx, session)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	toolMap := make(map[string]*mcp.Tool, len(tools))
	for _, tool := range tools {
		if tool == nil || strings.TrimSpace(tool.Name) == "" {
			continue
		}
		toolMap[tool.Name] = cloneTool(tool)
	}
	var info *mcp.Implementation
	if init := session.InitializeResult(); init != nil && init.ServerInfo != nil {
		copyInfo := *init.ServerInfo
		info = &copyInfo
	}
	logger.Info("upstream MCP connected",
		"source", spec.Name,
		"mode", mode,
		"transport", transportKind,
		"tools", len(toolMap),
	)
	return &source{
		name:      spec.Name,
		mode:      mode,
		transport: transportKind,
		timeout:   timeout,
		session:   session,
		info:      info,
		tools:     toolMap,
	}, nil
}

func listAllTools(ctx context.Context, session *mcp.ClientSession) ([]*mcp.Tool, error) {
	var tools []*mcp.Tool
	cursor := ""
	for {
		result, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		tools = append(tools, result.Tools...)
		if result.NextCursor == "" {
			return tools, nil
		}
		cursor = result.NextCursor
	}
}

func (b *Bridge) registerDirect(server *mcp.Server, src *source, used map[string]struct{}) {
	names := make([]string, 0, len(src.tools))
	for name := range src.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, original := range names {
		upstreamTool := src.tools[original]
		downstreamName := uniqueToolName(sanitizeToolName(src.name+"__"+original), used)
		tool := cloneTool(upstreamTool)
		tool.Name = downstreamName
		if strings.TrimSpace(tool.Title) == "" {
			tool.Title = src.name + ": " + original
		}
		server.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			callCtx, cancel := context.WithTimeout(ctx, src.timeout)
			defer cancel()
			var args any = map[string]any{}
			if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					return nil, fmt.Errorf("decode bridged arguments: %w", err)
				}
			}
			result, err := src.session.CallTool(callCtx, &mcp.CallToolParams{
				Name:      original,
				Arguments: args,
			})
			if err != nil {
				return nil, err
			}
			return b.rewriteResourceLinks(src, result)
		})
	}
}

func (b *Bridge) registerCatalog(server *mcp.Server, used map[string]struct{}) {
	listName := uniqueToolName("mcp_list_sources", used)
	searchName := uniqueToolName("mcp_search_tools", used)
	getName := uniqueToolName("mcp_get_tool", used)
	callName := uniqueToolName("mcp_call_tool", used)

	mcp.AddTool(server, &mcp.Tool{Name: listName, Description: "List privately indexed upstream MCP servers."},
		func(context.Context, *mcp.CallToolRequest, ListSourcesInput) (*mcp.CallToolResult, ListSourcesOutput, error) {
			return nil, b.listSources(), nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: searchName, Description: "Search tools exposed by privately indexed upstream MCP servers."},
		func(_ context.Context, _ *mcp.CallToolRequest, in SearchToolsInput) (*mcp.CallToolResult, SearchToolsOutput, error) {
			return nil, b.searchTools(in), nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: getName, Description: "Get one upstream MCP tool definition without exposing every upstream tool in the main catalog."},
		func(_ context.Context, _ *mcp.CallToolRequest, in GetToolInput) (*mcp.CallToolResult, ToolInfo, error) {
			info, err := b.getTool(in.Source, in.Name)
			return nil, info, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: callName, Description: "Call one tool on a privately indexed upstream MCP server."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in CallToolInput) (*mcp.CallToolResult, any, error) {
			result, err := b.callTool(ctx, in.Source, in.Name, in.Arguments)
			if err != nil {
				return nil, nil, err
			}
			return result, result.StructuredContent, nil
		})
}

func (b *Bridge) listSources() ListSourcesOutput {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var sources []SourceInfo
	for _, src := range b.sources {
		if src.mode != "catalog" {
			continue
		}
		row := SourceInfo{
			Name:      src.name,
			Mode:      src.mode,
			Transport: src.transport,
			ToolCount: len(src.tools),
		}
		if src.info != nil {
			row.ServerName = src.info.Name
			row.Version = src.info.Version
		}
		sources = append(sources, row)
	}
	sort.Slice(sources, func(i, j int) bool { return strings.ToLower(sources[i].Name) < strings.ToLower(sources[j].Name) })
	return ListSourcesOutput{Sources: sources}
}

func (b *Bridge) searchTools(in SearchToolsInput) SearchToolsOutput {
	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	query := strings.ToLower(strings.TrimSpace(in.Query))
	sourceFilter := strings.ToLower(strings.TrimSpace(in.Source))

	b.mu.RLock()
	defer b.mu.RUnlock()
	var all []ToolInfo
	for _, src := range b.sources {
		if src.mode != "catalog" {
			continue
		}
		if sourceFilter != "" && strings.ToLower(src.name) != sourceFilter {
			continue
		}
		for _, tool := range src.tools {
			info := toolInfo(src.name, tool)
			if query != "" {
				haystack := strings.ToLower(info.Source + "\n" + info.Name + "\n" + info.Title + "\n" + info.Description)
				if !strings.Contains(haystack, query) {
					continue
				}
			}
			all = append(all, info)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if strings.EqualFold(all[i].Source, all[j].Source) {
			return strings.ToLower(all[i].Name) < strings.ToLower(all[j].Name)
		}
		return strings.ToLower(all[i].Source) < strings.ToLower(all[j].Source)
	})
	total := len(all)
	if len(all) > limit {
		all = all[:limit]
	}
	return SearchToolsOutput{Tools: all, Total: total}
}

func (b *Bridge) getTool(sourceName, toolName string) (ToolInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	src := b.sources[sourceName]
	if src == nil || src.mode != "catalog" {
		return ToolInfo{}, fmt.Errorf("unknown catalog MCP source %q", sourceName)
	}
	tool := src.tools[toolName]
	if tool == nil {
		return ToolInfo{}, fmt.Errorf("unknown MCP tool %q on source %q", toolName, sourceName)
	}
	return toolInfo(src.name, tool), nil
}

func (b *Bridge) callTool(ctx context.Context, sourceName, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	b.mu.RLock()
	src := b.sources[sourceName]
	b.mu.RUnlock()
	if src == nil || src.mode != "catalog" {
		return nil, fmt.Errorf("unknown catalog MCP source %q", sourceName)
	}
	if src.tools[toolName] == nil {
		return nil, fmt.Errorf("unknown MCP tool %q on source %q", toolName, sourceName)
	}
	callCtx, cancel := context.WithTimeout(ctx, src.timeout)
	defer cancel()
	result, err := src.session.CallTool(callCtx, &mcp.CallToolParams{Name: toolName, Arguments: args})
	if err != nil {
		return nil, err
	}
	return b.rewriteResourceLinks(src, result)
}

func (b *Bridge) rewriteResourceLinks(src *source, result *mcp.CallToolResult) (*mcp.CallToolResult, error) {
	if result == nil || len(result.Content) == 0 {
		return result, nil
	}
	copyResult := *result
	copyResult.Content = append([]mcp.Content(nil), result.Content...)
	for i, content := range copyResult.Content {
		link, ok := content.(*mcp.ResourceLink)
		if !ok || link == nil || strings.TrimSpace(link.URI) == "" {
			continue
		}
		if link.Size != nil && *link.Size > b.maxResourceBytes {
			return nil, fmt.Errorf("upstream MCP resource %q exceeds artifactEgress.maxFileBytes", link.URI)
		}
		token, err := randomCapabilityToken()
		if err != nil {
			continue
		}
		b.mu.Lock()
		if len(b.resources) >= 512 {
			for key := range b.resources {
				delete(b.resources, key)
				break
			}
		}
		b.resources[token] = resourceRef{source: src, uri: link.URI}
		b.mu.Unlock()
		copyLink := *link
		copyLink.URI = resourcePrefix + token
		copyResult.Content[i] = &copyLink
	}
	return &copyResult, nil
}

func (b *Bridge) readResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	if req == nil || req.Params == nil {
		return nil, mcp.ResourceNotFoundError("")
	}
	uri := req.Params.URI
	token := strings.TrimPrefix(uri, resourcePrefix)
	if token == uri || token == "" || strings.Contains(token, "/") {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	b.mu.RLock()
	ref, ok := b.resources[token]
	b.mu.RUnlock()
	if !ok || ref.source == nil {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	readCtx, cancel := context.WithTimeout(ctx, ref.source.timeout)
	defer cancel()
	result, err := ref.source.session.ReadResource(readCtx, &mcp.ReadResourceParams{URI: ref.uri})
	if err != nil {
		return nil, err
	}
	copyResult := *result
	copyResult.Contents = make([]*mcp.ResourceContents, 0, len(result.Contents))
	var total int64
	for _, item := range result.Contents {
		if item == nil {
			continue
		}
		copyItem := *item
		total += int64(len(copyItem.Text)) + int64(len(copyItem.Blob))
		if total > b.maxResourceBytes {
			return nil, errors.New("upstream MCP resource exceeds artifactEgress.maxFileBytes")
		}
		copyItem.URI = uri
		copyResult.Contents = append(copyResult.Contents, &copyItem)
	}
	return &copyResult, nil
}

func randomCapabilityToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func (b *Bridge) hasCatalogSources() bool {
	for _, src := range b.sources {
		if src.mode == "catalog" {
			return true
		}
	}
	return false
}

func toolInfo(sourceName string, tool *mcp.Tool) ToolInfo {
	return ToolInfo{
		Source:       sourceName,
		Name:         tool.Name,
		Title:        tool.Title,
		Description:  tool.Description,
		InputSchema:  tool.InputSchema,
		OutputSchema: tool.OutputSchema,
	}
}

func normalizedTransport(spec config.UpstreamMCPConfig) string {
	value := strings.ToLower(strings.TrimSpace(spec.Transport))
	if value == "" {
		if spec.URL != "" {
			return "streamable_http"
		}
		return "stdio"
	}
	switch value {
	case "streamable-http", "http":
		return "streamable_http"
	default:
		return value
	}
}

func uniqueToolName(base string, used map[string]struct{}) string {
	if _, exists := used[base]; !exists {
		used[base] = struct{}{}
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if _, exists := used[candidate]; !exists {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

func sanitizeToolName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "upstream_tool"
	}
	if len(out) > 96 {
		out = out[:96]
	}
	return out
}

func cloneTool(in *mcp.Tool) *mcp.Tool {
	if in == nil {
		return nil
	}
	out := *in
	if out.InputSchema == nil {
		out.InputSchema = map[string]any{"type": "object"}
	}
	if in.Annotations != nil {
		annotations := *in.Annotations
		out.Annotations = &annotations
	}
	out.Icons = append([]mcp.Icon(nil), in.Icons...)
	return &out
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	copyReq := req.Clone(req.Context())
	copyReq.Header = req.Header.Clone()
	for key, value := range t.headers {
		copyReq.Header.Set(key, value)
	}
	return t.base.RoundTrip(copyReq)
}
