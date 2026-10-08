package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// KeySize is the length in bytes of the master key (AES-256).
const KeySize = 32

// ErrNoKey indicates that no master key is available yet.
var ErrNoKey = errors.New("crypto: no master key available")

// ErrIntegrity indicates that encrypted data failed authentication.
var ErrIntegrity = errors.New("crypto: encrypted data failed integrity check")

// MinSealedSize is the smallest possible length of a value produced by Encrypt:
// a nonce plus the authentication tag with an empty plaintext. A sensitive
// value stored with a shorter blob cannot be a valid ciphertext.
var MinSealedSize = func() int {
	block, err := aes.NewCipher(make([]byte, KeySize))
	if err != nil {
		return 0
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0
	}
	return gcm.NonceSize() + gcm.Overhead()
}()

// Encrypt seals plaintext with AES-256-GCM using a random nonce prepended to
// the returned ciphertext.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt opens data produced by Encrypt.
func Decrypt(key, data []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, ErrIntegrity
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrIntegrity
	}
	return plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("crypto: key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
