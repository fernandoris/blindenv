package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/argon2"
)

// KeyProvider supplies the 32-byte master key that protects the vault.
type KeyProvider interface {
	// MasterKey returns the existing master key, or ErrNoKey when none exists.
	MasterKey() ([]byte, error)
	// EnsureMasterKey returns the existing master key or creates and persists
	// a new one.
	EnsureMasterKey() ([]byte, error)
}

// StaticProvider is a KeyProvider backed by an in-memory key. It is intended
// for tests.
type StaticProvider struct {
	key []byte
}

// NewStaticProvider returns a provider that always yields key.
func NewStaticProvider(key []byte) *StaticProvider {
	cp := make([]byte, len(key))
	copy(cp, key)
	return &StaticProvider{key: cp}
}

// MasterKey implements KeyProvider.
func (p *StaticProvider) MasterKey() ([]byte, error) {
	if len(p.key) == 0 {
		return nil, ErrNoKey
	}
	return p.key, nil
}

// EnsureMasterKey implements KeyProvider.
func (p *StaticProvider) EnsureMasterKey() ([]byte, error) { return p.MasterKey() }

const (
	keyringService = "blindenv"
	keyringUser    = "master"
)

// OSKeyringProvider stores the master key in the operating system keyring
// (macOS Keychain, Windows Credential Manager, Linux Secret Service).
type OSKeyringProvider struct{}

// NewOSKeyringProvider returns a provider backed by the OS keyring.
func NewOSKeyringProvider() *OSKeyringProvider { return &OSKeyringProvider{} }

// MasterKey implements KeyProvider.
func (p *OSKeyringProvider) MasterKey() ([]byte, error) {
	encoded, err := keyring.Get(keyringService, keyringUser)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrNoKey
	}
	if err != nil {
		return nil, fmt.Errorf("crypto: read keyring: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("crypto: decode keyring value: %w", err)
	}
	return key, nil
}

// EnsureMasterKey implements KeyProvider.
func (p *OSKeyringProvider) EnsureMasterKey() ([]byte, error) {
	key, err := p.MasterKey()
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, ErrNoKey) {
		return nil, err
	}
	key = make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("crypto: generate master key: %w", err)
	}
	if err := keyring.Set(keyringService, keyringUser, base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("crypto: write keyring: %w", err)
	}
	return key, nil
}

// Argon2id parameters for passphrase-based key derivation.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
	saltSize     = 32
)

// PassphraseProvider derives the master key from a user passphrase with
// Argon2id and a salt persisted next to the vault.
type PassphraseProvider struct {
	Passphrase []byte
	SaltPath   string
}

// NewPassphraseProvider returns a passphrase-based provider.
func NewPassphraseProvider(passphrase, saltPath string) *PassphraseProvider {
	return &PassphraseProvider{Passphrase: []byte(passphrase), SaltPath: saltPath}
}

// MasterKey implements KeyProvider.
func (p *PassphraseProvider) MasterKey() ([]byte, error) {
	salt, err := os.ReadFile(p.SaltPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoKey
	}
	if err != nil {
		return nil, fmt.Errorf("crypto: read salt: %w", err)
	}
	return p.derive(salt), nil
}

// EnsureMasterKey implements KeyProvider.
func (p *PassphraseProvider) EnsureMasterKey() ([]byte, error) {
	salt, err := os.ReadFile(p.SaltPath)
	if errors.Is(err, os.ErrNotExist) {
		salt = make([]byte, saltSize)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, fmt.Errorf("crypto: generate salt: %w", err)
		}
		if err := os.WriteFile(p.SaltPath, salt, 0o600); err != nil {
			return nil, fmt.Errorf("crypto: write salt: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("crypto: read salt: %w", err)
	}
	return p.derive(salt), nil
}

func (p *PassphraseProvider) derive(salt []byte) []byte {
	return argon2.IDKey(p.Passphrase, salt, argonTime, argonMemory, argonThreads, KeySize)
}

// DeriveKey derives a 32-byte key from a passphrase and salt with Argon2id.
// It is used for passphrase-protected backups.
func DeriveKey(passphrase, salt []byte) []byte {
	return argon2.IDKey(passphrase, salt, argonTime, argonMemory, argonThreads, KeySize)
}

// NewSalt returns a cryptographically random salt of the given size.
func NewSalt(size int) ([]byte, error) {
	salt := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("crypto: generate salt: %w", err)
	}
	return salt, nil
}

// ResolveProvider selects a key provider for the given vault. An explicit
// passphrase always wins (used for headless/CI and deterministic unlock);
// otherwise the OS keyring is used when reachable.
func ResolveProvider(saltPath, passphrase string) (KeyProvider, error) {
	if passphrase != "" {
		return NewPassphraseProvider(passphrase, saltPath), nil
	}
	kp := NewOSKeyringProvider()
	if _, err := kp.MasterKey(); err == nil || errors.Is(err, ErrNoKey) {
		return kp, nil
	}
	return nil, fmt.Errorf("crypto: OS keyring unavailable and no passphrase provided (set BLINDENV_PASSPHRASE)")
}
