package db

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

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
