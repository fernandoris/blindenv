package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	itPassphrase = "integration-passphrase"
	itSecret     = "sk-integration-998877"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "blindenv")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build binary: %v\n%s", err, out)
	}
	return bin
}

func seedIntegrationVault(t *testing.T) string {
	t.Helper()
	vault := filepath.Join(t.TempDir(), "vault.db")
	t.Setenv(EnvVault, vault)
	t.Setenv("BLINDENV_PASSPHRASE", itPassphrase)

	store, err := openVault(vault)
	if err != nil {
		t.Fatalf("openVault: %v", err)
	}
	ctx := context.Background()
	if _, err := store.CreateProject(ctx, "my-api"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := store.CreateEnvironment(ctx, "my-api", "staging"); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if _, err := store.PutSecret(ctx, "my-api", "", "REGION", "eu-west-1"); err != nil {
		t.Fatalf("PutSecret: %v", err)
	}
	if _, err := store.PutSecret(ctx, "my-api", "staging", "API_KEY", itSecret); err != nil {
		t.Fatalf("PutSecret: %v", err)
	}
	if err := store.SetAllowExecute(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowExecute: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return vault
}

func newClient(t *testing.T, bin, vault string) *client.Client {
	t.Helper()
	env := append(os.Environ(),
		EnvVault+"="+vault,
		"BLINDENV_PASSPHRASE="+itPassphrase,
		"BLINDENV_PROJECT=my-api",
		"BLINDENV_ENV=staging",
	)
	c, err := client.NewStdioMCPClient(bin, env, "mcp")
	if err != nil {
		t.Fatalf("NewStdioMCPClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "blindenv-integration", Version: "0.1.0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

func callTool(t *testing.T, c *client.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := c.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	return res
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatalf("empty tool result")
	}
	return mcp.GetTextFromContent(res.Content[0])
}

func echoCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo %API_KEY%"}
	}
	return "sh", []string{"-c", "echo $API_KEY"}
}

func TestIntegrationMCPFlow(t *testing.T) {
	bin := buildBinary(t)
	vault := seedIntegrationVault(t)
	c := newClient(t, bin, vault)

	tools, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"list_secret_keys", "get_context", "proxy_http_request", "execute_with_secrets"} {
		if !names[want] {
			t.Fatalf("tool %q missing from %v", want, names)
		}
	}

	// list_secret_keys: names only.
	listText := resultText(t, callTool(t, c, "list_secret_keys", nil))
	if !strings.Contains(listText, "API_KEY") || !strings.Contains(listText, "REGION") {
		t.Fatalf("list missing names: %s", listText)
	}
	if strings.Contains(listText, itSecret) {
		t.Fatalf("list leaked a value: %s", listText)
	}

	// get_context: no values.
	ctxText := resultText(t, callTool(t, c, "get_context", nil))
	if strings.Contains(ctxText, itSecret) {
		t.Fatalf("get_context leaked a value: %s", ctxText)
	}
	var ctxPayload map[string]any
	if err := json.Unmarshal([]byte(ctxText), &ctxPayload); err != nil {
		t.Fatalf("get_context not JSON: %v", err)
	}
	if ctxPayload["project"] != "my-api" {
		t.Fatalf("context project = %v", ctxPayload["project"])
	}

	// execute_with_secrets: injected and redacted.
	command, args := echoCommand()
	execText := resultText(t, callTool(t, c, "execute_with_secrets", map[string]any{
		"command": command,
		"args":    toAnySlice(args),
	}))
	if strings.Contains(execText, itSecret) {
		t.Fatalf("execute leaked a value: %s", execText)
	}
	if !strings.Contains(execText, "[BLINDENV_REDACTED:API_KEY]") {
		t.Fatalf("execute output not redacted: %s", execText)
	}

	// proxy_http_request: substitution + response redaction.
	var backendAuth string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "auth="+backendAuth)
	}))
	defer backend.Close()

	proxyText := resultText(t, callTool(t, c, "proxy_http_request", map[string]any{
		"url":     backend.URL,
		"method":  "GET",
		"headers": map[string]any{"Authorization": "Bearer {{API_KEY}}"},
	}))
	if backendAuth != "Bearer "+itSecret {
		t.Fatalf("backend saw %q, want real value", backendAuth)
	}
	if strings.Contains(proxyText, itSecret) {
		t.Fatalf("proxy leaked a value: %s", proxyText)
	}
}

func toAnySlice(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}
