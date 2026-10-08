package db

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/fernandoris/blindenv/pkg/crypto"
)

func openKeyedStore(t *testing.T, path string, key []byte) *Store {
	t.Helper()
	s, err := Open(path, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func seedSensitive(t *testing.T, s *Store, project, env string, n int) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if env != "" {
		if _, err := s.CreateEnvironment(ctx, project, env); err != nil {
			t.Fatalf("CreateEnvironment: %v", err)
		}
	}
	for i := 0; i < n; i++ {
		if _, err := s.PutSecret(ctx, project, env, fmt.Sprintf("KEY_%02d", i), fmt.Sprintf("value-%02d-abcdef", i)); err != nil {
			t.Fatalf("PutSecret: %v", err)
		}
	}
}

func wrongKey() []byte {
	k := make([]byte, crypto.KeySize)
	k[0] = 0x42
	return k
}

func TestResolveDetailedWrongKeyIsMismatch(t *testing.T) {
	ctx := context.Background()
	key := make([]byte, crypto.KeySize)
	path := filepath.Join(t.TempDir(), "vault.db")
	s := openKeyedStore(t, path, key)
	seedSensitive(t, s, "my-api", "staging", 2)
	_ = s.Close()

	s2 := openKeyedStore(t, path, wrongKey())
	if _, err := s2.ResolveDetailed(ctx, "my-api", "staging"); !errors.Is(err, ErrMasterKeyMismatch) {
		t.Fatalf("err = %v, want ErrMasterKeyMismatch", err)
	}
}

func TestResolveDetailedPartialFailureIsPerDefinition(t *testing.T) {
	ctx := context.Background()
	key := make([]byte, crypto.KeySize)
	path := filepath.Join(t.TempDir(), "vault.db")
	s := openKeyedStore(t, path, key)
	seedSensitive(t, s, "my-api", "staging", 2)
	// A long but invalid blob fails authentication without being structural.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE secrets SET value_enc = ? WHERE key = 'KEY_01'`,
		[]byte("this-is-not-a-valid-gcm-ciphertext-000")); err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	_, err := s.ResolveDetailed(ctx, "my-api", "staging")
	if err == nil {
		t.Fatal("want an error")
	}
	if errors.Is(err, ErrMasterKeyMismatch) {
		t.Fatalf("err = %v, want a per-definition error, not a key mismatch", err)
	}
	if !errors.Is(err, crypto.ErrIntegrity) {
		t.Fatalf("err = %v, want a crypto integrity error", err)
	}
}

func TestResolveDetailedInconsistentDefinition(t *testing.T) {
	ctx := context.Background()
	key := make([]byte, crypto.KeySize)
	path := filepath.Join(t.TempDir(), "vault.db")
	s := openKeyedStore(t, path, key)
	seedSensitive(t, s, "my-api", "staging", 1)
	if _, err := s.db.ExecContext(ctx, `UPDATE secrets SET value_enc = ? WHERE key = 'KEY_00'`, []byte{}); err != nil {
		t.Fatalf("blank: %v", err)
	}

	_, err := s.ResolveDetailed(ctx, "my-api", "staging")
	if !errors.Is(err, ErrInconsistentDefinition) {
		t.Fatalf("err = %v, want ErrInconsistentDefinition", err)
	}
	if errors.Is(err, ErrMasterKeyMismatch) {
		t.Fatalf("err = %v, must not be a key mismatch", err)
	}
}

func TestResolveDetailedConfigOnlyWithWrongKey(t *testing.T) {
	ctx := context.Background()
	key := make([]byte, crypto.KeySize)
	path := filepath.Join(t.TempDir(), "vault.db")
	s := openKeyedStore(t, path, key)
	if _, err := s.CreateProject(ctx, "my-api"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := s.CreateEnvironment(ctx, "my-api", "staging"); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if _, err := s.PutSecretMeta(ctx, "my-api", "staging", "ENDPOINT", "https://a.example/v3",
		SecretMeta{Kind: KindURL, Sensitive: sensitive(false)}); err != nil {
		t.Fatalf("PutSecretMeta: %v", err)
	}
	_ = s.Close()

	s2 := openKeyedStore(t, path, wrongKey())
	got, err := s2.ResolveDetailed(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("config-only resolution must not decrypt: %v", err)
	}
	if got["ENDPOINT"].Value != "https://a.example/v3" {
		t.Fatalf("ENDPOINT = %+v", got["ENDPOINT"])
	}
}

func TestVerifyMasterKey(t *testing.T) {
	ctx := context.Background()
	key := make([]byte, crypto.KeySize)
	path := filepath.Join(t.TempDir(), "vault.db")
	s := openKeyedStore(t, path, key)
	if err := s.VerifyMasterKey(ctx); err != nil {
		t.Fatalf("empty vault: %v", err)
	}
	seedSensitive(t, s, "my-api", "staging", 2)
	if err := s.VerifyMasterKey(ctx); err != nil {
		t.Fatalf("matching key: %v", err)
	}
	_ = s.Close()

	s2 := openKeyedStore(t, path, wrongKey())
	if err := s2.VerifyMasterKey(ctx); !errors.Is(err, ErrMasterKeyMismatch) {
		t.Fatalf("wrong key: %v, want ErrMasterKeyMismatch", err)
	}
}

func TestVerifyMasterKeyPartialStillUnlocks(t *testing.T) {
	ctx := context.Background()
	key := make([]byte, crypto.KeySize)
	path := filepath.Join(t.TempDir(), "vault.db")
	s := openKeyedStore(t, path, key)
	seedSensitive(t, s, "my-api", "staging", 2)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE secrets SET value_enc = ? WHERE key = 'KEY_00'`,
		[]byte("this-is-not-a-valid-gcm-ciphertext-000")); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if err := s.VerifyMasterKey(ctx); err != nil {
		t.Fatalf("a partially readable vault must still unlock: %v", err)
	}
}

func TestVerifyMasterKeyInconsistentOnly(t *testing.T) {
	ctx := context.Background()
	key := make([]byte, crypto.KeySize)
	path := filepath.Join(t.TempDir(), "vault.db")
	s := openKeyedStore(t, path, key)
	seedSensitive(t, s, "my-api", "staging", 1)
	if _, err := s.db.ExecContext(ctx, `UPDATE secrets SET value_enc = ? WHERE key = 'KEY_00'`, []byte{}); err != nil {
		t.Fatalf("blank: %v", err)
	}
	if err := s.VerifyMasterKey(ctx); !errors.Is(err, ErrInconsistentDefinition) {
		t.Fatalf("err = %v, want ErrInconsistentDefinition", err)
	}
}
