package mcp

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/fernandoris/blindenv/pkg/db"
	"github.com/fernandoris/blindenv/pkg/version"
)

// Environment variables that pin the MCP context.
const (
	EnvProject     = "BLINDENV_PROJECT"
	EnvEnvironment = "BLINDENV_ENV"
	EnvClient      = "BLINDENV_CLIENT"
	EnvPassphrase  = "BLINDENV_PASSPHRASE"
)

// Config configures the MCP server.
type Config struct {
	Store       *db.Store
	Project     string
	Environment string
	Client      string
	Logger      *log.Logger
}

// Server is the BlindEnv MCP server.
type Server struct {
	cfg    Config
	logger *log.Logger
}

// New builds a Server. Logs always go to stderr so stdout stays reserved for
// the MCP protocol.
func New(cfg Config) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "blindenv: ", log.LstdFlags)
	}
	if cfg.Client == "" {
		cfg.Client = os.Getenv(EnvClient)
	}
	return &Server{cfg: cfg, logger: logger}
}

// MCP returns the configured *server.MCPServer.
func (s *Server) MCP() *server.MCPServer {
	srv := server.NewMCPServer("blindenv", version.Version,
		server.WithToolCapabilities(false),
		server.WithInstructions(instructions),
	)
	s.registerTools(srv)
	return srv
}

// Serve runs the MCP server over stdio until the client disconnects.
func (s *Server) Serve() error {
	return server.ServeStdio(s.MCP())
}

const instructions = "BlindEnv gives you access to dev/pre-prod secrets without revealing their values. " +
	"Start by discovering available keys with discover_secrets (names, scope and environment, never values). " +
	"Then choose a project and environment and read the shell's secret reference from get_context. " +
	"Use proxy_http_request for HTTP calls with {{SECRET_NAME}} substitution, or execute_with_secrets to run " +
	"commands with secrets injected; both require an explicit environment, and in a command {{SECRET_NAME}} is " +
	"translated to the shell's native environment reference so the value is never placed on the command line. " +
	"Use open_in_browser to hand a URL with {{SECRET_NAME}} substitution to the local default browser, when the " +
	"project enables it. " +
	"Use list_secret_keys for the effective keys of the resolved context and get_context for the active OS, " +
	"project and execution capability. Never try to print or echo a secret value."

func (s *Server) resolveContext(project, environment string) (string, string, error) {
	if project == "" {
		project = s.cfg.Project
	}
	if environment == "" {
		environment = s.cfg.Environment
	}
	if project == "" {
		return "", "", fmt.Errorf("no project selected: pass the project argument or set %s", EnvProject)
	}
	return project, environment, nil
}

// resolveContextForSecrets resolves the context and requires a non-empty
// environment, so secret-consuming tools never silently operate on only the
// global and project-global scopes.
func (s *Server) resolveContextForSecrets(project, environment string) (string, string, error) {
	project, environment, err := s.resolveContext(project, environment)
	if err != nil {
		return "", "", err
	}
	if environment == "" {
		return "", "", fmt.Errorf("an environment is required to use secrets: pass the environment argument or set %s", EnvEnvironment)
	}
	return project, environment, nil
}

func (s *Server) audit(ctx context.Context, project, environment, tool string, keys []string, command string, exit *int, redactions, substitutions int) {
	entry := db.AuditEntry{
		Timestamp:     time.Now(),
		Client:        s.cfg.Client,
		Project:       project,
		Environment:   environment,
		Tool:          tool,
		KeyNames:      keys,
		Command:       command,
		ExitCode:      exit,
		Redactions:    redactions,
		Substitutions: substitutions,
	}
	if len(keys) > 0 {
		if scopes, err := s.cfg.Store.EffectiveScopes(ctx, project, environment); err == nil {
			entry.KeyScopes = make([]db.Scope, 0, len(keys))
			for _, k := range keys {
				entry.KeyScopes = append(entry.KeyScopes, scopes[k])
			}
		}
	}
	if err := s.cfg.Store.AppendAudit(ctx, entry); err != nil {
		s.logger.Printf("audit error: %v", err)
	}
}

func shellHint() string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "bash"
}
