package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/fernandoris/blindenv/pkg/db"
)

func TestVaultMismatchReportedByValueTools(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.cfg.VaultErr = db.ErrMasterKeyMismatch
	ctx := context.Background()

	res, err := srv.handleExecute(ctx, call("execute_with_secrets", map[string]any{
		"command": "sh",
		"args":    []any{"-c", "echo hi"},
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "master key") || !strings.Contains(text, "passphrase") {
		t.Fatalf("execute text = %q, want an actionable mismatch", text)
	}
	if strings.Contains(text, "sk-abcdef123456") || strings.Contains(text, "eu-west-1") {
		t.Fatalf("mismatch error leaked a value: %q", text)
	}

	ctxRes, err := srv.handleGetContext(ctx, call("get_context", nil))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !strings.Contains(resultText(t, ctxRes), "master key") {
		t.Fatalf("get_context text = %q, want the mismatch", resultText(t, ctxRes))
	}
}

func TestDiscoveryWorksDespiteVaultMismatch(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.cfg.VaultErr = db.ErrMasterKeyMismatch
	ctx := context.Background()

	listRes, err := srv.handleListSecretKeys(ctx, call("list_secret_keys", nil))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	listText := resultText(t, listRes)
	if strings.Contains(listText, "master key") {
		t.Fatalf("list must not report the mismatch: %q", listText)
	}
	if !strings.Contains(listText, "API_KEY") {
		t.Fatalf("list missing names: %q", listText)
	}

	discoverRes, err := srv.handleDiscoverSecrets(ctx, call("discover_secrets", nil))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !strings.Contains(resultText(t, discoverRes), "API_KEY") {
		t.Fatalf("discover missing names: %q", resultText(t, discoverRes))
	}
}
