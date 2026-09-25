package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setExecLimits(t *testing.T, timeout time.Duration, budget int, waitDelay time.Duration) {
	t.Helper()
	oldTimeout, oldBudget, oldDelay := execTimeout, maxOutputBytes, CommandWaitDelay
	execTimeout, maxOutputBytes, CommandWaitDelay = timeout, budget, waitDelay
	t.Cleanup(func() {
		execTimeout, maxOutputBytes, CommandWaitDelay = oldTimeout, oldBudget, oldDelay
	})
}

func execPayload(t *testing.T, text string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("result not JSON: %v (%s)", err, text)
	}
	return payload
}

func TestExecuteBoundsLargeOutput(t *testing.T) {
	setExecLimits(t, 10*time.Second, 2000, 300*time.Millisecond)
	srv, store := newTestServer(t)
	ctx := context.Background()
	if err := store.SetAllowExecute(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowExecute: %v", err)
	}

	res, err := srv.handleExecute(ctx, call("execute_with_secrets", map[string]any{
		"command": "sh",
		"args":    []any{"-c", "yes hello | head -c 100000"},
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	payload := execPayload(t, resultText(t, res))
	stdout, _ := payload["stdout"].(string)
	if len(stdout) > maxOutputBytes+len(truncatedSuffix) {
		t.Fatalf("stdout length %d exceeds budget+marker", len(stdout))
	}
	if !strings.Contains(stdout, "output truncated") {
		t.Fatalf("expected truncation indicator in %d bytes", len(stdout))
	}

	entries, err := store.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(entries))
	}
	if entries[0].ExitCode == nil || *entries[0].ExitCode != 0 {
		t.Fatalf("audit exit code = %v, want 0", entries[0].ExitCode)
	}
}

func TestExecuteSecretAtBoundaryNoLeak(t *testing.T) {
	setExecLimits(t, 10*time.Second, 100, 300*time.Millisecond)
	srv, store := newTestServer(t)
	ctx := context.Background()
	if err := store.SetAllowExecute(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowExecute: %v", err)
	}

	const secret = "sk-abcdef123456"
	res, err := srv.handleExecute(ctx, call("execute_with_secrets", map[string]any{
		"command": "sh",
		"args": []any{"-c", `printf '%s' "$1"; printf '%s' "$2"; printf '%s' "$3"`,
			"sh", strings.Repeat("x", 95), secret, strings.Repeat("y", 50)},
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	payload := execPayload(t, resultText(t, res))
	stdout, _ := payload["stdout"].(string)
	if strings.Contains(stdout, secret) {
		t.Fatalf("secret leaked in %q", stdout)
	}
	if strings.Contains(stdout, secret[:9]) {
		t.Fatalf("secret fragment leaked in %q", stdout)
	}
}

func TestExecuteTimeoutReturns(t *testing.T) {
	setExecLimits(t, 400*time.Millisecond, 100000, 200*time.Millisecond)
	srv, store := newTestServer(t)
	ctx := context.Background()
	if err := store.SetAllowExecute(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowExecute: %v", err)
	}

	start := time.Now()
	res, err := srv.handleExecute(ctx, call("execute_with_secrets", map[string]any{
		"command": "sh",
		"args":    []any{"-c", "sleep 30"},
	}))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected timeout error result, got %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "timed out") {
		t.Fatalf("expected timeout message, got %s", resultText(t, res))
	}
	if elapsed > 5*time.Second {
		t.Fatalf("handler did not return within deadline: %s", elapsed)
	}
}

func TestExecuteDescendantHoldingPipeReturns(t *testing.T) {
	setExecLimits(t, 10*time.Second, 100000, 300*time.Millisecond)
	srv, store := newTestServer(t)
	ctx := context.Background()
	if err := store.SetAllowExecute(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowExecute: %v", err)
	}

	start := time.Now()
	res, err := srv.handleExecute(ctx, call("execute_with_secrets", map[string]any{
		"command": "sh",
		"args":    []any{"-c", "while true; do echo filler; sleep 0.05; done & exit 0"},
	}))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
	if elapsed > 5*time.Second {
		t.Fatalf("handler blocked on descendant: %s", elapsed)
	}
}

func TestExecuteKillsDescendantAfterReturn(t *testing.T) {
	setExecLimits(t, 10*time.Second, 100000, 300*time.Millisecond)
	srv, store := newTestServer(t)
	ctx := context.Background()
	if err := store.SetAllowExecute(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowExecute: %v", err)
	}

	marker := filepath.Join(t.TempDir(), "marker")
	if _, err := srv.handleExecute(ctx, call("execute_with_secrets", map[string]any{
		"command": "sh",
		"args":    []any{"-c", `(sleep 1; touch "$1") & exit 0`, "sh", marker},
	})); err != nil {
		t.Fatalf("handler: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("descendant survived and wrote its marker after the handler returned")
	}
}
