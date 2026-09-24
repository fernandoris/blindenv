package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fernandoris/blindenv/pkg/crypto"
	_ "modernc.org/sqlite" // register the pure-Go SQLite driver
)

// MinSecretLength is the shortest value the redaction engine will substitute.
// Shorter values are stored but flagged as unredactable.
const MinSecretLength = 6

var (
	// ErrProjectNotFound is returned when a project slug does not exist.
	ErrProjectNotFound = errors.New("db: project not found")
	// ErrEnvironmentNotFound is returned when an environment does not exist.
	ErrEnvironmentNotFound = errors.New("db: environment not found")
	// ErrSecretNotFound is returned when a secret does not exist.
	ErrSecretNotFound = errors.New("db: secret not found")
	// ErrDuplicate is returned when a unique constraint is violated.
	ErrDuplicate = errors.New("db: duplicate")
	// ErrInvalidName is returned for empty or reserved names.
	ErrInvalidName = errors.New("db: invalid name")
)

// Store is the encrypted secret repository backed by embedded SQLite.
type Store struct {
	db  *sql.DB
	key []byte
}

// Open opens (creating if needed) the vault at path and returns a Store that
// encrypts and decrypts values with the given master key.
func Open(path string, key []byte) (*Store, error) {
	if len(key) != crypto.KeySize {
		return nil, fmt.Errorf("db: master key must be %d bytes, got %d", crypto.KeySize, len(key))
	}
	dsn := "file:" + path +
		"?_pragma=busy_timeout(10000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(4)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db: ping %s: %w", path, err)
	}
	s := &Store{db: sqlDB, key: key}
	if err := s.migrate(context.Background()); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error { return s.db.Close() }

func nowString() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func parseTime(v string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, v)
	return t
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// --- Projects ---

// CreateProject creates a project with the given slug.
func (s *Store) CreateProject(ctx context.Context, slug string) (Project, error) {
	if strings.TrimSpace(slug) == "" || strings.HasPrefix(slug, "_") {
		return Project{}, ErrInvalidName
	}
	now := nowString()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO projects (slug, allow_execute, created_at) VALUES (?, 0, ?)`, slug, now)
	if isUniqueViolation(err) {
		return Project{}, fmt.Errorf("%w: project %q", ErrDuplicate, slug)
	}
	if err != nil {
		return Project{}, fmt.Errorf("db: create project: %w", err)
	}
	id, _ := res.LastInsertId()
	return Project{ID: id, Slug: slug, CreatedAt: parseTime(now)}, nil
}

// GetProject returns a project by slug.
func (s *Store) GetProject(ctx context.Context, slug string) (Project, error) {
	var (
		p       Project
		allow   int
		created string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, slug, allow_execute, created_at FROM projects WHERE slug = ?`, slug).
		Scan(&p.ID, &p.Slug, &allow, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrProjectNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("db: get project: %w", err)
	}
	p.AllowExecute = allow != 0
	p.CreatedAt = parseTime(created)
	return p, nil
}

// ListProjects returns all projects ordered by slug.
func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, slug, allow_execute, created_at FROM projects ORDER BY slug`)
	if err != nil {
		return nil, fmt.Errorf("db: list projects: %w", err)
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var (
			p       Project
			allow   int
			created string
		)
		if err := rows.Scan(&p.ID, &p.Slug, &allow, &created); err != nil {
			return nil, err
		}
		p.AllowExecute = allow != 0
		p.CreatedAt = parseTime(created)
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteProject removes a project and, by cascade, its environments, secrets
// and audit rows.
func (s *Store) DeleteProject(ctx context.Context, slug string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE slug = ?`, slug)
	if err != nil {
		return fmt.Errorf("db: delete project: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// SetAllowExecute enables or disables secret execution for a project.
func (s *Store) SetAllowExecute(ctx context.Context, slug string, allow bool) error {
	value := 0
	if allow {
		value = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE projects SET allow_execute = ? WHERE slug = ?`, value, slug)
	if err != nil {
		return fmt.Errorf("db: set allow_execute: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrProjectNotFound
	}
	return nil
}

func (s *Store) projectID(ctx context.Context, slug string) (int64, error) {
	p, err := s.GetProject(ctx, slug)
	if err != nil {
		return 0, err
	}
	return p.ID, nil
}

// --- Environments ---

// CreateEnvironment creates a named environment within a project.
func (s *Store) CreateEnvironment(ctx context.Context, slug, name string) (Environment, error) {
	if strings.TrimSpace(name) == "" || strings.HasPrefix(name, "_") {
		return Environment{}, ErrInvalidName
	}
	pid, err := s.projectID(ctx, slug)
	if err != nil {
		return Environment{}, err
	}
	now := nowString()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO environments (project_id, name, created_at) VALUES (?, ?, ?)`, pid, name, now)
	if isUniqueViolation(err) {
		return Environment{}, fmt.Errorf("%w: environment %q", ErrDuplicate, name)
	}
	if err != nil {
		return Environment{}, fmt.Errorf("db: create environment: %w", err)
	}
	id, _ := res.LastInsertId()
	return Environment{ID: id, ProjectID: pid, Name: name, CreatedAt: parseTime(now)}, nil
}

func (s *Store) environmentID(ctx context.Context, pid int64, name string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM environments WHERE project_id = ? AND name = ?`, pid, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrEnvironmentNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("db: get environment: %w", err)
	}
	return id, nil
}

// ListEnvironments returns the environments of a project ordered by name.
func (s *Store) ListEnvironments(ctx context.Context, slug string) ([]Environment, error) {
	pid, err := s.projectID(ctx, slug)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, created_at FROM environments WHERE project_id = ? ORDER BY name`, pid)
	if err != nil {
		return nil, fmt.Errorf("db: list environments: %w", err)
	}
	defer rows.Close()
	var out []Environment
	for rows.Next() {
		var (
			e       Environment
			created string
		)
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Name, &created); err != nil {
			return nil, err
		}
		e.CreatedAt = parseTime(created)
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteEnvironment removes an environment and its secrets.
func (s *Store) DeleteEnvironment(ctx context.Context, slug, name string) error {
	pid, err := s.projectID(ctx, slug)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM environments WHERE project_id = ? AND name = ?`, pid, name)
	if err != nil {
		return fmt.Errorf("db: delete environment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEnvironmentNotFound
	}
	return nil
}

// --- Secrets ---

// PutSecret stores a secret value. An empty environment means the project
// global scope. It returns true when the value is shorter than
// MinSecretLength and therefore cannot be reliably redacted.
func (s *Store) PutSecret(ctx context.Context, slug, environment, key, value string) (bool, error) {
	if strings.TrimSpace(key) == "" {
		return false, ErrInvalidName
	}
	pid, err := s.projectID(ctx, slug)
	if err != nil {
		return false, err
	}
	var envID *int64
	if environment != "" {
		id, err := s.environmentID(ctx, pid, environment)
		if err != nil {
			return false, err
		}
		envID = &id
	}
	enc, err := crypto.Encrypt(s.key, []byte(value))
	if err != nil {
		return false, err
	}
	now := nowString()

	var res sql.Result
	if envID == nil {
		res, err = s.db.ExecContext(ctx,
			`UPDATE secrets SET value_enc = ?, updated_at = ? WHERE project_id = ? AND environment_id IS NULL AND key = ?`,
			enc, now, pid, key)
	} else {
		res, err = s.db.ExecContext(ctx,
			`UPDATE secrets SET value_enc = ?, updated_at = ? WHERE project_id = ? AND environment_id = ? AND key = ?`,
			enc, now, pid, *envID, key)
	}
	if err != nil {
		return false, fmt.Errorf("db: update secret: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if envID == nil {
			_, err = s.db.ExecContext(ctx,
				`INSERT INTO secrets (project_id, environment_id, key, value_enc, created_at, updated_at) VALUES (?, NULL, ?, ?, ?, ?)`,
				pid, key, enc, now, now)
		} else {
			_, err = s.db.ExecContext(ctx,
				`INSERT INTO secrets (project_id, environment_id, key, value_enc, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
				pid, *envID, key, enc, now, now)
		}
		if err != nil && !isUniqueViolation(err) {
			return false, fmt.Errorf("db: insert secret: %w", err)
		}
	}
	return len(value) < MinSecretLength, nil
}

func (s *Store) keySet(ctx context.Context, pid int64, envID *int64) (map[string][]byte, error) {
	query := `SELECT key, value_enc FROM secrets WHERE project_id = ? AND environment_id IS NULL`
	args := []any{pid}
	if envID != nil {
		query = `SELECT key, value_enc FROM secrets WHERE project_id = ? AND (environment_id IS NULL OR environment_id = ?)`
		args = append(args, *envID)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("db: query secrets: %w", err)
	}
	defer rows.Close()
	out := make(map[string][]byte)
	for rows.Next() {
		var (
			key string
			enc []byte
		)
		if err := rows.Scan(&key, &enc); err != nil {
			return nil, err
		}
		out[key] = enc
	}
	return out, rows.Err()
}

func (s *Store) resolveEnv(ctx context.Context, slug, environment string) (int64, *int64, error) {
	pid, err := s.projectID(ctx, slug)
	if err != nil {
		return 0, nil, err
	}
	if environment == "" {
		return pid, nil, nil
	}
	id, err := s.environmentID(ctx, pid, environment)
	if err != nil {
		return 0, nil, err
	}
	return pid, &id, nil
}

// ListKeys returns the effective key names (union of globals and the
// environment) without duplicates, sorted.
func (s *Store) ListKeys(ctx context.Context, slug, environment string) ([]string, error) {
	pid, envID, err := s.resolveEnv(ctx, slug, environment)
	if err != nil {
		return nil, err
	}
	values, err := s.keySet(ctx, pid, envID)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

// ListSecrets returns metadata for the effective secrets, marking which
// environment entries override a global.
func (s *Store) ListSecrets(ctx context.Context, slug, environment string) ([]SecretInfo, error) {
	pid, envID, err := s.resolveEnv(ctx, slug, environment)
	if err != nil {
		return nil, err
	}
	globals, err := s.keySet(ctx, pid, nil)
	if err != nil {
		return nil, err
	}
	effective, err := s.keySet(ctx, pid, envID)
	if err != nil {
		return nil, err
	}
	var envKeys map[string][]byte
	if envID != nil {
		envKeys, err = s.envOnlyKeys(ctx, pid, *envID)
		if err != nil {
			return nil, err
		}
	}
	out := make([]SecretInfo, 0, len(effective))
	for key := range effective {
		info := SecretInfo{Key: key, Scope: ScopeGlobal}
		if envKeys != nil {
			if _, ok := envKeys[key]; ok {
				info.Scope = ScopeEnvironment
				info.Environment = environment
				_, info.Overrides = globals[key]
			}
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (s *Store) envOnlyKeys(ctx context.Context, pid, envID int64) (map[string][]byte, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value_enc FROM secrets WHERE project_id = ? AND environment_id = ?`, pid, envID)
	if err != nil {
		return nil, fmt.Errorf("db: query environment secrets: %w", err)
	}
	defer rows.Close()
	out := make(map[string][]byte)
	for rows.Next() {
		var (
			key string
			enc []byte
		)
		if err := rows.Scan(&key, &enc); err != nil {
			return nil, err
		}
		out[key] = enc
	}
	return out, rows.Err()
}

// Resolve returns the effective plaintext values for a project and
// environment, with environment values taking precedence over globals.
func (s *Store) Resolve(ctx context.Context, slug, environment string) (map[string]string, error) {
	pid, envID, err := s.resolveEnv(ctx, slug, environment)
	if err != nil {
		return nil, err
	}
	values, err := s.keySet(ctx, pid, envID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(values))
	for key, enc := range values {
		plain, err := crypto.Decrypt(s.key, enc)
		if err != nil {
			return nil, fmt.Errorf("db: decrypt %q: %w", key, err)
		}
		out[key] = string(plain)
	}
	return out, nil
}

// ResolveKey returns the effective plaintext value of a single key.
func (s *Store) ResolveKey(ctx context.Context, slug, environment, key string) (string, error) {
	all, err := s.Resolve(ctx, slug, environment)
	if err != nil {
		return "", err
	}
	value, ok := all[key]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrSecretNotFound, key)
	}
	return value, nil
}

// ScopeValues returns the plaintext values defined exclusively in the given
// scope: globals when environment is empty, otherwise only that environment's
// own secrets (not the fallback globals).
func (s *Store) ScopeValues(ctx context.Context, slug, environment string) (map[string]string, error) {
	pid, envID, err := s.resolveEnv(ctx, slug, environment)
	if err != nil {
		return nil, err
	}
	var enc map[string][]byte
	if envID == nil {
		enc, err = s.keySet(ctx, pid, nil)
	} else {
		enc, err = s.envOnlyKeys(ctx, pid, *envID)
	}
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(enc))
	for key, value := range enc {
		plain, err := crypto.Decrypt(s.key, value)
		if err != nil {
			return nil, fmt.Errorf("db: decrypt %q: %w", key, err)
		}
		out[key] = string(plain)
	}
	return out, nil
}

// DeleteSecret removes a secret from the given scope.
func (s *Store) DeleteSecret(ctx context.Context, slug, environment, key string) error {
	pid, envID, err := s.resolveEnv(ctx, slug, environment)
	if err != nil {
		return err
	}
	var res sql.Result
	if envID == nil {
		res, err = s.db.ExecContext(ctx,
			`DELETE FROM secrets WHERE project_id = ? AND environment_id IS NULL AND key = ?`, pid, key)
	} else {
		res, err = s.db.ExecContext(ctx,
			`DELETE FROM secrets WHERE project_id = ? AND environment_id = ? AND key = ?`, pid, *envID, key)
	}
	if err != nil {
		return fmt.Errorf("db: delete secret: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSecretNotFound
	}
	return nil
}

// --- Audit ---

// AppendAudit records a tool invocation. Values are never stored.
func (s *Store) AppendAudit(ctx context.Context, e AuditEntry) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (ts, client, project, environment, tool, key_names, command, exit_code, redactions)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Timestamp.UTC().Format(time.RFC3339Nano), e.Client, e.Project, e.Environment,
		e.Tool, strings.Join(e.KeyNames, ","), e.Command, e.ExitCode, e.Redactions)
	if err != nil {
		return fmt.Errorf("db: append audit: %w", err)
	}
	return nil
}

// ListAudit returns the most recent audit entries, newest first.
func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ts, client, project, environment, tool, key_names, command, exit_code, redactions
		 FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("db: list audit: %w", err)
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var (
			e    AuditEntry
			ts   string
			keys string
			exit sql.NullInt64
		)
		if err := rows.Scan(&e.ID, &ts, &e.Client, &e.Project, &e.Environment, &e.Tool,
			&keys, &e.Command, &exit, &e.Redactions); err != nil {
			return nil, err
		}
		e.Timestamp = parseTime(ts)
		if keys != "" {
			e.KeyNames = strings.Split(keys, ",")
		}
		if exit.Valid {
			v := int(exit.Int64)
			e.ExitCode = &v
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
