package db

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 1

const schemaSQL = `
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

CREATE TABLE IF NOT EXISTS secrets (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id     INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	environment_id INTEGER REFERENCES environments(id) ON DELETE CASCADE,
	key            TEXT    NOT NULL,
	value_enc      BLOB    NOT NULL,
	created_at     TEXT    NOT NULL,
	updated_at     TEXT    NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS secrets_global_idx
	ON secrets (project_id, key) WHERE environment_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS secrets_env_idx
	ON secrets (project_id, environment_id, key) WHERE environment_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS audit_log (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	ts          TEXT    NOT NULL,
	client      TEXT    NOT NULL DEFAULT '',
	project     TEXT    NOT NULL DEFAULT '',
	environment TEXT    NOT NULL DEFAULT '',
	tool        TEXT    NOT NULL,
	key_names   TEXT    NOT NULL DEFAULT '',
	command     TEXT    NOT NULL DEFAULT '',
	exit_code   INTEGER,
	redactions  INTEGER NOT NULL DEFAULT 0
);
`

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("db: apply schema: %w", err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_version`).Scan(&count); err != nil {
		return fmt.Errorf("db: read schema_version: %w", err)
	}
	if count == 0 {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES (?)`, schemaVersion); err != nil {
			return fmt.Errorf("db: init schema_version: %w", err)
		}
	}
	return nil
}

var _ = sql.ErrNoRows
