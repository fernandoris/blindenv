package crypto

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func TestStaticProvider(t *testing.T) {
	key := testKey(t)
	p := NewStaticProvider(key)

	got, err := p.MasterKey()
	if err != nil {
		t.Fatalf("MasterKey: %v", err)
	}
	if !bytes.Equal(got, key) {
		t.Fatalf("MasterKey = %x, want %x", got, key)
	}
}

func TestStaticProviderEmpty(t *testing.T) {
	if _, err := NewStaticProvider(nil).MasterKey(); !errors.Is(err, ErrNoKey) {
		t.Fatalf("err = %v, want ErrNoKey", err)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(t)
	plaintext := []byte("super-secret-value")

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Contains(ciphertext, plaintext) {
		t.Fatal("ciphertext contains plaintext")
	}

	got, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Decrypt = %q, want %q", got, plaintext)
	}
}

func TestEncryptProducesDistinctCiphertexts(t *testing.T) {
	key := testKey(t)
	plaintext := []byte("same-value")

	first, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	second, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("two encryptions of the same value produced identical ciphertexts")
	}
}

func TestDecryptDetectsTampering(t *testing.T) {
	key := testKey(t)
	ciphertext, err := Encrypt(key, []byte("value"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	ciphertext[len(ciphertext)-1] ^= 0xff

	if _, err := Decrypt(key, ciphertext); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
}

func TestDecryptRejectsShortInput(t *testing.T) {
	if _, err := Decrypt(testKey(t), []byte("short")); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
}

func TestPassphraseProviderDeterministic(t *testing.T) {
	saltPath := filepath.Join(t.TempDir(), "vault.salt")
	first := NewPassphraseProvider("correct horse", saltPath)
	if _, err := first.EnsureMasterKey(); err != nil {
		t.Fatalf("EnsureMasterKey: %v", err)
	}
	firstKey, err := first.MasterKey()
	if err != nil {
		t.Fatalf("MasterKey: %v", err)
	}

	second := NewPassphraseProvider("correct horse", saltPath)
	secondKey, err := second.MasterKey()
	if err != nil {
		t.Fatalf("MasterKey: %v", err)
	}
	if !bytes.Equal(firstKey, secondKey) {
		t.Fatal("same passphrase and salt derived different keys")
	}
}

func TestPassphraseProviderWrongPassphrase(t *testing.T) {
	saltPath := filepath.Join(t.TempDir(), "vault.salt")
	if _, err := NewPassphraseProvider("right", saltPath).EnsureMasterKey(); err != nil {
		t.Fatalf("EnsureMasterKey: %v", err)
	}
	rightKey, _ := NewPassphraseProvider("right", saltPath).MasterKey()
	wrongKey, _ := NewPassphraseProvider("wrong", saltPath).MasterKey()
	if bytes.Equal(rightKey, wrongKey) {
		t.Fatal("different passphrases derived the same key")
	}
}

func TestOSKeyringRoundTrip(t *testing.T) {
	if os.Getenv("BLINDENV_KEYRING_TEST") == "" {
		t.Skip("set BLINDENV_KEYRING_TEST=1 to exercise the real OS keyring")
	}
	p := NewOSKeyringProvider()
	key, err := p.EnsureMasterKey()
	if err != nil {
		t.Fatalf("EnsureMasterKey: %v", err)
	}
	if len(key) != KeySize {
		t.Fatalf("key length = %d, want %d", len(key), KeySize)
	}
	again, err := p.MasterKey()
	if err != nil {
		t.Fatalf("MasterKey: %v", err)
	}
	if !bytes.Equal(key, again) {
		t.Fatal("keyring returned a different key on second read")
	}
}

func TestPassphraseProviderMissingSalt(t *testing.T) {
	saltPath := filepath.Join(t.TempDir(), "missing.salt")
	if _, err := NewPassphraseProvider("x", saltPath).MasterKey(); !errors.Is(err, ErrNoKey) {
		t.Fatalf("err = %v, want ErrNoKey", err)
	}
}
