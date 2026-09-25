package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fernandoris/blindenv/pkg/mcp"
)

func seedVault(t *testing.T) string {
	t.Helper()
	vault := filepath.Join(t.TempDir(), "vault.db")
	t.Setenv(EnvVault, vault)
	t.Setenv(mcp.EnvPassphrase, "test-passphrase")

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
	if _, err := store.PutSecret(ctx, "my-api", "staging", "API_KEY", "sk-cli-abcdef"); err != nil {
		t.Fatalf("PutSecret: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return vault
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), runErr
}

func TestUnknownSubcommand(t *testing.T) {
	if err := run([]string{"bogus"}); err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestNoValuePrintingSubcommands(t *testing.T) {
	seedVault(t)
	for _, name := range []string{"get", "export", "value"} {
		if err := run([]string{name, "my-api/staging", "API_KEY"}); err == nil {
			t.Fatalf("subcommand %q should not exist", name)
		}
	}
}

func TestVersionAndHelp(t *testing.T) {
	out, err := captureStdout(t, func() error { return run([]string{"version"}) })
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out, "blindenv") {
		t.Fatalf("version output = %q", out)
	}
	if _, err := captureStdout(t, func() error { return run([]string{"help"}) }); err != nil {
		t.Fatalf("help: %v", err)
	}
}

func TestRunInjectsAndRedacts(t *testing.T) {
	seedVault(t)
	out, err := captureStdout(t, func() error {
		return run([]string{"run", "my-api/staging", "--", "sh", "-c", "echo $API_KEY"})
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(out, "sk-cli-abcdef") {
		t.Fatalf("secret leaked: %q", out)
	}
	if !strings.Contains(out, "[BLINDENV_REDACTED:API_KEY]") {
		t.Fatalf("expected redaction marker, got %q", out)
	}
}

func TestRunInjectsIntoChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell redirection quoting differs on Windows; injection is covered by TestRunInjectsAndRedacts")
	}
	seedVault(t)
	outFile := filepath.Join(t.TempDir(), "child.txt")
	if _, err := captureStdout(t, func() error {
		return run([]string{"run", "my-api/staging", "--", "sh", "-c", `printf %s "$API_KEY" > "$1"`, "sh", outFile})
	}); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read child output: %v", err)
	}
	if strings.TrimSpace(string(got)) != "sk-cli-abcdef" {
		t.Fatalf("child env = %q, want secret value", got)
	}
}

func TestRunPropagatesExitCode(t *testing.T) {
	seedVault(t)
	err := run([]string{"run", "my-api/staging", "--", "sh", "-c", "exit 3"})
	var ee *exitError
	if !errors.As(err, &ee) {
		t.Fatalf("err = %v, want exitError", err)
	}
	if ee.code != 3 {
		t.Fatalf("exit code = %d, want 3", ee.code)
	}
}

// captureStdoutConcurrent pipes stdout to a reader that drains while the
// function runs, so a child producing more than the pipe buffer cannot block.
func captureStdoutConcurrent(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	return string(<-done), runErr
}

func TestRunStreamsLargeOutputRedacted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell loop syntax differs on Windows; streaming is covered elsewhere")
	}
	seedVault(t)
	out, err := captureStdoutConcurrent(t, func() error {
		return run([]string{"run", "my-api/staging", "--", "sh", "-c",
			`i=0; while [ $i -lt 5000 ]; do printf '%s padding\n' "$API_KEY"; i=$((i+1)); done`})
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(out, "sk-cli-abcdef") {
		t.Fatal("secret leaked from streamed output")
	}
	if !strings.Contains(out, "[BLINDENV_REDACTED:API_KEY]") {
		t.Fatal("expected redaction marker in streamed output")
	}
}

func TestRunBackgroundDescendantReturns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group semantics differ on Windows")
	}
	seedVault(t)
	start := time.Now()
	if err := run([]string{"run", "my-api/staging", "--", "sh", "-c", "sleep 30 & exit 0"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("run hung on background descendant: %s", elapsed)
	}
}
