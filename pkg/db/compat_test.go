package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/fernandoris/blindenv/pkg/crypto"
)

// --- Schema seeds for each historical version ---

const seedV1 = `
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
INSERT INTO projects (slug, allow_execute, created_at) VALUES ('my-api', 1, 't');
INSERT INTO environments (project_id, name, created_at) VALUES (1, 'staging', 't');
`

const seedV2 = `
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
`

const seedV3 = `
CREATE TABLE schema_version (version INTEGER NOT NULL);
INSERT INTO schema_version (version) VALUES (3);
CREATE TABLE projects (
	id INTEGER PRIMARY KEY AUTOINCREMENT, slug TEXT NOT NULL UNIQUE,
	allow_execute INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL);
INSERT INTO projects (slug, allow_execute, created_at) VALUES ('my-api', 1, 't');
`

const seedV4 = `
CREATE TABLE schema_version (version INTEGER NOT NULL);
INSERT INTO schema_version (version) VALUES (4);
CREATE TABLE audit_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT NOT NULL, client TEXT NOT NULL DEFAULT '',
	project TEXT NOT NULL DEFAULT '', environment TEXT NOT NULL DEFAULT '', tool TEXT NOT NULL,
	key_names TEXT NOT NULL DEFAULT '', key_scopes TEXT NOT NULL DEFAULT '',
	command TEXT NOT NULL DEFAULT '', exit_code INTEGER,
	redactions INTEGER NOT NULL DEFAULT 0);
INSERT INTO audit_log (ts, tool, command, redactions) VALUES ('t', 'list_secret_keys', '', 5);
`

func testKey(t *testing.T) []byte {
	t.Helper()
	key, err := crypto.NewSalt(crypto.KeySize)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	return key
}

func seedRaw(t *testing.T, path, sqlText string) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := raw.Exec(sqlText); err != nil {
		t.Fatalf("seed raw (v?): %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}
}

func rawSchemaVersion(t *testing.T, path string) int {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer raw.Close()
	var v int
	if err := raw.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&v); err != nil {
		t.Fatalf("read version: %v", err)
	}
	return v
}

func mustGlob(t *testing.T, pattern string) []string {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	return matches
}

// TestMigrationSnapshotsEachSourceVersion verifies that opening every supported
// older schema migrates it, records the migration, and writes a restorable
// snapshot of the pre-migration vault.
func TestMigrationSnapshotsEachSourceVersion(t *testing.T) {
	cases := []struct {
		version int
		seed    string
	}{
		{1, seedV1},
		{2, seedV2},
		{3, seedV3},
		{4, seedV4},
	}
	for _, tc := range cases {
		t.Run("v"+strconv.Itoa(tc.version), func(t *testing.T) {
			key := testKey(t)
			path := filepath.Join(t.TempDir(), "vault.db")
			seedRaw(t, path, tc.seed)

			s, err := Open(path, key)
			if err != nil {
				t.Fatalf("Open migrated: %v", err)
			}
			defer s.Close()

			m := s.Migration()
			if m.From != tc.version || m.To != schemaVersion {
				t.Fatalf("migration = %+v, want From=%d To=%d", m, tc.version, schemaVersion)
			}
			wantSnap := path + ".v" + strconv.Itoa(tc.version) + ".bak"
			if m.Snapshot != wantSnap {
				t.Fatalf("snapshot = %q, want %q", m.Snapshot, wantSnap)
			}
			if _, err := os.Stat(wantSnap); err != nil {
				t.Fatalf("snapshot missing: %v", err)
			}
			if got := rawSchemaVersion(t, wantSnap); got != tc.version {
				t.Fatalf("snapshot schema = %d, want %d", got, tc.version)
			}
			if got := rawSchemaVersion(t, path); got != schemaVersion {
				t.Fatalf("migrated schema = %d, want %d", got, schemaVersion)
			}
		})
	}
}

// TestOpenNewVaultDoesNotSnapshot verifies a freshly created vault is silent.
func TestOpenNewVaultDoesNotSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.db")
	s, err := Open(path, testKey(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if m := s.Migration(); m.From != 0 || m.To != 0 || m.Snapshot != "" {
		t.Fatalf("fresh vault migration = %+v, want zero", m)
	}
	if matches := mustGlob(t, path+".v*.bak"); len(matches) != 0 {
		t.Fatalf("fresh vault wrote snapshots: %v", matches)
	}
}

// TestOpenCurrentVaultDoesNotSnapshot verifies the steady state stays silent.
func TestOpenCurrentVaultDoesNotSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.db")
	key := testKey(t)

	s, err := Open(path, key)
	if err != nil {
		t.Fatalf("Open first: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(path, key)
	if err != nil {
		t.Fatalf("Open again: %v", err)
	}
	defer s2.Close()
	if m := s2.Migration(); m.From != 0 || m.To != 0 {
		t.Fatalf("current vault migration = %+v, want zero", m)
	}
	if matches := mustGlob(t, path+".v*.bak"); len(matches) != 0 {
		t.Fatalf("current vault wrote snapshots: %v", matches)
	}
}

// TestExistingSnapshotPreserved verifies a snapshot for the same source version
// is never overwritten.
func TestExistingSnapshotPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	seedRaw(t, path, seedV4)
	dest := path + ".v4.bak"
	if err := os.WriteFile(dest, []byte("keepme"), 0o600); err != nil {
		t.Fatalf("pre-seed snapshot: %v", err)
	}

	s, err := Open(path, testKey(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if m := s.Migration(); m.Snapshot != dest {
		t.Fatalf("snapshot = %q, want %q", m.Snapshot, dest)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(got) != "keepme" {
		t.Fatalf("existing snapshot overwritten: %q", got)
	}
}

// TestSchemaTooNewRefusedUntouched verifies an older build refuses a newer
// vault without modifying it.
func TestSchemaTooNewRefusedUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	seedRaw(t, path, `
CREATE TABLE schema_version (version INTEGER NOT NULL);
INSERT INTO schema_version (version) VALUES (6);
CREATE TABLE projects (
	id INTEGER PRIMARY KEY AUTOINCREMENT, slug TEXT NOT NULL UNIQUE,
	allow_execute INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL);
INSERT INTO projects (slug, allow_execute, created_at) VALUES ('my-api', 1, 't');
`)

	_, err := Open(path, testKey(t))
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("err = %v, want ErrSchemaTooNew", err)
	}
	if !strings.Contains(err.Error(), "not modified") {
		t.Fatalf("error lacks reassurance: %v", err)
	}
	if got := rawSchemaVersion(t, path); got != 6 {
		t.Fatalf("version = %d, want 6 (refusal must not lower it)", got)
	}
	if matches := mustGlob(t, path+".v*.bak"); len(matches) != 0 {
		t.Fatalf("refusal wrote snapshots: %v", matches)
	}
}

// TestRecordSchemaVersionMonotonic verifies the marker never regresses even if
// the guard is bypassed.
func TestRecordSchemaVersionMonotonic(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `UPDATE schema_version SET version = ?`, schemaVersion+3); err != nil {
		t.Fatalf("bump: %v", err)
	}
	if err := s.recordSchemaVersion(ctx); err != nil {
		t.Fatalf("recordSchemaVersion: %v", err)
	}
	if got := rawSchemaVersion(t, s.path); got != schemaVersion+3 {
		t.Fatalf("version = %d, want %d (must not lower)", got, schemaVersion+3)
	}
}

// TestSnapshotFailureAbortsBeforeDDL verifies that when the snapshot cannot be
// written, the open fails and no migration DDL is applied.
func TestSnapshotFailureAbortsBeforeDDL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	seedRaw(t, path, seedV4)
	// A directory at the snapshot path makes the snapshot impossible.
	if err := os.Mkdir(path+".v4.bak", 0o700); err != nil {
		t.Fatalf("mkdir snapshot collision: %v", err)
	}

	_, err := Open(path, testKey(t))
	if err == nil {
		t.Fatal("expected Open to fail when the snapshot cannot be written")
	}
	if got := rawSchemaVersion(t, path); got != 4 {
		t.Fatalf("version = %d, want 4 (no migration should have run)", got)
	}
	// The v5 substitutions column must be absent, proving DDL did not run.
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer raw.Close()
	h, err := columnExistsRaw(raw, "audit_log", "substitutions")
	if err != nil {
		t.Fatalf("column exists: %v", err)
	}
	if h {
		t.Fatal("migration DDL ran despite snapshot failure")
	}
}

func columnExistsRaw(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// TestMigrationRoundTripRestore migrates a vault, restores the snapshot, and
// verifies the pre-migration values survive.
func TestMigrationRoundTripRestore(t *testing.T) {
	key := testKey(t)
	path := filepath.Join(t.TempDir(), "vault.db")
	seedRaw(t, path, `
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
`)
	enc, err := crypto.Encrypt(key, []byte("legacy-value-123"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO secrets (project_id, environment, key, value_enc, created_at, updated_at) VALUES (1, 'staging', 'API_KEY', ?, 't', 't')`, enc); err != nil {
		t.Fatalf("seed secret: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	ctx := context.Background()
	s, err := Open(path, key)
	if err != nil {
		t.Fatalf("Open migrated: %v", err)
	}
	got, err := s.Resolve(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["API_KEY"] != "legacy-value-123" {
		t.Fatalf("value = %q", got["API_KEY"])
	}
	m := s.Migration()
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Simulate the documented recovery: stop, replace the vault with the
	// snapshot, drop stale WAL sidecars, then reopen.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
	if err := os.Rename(m.Snapshot, path); err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}
	s2, err := Open(path, key)
	if err != nil {
		t.Fatalf("Open restored: %v", err)
	}
	defer s2.Close()
	got2, err := s2.Resolve(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("Resolve restored: %v", err)
	}
	if got2["API_KEY"] != "legacy-value-123" {
		t.Fatalf("restored value = %q", got2["API_KEY"])
	}
}
