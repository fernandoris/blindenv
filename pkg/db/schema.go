package db

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 2

const baseSchemaSQL = `
CREATE TABLE IF NOT EXISTS schema_version (
	version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	slug          TEXT    NOT NULL UNIQUE,
	allow_execute INTEGER NOT NULL DEFAULT 0,
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
	command     TEXT    NOT NULL DEFAULT '',
	exit_code   INTEGER,
	redactions  INTEGER NOT NULL DEFAULT 0
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

	hasKeyScopes, err := s.columnExists(ctx, "audit_log", "key_scopes")
	if err != nil {
		return err
	}
	if !hasKeyScopes {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE audit_log ADD COLUMN key_scopes TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("db: add audit key_scopes: %w", err)
		}
	}

	return s.recordSchemaVersion(ctx)
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
	if _, err := s.db.ExecContext(ctx, `UPDATE schema_version SET version = ?`, schemaVersion); err != nil {
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
