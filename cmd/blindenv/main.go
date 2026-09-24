// Command blindenv is the BlindEnv CLI entrypoint.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fernandoris/blindenv/pkg/crypto"
	"github.com/fernandoris/blindenv/pkg/db"
	"github.com/fernandoris/blindenv/pkg/version"
)

// EnvVault overrides the default vault path.
const EnvVault = "BLINDENV_VAULT"

type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func main() {
	if err := run(os.Args[1:]); err != nil {
		var ee *exitError
		if errors.As(err, &ee) {
			os.Exit(ee.code)
		}
		fmt.Fprintln(os.Stderr, "blindenv: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	vaultPath := os.Getenv(EnvVault)
	var err error
	args, vaultPath, err = extractVaultFlag(args, vaultPath)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		printUsage(os.Stdout)
		return nil
	}

	switch args[0] {
	case "version":
		printVersion(os.Stdout)
		return nil
	case "mcp":
		return cmdMCP(vaultPath)
	case "ui":
		return cmdUI(vaultPath, args[1:])
	case "run":
		return cmdRun(vaultPath, args[1:])
	case "backup":
		return cmdBackup(vaultPath, args[1:])
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (try 'blindenv help')", args[0])
	}
}

func printUsage(w *os.File) {
	fmt.Fprintf(w, `blindenv - local secret manager for AI agents

Usage:
  blindenv mcp                     Run the MCP server over stdio
  blindenv ui [--port N] [--dev]   Open the local dashboard
  blindenv run <project>/<env> -- <command> [args...]
                                   Run a command with secrets injected
  blindenv backup export <file>    Write a passphrase-encrypted backup
  blindenv backup import <file>    Restore from a passphrase-encrypted backup
  blindenv version                 Print version information
  blindenv help                    Show this help

Environment:
  %s        Override the vault path
  BLINDENV_PROJECT                 Default project for the MCP server
  BLINDENV_ENV                     Default environment for the MCP server
  BLINDENV_PASSPHRASE              Passphrase when no OS keyring is available
  BLINDENV_BACKUP_PASSPHRASE       Passphrase for backup export/import
`, EnvVault)
}

func printVersion(w *os.File) {
	fmt.Fprintf(w, "blindenv %s (commit %s, built %s, %s %s/%s, %s)\n",
		version.Version, version.Commit, version.Date,
		runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.Compiler)
}

func extractVaultFlag(args []string, current string) ([]string, string, error) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--vault":
			if i+1 >= len(args) {
				return nil, "", errors.New("--vault requires a path")
			}
			current = args[i+1]
			i++
		case strings.HasPrefix(arg, "--vault="):
			current = strings.TrimPrefix(arg, "--vault=")
		default:
			out = append(out, arg)
		}
	}
	return out, current, nil
}

func defaultVaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(dir, "blindenv", "vault.db"), nil
}

// openVault resolves the vault path and key provider, creating the master key
// on first use.
func openVault(vaultPath string) (*db.Store, error) {
	if vaultPath == "" {
		path, err := defaultVaultPath()
		if err != nil {
			return nil, err
		}
		vaultPath = path
	}
	if err := os.MkdirAll(filepath.Dir(vaultPath), 0o700); err != nil {
		return nil, fmt.Errorf("create vault directory: %w", err)
	}
	saltPath := vaultPath + ".salt"
	provider, err := crypto.ResolveProvider(saltPath, os.Getenv("BLINDENV_PASSPHRASE"))
	if err != nil {
		return nil, err
	}

	_, statErr := os.Stat(vaultPath)
	exists := statErr == nil

	var key []byte
	if exists {
		key, err = provider.MasterKey()
	} else {
		key, err = provider.EnsureMasterKey()
	}
	if err != nil {
		return nil, fmt.Errorf("unlock vault: %w", err)
	}
	return db.Open(vaultPath, key)
}

func splitSelector(selector string) (project, environment string) {
	if i := strings.Index(selector, "/"); i >= 0 {
		return selector[:i], selector[i+1:]
	}
	return selector, ""
}
