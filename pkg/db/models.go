package db

import "time"

// Scope identifies where a secret is defined.
type Scope string

const (
	// ScopeGlobal marks a secret that applies to every environment.
	ScopeGlobal Scope = "global"
	// ScopeEnvironment marks a secret defined for a specific environment.
	ScopeEnvironment Scope = "environment"
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
	// Overrides is true when an environment secret shadows a global one.
	Overrides bool
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
	Command     string
	ExitCode    *int
	Redactions  int
}
