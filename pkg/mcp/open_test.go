package mcp

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
)

// captureLauncher installs a browser-launcher override that records the
// resolved URL and restores the previous override on cleanup.
func captureLauncher(t *testing.T) *capturedLaunch {
	t.Helper()
	prev := browserLauncher
	got := &capturedLaunch{}
	browserLauncher = func(_ context.Context, url string) error {
		got.url = url
		got.invoked = true
		return nil
	}
	t.Cleanup(func() { browserLauncher = prev })
	return got
}

type capturedLaunch struct {
	invoked bool
	url     string
}

func TestOpenDisabledByDefault(t *testing.T) {
	srv, _ := newTestServer(t)
	launcher := captureLauncher(t)
	res, err := srv.handleOpen(context.Background(), call("open_in_browser", map[string]any{
		"url": "https://example.test/?token={{API_KEY}}",
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result when allow_open is disabled")
	}
	if launcher.invoked {
		t.Fatal("launcher was invoked despite allow_open being disabled")
	}
	if !strings.Contains(strings.ToLower(resultText(t, res)), "allow_open") {
		t.Fatalf("error should mention allow_open: %s", resultText(t, res))
	}
}

func TestOpenRequiresExplicitEnvironment(t *testing.T) {
	srv, store := newTestServer(t)
	srv.cfg.Environment = ""
	ctx := context.Background()
	if err := store.SetAllowOpen(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowOpen: %v", err)
	}
	launcher := captureLauncher(t)
	res, err := srv.handleOpen(ctx, call("open_in_browser", map[string]any{
		"url":     "https://example.test/?token={{API_KEY}}",
		"project": "my-api",
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error when no environment is available")
	}
	if launcher.invoked {
		t.Fatal("launcher was invoked despite a missing environment")
	}
}

func TestOpenResolvesSensitiveTagWithoutLeak(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	if err := store.SetAllowOpen(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowOpen: %v", err)
	}
	launcher := captureLauncher(t)
	res, err := srv.handleOpen(ctx, call("open_in_browser", map[string]any{
		"url": "https://idp.test/authorize?token={{API_KEY}}&region={{REGION}}",
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
	if !launcher.invoked {
		t.Fatal("launcher was not invoked")
	}
	url := launcher.url
	if !strings.Contains(url, "sk-abcdef123456") || !strings.Contains(url, "eu-west-1") {
		t.Fatalf("resolved URL missing values: %s", url)
	}

	// The result returned to the agent must not contain the value or the URL.
	text := resultText(t, res)
	if strings.Contains(text, "sk-abcdef123456") || strings.Contains(text, "idp.test") {
		t.Fatalf("resolved URL leaked into result: %s", text)
	}

	// The audit must store the template, never the resolved URL or the value.
	entries, err := store.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Tool != "open_in_browser" {
		t.Fatalf("audit tool = %q", e.Tool)
	}
	if strings.Contains(e.Command, "sk-abcdef123456") || strings.Contains(e.Command, "eu-west-1") {
		t.Fatalf("value leaked into audit command: %q", e.Command)
	}
	if !strings.Contains(e.Command, "{{API_KEY}}") {
		t.Fatalf("audit command should keep the template: %q", e.Command)
	}
	if len(e.KeyNames) != 2 {
		t.Fatalf("audit key names = %v, want API_KEY and REGION", e.KeyNames)
	}
}

func TestOpenNonMatchingTagPassesThrough(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	if err := store.SetAllowOpen(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowOpen: %v", err)
	}
	launcher := captureLauncher(t)
	res, err := srv.handleOpen(ctx, call("open_in_browser", map[string]any{
		"url": "https://example.test/?q={{NOT_A_KEY}}",
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
	if launcher.url != "https://example.test/?q={{NOT_A_KEY}}" {
		t.Fatalf("non-matching tag was altered: %s", launcher.url)
	}
}

func TestOpenRequiresURL(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	if err := store.SetAllowOpen(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowOpen: %v", err)
	}
	res, err := srv.handleOpen(ctx, call("open_in_browser", map[string]any{}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error when url is missing")
	}
}

func TestOpenToolRegistered(t *testing.T) {
	srv, _ := newTestServer(t)
	tools := srv.MCP().ListTools()
	tool, ok := tools["open_in_browser"]
	if !ok {
		t.Fatal("open_in_browser is not registered")
	}
	if tool.Tool.Name != "open_in_browser" {
		t.Fatalf("registered name = %q", tool.Tool.Name)
	}
	if len(tool.Tool.InputSchema.Required) != 1 || tool.Tool.InputSchema.Required[0] != "url" {
		t.Fatalf("open_in_browser should require url: %+v", tool.Tool.InputSchema.Required)
	}
}

func TestHasDisplay(t *testing.T) {
	empty := func(string) string { return "" }
	some := func(k string) string {
		if k == "DISPLAY" {
			return ":0"
		}
		return ""
	}
	if !hasDisplay("darwin", empty) {
		t.Fatal("non-linux should always report a display")
	}
	if hasDisplay("linux", empty) {
		t.Fatal("linux without DISPLAY/WAYLAND_DISPLAY should report no display")
	}
	if !hasDisplay("linux", some) {
		t.Fatal("linux with DISPLAY should report a display")
	}
}

func TestResolveLauncherNeverUsesShell(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		name, args := resolveLauncher(goos, "https://example.test/")
		switch name {
		case "sh", "bash", "cmd", "powershell", "pwsh", "cmd.exe":
			t.Fatalf("%s: launcher must not be a shell", goos)
		}
		if len(args) == 0 || args[len(args)-1] != "https://example.test/" {
			t.Fatalf("%s: URL must be passed as an argument: %v", goos, args)
		}
	}
}

func TestOpenErrorDoesNotContainResolvedURL(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	if err := store.SetAllowOpen(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowOpen: %v", err)
	}
	prev := browserLauncher
	browserLauncher = func(_ context.Context, _ string) error {
		return errors.New("boom")
	}
	t.Cleanup(func() { browserLauncher = prev })

	res, err := srv.handleOpen(ctx, call("open_in_browser", map[string]any{
		"url": "https://idp.test/authorize?token={{API_KEY}}",
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error when the launcher fails")
	}
	text := resultText(t, res)
	if strings.Contains(text, "sk-abcdef123456") || strings.Contains(text, "idp.test") {
		t.Fatalf("resolved URL leaked into error: %s", text)
	}
}

func TestOpenHeadlessLinuxRefused(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("headless display guard only applies on Linux")
	}
	srv, store := newTestServer(t)
	ctx := context.Background()
	if err := store.SetAllowOpen(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowOpen: %v", err)
	}
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	launcher := captureLauncher(t)
	res, err := srv.handleOpen(ctx, call("open_in_browser", map[string]any{
		"url": "https://example.test/?token={{API_KEY}}",
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected refusal on headless Linux")
	}
	if launcher.invoked {
		t.Fatal("launcher was invoked on a headless Linux session")
	}
	if !strings.Contains(strings.ToLower(resultText(t, res)), "display") {
		t.Fatalf("error should mention the missing display: %s", resultText(t, res))
	}
}
