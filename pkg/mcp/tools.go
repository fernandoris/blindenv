package mcp

import (
	"context"
	"runtime"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/fernandoris/blindenv/pkg/db"
)

func (s *Server) registerTools(srv *mcpserver.MCPServer) {
	srv.AddTool(listKeysTool(), s.handleListSecretKeys)
	srv.AddTool(discoverTool(), s.handleDiscoverSecrets)
	srv.AddTool(getContextTool(), s.handleGetContext)
	srv.AddTool(proxyTool(), s.handleProxy)
	srv.AddTool(executeTool(), s.handleExecute)
	srv.AddTool(openTool(), s.handleOpen)
}

func listKeysTool() mcp.Tool {
	return mcp.NewTool("list_secret_keys",
		mcp.WithDescription("List the NAMES of secret keys available for a project and environment. Never returns values."),
		mcp.WithString("project", mcp.Description("Project slug; defaults to the configured project.")),
		mcp.WithString("environment", mcp.Description("Environment name; defaults to the configured environment.")),
	)
}

func discoverTool() mcp.Tool {
	return mcp.NewTool("discover_secrets",
		mcp.WithDescription("Discover the NAMES of all secret keys defined for a project, grouped by scope (global, environment, project, project_environment) with the environment name. Independent of the resolved environment and never returns values. Use it before proxy_http_request or execute_with_secrets."),
		mcp.WithString("project", mcp.Description("Project slug; defaults to the configured project.")),
		mcp.WithString("environment", mcp.Description("Environment name; optional, echoed in the response.")),
	)
}

func getContextTool() mcp.Tool {
	return mcp.NewTool("get_context",
		mcp.WithDescription("Return the execution context: OS, architecture, shell hint, the resolved shell's secret reference, active project and environment, and available key names. Never returns values."),
		mcp.WithString("project", mcp.Description("Project slug; defaults to the configured project.")),
		mcp.WithString("environment", mcp.Description("Environment name; defaults to the configured environment.")),
		mcp.WithString("shell", mcp.Description("Shell to report the secret reference for (bash, powershell, cmd, ...); defaults to the OS shell.")),
	)
}

func proxyTool() mcp.Tool {
	return mcp.NewTool("proxy_http_request",
		mcp.WithDescription("Perform an HTTP request, substituting {{SECRET_NAME}} tags in the URL, headers and body inside BlindEnv. Use this instead of curl for authenticated calls."),
		mcp.WithString("url", mcp.Required(), mcp.Description("Target URL.")),
		mcp.WithString("method", mcp.Description("HTTP method."), mcp.DefaultString("GET")),
		mcp.WithObject("headers", mcp.Description("Request headers; values may contain {{SECRET_NAME}}.")),
		mcp.WithString("body", mcp.Description("Request body; may contain {{SECRET_NAME}}.")),
		mcp.WithString("project", mcp.Description("Project slug; defaults to the configured project.")),
		mcp.WithString("environment", mcp.Description("Environment name; defaults to the configured environment.")),
	)
}

func executeTool() mcp.Tool {
	return mcp.NewTool("execute_with_secrets",
		mcp.WithDescription("Run a local command with the project's secrets injected into its environment. stdout/stderr are redacted before returning. Requires allow_execute on the project."),
		mcp.WithString("command", mcp.Required(), mcp.Description("Executable to run, or a script when shell is set.")),
		mcp.WithArray("args", mcp.Description("Arguments passed to the command."), mcp.WithStringItems()),
		mcp.WithString("shell", mcp.Description("Optional shell to run the command with (bash, powershell, cmd, ...).")),
		mcp.WithString("project", mcp.Description("Project slug; defaults to the configured project.")),
		mcp.WithString("environment", mcp.Description("Environment name; defaults to the configured environment.")),
	)
}

func openTool() mcp.Tool {
	return mcp.NewTool("open_in_browser",
		mcp.WithDescription("Open a URL in the local default browser, substituting {{SECRET_NAME}} tags inside BlindEnv. The resolved URL is handed to the OS launcher and never returned to you. Requires allow_open on the project."),
		mcp.WithString("url", mcp.Required(), mcp.Description("URL to open; may contain {{SECRET_NAME}}.")),
		mcp.WithString("project", mcp.Description("Project slug; defaults to the configured project.")),
		mcp.WithString("environment", mcp.Description("Environment name; defaults to the configured environment.")),
	)
}

// vaultUnavailable returns a tool error when the master key did not match the
// vault at unlock. Only the value-consuming Tools call it; discovery and
// listing keep working without the key.
func (s *Server) vaultUnavailable() *mcp.CallToolResult {
	if s.cfg.VaultErr == nil {
		return nil
	}
	return mcp.NewToolResultError(s.cfg.VaultErr.Error())
}

func (s *Server) handleListSecretKeys(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, environment, err := s.resolveContext(req.GetString("project", ""), req.GetString("environment", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	keys, err := s.cfg.Store.ListKeys(ctx, project, environment)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if keys == nil {
		keys = []string{}
	}
	s.audit(ctx, project, environment, "list_secret_keys", keys, "", nil, 0, 0)
	return mcp.NewToolResultJSON(map[string]any{
		"project":     project,
		"environment": environment,
		"keys":        keys,
	})
}

func (s *Server) handleDiscoverSecrets(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, environment, err := s.resolveContext(req.GetString("project", ""), req.GetString("environment", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	scoped, err := s.cfg.Store.ListScopedKeys(ctx, project)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	envScopes := make(map[string][]discoverKeyView, len(scoped.Environments))
	for name, keys := range scoped.Environments {
		envScopes[name] = toDiscoverViews(keys)
	}
	projectEnvs := make(map[string][]discoverKeyView, len(scoped.ProjectEnvironments))
	for name, keys := range scoped.ProjectEnvironments {
		projectEnvs[name] = toDiscoverViews(keys)
	}
	s.audit(ctx, project, environment, "discover_secrets", nil, "", nil, 0, 0)
	return mcp.NewToolResultJSON(map[string]any{
		"project":     project,
		"environment": environment,
		"scopes": map[string]any{
			"global":              toDiscoverViews(scoped.Global),
			"environment":         envScopes,
			"project":             toDiscoverViews(scoped.Project),
			"project_environment": projectEnvs,
		},
	})
}

type discoverKeyView struct {
	Key       string `json:"key"`
	Type      string `json:"type"`
	Hint      string `json:"hint,omitempty"`
	Sensitive bool   `json:"sensitive"`
}

func toDiscoverViews(in []db.SecretInfo) []discoverKeyView {
	out := make([]discoverKeyView, 0, len(in))
	for _, s := range in {
		out = append(out, discoverKeyView{
			Key:       s.Key,
			Type:      string(s.Kind),
			Hint:      s.Hint,
			Sensitive: s.Sensitive,
		})
	}
	return out
}

type contextKeyView struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`
	Hint        string   `json:"hint,omitempty"`
	Sensitive   bool     `json:"sensitive"`
	Value       string   `json:"value,omitempty"`
	Scope       string   `json:"scope"`
	Environment string   `json:"environment,omitempty"`
	Overrides   []string `json:"overrides,omitempty"`
}

func (s *Server) handleGetContext(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, environment, err := s.resolveContext(req.GetString("project", ""), req.GetString("environment", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if res := s.vaultUnavailable(); res != nil {
		return res, nil
	}
	proj, err := s.cfg.Store.GetProject(ctx, project)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	resolved, err := s.cfg.Store.ResolveDetailed(ctx, project, environment)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	keys := make([]contextKeyView, 0, len(resolved))
	for key, r := range resolved {
		view := contextKeyView{
			Key:       key,
			Type:      string(r.Kind),
			Hint:      r.Hint,
			Sensitive: r.Sensitive,
			Scope:     string(r.Scope),
		}
		if !r.Sensitive {
			view.Value = r.Value
		}
		if r.Scope == db.ScopeSharedEnvironment {
			view.Environment = r.Environment
		}
		if len(r.Overrides) > 0 {
			view.Overrides = make([]string, 0, len(r.Overrides))
			for _, scope := range r.Overrides {
				view.Overrides = append(view.Overrides, string(scope))
			}
		}
		keys = append(keys, view)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Key < keys[j].Key })
	s.audit(ctx, project, environment, "get_context", nil, "", nil, 0, 0)
	shellName := strings.TrimSpace(req.GetString("shell", ""))
	if shellName == "" {
		shellName = shellHint()
	}
	return mcp.NewToolResultJSON(map[string]any{
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"shell_hint":  shellHint(),
		"project":     project,
		"environment": environment,
		"secret_reference": map[string]any{
			"shell":  shellName,
			"native": referenceTemplate(classifyShell(shellName)),
			"tag":    "{{SECRET_NAME}}",
		},
		"allow_execute": proj.AllowExecute,
		"allow_open":    proj.AllowOpen,
		"secret_keys":   keys,
	})
}

func stringMapArg(req mcp.CallToolRequest, key string) map[string]string {
	raw, ok := req.GetArguments()[key]
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}
