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

// DeleteEnvironment removes an environment and its project secrets.
func (s *Store) DeleteEnvironment(ctx context.Context, slug, name string) error {
	pid, err := s.projectID(ctx, slug)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: delete environment: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM secrets WHERE project_id = ? AND environment = ?`, pid, name); err != nil {
		return fmt.Errorf("db: delete environment secrets: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM environments WHERE project_id = ? AND name = ?`, pid, name)
	if err != nil {
		return fmt.Errorf("db: delete environment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEnvironmentNotFound
	}
	return tx.Commit()
}

// ListSharedEnvironments returns the derived set of shared environment names:
// every project environment name plus any name that already carries an
// environment-global secret.
func (s *Store) ListSharedEnvironments(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name FROM environments
		 UNION
		 SELECT DISTINCT environment FROM secrets WHERE project_id IS NULL AND environment IS NOT NULL
		 ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("db: list shared environments: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// --- Secrets ---

// scopeWhere builds the predicate that selects definitions in exactly one
// scope. A nil project means shared; an empty environment means all
// environments.
func (s *Store) scopeWhere(pid *int64, environment string) (string, []any) {
	switch {
	case pid == nil && environment == "":
		return `project_id IS NULL AND environment IS NULL`, nil
	case pid == nil:
		return `project_id IS NULL AND environment = ?`, []any{environment}
	case environment == "":
		return `project_id = ? AND environment IS NULL`, []any{*pid}
	default:
		return `project_id = ? AND environment = ?`, []any{*pid, environment}
	}
}

// resolveScope maps a (project, environment) selector to storage keys. An
// empty project selects the shared scopes and is not validated against the
// projects table; a non-empty project with a non-empty environment must exist.
func (s *Store) resolveScope(ctx context.Context, project, environment string) (*int64, string, error) {
	if project == "" {
		return nil, environment, nil
	}
	id, err := s.projectID(ctx, project)
	if err != nil {
		return nil, "", err
	}
	if environment != "" {
		if _, err := s.environmentID(ctx, id, environment); err != nil {
			return nil, "", err
		}
	}
	return &id, environment, nil
}

func scopeOf(isProject, hasEnvironment bool) Scope {
	switch {
	case isProject && hasEnvironment:
		return ScopeProjectEnvironment
	case isProject:
		return ScopeProject
	case hasEnvironment:
		return ScopeSharedEnvironment
	default:
		return ScopeGlobal
	}
}

func priorityOf(isProject, hasEnvironment bool) int {
	switch {
	case isProject && hasEnvironment:
		return 4
	case isProject:
		return 3
	case hasEnvironment:
		return 2
	default:
		return 1
	}
}

type resolvedSecret struct {
	key      string
	scope    Scope
	env      string
	enc      []byte
	shadowed []Scope
}

type candidate struct {
	scope Scope
	env   string
	enc   []byte
	prio  int
}

// effectiveSecrets resolves the winner for every key applicable to a project
// and environment. A nil project means the shared scopes.
func (s *Store) effectiveSecrets(ctx context.Context, pid *int64, environment string) (map[string]resolvedSecret, error) {
	var pidArg any
	if pid != nil {
		pidArg = *pid
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value_enc, project_id, environment FROM secrets
		 WHERE (project_id = ? OR project_id IS NULL)
		   AND (environment = ? OR environment IS NULL)`,
		pidArg, environment)
	if err != nil {
		return nil, fmt.Errorf("db: query effective secrets: %w", err)
	}
	defer rows.Close()

	byKey := make(map[string][]candidate)
	for rows.Next() {
		var (
			key  string
			enc  []byte
			proj sql.NullInt64
			env  sql.NullString
		)
		if err := rows.Scan(&key, &enc, &proj, &env); err != nil {
			return nil, err
		}
		byKey[key] = append(byKey[key], candidate{
			scope: scopeOf(proj.Valid, env.Valid),
			env:   env.String,
			enc:   enc,
			prio:  priorityOf(proj.Valid, env.Valid),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make(map[string]resolvedSecret, len(byKey))
	for key, cands := range byKey {
		winner := 0
		for i := range cands {
			if cands[i].prio > cands[winner].prio {
				winner = i
			}
		}
		others := make([]candidate, 0, len(cands)-1)
		for i, c := range cands {
			if i != winner {
				others = append(others, c)
			}
		}
		sort.Slice(others, func(i, j int) bool { return others[i].prio > others[j].prio })
		shadowed := make([]Scope, 0, len(others))
		for _, c := range others {
			shadowed = append(shadowed, c.scope)
		}
		out[key] = resolvedSecret{
			key:      key,
			scope:    cands[winner].scope,
			env:      cands[winner].env,
			enc:      cands[winner].enc,
			shadowed: shadowed,
		}
	}
	return out, nil
}

// PutSecret stores a secret value. An empty project selects the shared scopes
// and an empty environment the all-environments scope, so the four
// combinations address the four scopes. It returns true when the value is
// shorter than MinSecretLength and therefore cannot be reliably redacted.
func (s *Store) PutSecret(ctx context.Context, project, environment, key, value string) (bool, error) {
	if strings.TrimSpace(key) == "" {
		return false, ErrInvalidName
	}
	pid, env, err := s.resolveScope(ctx, project, environment)
	if err != nil {
		return false, err
	}
	enc, err := crypto.Encrypt(s.key, []byte(value))
	if err != nil {
		return false, err
	}
	now := nowString()
	where, wargs := s.scopeWhere(pid, env)

	updateArgs := append([]any{enc, now}, wargs...)
	updateArgs = append(updateArgs, key)
	res, err := s.db.ExecContext(ctx,
		`UPDATE secrets SET value_enc = ?, updated_at = ? WHERE `+where+` AND key = ?`, updateArgs...)
	if err != nil {
		return false, fmt.Errorf("db: update secret: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var pidArg, envArg any
		if pid != nil {
			pidArg = *pid
		}
		if env != "" {
			envArg = env
		}
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO secrets (project_id, environment, key, value_enc, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			pidArg, envArg, key, enc, now, now)
		if err != nil {
			if isUniqueViolation(err) {
				if _, uerr := s.db.ExecContext(ctx,
					`UPDATE secrets SET value_enc = ?, updated_at = ? WHERE `+where+` AND key = ?`, updateArgs...); uerr != nil {
					return false, fmt.Errorf("db: update secret after conflict: %w", uerr)
				}
			} else {
				return false, fmt.Errorf("db: insert secret: %w", err)
			}
		}
	}
	return len(value) < MinSecretLength, nil
}

// Resolve returns the effective plaintext values for a project and
// environment, applying the four-tier precedence.
func (s *Store) Resolve(ctx context.Context, project, environment string) (map[string]string, error) {
	pid, _, err := s.resolveScope(ctx, project, environment)
	if err != nil {
		return nil, err
	}
	effective, err := s.effectiveSecrets(ctx, pid, environment)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(effective))
	for key, r := range effective {
		plain, err := crypto.Decrypt(s.key, r.enc)
		if err != nil {
			return nil, fmt.Errorf("db: decrypt %q: %w", key, err)
		}
		out[key] = string(plain)
	}
	return out, nil
}

// ResolveKey returns the effective plaintext value of a single key.
func (s *Store) ResolveKey(ctx context.Context, project, environment, key string) (string, error) {
	all, err := s.Resolve(ctx, project, environment)
	if err != nil {
		return "", err
	}
	value, ok := all[key]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrSecretNotFound, key)
	}
	return value, nil
}

// ListKeys returns the effective key names without duplicates, sorted.
func (s *Store) ListKeys(ctx context.Context, project, environment string) ([]string, error) {
	pid, _, err := s.resolveScope(ctx, project, environment)
	if err != nil {
		return nil, err
	}
	effective, err := s.effectiveSecrets(ctx, pid, environment)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(effective))
	for k := range effective {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

// ListSecrets returns metadata for the effective secrets with their winning
// scope and the broader scopes they shadow.
func (s *Store) ListSecrets(ctx context.Context, project, environment string) ([]SecretInfo, error) {
	pid, _, err := s.resolveScope(ctx, project, environment)
	if err != nil {
		return nil, err
	}
	effective, err := s.effectiveSecrets(ctx, pid, environment)
	if err != nil {
		return nil, err
	}
	out := make([]SecretInfo, 0, len(effective))
	for key, r := range effective {
		out = append(out, SecretInfo{Key: key, Scope: r.scope, Environment: r.env, Overrides: r.shadowed})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// EffectiveScopes returns the winning scope for every effective key of a
// project and environment, without values.
func (s *Store) EffectiveScopes(ctx context.Context, project, environment string) (map[string]Scope, error) {
	pid, _, err := s.resolveScope(ctx, project, environment)
	if err != nil {
		return nil, err
	}
	effective, err := s.effectiveSecrets(ctx, pid, environment)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Scope, len(effective))
	for key, r := range effective {
		out[key] = r.scope
	}
	return out, nil
}

func (s *Store) decryptScope(ctx context.Context, pid *int64, environment string) (map[string]string, error) {
	where, args := s.scopeWhere(pid, environment)
	rows, err := s.db.QueryContext(ctx, `SELECT key, value_enc FROM secrets WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("db: query secrets: %w", err)
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var (
			key string
			enc []byte
		)
		if err := rows.Scan(&key, &enc); err != nil {
			return nil, err
		}
		plain, err := crypto.Decrypt(s.key, enc)
		if err != nil {
			return nil, fmt.Errorf("db: decrypt %q: %w", key, err)
		}
		out[key] = string(plain)
	}
	return out, rows.Err()
}

// ScopeValues returns the plaintext values defined exclusively in the given
// scope: shared global when project and environment are empty, shared
// environment-global when only project is empty, project-global when only
// environment is empty, and the project + environment scope otherwise.
func (s *Store) ScopeValues(ctx context.Context, project, environment string) (map[string]string, error) {
	pid, env, err := s.resolveScope(ctx, project, environment)
	if err != nil {
		return nil, err
	}
	return s.decryptScope(ctx, pid, env)
}

type broaderDef struct {
	pid   *int64
	env   string
	scope Scope
}

func (s *Store) broaderDefs(ctx context.Context, project string, pid *int64, environment string) ([]broaderDef, error) {
	switch {
	case project == "" && environment == "":
		return nil, nil
	case project == "":
		return []broaderDef{{nil, "", ScopeGlobal}}, nil
	case environment == "":
		envs, err := s.ListEnvironments(ctx, project)
		if err != nil {
			return nil, err
		}
		defs := make([]broaderDef, 0, len(envs)+1)
		for _, e := range envs {
			defs = append(defs, broaderDef{nil, e.Name, ScopeSharedEnvironment})
		}
		defs = append(defs, broaderDef{nil, "", ScopeGlobal})
		return defs, nil
	default:
		return []broaderDef{
			{pid, "", ScopeProject},
			{nil, environment, ScopeSharedEnvironment},
			{nil, "", ScopeGlobal},
		}, nil
	}
}

func (s *Store) scopeDefines(ctx context.Context, pid *int64, environment, key string) (bool, error) {
	where, args := s.scopeWhere(pid, environment)
	args = append(args, key)
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM secrets WHERE `+where+` AND key = ? LIMIT 1`, args...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("db: scope defines: %w", err)
	}
	return true, nil
}

// ScopeSecrets returns the secrets defined exclusively in the given scope,
// each annotated with the broader scopes it shadows.
func (s *Store) ScopeSecrets(ctx context.Context, project, environment string) ([]SecretInfo, error) {
	pid, env, err := s.resolveScope(ctx, project, environment)
	if err != nil {
		return nil, err
	}
	where, args := s.scopeWhere(pid, env)
	rows, err := s.db.QueryContext(ctx, `SELECT key FROM secrets WHERE `+where+` ORDER BY key`, args...)
	if err != nil {
		return nil, fmt.Errorf("db: query scope secrets: %w", err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	defs, err := s.broaderDefs(ctx, project, pid, environment)
	if err != nil {
		return nil, err
	}
	out := make([]SecretInfo, 0, len(keys))
	for _, key := range keys {
		var overrides []Scope
		seen := make(map[Scope]bool)
		for _, d := range defs {
			if seen[d.scope] {
				continue
			}
			ok, err := s.scopeDefines(ctx, d.pid, d.env, key)
			if err != nil {
				return nil, err
			}
			if ok {
				overrides = append(overrides, d.scope)
				seen[d.scope] = true
			}
		}
		out = append(out, SecretInfo{Key: key, Scope: scopeOf(pid != nil, env != ""), Environment: env, Overrides: overrides})
	}
	return out, nil
}

// DeleteSecret removes a secret from the given scope.
func (s *Store) DeleteSecret(ctx context.Context, project, environment, key string) error {
	pid, env, err := s.resolveScope(ctx, project, environment)
	if err != nil {
		return err
	}
	where, args := s.scopeWhere(pid, env)
	args = append(args, key)
	res, err := s.db.ExecContext(ctx, `DELETE FROM secrets WHERE `+where+` AND key = ?`, args...)
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
	scopes := make([]string, 0, len(e.KeyScopes))
	for _, sc := range e.KeyScopes {
		scopes = append(scopes, string(sc))
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (ts, client, project, environment, tool, key_names, key_scopes, command, exit_code, redactions)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Timestamp.UTC().Format(time.RFC3339Nano), e.Client, e.Project, e.Environment,
		e.Tool, strings.Join(e.KeyNames, ","), strings.Join(scopes, ","), e.Command, e.ExitCode, e.Redactions)
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
		`SELECT id, ts, client, project, environment, tool, key_names, key_scopes, command, exit_code, redactions
		 FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("db: list audit: %w", err)
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var (
			e      AuditEntry
			ts     string
			keys   string
			scopes string
			exit   sql.NullInt64
		)
		if err := rows.Scan(&e.ID, &ts, &e.Client, &e.Project, &e.Environment, &e.Tool,
			&keys, &scopes, &e.Command, &exit, &e.Redactions); err != nil {
			return nil, err
		}
		e.Timestamp = parseTime(ts)
		if keys != "" {
			e.KeyNames = strings.Split(keys, ",")
		}
		if scopes != "" {
			for _, sc := range strings.Split(scopes, ",") {
				e.KeyScopes = append(e.KeyScopes, Scope(sc))
			}
		}
		if exit.Valid {
			v := int(exit.Int64)
			e.ExitCode = &v
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
