package db

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fernandoris/blindenv/pkg/crypto"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	key, err := crypto.NewSalt(crypto.KeySize)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "vault.db")
	s, err := Open(path, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestCreateProjectAndDuplicate(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()

	if _, err := s.CreateProject(ctx, "my-api"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := s.CreateProject(ctx, "my-api"); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate err = %v, want ErrDuplicate", err)
	}
	if _, err := s.GetProject(ctx, "missing"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("missing project err = %v, want ErrProjectNotFound", err)
	}
}

func TestEnvironmentLifecycle(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := s.CreateProject(ctx, "my-api"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := s.CreateEnvironment(ctx, "my-api", "staging"); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if _, err := s.CreateEnvironment(ctx, "my-api", "staging"); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate env err = %v, want ErrDuplicate", err)
	}
	envs, err := s.ListEnvironments(ctx, "my-api")
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	if len(envs) != 1 || envs[0].Name != "staging" {
		t.Fatalf("envs = %+v, want one staging", envs)
	}
}

func TestGlobalAndEnvironmentPrecedence(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := s.CreateProject(ctx, "my-api"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := s.CreateEnvironment(ctx, "my-api", "staging"); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if _, err := s.PutSecret(ctx, "my-api", "", "REGION", "eu-west-1"); err != nil {
		t.Fatalf("PutSecret global: %v", err)
	}
	if _, err := s.PutSecret(ctx, "my-api", "", "DB_HOST", "shared.db"); err != nil {
		t.Fatalf("PutSecret global: %v", err)
	}
	if _, err := s.PutSecret(ctx, "my-api", "staging", "DB_HOST", "staging.db"); err != nil {
		t.Fatalf("PutSecret env: %v", err)
	}

	got, err := s.Resolve(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["REGION"] != "eu-west-1" {
		t.Errorf("REGION = %q, want global value", got["REGION"])
	}
	if got["DB_HOST"] != "staging.db" {
		t.Errorf("DB_HOST = %q, want environment value", got["DB_HOST"])
	}

	// Another environment falls back to the global.
	other, err := s.Resolve(ctx, "my-api", "")
	if err != nil {
		t.Fatalf("Resolve global: %v", err)
	}
	if other["DB_HOST"] != "shared.db" {
		t.Errorf("global DB_HOST = %q, want shared.db", other["DB_HOST"])
	}
}

func TestListKeysUnionAndOverride(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, "my-api")
	_, _ = s.CreateEnvironment(ctx, "my-api", "staging")
	_, _ = s.PutSecret(ctx, "my-api", "", "REGION", "eu-west-1")
	_, _ = s.PutSecret(ctx, "my-api", "staging", "DB_HOST", "staging.db")
	_, _ = s.PutSecret(ctx, "my-api", "staging", "REGION", "eu-west-2")

	keys, err := s.ListKeys(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 2 || keys[0] != "DB_HOST" || keys[1] != "REGION" {
		t.Fatalf("keys = %v, want [DB_HOST REGION]", keys)
	}

	secrets, err := s.ListSecrets(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	byKey := map[string]SecretInfo{}
	for _, si := range secrets {
		byKey[si.Key] = si
	}
	if byKey["DB_HOST"].Scope != ScopeEnvironment || byKey["DB_HOST"].Overrides {
		t.Errorf("DB_HOST = %+v, want environment scope without override", byKey["DB_HOST"])
	}
	if byKey["REGION"].Scope != ScopeEnvironment || !byKey["REGION"].Overrides {
		t.Errorf("REGION = %+v, want environment scope with override", byKey["REGION"])
	}
}

func TestGlobalAndEnvironmentSameKeyAllowed(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, "my-api")
	_, _ = s.CreateEnvironment(ctx, "my-api", "staging")
	if _, err := s.PutSecret(ctx, "my-api", "", "API_KEY", "global"); err != nil {
		t.Fatalf("global: %v", err)
	}
	if _, err := s.PutSecret(ctx, "my-api", "staging", "API_KEY", "env"); err != nil {
		t.Fatalf("env: %v", err)
	}
}

func TestShortValueWarning(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, "my-api")
	short, err := s.PutSecret(ctx, "my-api", "", "TINY", "dev")
	if err != nil {
		t.Fatalf("PutSecret: %v", err)
	}
	if !short {
		t.Fatal("short value not flagged")
	}
	long, err := s.PutSecret(ctx, "my-api", "", "LONG", "a-long-value")
	if err != nil {
		t.Fatalf("PutSecret: %v", err)
	}
	if long {
		t.Fatal("long value wrongly flagged as short")
	}
}

func TestAllowExecuteToggle(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, "my-api")
	if err := s.SetAllowExecute(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowExecute: %v", err)
	}
	p, err := s.GetProject(ctx, "my-api")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if !p.AllowExecute {
		t.Fatal("allow_execute not persisted")
	}
}

func TestAuditLog(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	exit := 1
	if err := s.AppendAudit(ctx, AuditEntry{
		Timestamp:   time.Now(),
		Client:      "opencode",
		Project:     "my-api",
		Environment: "staging",
		Tool:        "execute_with_secrets",
		KeyNames:    []string{"API_KEY", "DB_HOST"},
		Command:     "npm run migrate",
		ExitCode:    &exit,
		Redactions:  2,
	}); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}
	entries, err := s.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.ExitCode == nil || *e.ExitCode != 1 || e.Redactions != 2 {
		t.Fatalf("entry = %+v, want exit 1 and 2 redactions", e)
	}
	if len(e.KeyNames) != 2 || e.KeyNames[0] != "API_KEY" {
		t.Fatalf("key names = %v", e.KeyNames)
	}
}

func TestManyConcurrentWriters(t *testing.T) {
	key, err := crypto.NewSalt(crypto.KeySize)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "vault.db")

	writer, err := Open(path, key)
	if err != nil {
		t.Fatalf("Open writer: %v", err)
	}
	defer writer.Close()
	ctx := context.Background()
	if _, err := writer.CreateProject(ctx, "my-api"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	const writers = 4
	const perWriter = 5
	stores := make([]*Store, 0, writers)
	for i := 0; i < writers; i++ {
		s, err := Open(path, key)
		if err != nil {
			t.Fatalf("Open writer %d: %v", i, err)
		}
		defer s.Close()
		stores = append(stores, s)
	}

	var wg sync.WaitGroup
	errs := make(chan error, writers*perWriter)
	for i, s := range stores {
		for j := 0; j < perWriter; j++ {
			wg.Add(1)
			go func(i, j int, s *Store) {
				defer wg.Done()
				key := "K" + string(rune('A'+i)) + string(rune('0'+j))
				if _, err := s.PutSecret(ctx, "my-api", "", key, "value-"+key); err != nil {
					errs <- err
				}
			}(i, j, s)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent write: %v", err)
	}

	keys, err := writer.ListKeys(ctx, "my-api", "")
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != writers*perWriter {
		t.Fatalf("keys = %d, want %d", len(keys), writers*perWriter)
	}
}

func TestJournalModeWAL(t *testing.T) {
	s, _ := openTestStore(t)
	var mode string
	if err := s.db.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

func TestConcurrentStores(t *testing.T) {
	key, err := crypto.NewSalt(crypto.KeySize)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "vault.db")

	first, err := Open(path, key)
	if err != nil {
		t.Fatalf("Open first: %v", err)
	}
	defer first.Close()
	second, err := Open(path, key)
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	defer second.Close()

	ctx := context.Background()
	if _, err := first.CreateProject(ctx, "my-api"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, store := range []*Store{first, second} {
		wg.Add(1)
		go func(i int, store *Store) {
			defer wg.Done()
			key := "KEY"
			if i == 1 {
				key = "OTHER"
			}
			if _, err := store.PutSecret(ctx, "my-api", "", key, "some-value"); err != nil {
				errs <- err
			}
		}(i, store)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent write: %v", err)
	}

	keys, err := first.ListKeys(ctx, "my-api", "")
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("keys = %v, want 2", keys)
	}
}
