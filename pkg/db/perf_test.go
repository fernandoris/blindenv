package db

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fernandoris/blindenv/pkg/crypto"
)

func openBenchStore(b *testing.B) *Store {
	b.Helper()
	key, err := crypto.NewSalt(crypto.KeySize)
	if err != nil {
		b.Fatalf("key: %v", err)
	}
	s, err := Open(filepath.Join(b.TempDir(), "vault.db"), key)
	if err != nil {
		b.Fatalf("Open: %v", err)
	}
	b.Cleanup(func() { _ = s.Close() })
	return s
}

// BenchmarkResolveAllSensitive measures the unchanged hot path (all sensitive
// definitions) so it can be compared against the pre-metadata implementation.
func BenchmarkResolveAllSensitive(b *testing.B) {
	ctx := context.Background()
	s := openBenchStore(b)
	_, _ = s.CreateProject(ctx, "my-api")
	_, _ = s.CreateEnvironment(ctx, "my-api", "staging")
	const keys = 200
	for i := 0; i < keys; i++ {
		k := fmt.Sprintf("KEY_%03d", i)
		_, _ = s.PutSecret(ctx, "", "", k, fmt.Sprintf("global-value-%03d-abcdef", i))
		_, _ = s.PutSecret(ctx, "", "staging", k, fmt.Sprintf("shared-value-%03d-abcdef", i))
		_, _ = s.PutSecret(ctx, "my-api", "", k, fmt.Sprintf("project-value-%03d-abcdef", i))
		_, _ = s.PutSecret(ctx, "my-api", "staging", k, fmt.Sprintf("env-value-%03d-abcdef", i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Resolve(ctx, "my-api", "staging"); err != nil {
			b.Fatalf("Resolve: %v", err)
		}
	}
}

// BenchmarkEffectiveSecretsMixed measures resolution with half the definitions
// non-sensitive, across all four scopes.
func BenchmarkEffectiveSecretsMixed(b *testing.B) {
	ctx := context.Background()
	s := openBenchStore(b)
	_, _ = s.CreateProject(ctx, "my-api")
	_, _ = s.CreateEnvironment(ctx, "my-api", "staging")
	const keys = 200
	for i := 0; i < keys; i++ {
		k := fmt.Sprintf("KEY_%03d", i)
		secret := i%2 == 0
		meta := SecretMeta{Sensitive: &secret}
		if !secret {
			meta.Kind = KindURL
		}
		put := func(project, env, value string) {
			if _, err := s.PutSecretMeta(ctx, project, env, k, value, meta); err != nil {
				b.Fatalf("PutSecretMeta: %v", err)
			}
		}
		put("", "", fmt.Sprintf("global-value-%03d-abcdef", i))
		put("", "staging", fmt.Sprintf("shared-value-%03d-abcdef", i))
		put("my-api", "", fmt.Sprintf("project-value-%03d-abcdef", i))
		put("my-api", "staging", fmt.Sprintf("env-value-%03d-abcdef", i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.ResolveDetailed(ctx, "my-api", "staging"); err != nil {
			b.Fatalf("ResolveDetailed: %v", err)
		}
	}
}

// BenchmarkListScopedKeys measures the discovery path, which is an N+1 query
// per environment and now also fetches the metadata columns.
func BenchmarkListScopedKeys(b *testing.B) {
	ctx := context.Background()
	s := openBenchStore(b)
	_, _ = s.CreateProject(ctx, "my-api")
	const envs = 20
	const keys = 20
	for e := 0; e < envs; e++ {
		env := fmt.Sprintf("env-%02d", e)
		_, _ = s.CreateEnvironment(ctx, "my-api", env)
		for i := 0; i < keys; i++ {
			k := fmt.Sprintf("KEY_%03d", i)
			_, _ = s.PutSecret(ctx, "my-api", env, k, fmt.Sprintf("value-%03d-abcdef", i))
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.ListScopedKeys(ctx, "my-api"); err != nil {
			b.Fatalf("ListScopedKeys: %v", err)
		}
	}
}

// legacySecretSchema is a schema-v4 vault with a populated secrets table, used
// to benchmark and bound the pre-migration snapshot and migration paths.
const legacySecretSchema = `
CREATE TABLE schema_version (version INTEGER NOT NULL);
INSERT INTO schema_version (version) VALUES (4);
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

// writeLegacyVault seeds a schema-v4 vault with n ~200-byte secret rows.
func writeLegacyVault(tb testing.TB, path string, n int) {
	tb.Helper()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		tb.Fatalf("open raw: %v", err)
	}
	if _, err := raw.Exec(legacySecretSchema); err != nil {
		tb.Fatalf("seed schema: %v", err)
	}
	tx, err := raw.Begin()
	if err != nil {
		tb.Fatalf("begin: %v", err)
	}
	stmt, err := tx.Prepare(
		`INSERT INTO secrets (project_id, environment, key, value_enc, created_at, updated_at) VALUES (1, 'staging', ?, ?, 't', 't')`)
	if err != nil {
		tb.Fatalf("prepare: %v", err)
	}
	val := bytes.Repeat([]byte("x"), 200)
	for i := 0; i < n; i++ {
		if _, err := stmt.Exec(fmt.Sprintf("KEY_%05d", i), val); err != nil {
			tb.Fatalf("insert: %v", err)
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		tb.Fatalf("commit: %v", err)
	}
	if err := raw.Close(); err != nil {
		tb.Fatalf("close raw: %v", err)
	}
}

// BenchmarkMigrateSnapshot measures the one-time cost of migrating a legacy
// vault, including the pre-migration snapshot, so it can be tracked as the
// vault grows.
func BenchmarkMigrateSnapshot(b *testing.B) {
	key := make([]byte, crypto.KeySize)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		dir, err := os.MkdirTemp("", "blindenv-bench-")
		if err != nil {
			b.Fatalf("temp dir: %v", err)
		}
		path := filepath.Join(dir, "vault.db")
		writeLegacyVault(b, path, 200)
		b.StartTimer()
		s, err := Open(path, key)
		if err != nil {
			b.Fatalf("Open: %v", err)
		}
		_ = s.Close()
		b.StopTimer()
		_ = os.RemoveAll(dir)
		b.StartTimer()
	}
}

// BenchmarkOpenCurrent measures the steady-state cost of opening a vault that
// is already at the supported version (no snapshot, no migration).
func BenchmarkOpenCurrent(b *testing.B) {
	key := make([]byte, crypto.KeySize)
	dir, err := os.MkdirTemp("", "blindenv-bench-")
	if err != nil {
		b.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "vault.db")
	s, err := Open(path, key)
	if err != nil {
		b.Fatalf("Open: %v", err)
	}
	_ = s.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, err := Open(path, key)
		if err != nil {
			b.Fatalf("Open: %v", err)
		}
		_ = s.Close()
	}
}

// TestLargeVaultMigrationBounded guards against a snapshot implementation that
// buffers the vault: a large legacy vault must migrate within a generous
// budget.
func TestLargeVaultMigrationBounded(t *testing.T) {
	key := make([]byte, crypto.KeySize)
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.db")
	writeLegacyVault(t, path, 5000)

	start := time.Now()
	s, err := Open(path, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if elapsed := time.Since(start); elapsed > 60*time.Second {
		t.Fatalf("migration took %s, want bounded", elapsed)
	}
	m := s.Migration()
	if m.To != schemaVersion {
		t.Fatalf("migration = %+v, want To=%d", m, schemaVersion)
	}
	if _, err := os.Stat(m.Snapshot); err != nil {
		t.Fatalf("snapshot missing: %v", err)
	}
}

// snapshotAllocs returns the allocations of one pre-migration snapshot on a
// current vault seeded with n secret rows.
func snapshotAllocs(t *testing.T, n int) float64 {
	t.Helper()
	key := make([]byte, crypto.KeySize)
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.db")
	writeLegacyVault(t, path, n)
	s, err := Open(path, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	dest := path + ".v99.bak"
	return testing.AllocsPerRun(3, func() {
		_ = os.Remove(dest)
		if _, err := s.snapshotVault(ctx, 99); err != nil {
			t.Fatalf("snapshotVault: %v", err)
		}
	})
}

// TestSnapshotAllocationsBounded proves the snapshot is produced by SQLite and
// not by buffering the vault in Go: allocations must not scale with vault size.
func TestSnapshotAllocationsBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds large vaults")
	}
	small := snapshotAllocs(t, 50)
	large := snapshotAllocs(t, 5000)
	t.Logf("snapshot allocs: small=%.0f large=%.0f", small, large)
	if large > small*2+50 {
		t.Fatalf("snapshot allocs scaled with vault size: small=%.0f large=%.0f", small, large)
	}
	if large > 2000 {
		t.Fatalf("snapshot allocs = %.0f, want bounded", large)
	}
}

// TestOpenCurrentAllocationsBounded bounds the allocations of opening an
// already-current vault, the hot path on every process start.
func TestOpenCurrentAllocationsBounded(t *testing.T) {
	key := make([]byte, crypto.KeySize)
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.db")
	s, err := Open(path, key)
	if err != nil {
		t.Fatalf("seed Open: %v", err)
	}
	_ = s.Close()

	allocs := testing.AllocsPerRun(5, func() {
		s, err := Open(path, key)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		_ = s.Close()
	})
	if allocs > 4000 {
		t.Fatalf("open-current allocs = %.0f, want bounded", allocs)
	}
	t.Logf("open-current allocs = %.0f", allocs)
}
