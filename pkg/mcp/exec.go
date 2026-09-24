package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/fernandoris/blindenv/pkg/db"
)

const (
	execTimeout     = 60 * time.Second
	maxOutputBytes  = 100_000
	truncatedSuffix = "\n... [output truncated]"
)

type execResult struct {
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	Redactions int    `json:"redactions"`
}

func (s *Server) handleExecute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, environment, err := s.resolveContext(req.GetString("project", ""), req.GetString("environment", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	command := req.GetString("command", "")
	if command == "" {
		return mcp.NewToolResultError("command is required"), nil
	}
	args := req.GetStringSlice("args", nil)
	shell := req.GetString("shell", "")

	proj, err := s.cfg.Store.GetProject(ctx, project)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if !proj.AllowExecute {
		s.audit(ctx, project, environment, "execute_with_secrets", nil, command, nil, 0)
		return mcp.NewToolResultError("execution with secrets is disabled for this project; enable allow_execute in the BlindEnv UI"), nil
	}

	secrets, err := s.cfg.Store.Resolve(ctx, project, environment)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	runCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	var cmd *exec.Cmd
	if shell != "" {
		cmd = shellCommand(runCtx, shell, command)
	} else {
		cmd = exec.CommandContext(runCtx, command, args...)
	}
	cmd.Env = ChildEnv(secrets)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if runCtx.Err() != nil {
			return mcp.NewToolResultError(fmt.Sprintf("command timed out after %s", execTimeout)), nil
		} else {
			s.audit(ctx, project, environment, "execute_with_secrets", keysOf(secrets), strings.Join(append([]string{command}, args...), " "), nil, 0)
			return mcp.NewToolResultError(fmt.Sprintf("failed to run command: %v", runErr)), nil
		}
	}
	_ = start

	redactor := NewRedactor(secrets, db.MinSecretLength)
	stdoutText, stdoutRedactions := redactor.Redact(stdout.Bytes())
	stderrText, stderrRedactions := redactor.Redact(stderr.Bytes())
	redactions := stdoutRedactions + stderrRedactions

	auditCommand := strings.Join(append([]string{command}, args...), " ")
	s.audit(ctx, project, environment, "execute_with_secrets", keysOf(secrets), auditCommand, &exitCode, redactions)

	return mcp.NewToolResultJSON(execResult{
		ExitCode:   exitCode,
		Stdout:     truncate(stdoutText),
		Stderr:     truncate(stderrText),
		Redactions: redactions,
	})
}

func shellCommand(ctx context.Context, shell, script string) *exec.Cmd {
	switch strings.ToLower(filepath.Base(shell)) {
	case "cmd", "cmd.exe":
		return exec.CommandContext(ctx, shell, "/c", script)
	case "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return exec.CommandContext(ctx, shell, "-NoProfile", "-Command", script)
	default:
		return exec.CommandContext(ctx, shell, "-c", script)
	}
}

// buildEnv overlays the resolved secrets on the current environment while
// stripping every BLINDENV_* variable so the master key, passphrase or other
// context never leak into the child process.
func ChildEnv(secrets map[string]string) []string {
	env := make([]string, 0, len(os.Environ())+len(secrets))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "BLINDENV_") {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range secrets {
		env = append(env, k+"="+v)
	}
	return env
}

func truncate(s string) string {
	if len(s) <= maxOutputBytes {
		return s
	}
	return s[:maxOutputBytes] + truncatedSuffix
}

func keysOf(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
