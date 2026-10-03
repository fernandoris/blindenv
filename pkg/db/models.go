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

// SecretKind classifies a secret value's format and intended use. The empty
// string is normalized to KindText.
type SecretKind string

const (
	// KindText is an unclassified value.
	KindText SecretKind = "text"
	// KindURL is a URL, often already carrying a path.
	KindURL SecretKind = "url"
	// KindHost is a host name or host:port.
	KindHost SecretKind = "host"
	// KindConnString is a connection string or DSN.
	KindConnString SecretKind = "connection-string"
	// KindToken is a bearer token or API key; always sensitive.
	KindToken SecretKind = "token"
	// KindPassword is a password; always sensitive.
	KindPassword SecretKind = "password"
)

// MaxHintLength bounds the optional usage hint.
const MaxHintLength = 200

// SecretMeta is the optional metadata attached to a secret definition. A nil
// Sensitive means the value is sensitive (the default).
type SecretMeta struct {
	Kind      SecretKind
	Hint      string
	Sensitive *bool
}

// IsSensitive reports the effective sensitivity, defaulting to true.
func (m SecretMeta) IsSensitive() bool { return m.Sensitive == nil || *m.Sensitive }

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
	Kind        SecretKind
	Hint        string
	Sensitive   bool
	// Overrides lists the broader scopes this definition shadows, most
	// specific first.
	Overrides []Scope
}

// ResolvedSecret is the effective definition of a key for a project and
// environment, including the effective plaintext. Callers must honour
// Sensitive before exposing the value to an agent or a listing.
type ResolvedSecret struct {
	Key         string
	Value       string
	Sensitive   bool
	Kind        SecretKind
	Hint        string
	Scope       Scope
	Environment string
	Overrides   []Scope
}

// SecretEntry is a value together with the metadata defined in exactly one
// scope. It is used for backup snapshotting.
type SecretEntry struct {
	Key       string
	Value     string
	Sensitive bool
	Kind      SecretKind
	Hint      string
}

// ScopedKeys groups the key definitions in each scope applicable to a project.
// It never contains values.
type ScopedKeys struct {
	// Global is the shared global scope.
	Global []SecretInfo
	// Environments maps an environment name to the keys defined in the
	// environment-global scope for that name.
	Environments map[string][]SecretInfo
	// Project is the project-global scope.
	Project []SecretInfo
	// ProjectEnvironments maps an environment name to the keys defined in the
	// project + environment scope for that name.
	ProjectEnvironments map[string][]SecretInfo
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
