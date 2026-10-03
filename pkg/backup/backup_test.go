package backup

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/fernandoris/blindenv/pkg/crypto"
	"github.com/fernandoris/blindenv/pkg/db"
)

func newStore(t *testing.T) *db.Store {
	t.Helper()
	key, err := crypto.NewSalt(crypto.KeySize)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	store, err := db.Open(filepath.Join(t.TempDir(), "vault.db"), key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func seed(t *testing.T, store *db.Store) {
	t.Helper()
	ctx := context.Background()
	if _, err := store.CreateProject(ctx, "my-api"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := store.SetAllowExecute(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowExecute: %v", err)
	}
	if err := store.SetAllowOpen(ctx, "my-api", true); err != nil {
		t.Fatalf("SetAllowOpen: %v", err)
	}
	if _, err := store.CreateEnvironment(ctx, "my-api", "staging"); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if _, err := store.PutSecret(ctx, "my-api", "", "REGION", "eu-west-1"); err != nil {
		t.Fatalf("PutSecret: %v", err)
	}
	if _, err := store.PutSecret(ctx, "my-api", "staging", "API_KEY", "sk-round-trip-123"); err != nil {
		t.Fatalf("PutSecret: %v", err)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	source := newStore(t)
	seed(t, source)

	blob, err := Export(ctx, source, "backup-pass")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	target := newStore(t)
	if err := Import(ctx, target, "backup-pass", blob); err != nil {
		t.Fatalf("Import: %v", err)
	}

	got, err := target.Resolve(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["API_KEY"] != "sk-round-trip-123" || got["REGION"] != "eu-west-1" {
		t.Fatalf("restored values = %v", got)
	}
	p, err := target.GetProject(ctx, "my-api")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if !p.AllowExecute {
		t.Fatal("allow_execute not restored")
	}
	if !p.AllowOpen {
		t.Fatal("allow_open not restored")
	}
}

func TestImportWrongPassphraseDoesNotMutate(t *testing.T) {
	ctx := context.Background()
	source := newStore(t)
	seed(t, source)
	blob, err := Export(ctx, source, "right-pass")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	target := newStore(t)
	if err := Import(ctx, target, "wrong-pass", blob); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("err = %v, want ErrWrongPassphrase", err)
	}
	projects, err := target.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("target mutated: %v", projects)
	}
}

func TestImportUnrecognized(t *testing.T) {
	target := newStore(t)
	if err := Import(context.Background(), target, "x", []byte("not a backup")); !errors.Is(err, ErrUnrecognized) {
		t.Fatalf("err = %v, want ErrUnrecognized", err)
	}
}

func TestImportDuplicateProject(t *testing.T) {
	ctx := context.Background()
	source := newStore(t)
	seed(t, source)
	blob, err := Export(ctx, source, "pass")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	target := newStore(t)
	seed(t, target)
	if err := Import(ctx, target, "pass", blob); err == nil {
		t.Fatal("expected error importing into a vault that already has the project")
	}
}

func TestSharedScopesRoundTrip(t *testing.T) {
	ctx := context.Background()
	source := newStore(t)
	seed(t, source)
	if _, err := source.PutSecret(ctx, "", "", "GLOBAL_TOKEN", "global-value-123"); err != nil {
		t.Fatalf("PutSecret global: %v", err)
	}
	if _, err := source.PutSecret(ctx, "", "staging", "SHARED_ENV", "shared-env-value-123"); err != nil {
		t.Fatalf("PutSecret shared env: %v", err)
	}

	blob, err := Export(ctx, source, "backup-pass")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	target := newStore(t)
	if err := Import(ctx, target, "backup-pass", blob); err != nil {
		t.Fatalf("Import: %v", err)
	}

	global, err := target.ScopeValues(ctx, "", "")
	if err != nil {
		t.Fatalf("ScopeValues global: %v", err)
	}
	if global["GLOBAL_TOKEN"] != "global-value-123" {
		t.Fatalf("global token = %q", global["GLOBAL_TOKEN"])
	}
	sharedEnv, err := target.ScopeValues(ctx, "", "staging")
	if err != nil {
		t.Fatalf("ScopeValues shared env: %v", err)
	}
	if sharedEnv["SHARED_ENV"] != "shared-env-value-123" {
		t.Fatalf("shared env value = %q", sharedEnv["SHARED_ENV"])
	}

	// A project that does not define them resolves the shared values.
	got, err := target.Resolve(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["GLOBAL_TOKEN"] != "global-value-123" || got["SHARED_ENV"] != "shared-env-value-123" {
		t.Fatalf("resolved shared values = %v", got)
	}
}

func TestImportV1Backup(t *testing.T) {
	ctx := context.Background()
	snapshot := data{Projects: []project{{
		Slug:         "legacy-api",
		AllowExecute: true,
		Globals:      map[string]string{"REGION": "eu-west-1"},
		Environments: []environment{{Name: "staging", Secrets: map[string]string{"API_KEY": "sk-legacy-123"}}},
	}}}
	plain, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	salt, err := crypto.NewSalt(saltSize)
	if err != nil {
		t.Fatalf("salt: %v", err)
	}
	ciphertext, err := crypto.Encrypt(crypto.DeriveKey([]byte("legacy-pass"), salt), plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	blob := append(append([]byte{}, magicV1...), salt...)
	blob = append(blob, ciphertext...)

	target := newStore(t)
	if err := Import(ctx, target, "legacy-pass", blob); err != nil {
		t.Fatalf("Import v1: %v", err)
	}
	got, err := target.Resolve(ctx, "legacy-api", "staging")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["REGION"] != "eu-west-1" || got["API_KEY"] != "sk-legacy-123" {
		t.Fatalf("imported v1 values = %v", got)
	}
	p, err := target.GetProject(ctx, "legacy-api")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if !p.AllowExecute {
		t.Fatal("allow_execute not restored from v1")
	}
}

func TestMetadataRoundTrip(t *testing.T) {
	ctx := context.Background()
	source := newStore(t)
	seed(t, source)
	_, _ = source.PutSecretMeta(ctx, "my-api", "staging", "ENDPOINT", "https://rancher.example.com/v3", db.SecretMeta{
		Kind: db.KindURL, Hint: "base includes /v3", Sensitive: boolPtr(false),
	})
	_, _ = source.PutSecretMeta(ctx, "my-api", "staging", "TOKEN", "sk-token-123456", db.SecretMeta{
		Kind: db.KindToken, Hint: "bearer", Sensitive: boolPtr(true),
	})
	_, _ = source.PutSecretMeta(ctx, "", "", "SHARED_CONFIG", "shared-value-123", db.SecretMeta{
		Kind: db.KindHost, Hint: "shared host", Sensitive: boolPtr(false),
	})

	blob, err := Export(ctx, source, "backup-pass")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	target := newStore(t)
	if err := Import(ctx, target, "backup-pass", blob); err != nil {
		t.Fatalf("Import: %v", err)
	}

	entries, err := target.ScopeEntries(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("ScopeEntries: %v", err)
	}
	ep := entries["ENDPOINT"]
	if ep.Value != "https://rancher.example.com/v3" || ep.Sensitive || ep.Kind != db.KindURL || ep.Hint != "base includes /v3" {
		t.Fatalf("ENDPOINT = %+v", ep)
	}
	tok := entries["TOKEN"]
	if tok.Value != "sk-token-123456" || !tok.Sensitive || tok.Kind != db.KindToken || tok.Hint != "bearer" {
		t.Fatalf("TOKEN = %+v", tok)
	}
	global, err := target.ScopeEntries(ctx, "", "")
	if err != nil {
		t.Fatalf("ScopeEntries global: %v", err)
	}
	if sc := global["SHARED_CONFIG"]; sc.Value != "shared-value-123" || sc.Sensitive || sc.Kind != db.KindHost {
		t.Fatalf("SHARED_CONFIG = %+v", sc)
	}
}

func TestImportV1BackupDefaultsToSensitiveText(t *testing.T) {
	ctx := context.Background()
	snapshot := data{Projects: []project{{
		Slug:         "legacy-api",
		AllowExecute: false,
		Globals:      map[string]string{"REGION": "eu-west-1"},
	}}}
	plain, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	salt, err := crypto.NewSalt(saltSize)
	if err != nil {
		t.Fatalf("salt: %v", err)
	}
	ciphertext, err := crypto.Encrypt(crypto.DeriveKey([]byte("legacy-pass"), salt), plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	blob := append(append([]byte{}, magicV1...), salt...)
	blob = append(blob, ciphertext...)

	target := newStore(t)
	if err := Import(ctx, target, "legacy-pass", blob); err != nil {
		t.Fatalf("Import v1: %v", err)
	}
	entries, err := target.ScopeEntries(ctx, "legacy-api", "")
	if err != nil {
		t.Fatalf("ScopeEntries: %v", err)
	}
	e := entries["REGION"]
	if e.Value != "eu-west-1" || !e.Sensitive || e.Kind != db.KindText || e.Hint != "" {
		t.Fatalf("legacy metadata = %+v, want sensitive text", e)
	}
}

func boolPtr(v bool) *bool { return &v }
