package db

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fernandoris/blindenv/pkg/crypto"
)

func strPtr(v string) *string { return &v }

// readStoredSecret returns the raw columns of one definition so tests can
// assert storage-level behavior without going through the resolver.
func readStoredSecret(t *testing.T, s *Store, project, environment, key string) (
	enc []byte, plain sql.NullString, sensitive bool, kind, hint, createdAt, updatedAt string,
) {
	t.Helper()
	ctx := context.Background()
	pid, env, err := s.resolveScope(ctx, project, environment)
	if err != nil {
		t.Fatalf("resolveScope: %v", err)
	}
	where, args := s.scopeWhere(pid, env)
	args = append(args, key)
	err = s.db.QueryRowContext(ctx,
		`SELECT value_enc, value_plain, sensitive, kind, hint, created_at, updated_at FROM secrets WHERE `+where+` AND key = ?`,
		args...).Scan(&enc, &plain, &sensitive, &kind, &hint, &createdAt, &updatedAt)
	if err != nil {
		t.Fatalf("read stored secret: %v", err)
	}
	return enc, plain, sensitive, kind, hint, createdAt, updatedAt
}

func TestUpdateSecretPreservesIdentityAndReencrypts(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := s.CreateProject(ctx, "my-api"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := s.CreateEnvironment(ctx, "my-api", "staging"); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if _, err := s.PutSecretMeta(ctx, "my-api", "staging", "API_KEY", "original-secret-1", SecretMeta{
		Kind: KindURL, Hint: "base includes /v3",
	}); err != nil {
		t.Fatalf("PutSecretMeta: %v", err)
	}

	encBefore, _, sensitiveBefore, kindBefore, hintBefore, createdBefore, updatedBefore := readStoredSecret(t, s, "my-api", "staging", "API_KEY")
	if !sensitiveBefore || kindBefore != string(KindURL) || hintBefore != "base includes /v3" {
		t.Fatalf("unexpected initial metadata: sensitive=%v kind=%q hint=%q", sensitiveBefore, kindBefore, hintBefore)
	}

	time.Sleep(2 * time.Millisecond)
	short, err := s.UpdateSecret(ctx, "my-api", "staging", "API_KEY", strPtr("replacement-secret-2"), nil)
	if err != nil {
		t.Fatalf("UpdateSecret: %v", err)
	}
	if short {
		t.Fatalf("long replacement reported as short")
	}

	encAfter, plainAfter, sensitiveAfter, kindAfter, hintAfter, createdAfter, updatedAfter := readStoredSecret(t, s, "my-api", "staging", "API_KEY")
	if bytes.Equal(encBefore, encAfter) {
		t.Fatalf("ciphertext did not change on update")
	}
	if !sensitiveAfter || plainAfter.Valid {
		t.Fatalf("sensitive representation changed: sensitive=%v plain=%v", sensitiveAfter, plainAfter)
	}
	if kindAfter != kindBefore || hintAfter != hintBefore {
		t.Fatalf("metadata not preserved: kind=%q hint=%q", kindAfter, hintAfter)
	}
	if createdAfter != createdBefore {
		t.Fatalf("created_at changed: before=%q after=%q", createdBefore, createdAfter)
	}
	if updatedAfter == updatedBefore {
		t.Fatalf("updated_at did not advance")
	}

	got, err := s.Resolve(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["API_KEY"] != "replacement-secret-2" {
		t.Fatalf("resolved value = %q", got["API_KEY"])
	}
}

func TestUpdateSecretNonSensitiveStaysCleartext(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := s.PutSecretMeta(ctx, "", "", "ENDPOINT", "https://old.example.com/v3", SecretMeta{
		Kind: KindURL, Sensitive: sensitive(false),
	}); err != nil {
		t.Fatalf("PutSecretMeta: %v", err)
	}

	if _, err := s.UpdateSecret(ctx, "", "", "ENDPOINT", strPtr("https://new.example.com/v3"), nil); err != nil {
		t.Fatalf("UpdateSecret: %v", err)
	}

	enc, plain, isSensitive, _, _, _, _ := readStoredSecret(t, s, "", "", "ENDPOINT")
	if isSensitive {
		t.Fatalf("value became sensitive on update")
	}
	if len(enc) != 0 {
		t.Fatalf("non-sensitive value stored ciphertext")
	}
	if plain.String != "https://new.example.com/v3" {
		t.Fatalf("stored cleartext = %q", plain.String)
	}
}

func TestUpdateSecretSensitivityFlipReencodes(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := s.PutSecretMeta(ctx, "", "", "ENDPOINT", "https://config.example.com/v3", SecretMeta{
		Kind: KindURL, Sensitive: sensitive(false),
	}); err != nil {
		t.Fatalf("PutSecretMeta: %v", err)
	}

	toSensitive := SecretMeta{Kind: KindURL, Sensitive: sensitive(true)}
	if _, err := s.UpdateSecret(ctx, "", "", "ENDPOINT", nil, &toSensitive); err != nil {
		t.Fatalf("flip to sensitive: %v", err)
	}
	enc, plain, isSensitive, _, _, _, _ := readStoredSecret(t, s, "", "", "ENDPOINT")
	if !isSensitive || plain.Valid {
		t.Fatalf("flip to sensitive left sensitive=%v plain=%v", isSensitive, plain)
	}
	if value, err := s.decryptCiphertext("ENDPOINT", enc); err != nil || value != "https://config.example.com/v3" {
		t.Fatalf("value after flip = %q, %v", value, err)
	}

	toConfig := SecretMeta{Kind: KindURL, Sensitive: sensitive(false)}
	if _, err := s.UpdateSecret(ctx, "", "", "ENDPOINT", nil, &toConfig); err != nil {
		t.Fatalf("flip to non-sensitive: %v", err)
	}
	enc, plain, isSensitive, _, _, _, _ = readStoredSecret(t, s, "", "", "ENDPOINT")
	if isSensitive || len(enc) != 0 {
		t.Fatalf("flip to non-sensitive left sensitive=%v enc=%d", isSensitive, len(enc))
	}
	if plain.String != "https://config.example.com/v3" {
		t.Fatalf("value after flip back = %q", plain.String)
	}
}

func TestUpdateSecretRejectsEmptyValueAndKeepsOnNil(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := s.PutSecret(ctx, "", "", "API_KEY", "original-secret-1"); err != nil {
		t.Fatalf("PutSecret: %v", err)
	}

	if _, err := s.UpdateSecret(ctx, "", "", "API_KEY", strPtr(""), nil); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("empty value err = %v, want ErrInvalidMetadata", err)
	}
	if _, err := s.UpdateSecret(ctx, "", "", "API_KEY", nil, nil); err != nil {
		t.Fatalf("nil value update: %v", err)
	}

	got, err := s.Resolve(ctx, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["API_KEY"] != "original-secret-1" {
		t.Fatalf("value after rejected and nil updates = %q", got["API_KEY"])
	}
}

func TestUpdateSecretValidatesHintAgainstEffectiveValue(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := s.PutSecretMeta(ctx, "", "", "API_KEY", "initial-secret-1", SecretMeta{
		Hint: "mentions old-secret-123",
	}); err != nil {
		t.Fatalf("PutSecretMeta: %v", err)
	}

	if _, err := s.UpdateSecret(ctx, "", "", "API_KEY", strPtr("old-secret-123"), nil); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("preserved hint collision err = %v, want ErrInvalidMetadata", err)
	}
	if _, err := s.UpdateSecret(ctx, "", "", "API_KEY", strPtr("brand-new-secret"), &SecretMeta{
		Hint: "leaks brand-new-secret",
	}); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("supplied hint collision err = %v, want ErrInvalidMetadata", err)
	}

	got, err := s.Resolve(ctx, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["API_KEY"] != "initial-secret-1" {
		t.Fatalf("rejected update changed value to %q", got["API_KEY"])
	}
}

func TestUpdateSecretReportsShortValue(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := s.PutSecret(ctx, "", "", "API_KEY", "long-enough-value"); err != nil {
		t.Fatalf("PutSecret: %v", err)
	}

	short, err := s.UpdateSecret(ctx, "", "", "API_KEY", strPtr("short"), nil)
	if err != nil {
		t.Fatalf("UpdateSecret short: %v", err)
	}
	if !short {
		t.Fatalf("short value not reported")
	}
	got, err := s.Resolve(ctx, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["API_KEY"] != "short" {
		t.Fatalf("stored short value = %q", got["API_KEY"])
	}

	short, err = s.UpdateSecret(ctx, "", "", "API_KEY", strPtr("longer-value"), nil)
	if err != nil {
		t.Fatalf("UpdateSecret long: %v", err)
	}
	if short {
		t.Fatalf("long value reported as short")
	}
}

func TestUpdateSecretRequiresExistingDefinition(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := s.CreateProject(ctx, "my-api"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := s.CreateEnvironment(ctx, "my-api", "staging"); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}

	if _, err := s.UpdateSecret(ctx, "my-api", "staging", "MISSING", strPtr("value-123"), nil); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("missing key err = %v, want ErrSecretNotFound", err)
	}
	infos, err := s.ScopeSecrets(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("ScopeSecrets: %v", err)
	}
	if len(infos) != 0 {
		t.Fatalf("update created a definition: %+v", infos)
	}

	if _, err := s.PutSecret(ctx, "my-api", "", "GLOBAL_KEY", "global-value-1"); err != nil {
		t.Fatalf("PutSecret broader: %v", err)
	}
	if _, err := s.UpdateSecret(ctx, "my-api", "staging", "GLOBAL_KEY", strPtr("attempted-value-2"), nil); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("broader scope err = %v, want ErrSecretNotFound", err)
	}
	got, err := s.Resolve(ctx, "my-api", "staging")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["GLOBAL_KEY"] != "global-value-1" {
		t.Fatalf("broader definition changed to %q", got["GLOBAL_KEY"])
	}
}

func TestConcurrentSecretUpdates(t *testing.T) {
	key, err := crypto.NewSalt(crypto.KeySize)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "vault.db")

	writer, err := Open(path, key)
	if err != nil {
		t.Fatalf("Open writer: %v", err)
	}
	defer writer.Close()
	ctx := context.Background()
	if _, err := writer.CreateProject(ctx, "my-api"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := writer.PutSecret(ctx, "my-api", "", "ROTATE", "initial-value-0"); err != nil {
		t.Fatalf("PutSecret: %v", err)
	}

	const writers = 4
	const perWriter = 5
	stores := make([]*Store, 0, writers)
	for i := 0; i < writers; i++ {
		s, err := Open(path, key)
		if err != nil {
			t.Fatalf("Open writer %d: %v", i, err)
		}
		defer s.Close()
		stores = append(stores, s)
	}

	var wg sync.WaitGroup
	errs := make(chan error, writers*perWriter)
	for i, s := range stores {
		for j := 0; j < perWriter; j++ {
			wg.Add(1)
			go func(i, j int, s *Store) {
				defer wg.Done()
				value := fmt.Sprintf("value-%d-%d-abcdef", i, j)
				if _, err := s.UpdateSecret(ctx, "my-api", "", "ROTATE", &value, nil); err != nil {
					errs <- err
				}
			}(i, j, s)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent update: %v", err)
	}

	infos, err := writer.ScopeSecrets(ctx, "my-api", "")
	if err != nil {
		t.Fatalf("ScopeSecrets: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("definitions = %d, want 1", len(infos))
	}
	got, err := writer.Resolve(ctx, "my-api", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.HasPrefix(got["ROTATE"], "value-") {
		t.Fatalf("final value = %q", got["ROTATE"])
	}
}
