package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/mark3labs/mcp-go/mcp"
)

// browserLauncher opens a URL in the default browser. The default
// implementation resolves the per-OS launcher and runs it without a shell.
// Tests replace it to capture the resolved URL without opening a browser.
var browserLauncher = runBrowserLauncher

// resolveLauncher returns the platform command that opens a URL, with its
// leading arguments. It never wraps the URL in a shell.
func resolveLauncher(goos, url string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}

// runBrowserLauncher resolves and starts the OS launcher without a shell.
func runBrowserLauncher(ctx context.Context, url string) error {
	name, args := resolveLauncher(runtime.GOOS, url)
	cmd := exec.CommandContext(ctx, name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// hasDisplay reports whether a Linux session has a graphical display. Other
// operating systems always report true.
func hasDisplay(goos string, lookup func(string) string) bool {
	if goos != "linux" {
		return true
	}
	return lookup("DISPLAY") != "" || lookup("WAYLAND_DISPLAY") != ""
}

// displayAvailable reports whether the current session has a graphical display.
// Tests replace it to exercise the open path without a display.
var displayAvailable = func() bool { return hasDisplay(runtime.GOOS, os.Getenv) }

type openResult struct {
	Opened        bool     `json:"opened"`
	Substitutions int      `json:"substitutions"`
	UnmatchedTags []string `json:"unmatched_tags"`
}

func (s *Server) handleOpen(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, environment, err := s.resolveContextForSecrets(req.GetString("project", ""), req.GetString("environment", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	urlTemplate := req.GetString("url", "")
	if urlTemplate == "" {
		return mcp.NewToolResultError("url is required"), nil
	}

	proj, err := s.cfg.Store.GetProject(ctx, project)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if !proj.AllowOpen {
		s.audit(ctx, project, environment, "open_in_browser", nil, urlTemplate, nil, 0, 0)
		return mcp.NewToolResultError("opening URLs in the browser is disabled for this project; enable allow_open in the BlindEnv UI"), nil
	}

	resolved, err := s.cfg.Store.ResolveDetailed(ctx, project, environment)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	secrets := valuesOf(resolved)
	unmatched := newUnmatchedCollector(keySet(secrets))
	resolvedURL, substitutions := substitute(urlTemplate, secrets)
	unmatched.scan(urlTemplate)

	// The audit records the unsubstituted template, never the resolved URL.
	s.audit(ctx, project, environment, "open_in_browser", keysOf(secrets), urlTemplate, nil, 0, substitutions)

	if !displayAvailable() {
		return mcp.NewToolResultError("no graphical display detected (DISPLAY and WAYLAND_DISPLAY are unset); open the URL from a graphical session"), nil
	}
	if err := browserLauncher(ctx, resolvedURL); err != nil {
		// Never echo the resolved URL back to the agent.
		return mcp.NewToolResultError(fmt.Sprintf("failed to launch browser: %v", err)), nil
	}

	return mcp.NewToolResultJSON(openResult{
		Opened:        true,
		Substitutions: substitutions,
		UnmatchedTags: unmatched.list(),
	})
}
