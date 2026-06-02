package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
)

const kekEnvVar = "GLEIPNIR_KEY_ENCRYPTION_KEY"

// LocalKeyProvider wraps DEKs with a single AES-256 KEK held in process. It is
// the dev/default provider; production swaps in a GCP KMS provider behind the
// same KeyProvider interface so a DB dump alone never yields plaintext.
type LocalKeyProvider struct {
	kek []byte // 32 bytes → AES-256
}

func NewLocalKeyProvider(kek []byte) (*LocalKeyProvider, error) {
	if len(kek) != 32 {
		return nil, fmt.Errorf("vault: key-encryption key must be 32 bytes, got %d", len(kek))
	}
	return &LocalKeyProvider{kek: kek}, nil
}

// NewLocalKeyProviderFromEnv reads the base64-encoded 32-byte KEK from
// GLEIPNIR_KEY_ENCRYPTION_KEY, failing fast if absent or malformed.
func NewLocalKeyProviderFromEnv() (*LocalKeyProvider, error) {
	raw := os.Getenv(kekEnvVar)
	if raw == "" {
		return nil, fmt.Errorf("vault: %s is required", kekEnvVar)
	}
	kek, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("vault: %s must be base64: %w", kekEnvVar, err)
	}
	return NewLocalKeyProvider(kek)
}

func (p *LocalKeyProvider) Wrap(_ context.Context, dek []byte) ([]byte, error) {
	return aesSeal(p.kek, dek)
}

func (p *LocalKeyProvider) Unwrap(_ context.Context, wrapped []byte) ([]byte, error) {
	return aesOpen(p.kek, wrapped)
}

func (p *LocalKeyProvider) KeyID() string { return "local" }
