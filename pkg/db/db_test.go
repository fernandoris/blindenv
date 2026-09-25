package db

import (
	"context"
	"database/sql"
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
	if byKey["DB_HOST"].Scope != ScopeProjectEnvironment || len(byKey["DB_HOST"].Overrides) != 0 {
		t.Errorf("DB_HOST = %+v, want project+environment scope without override", byKey["DB_HOST"])
	}
	if byKey["REGION"].Scope != ScopeProjectEnvironment || len(byKey["REGION"].Overrides) == 0 {
		t.Errorf("REGION = %+v, want project+environment scope with override", byKey["REGION"])
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

// --- Shared scopes ---

func TestFourScopesSameKey(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, "my-api")
	_, _ = s.CreateEnvironment(ctx, "my-api", "staging")

	inserts := []struct {
		project, env, value string
	}{
		{"", "", "v-global"},
		{"", "staging", "v-envglobal"},
		{"my-api", "", "v-project"},
		{"my-api", "staging", "v-projectenv"},
	}
	for _, in := range inserts {
		if _, err := s.PutSecret(ctx, in.project, in.env, "API_KEY", in.value); err != nil {
			t.Fatalf("PutSecret(%q,%q): %v", in.project, in.env, err)
		}
	}

	got, err := s.Resolve(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["API_KEY"] != "v-projectenv" {
		t.Fatalf("effective API_KEY = %q, want v-projectenv", got["API_KEY"])
	}

	scopes := []struct {
		project, env, want string
	}{
		{"", "", "v-global"},
		{"", "staging", "v-envglobal"},
		{"my-api", "", "v-project"},
		{"my-api", "staging", "v-projectenv"},
	}
	for _, sc := range scopes {
		vals, err := s.ScopeValues(ctx, sc.project, sc.env)
		if err != nil {
			t.Fatalf("ScopeValues(%q,%q): %v", sc.project, sc.env, err)
		}
		if vals["API_KEY"] != sc.want {
			t.Errorf("ScopeValues(%q,%q) API_KEY = %q, want %q", sc.project, sc.env, vals["API_KEY"], sc.want)
		}
	}

	shared, err := s.ListSharedEnvironments(ctx)
	if err != nil {
		t.Fatalf("ListSharedEnvironments: %v", err)
	}
	if len(shared) != 1 || shared[0] != "staging" {
		t.Fatalf("shared envs = %v, want [staging]", shared)
	}
}

func TestPrecedenceAcrossScopes(t *testing.T) {
	type entry struct {
		scope string
		value string
	}
	cases := []struct {
		name    string
		entries []entry
		want    string
		scope   Scope
	}{
		{"project_env_wins", []entry{{"global", "g"}, {"shared_env", "e"}, {"project", "p"}, {"project_env", "pe"}}, "pe", ScopeProjectEnvironment},
		{"project_beats_shared_env", []entry{{"global", "g"}, {"shared_env", "e"}, {"project", "p"}}, "p", ScopeProject},
		{"shared_env_beats_global", []entry{{"global", "g"}, {"shared_env", "e"}}, "e", ScopeSharedEnvironment},
		{"global_only", []entry{{"global", "g"}}, "g", ScopeGlobal},
		{"project_env_beats_global", []entry{{"global", "g"}, {"project_env", "pe"}}, "pe", ScopeProjectEnvironment},
	}

	for i, tc := range cases {
		slug := "proj-" + string(rune('a'+i))
		env := "env-" + string(rune('a'+i))
		key := "KEY_" + tc.name
		s, _ := openTestStore(t)
		ctx := context.Background()
		if _, err := s.CreateProject(ctx, slug); err != nil {
			t.Fatalf("%s: CreateProject: %v", tc.name, err)
		}
		if _, err := s.CreateEnvironment(ctx, slug, env); err != nil {
			t.Fatalf("%s: CreateEnvironment: %v", tc.name, err)
		}
		for _, e := range tc.entries {
			var project, environment string
			switch e.scope {
			case "global":
				project, environment = "", ""
			case "shared_env":
				project, environment = "", env
			case "project":
				project, environment = slug, ""
			case "project_env":
				project, environment = slug, env
			}
			if _, err := s.PutSecret(ctx, project, environment, key, e.value); err != nil {
				t.Fatalf("%s: PutSecret(%s): %v", tc.name, e.scope, err)
			}
		}
		got, err := s.Resolve(ctx, slug, env)
		if err != nil {
			t.Fatalf("%s: Resolve: %v", tc.name, err)
		}
		if got[key] != tc.want {
			t.Errorf("%s: value = %q, want %q", tc.name, got[key], tc.want)
		}
		sources, err := s.EffectiveScopes(ctx, slug, env)
		if err != nil {
			t.Fatalf("%s: EffectiveScopes: %v", tc.name, err)
		}
		if sources[key] != tc.scope {
			t.Errorf("%s: scope = %q, want %q", tc.name, sources[key], tc.scope)
		}
	}
}

func TestEffectiveScopesCascade(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, "my-api")
	_, _ = s.CreateEnvironment(ctx, "my-api", "staging")

	// Same key in all four tiers; each delete should reveal the next winner.
	_ = put(t, s, "", "", "K", "global")
	_ = put(t, s, "", "staging", "K", "shared")
	_ = put(t, s, "my-api", "", "K", "project")
	_ = put(t, s, "my-api", "staging", "K", "projectenv")

	steps := []struct {
		delProject, delEnv, want string
		wantScope                Scope
	}{
		{"my-api", "staging", "project", ScopeProject},
		{"my-api", "", "shared", ScopeSharedEnvironment},
		{"", "staging", "global", ScopeGlobal},
	}
	for _, st := range steps {
		if err := s.DeleteSecret(ctx, st.delProject, st.delEnv, "K"); err != nil {
			t.Fatalf("DeleteSecret(%q,%q): %v", st.delProject, st.delEnv, err)
		}
		got, err := s.Resolve(ctx, "my-api", "staging")
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got["K"] != st.want {
			t.Fatalf("after delete, value = %q, want %q", got["K"], st.want)
		}
		sources, err := s.EffectiveScopes(ctx, "my-api", "staging")
		if err != nil {
			t.Fatalf("EffectiveScopes: %v", err)
		}
		if sources["K"] != st.wantScope {
			t.Fatalf("after delete, scope = %q, want %q", sources["K"], st.wantScope)
		}
	}
}

func TestScopeSecretsOverrides(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, "my-api")
	_, _ = s.CreateEnvironment(ctx, "my-api", "staging")
	_ = put(t, s, "", "", "SHARED", "g")
	_ = put(t, s, "my-api", "", "SHARED", "p")

	projectGlobals, err := s.ScopeSecrets(ctx, "my-api", "")
	if err != nil {
		t.Fatalf("ScopeSecrets: %v", err)
	}
	if len(projectGlobals) != 1 || projectGlobals[0].Scope != ScopeProject {
		t.Fatalf("project globals = %+v", projectGlobals)
	}
	if len(projectGlobals[0].Overrides) != 1 || projectGlobals[0].Overrides[0] != ScopeGlobal {
		t.Fatalf("overrides = %v, want [global]", projectGlobals[0].Overrides)
	}

	envSecrets, err := s.ScopeSecrets(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("ScopeSecrets env: %v", err)
	}
	if len(envSecrets) != 0 {
		t.Fatalf("env secrets = %+v, want none", envSecrets)
	}

	globalSecrets, err := s.ScopeSecrets(ctx, "", "")
	if err != nil {
		t.Fatalf("ScopeSecrets global: %v", err)
	}
	if len(globalSecrets[0].Overrides) != 0 {
		t.Fatalf("global overrides = %v, want none", globalSecrets[0].Overrides)
	}
}

func TestDeleteEnvironmentRemovesSecrets(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, "my-api")
	_, _ = s.CreateEnvironment(ctx, "my-api", "staging")
	_ = put(t, s, "my-api", "staging", "ONLY_ENV", "value-123")

	if err := s.DeleteEnvironment(ctx, "my-api", "staging"); err != nil {
		t.Fatalf("DeleteEnvironment: %v", err)
	}
	keys, err := s.ListKeys(ctx, "my-api", "")
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("orphan keys remain after env delete: %v", keys)
	}
}

func TestListSharedEnvironmentsIncludesSecretOnlyNames(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, "my-api")
	_, _ = s.CreateEnvironment(ctx, "my-api", "dev")
	_ = put(t, s, "", "orphan-env", "K", "value-123")

	shared, err := s.ListSharedEnvironments(ctx)
	if err != nil {
		t.Fatalf("ListSharedEnvironments: %v", err)
	}
	if len(shared) != 2 || shared[0] != "dev" || shared[1] != "orphan-env" {
		t.Fatalf("shared envs = %v, want [dev orphan-env]", shared)
	}
}

func put(t *testing.T, s *Store, project, environment, key, value string) string {
	t.Helper()
	if _, err := s.PutSecret(context.Background(), project, environment, key, value); err != nil {
		t.Fatalf("PutSecret(%q,%q,%s): %v", project, environment, key, err)
	}
	return value
}

func TestMigrateV1ToV2(t *testing.T) {
	key, err := crypto.NewSalt(crypto.KeySize)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "vault.db")

	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	v1 := `
CREATE TABLE schema_version (version INTEGER NOT NULL);
INSERT INTO schema_version (version) VALUES (1);
CREATE TABLE projects (
	id INTEGER PRIMARY KEY AUTOINCREMENT, slug TEXT NOT NULL UNIQUE,
	allow_execute INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL);
CREATE TABLE environments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	name TEXT NOT NULL, created_at TEXT NOT NULL, UNIQUE (project_id, name));
CREATE TABLE secrets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	environment_id INTEGER REFERENCES environments(id) ON DELETE CASCADE,
	key TEXT NOT NULL, value_enc BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE UNIQUE INDEX secrets_global_idx ON secrets (project_id, key) WHERE environment_id IS NULL;
CREATE UNIQUE INDEX secrets_env_idx ON secrets (project_id, environment_id, key) WHERE environment_id IS NOT NULL;
CREATE TABLE audit_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT NOT NULL, client TEXT NOT NULL DEFAULT '',
	project TEXT NOT NULL DEFAULT '', environment TEXT NOT NULL DEFAULT '', tool TEXT NOT NULL,
	key_names TEXT NOT NULL DEFAULT '', command TEXT NOT NULL DEFAULT '', exit_code INTEGER,
	redactions INTEGER NOT NULL DEFAULT 0);
INSERT INTO projects (slug, allow_execute, created_at) VALUES ('my-api', 1, '2026-01-01T00:00:00Z');
INSERT INTO environments (project_id, name, created_at) VALUES (1, 'staging', '2026-01-01T00:00:00Z');
`
	if _, err := raw.Exec(v1); err != nil {
		t.Fatalf("seed v1: %v", err)
	}
	encGlobal, err := crypto.Encrypt(key, []byte("eu-west-1-value"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	encEnv, err := crypto.Encrypt(key, []byte("staging-value-123"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO secrets (project_id, environment_id, key, value_enc, created_at, updated_at) VALUES (1, NULL, 'REGION', ?, 't', 't')`, encGlobal); err != nil {
		t.Fatalf("seed global: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO secrets (project_id, environment_id, key, value_enc, created_at, updated_at) VALUES (1, 1, 'API_KEY', ?, 't', 't')`, encEnv); err != nil {
		t.Fatalf("seed env: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	s, err := Open(path, key)
	if err != nil {
		t.Fatalf("Open migrated: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	got, err := s.Resolve(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["REGION"] != "eu-west-1-value" || got["API_KEY"] != "staging-value-123" {
		t.Fatalf("migrated values = %v", got)
	}
	global, err := s.Resolve(ctx, "my-api", "")
	if err != nil {
		t.Fatalf("Resolve global: %v", err)
	}
	if _, ok := global["API_KEY"]; ok {
		t.Fatal("env secret leaked into project-global after migration")
	}

	if err := s.AppendAudit(ctx, AuditEntry{Timestamp: time.Now(), Tool: "list_secret_keys", KeyNames: []string{"REGION"}, KeyScopes: []Scope{ScopeGlobal}}); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}
	entries, err := s.ListAudit(ctx, 1)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(entries) != 1 || len(entries[0].KeyScopes) != 1 || entries[0].KeyScopes[0] != ScopeGlobal {
		t.Fatalf("audit key scopes = %+v", entries)
	}
}
