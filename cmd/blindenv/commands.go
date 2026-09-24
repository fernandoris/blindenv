package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/fernandoris/blindenv/pkg/backup"
	"github.com/fernandoris/blindenv/pkg/db"
	"github.com/fernandoris/blindenv/pkg/mcp"
	"github.com/fernandoris/blindenv/pkg/web"
)

func cmdMCP(vaultPath string) error {
	store, err := openVault(vaultPath)
	if err != nil {
		return err
	}
	defer store.Close()

	srv := mcp.New(mcp.Config{
		Store:       store,
		Project:     os.Getenv(mcp.EnvProject),
		Environment: os.Getenv(mcp.EnvEnvironment),
		Client:      os.Getenv(mcp.EnvClient),
	})
	return srv.Serve()
}

func cmdUI(vaultPath string, args []string) error {
	fs := flag.NewFlagSet("ui", flag.ContinueOnError)
	port := fs.Int("port", 8080, "port to listen on")
	dev := fs.Bool("dev", false, "serve UI assets from disk instead of the embedded copy")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := openVault(vaultPath)
	if err != nil {
		return err
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	return web.Serve(ctx, web.Options{Store: store, Addr: addr, Dev: *dev})
}

func cmdBackup(vaultPath string, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: blindenv backup export|import <file>")
	}
	action, path := args[0], args[1]
	passphrase := os.Getenv("BLINDENV_BACKUP_PASSPHRASE")
	if passphrase == "" {
		return errors.New("set BLINDENV_BACKUP_PASSPHRASE to encrypt or decrypt the backup")
	}
	store, err := openVault(vaultPath)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx := context.Background()

	switch action {
	case "export":
		blob, err := backup.Export(ctx, store, passphrase)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, blob, 0o600); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}
		return nil
	case "import":
		blob, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read backup: %w", err)
		}
		return backup.Import(ctx, store, passphrase, blob)
	default:
		return fmt.Errorf("unknown backup action %q", action)
	}
}

func cmdRun(vaultPath string, args []string) error {
	sep := -1
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}
	if sep != 1 || sep == len(args)-1 {
		return errors.New("usage: blindenv run <project>/<environment> -- <command> [args...]")
	}
	project, environment := splitSelector(args[0])
	rest := args[sep+1:]

	store, err := openVault(vaultPath)
	if err != nil {
		return err
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	secrets, err := store.Resolve(ctx, project, environment)
	if err != nil {
		return err
	}
	redactor := mcp.NewRedactor(secrets, db.MinSecretLength)

	cmd := exec.CommandContext(ctx, rest[0], rest[1:]...)
	cmd.Env = mcp.ChildEnv(secrets)
	cmd.Stdin = os.Stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	outText, _ := redactor.Redact(stdout.Bytes())
	errText, _ := redactor.Redact(stderr.Bytes())
	fmt.Fprint(os.Stdout, outText)
	fmt.Fprint(os.Stderr, errText)

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return &exitError{code: exitErr.ExitCode(), err: fmt.Errorf("command exited with code %d", exitErr.ExitCode())}
		}
		return runErr
	}
	return nil
}
