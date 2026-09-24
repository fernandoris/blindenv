package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/fernandoris/blindenv/pkg/crypto"
	"github.com/fernandoris/blindenv/pkg/db"
)

func newTestServer(t *testing.T) (*Server, *db.Store) {
	t.Helper()
	key, err := crypto.NewSalt(crypto.KeySize)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	store, err := db.Open(filepath.Join(t.TempDir(), "vault.db"), key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

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
	if _, err := store.PutSecret(ctx, "my-api", "staging", "API_KEY", "sk-abcdef123456"); err != nil {
		t.Fatalf("PutSecret: %v", err)
	}

	srv := New(Config{Store: store, Project: "my-api", Environment: "staging", Client: "test"})
	return srv, store
}

func call(name string, args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: args}}
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("empty result content")
	}
	return mcp.GetTextFromContent(res.Content[0])
}

func TestListSecretKeysReturnsNamesOnly(t *testing.T) {
	srv, _ := newTestServer(t)
	res, err := srv.handleListSecretKeys(context.Background(), call("list_secret_keys", nil))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "API_KEY") || !strings.Contains(text, "REGION") {
		t.Fatalf("missing key names in %s", text)
	}
	if strings.Contains(text, "sk-abcdef123456") || strings.Contains(text, "eu-west-1") {
		t.Fatalf("value leaked in %s", text)
	}
}

func TestGetContextNoValues(t *testing.T) {
	srv, _ := newTestServer(t)
	res, err := srv.handleGetContext(context.Background(), call("get_context", nil))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	text := resultText(t, res)
	if strings.Contains(text, "sk-abcdef123456") {
		t.Fatalf("value leaked in %s", text)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("context not JSON: %v", err)
	}
	if payload["project"] != "my-api" || payload["environment"] != "staging" {
		t.Fatalf("context = %v", payload)
	}
}

func TestContextOverridePerCall(t *testing.T) {
	srv, _ := newTestServer(t)
	if _, err := srv.cfg.Store.CreateProject(context.Background(), "other"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := srv.cfg.Store.CreateEnvironment(context.Background(), "other", "prod"); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if _, err := srv.cfg.Store.PutSecret(context.Background(), "other", "prod", "OTHER_KEY", "other-secret-value"); err != nil {
		t.Fatalf("PutSecret: %v", err)
	}
	res, err := srv.handleListSecretKeys(context.Background(), call("list_secret_keys", map[string]any{"project": "other", "environment": "prod"}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "OTHER_KEY") || strings.Contains(text, "API_KEY") {
		t.Fatalf("override not applied: %s", text)
	}
}

func TestExecuteDisabledByDefault(t *testing.T) {
	srv, _ := newTestServer(t)
	res, err := srv.handleExecute(context.Background(), call("execute_with_secrets", map[string]any{
		"command": "sh",
		"args":    []any{"-c", "echo $API_KEY"},
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result when allow_execute is disabled")
	}
}

func TestExecuteInjectsAndRedacts(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	if err := store.SetAllowExecute(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowExecute: %v", err)
	}
	res, err := srv.handleExecute(ctx, call("execute_with_secrets", map[string]any{
		"command": "sh",
		"args":    []any{"-c", "echo $API_KEY"},
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	text := resultText(t, res)
	if strings.Contains(text, "sk-abcdef123456") {
		t.Fatalf("secret value leaked in %s", text)
	}
	if !strings.Contains(text, "[BLINDENV_REDACTED:API_KEY]") {
		t.Fatalf("expected redaction marker in %s", text)
	}
}

func TestExecuteAuditsInvocation(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	if err := store.SetAllowExecute(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowExecute: %v", err)
	}
	if _, err := srv.handleExecute(ctx, call("execute_with_secrets", map[string]any{
		"command": "sh",
		"args":    []any{"-c", "echo $API_KEY"},
	})); err != nil {
		t.Fatalf("handler: %v", err)
	}
	entries, err := store.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(entries))
	}
	if entries[0].Tool != "execute_with_secrets" || entries[0].ExitCode == nil || *entries[0].ExitCode != 0 {
		t.Fatalf("audit entry = %+v", entries[0])
	}
}

func TestExecuteStripsBlindenEnv(t *testing.T) {
	srv, store := newTestServer(t)
	ctx := context.Background()
	if err := store.SetAllowExecute(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowExecute: %v", err)
	}
	t.Setenv(EnvPassphrase, "super-secret-passphrase")
	res, err := srv.handleExecute(ctx, call("execute_with_secrets", map[string]any{
		"command": "sh",
		"args":    []any{"-c", "env"},
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	text := resultText(t, res)
	if strings.Contains(text, "BLINDENV_PASSPHRASE") || strings.Contains(text, "super-secret-passphrase") {
		t.Fatalf("BLINDENV_* leaked to child: %s", text)
	}
}

func TestProxySubstitutesAndRedacts(t *testing.T) {
	var gotAuth string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "echo:"+gotAuth)
	}))
	defer backend.Close()

	srv, _ := newTestServer(t)
	res, err := srv.handleProxy(context.Background(), call("proxy_http_request", map[string]any{
		"url":     backend.URL,
		"method":  "GET",
		"headers": map[string]any{"Authorization": "Bearer {{API_KEY}}"},
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if gotAuth != "Bearer sk-abcdef123456" {
		t.Fatalf("backend got %q, want real value", gotAuth)
	}
	text := resultText(t, res)
	if strings.Contains(text, "sk-abcdef123456") {
		t.Fatalf("secret leaked in response: %s", text)
	}
	if !strings.Contains(text, "[BLINDENV_REDACTED:API_KEY]") {
		t.Fatalf("expected redaction marker in %s", text)
	}
}

func TestProxyJSONBodyValidity(t *testing.T) {
	var gotBody string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, "ok")
	}))
	defer backend.Close()

	srv, store := newTestServer(t)
	if _, err := store.PutSecret(context.Background(), "my-api", "staging", "QUOTED", `ab"cd123456`); err != nil {
		t.Fatalf("PutSecret: %v", err)
	}

	res, err := srv.handleProxy(context.Background(), call("proxy_http_request", map[string]any{
		"url":    backend.URL,
		"method": "POST",
		"body":   `{"token":"{{QUOTED}}","n":1}`,
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(gotBody), &parsed); err != nil {
		t.Fatalf("body is not valid JSON: %q (%v)", gotBody, err)
	}
	if parsed["token"] != `ab"cd123456` {
		t.Fatalf("token = %v", parsed["token"])
	}
}

func TestProxyCrossHostRedirectStripsAuth(t *testing.T) {
	var backendAuth string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "done")
	}))
	defer backend.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, backend.URL, http.StatusFound)
	}))
	defer redirector.Close()

	srv, _ := newTestServer(t)
	res, err := srv.handleProxy(context.Background(), call("proxy_http_request", map[string]any{
		"url":     redirector.URL,
		"method":  "GET",
		"headers": map[string]any{"Authorization": "Bearer {{API_KEY}}"},
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
	if backendAuth != "" {
		t.Fatalf("Authorization forwarded across hosts: %q", backendAuth)
	}
}
