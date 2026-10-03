package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
)

const schemaVersion = 5

// MigrationInfo describes the schema migration performed when a vault was
// opened. A zero From and To mean no migration was needed.
type MigrationInfo struct {
	// From is the schema version the vault had before opening.
	From int
	// To is the schema version after opening.
	To int
	// Snapshot is the path of the pre-migration copy, empty when none was made.
	Snapshot string
}

const baseSchemaSQL = `
CREATE TABLE IF NOT EXISTS schema_version (
	version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	slug          TEXT    NOT NULL UNIQUE,
	allow_execute INTEGER NOT NULL DEFAULT 0,
	allow_open    INTEGER NOT NULL DEFAULT 0,
	created_at    TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS environments (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	name       TEXT    NOT NULL,
	created_at TEXT    NOT NULL,
	UNIQUE (project_id, name)
);

CREATE TABLE IF NOT EXISTS audit_log (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	ts          TEXT    NOT NULL,
	client      TEXT    NOT NULL DEFAULT '',
	project     TEXT    NOT NULL DEFAULT '',
	environment TEXT    NOT NULL DEFAULT '',
	tool        TEXT    NOT NULL,
	key_names   TEXT    NOT NULL DEFAULT '',
	key_scopes  TEXT    NOT NULL DEFAULT '',
	command       TEXT    NOT NULL DEFAULT '',
	exit_code     INTEGER,
	redactions    INTEGER NOT NULL DEFAULT 0,
	substitutions INTEGER NOT NULL DEFAULT 0
);
`

// secretsSchemaSQL defines the scope-addressable secret table. A NULL
// project_id means the secret is shared across projects; a NULL environment
// means it applies to every environment. The four combinations are the four
// scopes the resolver understands.
const secretsSchemaSQL = `
CREATE TABLE IF NOT EXISTS secrets (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id  INTEGER REFERENCES projects(id) ON DELETE CASCADE,
	environment TEXT,
	key         TEXT    NOT NULL,
	value_enc   BLOB    NOT NULL,
	value_plain TEXT,
	sensitive   INTEGER NOT NULL DEFAULT 1,
	kind        TEXT    NOT NULL DEFAULT '',
	hint        TEXT    NOT NULL DEFAULT '',
	created_at  TEXT    NOT NULL,
	updated_at  TEXT    NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS secrets_global_idx
	ON secrets (key) WHERE project_id IS NULL AND environment IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS secrets_env_global_idx
	ON secrets (environment, key) WHERE project_id IS NULL AND environment IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS secrets_project_global_idx
	ON secrets (project_id, key) WHERE project_id IS NOT NULL AND environment IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS secrets_project_env_idx
	ON secrets (project_id, environment, key) WHERE project_id IS NOT NULL AND environment IS NOT NULL;
`

func (s *Store) migrate(ctx context.Context) error {
	// Read the persisted version before any write, so a vault produced by a
	// newer build is refused without being touched.
	existed, err := s.tableExists(ctx, "schema_version")
	if err != nil {
		return err
	}
	stored := 0
	if existed {
		stored, err = s.readSchemaVersion(ctx)
		if err != nil {
			return err
		}
		if stored > schemaVersion {
			return fmt.Errorf("%w: vault is v%d, this build supports v%d; upgrade blindenv. The vault was not modified",
				ErrSchemaTooNew, stored, schemaVersion)
		}
	}

	// Snapshot a vault that is about to change, before any DDL runs, so the
	// upgrade can be reversed by restoring the copy.
	upgrading := existed && stored < schemaVersion
	snapshot := ""
	if upgrading {
		snapshot, err = s.snapshotVault(ctx, stored)
		if err != nil {
			return err
		}
	}

	if _, err := s.db.ExecContext(ctx, baseSchemaSQL); err != nil {
		return fmt.Errorf("db: apply base schema: %w", err)
	}

	hasSecrets, err := s.tableExists(ctx, "secrets")
	if err != nil {
		return err
	}
	if hasSecrets {
		legacy, err := s.columnExists(ctx, "secrets", "environment_id")
		if err != nil {
			return err
		}
		if legacy {
			if err := s.migrateSecretsV1(ctx); err != nil {
				return err
			}
		}
	}

	if _, err := s.db.ExecContext(ctx, secretsSchemaSQL); err != nil {
		return fmt.Errorf("db: apply secrets schema: %w", err)
	}

	if err := s.migrateSecretsMetadata(ctx); err != nil {
		return err
	}

	if err := s.migrateAllowOpen(ctx); err != nil {
		return err
	}

	hasKeyScopes, err := s.columnExists(ctx, "audit_log", "key_scopes")
	if err != nil {
		return err
	}
	if !hasKeyScopes {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE audit_log ADD COLUMN key_scopes TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("db: add audit key_scopes: %w", err)
		}
	}

	if err := s.migrateAuditSubstitutions(ctx); err != nil {
		return err
	}

	if err := s.recordSchemaVersion(ctx); err != nil {
		return err
	}
	if upgrading {
		s.migration = MigrationInfo{From: stored, To: schemaVersion, Snapshot: snapshot}
	}
	return nil
}

// readSchemaVersion returns the persisted schema version, or zero when the
// table exists but holds no row.
func (s *Store) readSchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx, `SELECT version FROM schema_version LIMIT 1`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("db: read schema version: %w", err)
	}
	return v, nil
}

// snapshotVault writes a consistent, checkpointed copy of the vault beside it
// so an upgrade can be reversed. It never overwrites an existing snapshot for
// the same source version. The copy is produced by SQLite, so the vault is
// never loaded into process memory.
func (s *Store) snapshotVault(ctx context.Context, from int) (string, error) {
	if s.path == "" {
		return "", nil
	}
	dest := fmt.Sprintf("%s.v%d.bak", s.path, from)
	if info, err := os.Stat(dest); err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("db: snapshot path %s is a directory", dest)
		}
		return dest, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("db: stat snapshot: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, dest); err != nil {
		return "", fmt.Errorf("db: snapshot vault: %w", err)
	}
	if err := os.Chmod(dest, 0o600); err != nil {
		return "", fmt.Errorf("db: secure snapshot: %w", err)
	}
	return dest, nil
}

// migrateAuditSubstitutions adds the schema-v5 request-side substitution count
// to an existing audit_log. It is additive; existing rows default to zero.
func (s *Store) migrateAuditSubstitutions(ctx context.Context) error {
	hasSubstitutions, err := s.columnExists(ctx, "audit_log", "substitutions")
	if err != nil {
		return err
	}
	if hasSubstitutions {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE audit_log ADD COLUMN substitutions INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("db: add audit substitutions: %w", err)
	}
	return nil
}

// migrateSecretsMetadata adds the schema-v3 per-definition metadata columns to
// an existing secrets table. It is additive and preserves every definition;
// existing rows become sensitive with an empty type and hint. value_enc stays
// NOT NULL, so a non-sensitive row stores an empty blob there and its cleartext
// in value_plain.
func (s *Store) migrateSecretsMetadata(ctx context.Context) error {
	hasSensitive, err := s.columnExists(ctx, "secrets", "sensitive")
	if err != nil {
		return err
	}
	if hasSensitive {
		return nil
	}
	steps := []string{
		`ALTER TABLE secrets ADD COLUMN sensitive INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE secrets ADD COLUMN kind TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE secrets ADD COLUMN hint TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE secrets ADD COLUMN value_plain TEXT`,
	}
	for _, q := range steps {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("db: add secret metadata: %w", err)
		}
	}
	return nil
}

// migrateAllowOpen adds the schema-v4 per-project browser-open capability to an
// existing projects table. It is additive; existing projects default to
// disabled, so opening a URL requires an explicit opt-in after upgrade.
func (s *Store) migrateAllowOpen(ctx context.Context) error {
	hasAllowOpen, err := s.columnExists(ctx, "projects", "allow_open")
	if err != nil {
		return err
	}
	if hasAllowOpen {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`ALTER TABLE projects ADD COLUMN allow_open INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("db: add allow_open: %w", err)
	}
	return nil
}

// migrateSecretsV1 rebuilds the pre-shared-scopes secrets table (which keyed
// environments by foreign key) into the scope-addressable shape, preserving
// every definition by resolving environment ids to names.
func (s *Store) migrateSecretsV1(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin secrets migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	steps := []string{
		`CREATE TABLE secrets_v2 (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id  INTEGER REFERENCES projects(id) ON DELETE CASCADE,
			environment TEXT,
			key         TEXT    NOT NULL,
			value_enc   BLOB    NOT NULL,
			created_at  TEXT    NOT NULL,
			updated_at  TEXT    NOT NULL
		)`,
		`INSERT INTO secrets_v2 (id, project_id, environment, key, value_enc, created_at, updated_at)
			SELECT s.id, s.project_id, e.name, s.key, s.value_enc, s.created_at, s.updated_at
			FROM secrets s LEFT JOIN environments e ON e.id = s.environment_id`,
		`DROP TABLE secrets`,
		`ALTER TABLE secrets_v2 RENAME TO secrets`,
	}
	for _, q := range steps {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("db: migrate secrets: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: commit secrets migration: %w", err)
	}
	return nil
}

func (s *Store) recordSchemaVersion(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_version`).Scan(&count); err != nil {
		return fmt.Errorf("db: read schema_version: %w", err)
	}
	if count == 0 {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES (?)`, schemaVersion); err != nil {
			return fmt.Errorf("db: init schema_version: %w", err)
		}
		return nil
	}
	// Advance only: never lower a version already recorded.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE schema_version SET version = ? WHERE version < ?`, schemaVersion, schemaVersion); err != nil {
		return fmt.Errorf("db: update schema_version: %w", err)
	}
	return nil
}

func (s *Store) tableExists(ctx context.Context, name string) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n); err != nil {
		return false, fmt.Errorf("db: table exists: %w", err)
	}
	return n > 0, nil
}

func (s *Store) columnExists(ctx context.Context, table, column string) (bool, error) {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, fmt.Errorf("db: table info: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("db: table info scan: %w", err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
