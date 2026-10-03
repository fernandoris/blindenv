package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/fernandoris/blindenv/pkg/crypto"
)

func sensitive(v bool) *bool { return &v }

func TestMigrateV2ToV3AddsMetadataDefaults(t *testing.T) {
	key, err := crypto.NewSalt(crypto.KeySize)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "vault.db")

	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	v2 := `
CREATE TABLE schema_version (version INTEGER NOT NULL);
INSERT INTO schema_version (version) VALUES (2);
CREATE TABLE projects (
	id INTEGER PRIMARY KEY AUTOINCREMENT, slug TEXT NOT NULL UNIQUE,
	allow_execute INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL);
CREATE TABLE environments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	name TEXT NOT NULL, created_at TEXT NOT NULL, UNIQUE (project_id, name));
CREATE TABLE secrets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
	environment TEXT, key TEXT NOT NULL, value_enc BLOB NOT NULL,
	created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE audit_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT NOT NULL, client TEXT NOT NULL DEFAULT '',
	project TEXT NOT NULL DEFAULT '', environment TEXT NOT NULL DEFAULT '', tool TEXT NOT NULL,
	key_names TEXT NOT NULL DEFAULT '', key_scopes TEXT NOT NULL DEFAULT '',
	command TEXT NOT NULL DEFAULT '', exit_code INTEGER, redactions INTEGER NOT NULL DEFAULT 0);
INSERT INTO projects (slug, allow_execute, created_at) VALUES ('my-api', 0, 't');
INSERT INTO environments (project_id, name, created_at) VALUES (1, 'staging', 't');
`
	if _, err := raw.Exec(v2); err != nil {
		t.Fatalf("seed v2: %v", err)
	}
	enc, err := crypto.Encrypt(key, []byte("legacy-value-123"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO secrets (project_id, environment, key, value_enc, created_at, updated_at) VALUES (1, 'staging', 'API_KEY', ?, 't', 't')`, enc); err != nil {
		t.Fatalf("seed secret: %v", err)
	}
	_ = raw.Close()

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
	if got["API_KEY"] != "legacy-value-123" {
		t.Fatalf("migrated value = %q", got["API_KEY"])
	}
	infos, err := s.ListSecrets(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	if len(infos) != 1 || !infos[0].Sensitive || infos[0].Kind != KindText || infos[0].Hint != "" {
		t.Fatalf("migrated metadata = %+v, want sensitive text with no hint", infos)
	}
}

func TestNormalizeMeta(t *testing.T) {
	cases := []struct {
		name    string
		meta    SecretMeta
		want    SecretKind
		wantErr bool
	}{
		{"empty defaults to text", SecretMeta{}, KindText, false},
		{"known type kept", SecretMeta{Kind: KindURL}, KindURL, false},
		{"unknown type rejected", SecretMeta{Kind: "ip"}, "", true},
		{"token must be sensitive", SecretMeta{Kind: KindToken, Sensitive: sensitive(false)}, "", true},
		{"password must be sensitive", SecretMeta{Kind: KindPassword, Sensitive: sensitive(false)}, "", true},
		{"token sensitive ok", SecretMeta{Kind: KindToken, Sensitive: sensitive(true)}, KindToken, false},
		{"long hint rejected", SecretMeta{Hint: string(make([]byte, MaxHintLength+1))}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeMeta(tc.meta)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidMetadata) {
					t.Fatalf("err = %v, want ErrInvalidMetadata", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.Kind != tc.want {
				t.Fatalf("kind = %q, want %q", got.Kind, tc.want)
			}
		})
	}
	if err := validateHint(SecretMeta{Hint: "uses sk-abc123"}, "sk-abc123"); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("hint containing value not rejected: %v", err)
	}
	if err := validateHint(SecretMeta{Hint: "bearer token"}, "sk-abc123"); err != nil {
		t.Fatalf("clean hint rejected: %v", err)
	}
}

func TestPutSecretMetaSensitiveAndConfig(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, "my-api")

	if _, err := s.PutSecretMeta(ctx, "my-api", "", "TOKEN", "sk-abcdef123456", SecretMeta{
		Kind: KindToken, Hint: "send as Bearer", Sensitive: sensitive(true),
	}); err != nil {
		t.Fatalf("PutSecretMeta sensitive: %v", err)
	}
	if _, err := s.PutSecretMeta(ctx, "my-api", "", "ENDPOINT", "https://rancher.example.com/v3", SecretMeta{
		Kind: KindURL, Hint: "base includes /v3", Sensitive: sensitive(false),
	}); err != nil {
		t.Fatalf("PutSecretMeta config: %v", err)
	}

	var enc []byte
	var plain sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT value_enc, value_plain FROM secrets WHERE key = 'TOKEN'`).Scan(&enc, &plain); err != nil {
		t.Fatalf("query TOKEN: %v", err)
	}
	if len(enc) == 0 || plain.Valid {
		t.Fatalf("sensitive row not XORed: enc=%d plain=%v", len(enc), plain)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT value_enc, value_plain FROM secrets WHERE key = 'ENDPOINT'`).Scan(&enc, &plain); err != nil {
		t.Fatalf("query ENDPOINT: %v", err)
	}
	if len(enc) != 0 || !plain.Valid || plain.String != "https://rancher.example.com/v3" {
		t.Fatalf("config row not plaintext: enc=%d plain=%v", len(enc), plain)
	}
}

func TestSetSecretMetadataFlipsSensitivityWithoutLosingValue(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, "my-api")
	_, _ = s.PutSecretMeta(ctx, "my-api", "", "ENDPOINT", "https://rancher.example.com/v3",
		SecretMeta{Kind: KindURL, Hint: "base includes /v3", Sensitive: sensitive(true)})

	// Metadata-only change keeps the value.
	if err := s.SetSecretMetadata(ctx, "my-api", "", "ENDPOINT",
		SecretMeta{Kind: KindURL, Hint: "use /v3/clusters", Sensitive: sensitive(true)}); err != nil {
		t.Fatalf("SetSecretMetadata metadata-only: %v", err)
	}
	got, _ := s.Resolve(ctx, "my-api", "")
	if got["ENDPOINT"] != "https://rancher.example.com/v3" {
		t.Fatalf("value changed: %q", got["ENDPOINT"])
	}

	// Flip to config: value preserved, stored cleartext.
	if err := s.SetSecretMetadata(ctx, "my-api", "", "ENDPOINT",
		SecretMeta{Kind: KindURL, Hint: "use /v3/clusters", Sensitive: sensitive(false)}); err != nil {
		t.Fatalf("flip to config: %v", err)
	}
	var enc []byte
	var plain sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT value_enc, value_plain FROM secrets WHERE key = 'ENDPOINT'`).Scan(&enc, &plain); err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(enc) != 0 || !plain.Valid || plain.String != "https://rancher.example.com/v3" {
		t.Fatalf("flip did not store cleartext: enc=%d plain=%v", len(enc), plain)
	}
	got, _ = s.Resolve(ctx, "my-api", "")
	if got["ENDPOINT"] != "https://rancher.example.com/v3" {
		t.Fatalf("value lost on flip: %q", got["ENDPOINT"])
	}

	// Flip back to sensitive.
	if err := s.SetSecretMetadata(ctx, "my-api", "", "ENDPOINT",
		SecretMeta{Kind: KindToken, Sensitive: sensitive(true)}); err != nil {
		t.Fatalf("flip back: %v", err)
	}
	got, _ = s.Resolve(ctx, "my-api", "")
	if got["ENDPOINT"] != "https://rancher.example.com/v3" {
		t.Fatalf("value lost flipping back: %q", got["ENDPOINT"])
	}
}

func TestResolveDetailedMetadataWinningScope(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, "my-api")
	_, _ = s.CreateEnvironment(ctx, "my-api", "staging")

	_, _ = s.PutSecretMeta(ctx, "", "", "ENDPOINT", "https://global/v3",
		SecretMeta{Kind: KindURL, Hint: "global hint", Sensitive: sensitive(false)})
	_, _ = s.PutSecretMeta(ctx, "my-api", "staging", "ENDPOINT", "https://project/v3",
		SecretMeta{Kind: KindURL, Hint: "project hint", Sensitive: sensitive(true)})
	_, _ = s.PutSecretMeta(ctx, "", "staging", "REGION", "eu-west-1",
		SecretMeta{Kind: KindText, Sensitive: sensitive(false)})

	got, err := s.ResolveDetailed(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("ResolveDetailed: %v", err)
	}
	endpoint := got["ENDPOINT"]
	if endpoint.Value != "https://project/v3" || !endpoint.Sensitive || endpoint.Hint != "project hint" || endpoint.Scope != ScopeProjectEnvironment {
		t.Fatalf("effective ENDPOINT = %+v", endpoint)
	}
	region := got["REGION"]
	if region.Value != "eu-west-1" || region.Sensitive || region.Scope != ScopeSharedEnvironment {
		t.Fatalf("effective REGION = %+v", region)
	}

	infos, err := s.ListSecrets(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	byKey := map[string]SecretInfo{}
	for _, info := range infos {
		byKey[info.Key] = info
	}
	if byKey["REGION"].Kind != KindText || byKey["REGION"].Sensitive {
		t.Fatalf("REGION info = %+v", byKey["REGION"])
	}
}

func TestScopeSecretsIncludesMetadata(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, "my-api")
	_, _ = s.PutSecretMeta(ctx, "my-api", "", "ENDPOINT", "https://rancher/v3",
		SecretMeta{Kind: KindURL, Hint: "base url", Sensitive: sensitive(false)})

	infos, err := s.ScopeSecrets(ctx, "my-api", "")
	if err != nil {
		t.Fatalf("ScopeSecrets: %v", err)
	}
	if len(infos) != 1 || infos[0].Kind != KindURL || infos[0].Hint != "base url" || infos[0].Sensitive {
		t.Fatalf("scope metadata = %+v", infos)
	}
}

func TestScopeEntriesMixedMetadata(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, _ = s.PutSecretMeta(ctx, "", "", "GLOBAL_TOKEN", "global-token-123",
		SecretMeta{Kind: KindToken, Sensitive: sensitive(true)})
	_, _ = s.PutSecretMeta(ctx, "", "staging", "ENDPOINT", "https://shared/v3",
		SecretMeta{Kind: KindURL, Hint: "shared url", Sensitive: sensitive(false)})

	entries, err := s.ScopeEntries(ctx, "", "staging")
	if err != nil {
		t.Fatalf("ScopeEntries: %v", err)
	}
	e := entries["ENDPOINT"]
	if e.Value != "https://shared/v3" || e.Sensitive || e.Kind != KindURL || e.Hint != "shared url" {
		t.Fatalf("entry = %+v", e)
	}
	global, err := s.ScopeEntries(ctx, "", "")
	if err != nil {
		t.Fatalf("ScopeEntries global: %v", err)
	}
	if g := global["GLOBAL_TOKEN"]; g.Value != "global-token-123" || !g.Sensitive || g.Kind != KindToken {
		t.Fatalf("global entry = %+v", g)
	}
}
