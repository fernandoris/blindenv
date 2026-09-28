package db

import "time"

// Scope identifies where a secret is defined, from most to least specific:
// project + environment, project-global, environment-global (shared across
// projects) and global (shared across projects and environments).
type Scope string

const (
	// ScopeGlobal marks a secret that applies to every project and environment.
	ScopeGlobal Scope = "global"
	// ScopeSharedEnvironment marks a secret shared by every project that uses
	// the named environment.
	ScopeSharedEnvironment Scope = "environment"
	// ScopeProject marks a secret that applies to every environment of one
	// project.
	ScopeProject Scope = "project"
	// ScopeProjectEnvironment marks a secret defined for one environment of one
	// project.
	ScopeProjectEnvironment Scope = "project_environment"
)

// Project is a named container of environments and secrets.
type Project struct {
	ID           int64
	Slug         string
	AllowExecute bool
	CreatedAt    time.Time
}

// Environment is a named scope within a project.
type Environment struct {
	ID        int64
	ProjectID int64
	Name      string
	CreatedAt time.Time
}

// SecretInfo describes a secret without exposing its value.
type SecretInfo struct {
	Key         string
	Scope       Scope
	Environment string
	// Overrides lists the broader scopes this definition shadows, most
	// specific first.
	Overrides []Scope
}

// ScopedKeys groups the key names defined in each scope applicable to a
// project. It never contains values.
type ScopedKeys struct {
	// Global is the shared global scope.
	Global []string
	// Environments maps an environment name to the keys defined in the
	// environment-global scope for that name.
	Environments map[string][]string
	// Project is the project-global scope.
	Project []string
	// ProjectEnvironments maps an environment name to the keys defined in the
	// project + environment scope for that name.
	ProjectEnvironments map[string][]string
}

// AuditEntry records a single MCP tool invocation. It never contains secret
// values.
type AuditEntry struct {
	ID          int64
	Timestamp   time.Time
	Client      string
	Project     string
	Environment string
	Tool        string
	KeyNames    []string
	// KeyScopes records the scope each key in KeyNames resolved from, in the
	// same order.
	KeyScopes  []Scope
	Command    string
	ExitCode   *int
	Redactions int
}
