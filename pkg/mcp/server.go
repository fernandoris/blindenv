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
	"Use list_secret_keys to discover key names, get_context for the active OS and project, " +
	"proxy_http_request for HTTP calls with {{SECRET_NAME}} substitution, and execute_with_secrets to " +
	"run commands with secrets injected. Never try to print or echo a secret value."

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

func (s *Server) audit(ctx context.Context, project, environment, tool string, keys []string, command string, exit *int, redactions int) {
	entry := db.AuditEntry{
		Timestamp:   time.Now(),
		Client:      s.cfg.Client,
		Project:     project,
		Environment: environment,
		Tool:        tool,
		KeyNames:    keys,
		Command:     command,
		ExitCode:    exit,
		Redactions:  redactions,
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
