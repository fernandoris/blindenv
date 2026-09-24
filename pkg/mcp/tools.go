package mcp

import (
	"context"
	"runtime"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerTools(srv *mcpserver.MCPServer) {
	srv.AddTool(listKeysTool(), s.handleListSecretKeys)
	srv.AddTool(getContextTool(), s.handleGetContext)
	srv.AddTool(proxyTool(), s.handleProxy)
	srv.AddTool(executeTool(), s.handleExecute)
}

func listKeysTool() mcp.Tool {
	return mcp.NewTool("list_secret_keys",
		mcp.WithDescription("List the NAMES of secret keys available for a project and environment. Never returns values."),
		mcp.WithString("project", mcp.Description("Project slug; defaults to the configured project.")),
		mcp.WithString("environment", mcp.Description("Environment name; defaults to the configured environment.")),
	)
}

func getContextTool() mcp.Tool {
	return mcp.NewTool("get_context",
		mcp.WithDescription("Return the execution context: OS, architecture, shell hint, active project and environment, and available key names. Never returns values."),
		mcp.WithString("project", mcp.Description("Project slug; defaults to the configured project.")),
		mcp.WithString("environment", mcp.Description("Environment name; defaults to the configured environment.")),
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
	s.audit(ctx, project, environment, "list_secret_keys", keys, "", nil, 0)
	return mcp.NewToolResultJSON(map[string]any{
		"project":     project,
		"environment": environment,
		"keys":        keys,
	})
}

func (s *Server) handleGetContext(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	s.audit(ctx, project, environment, "get_context", nil, "", nil, 0)
	return mcp.NewToolResultJSON(map[string]any{
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"shell_hint":  shellHint(),
		"project":     project,
		"environment": environment,
		"secret_keys": keys,
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
