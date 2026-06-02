// Package vault holds Gleipnir's credential-at-rest cryptography: envelope
// encryption split into a provider-agnostic Vault and a pluggable KeyProvider
// that wraps the per-credential data key. The local provider wraps with an
// env KEK for dev; a GCP KMS provider implements the same interface for prod.
package vault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

// KeyProvider wraps and unwraps a data key (DEK). It is the only seam that
// differs between environments: dev wraps with a local AES KEK, prod wraps via
// GCP KMS Encrypt/Decrypt. The wrapped form is an opaque blob only the
// provider understands.
type KeyProvider interface {
	Wrap(ctx context.Context, dek []byte) (wrapped []byte, err error)
	Unwrap(ctx context.Context, wrapped []byte) (dek []byte, err error)
	// KeyID identifies the wrapping key for audit and rotation tracking.
	KeyID() string
}

// Vault envelope-encrypts secrets: a fresh random DEK encrypts the payload
// (AES-256-GCM), and the KeyProvider wraps that DEK. The two outputs map 1:1
// to a Credential's Ciphertext and WrappedKey columns. Rotating the wrapping
// key then re-wraps DEKs only, never re-encrypts payloads.
type Vault struct {
	provider KeyProvider
}

func New(p KeyProvider) *Vault { return &Vault{provider: p} }

// KeyID exposes the active wrapping key for stamping on stored credentials.
func (v *Vault) KeyID() string { return v.provider.KeyID() }

// Seal returns the DEK-encrypted payload and the wrapped DEK. Plaintext is
// never retained; the DEK is zeroed before returning.
func (v *Vault) Seal(ctx context.Context, plaintext []byte) (ciphertext, wrappedKey []byte, err error) {
	dek := make([]byte, 32)
	if _, err = io.ReadFull(rand.Reader, dek); err != nil {
		return nil, nil, err
	}
	defer zero(dek)

	ciphertext, err = aesSeal(dek, plaintext)
	if err != nil {
		return nil, nil, err
	}
	wrappedKey, err = v.provider.Wrap(ctx, dek)
	if err != nil {
		return nil, nil, err
	}
	return ciphertext, wrappedKey, nil
}

// Open reverses Seal.
func (v *Vault) Open(ctx context.Context, ciphertext, wrappedKey []byte) ([]byte, error) {
	dek, err := v.provider.Unwrap(ctx, wrappedKey)
	if err != nil {
		return nil, err
	}
	defer zero(dek)
	return aesOpen(dek, ciphertext)
}

// aesSeal AES-256-GCM-encrypts plaintext under key, prepending the nonce.
func aesSeal(key, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// aesOpen reverses aesSeal, reading the prepended nonce.
func aesOpen(key, blob []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(blob) < ns {
		return nil, errors.New("vault: ciphertext too short")
	}
	return gcm.Open(nil, blob[:ns], blob[ns:], nil)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
