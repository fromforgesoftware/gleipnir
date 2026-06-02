package vault_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fromforgesoftware/gleipnir/internal/vault"
)

func newKEK(t *testing.T) []byte {
	t.Helper()
	kek := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, kek)
	require.NoError(t, err)
	return kek
}

func newVault(t *testing.T, kek []byte) *vault.Vault {
	t.Helper()
	p, err := vault.NewLocalKeyProvider(kek)
	require.NoError(t, err)
	return vault.New(p)
}

func TestVault_SealOpenRoundTrip(t *testing.T) {
	v := newVault(t, newKEK(t))
	secret := []byte("binance-api-secret-XYZ-123")

	ct, wrapped, err := v.Seal(context.Background(), secret)
	require.NoError(t, err)
	assert.NotEmpty(t, ct)
	assert.NotEmpty(t, wrapped)
	assert.NotContains(t, string(ct), "binance-api-secret", "plaintext must not survive in ciphertext")

	got, err := v.Open(context.Background(), ct, wrapped)
	require.NoError(t, err)
	assert.Equal(t, secret, got)
}

func TestVault_FreshDEKPerSeal(t *testing.T) {
	v := newVault(t, newKEK(t))
	secret := []byte("same plaintext")

	ct1, w1, err := v.Seal(context.Background(), secret)
	require.NoError(t, err)
	ct2, w2, err := v.Seal(context.Background(), secret)
	require.NoError(t, err)

	assert.False(t, bytes.Equal(ct1, ct2), "fresh nonce → distinct ciphertext")
	assert.False(t, bytes.Equal(w1, w2), "fresh DEK → distinct wrapped key")
}

func TestVault_WrongKEKFailsToOpen(t *testing.T) {
	v1 := newVault(t, newKEK(t))
	v2 := newVault(t, newKEK(t))

	ct, wrapped, err := v1.Seal(context.Background(), []byte("secret"))
	require.NoError(t, err)

	_, err = v2.Open(context.Background(), ct, wrapped)
	assert.Error(t, err, "a different KEK must not unwrap the DEK")
}

func TestVault_TamperedCiphertextRejected(t *testing.T) {
	v := newVault(t, newKEK(t))
	ct, wrapped, err := v.Seal(context.Background(), []byte("secret"))
	require.NoError(t, err)

	ct[len(ct)-1] ^= 0xFF // flip a payload bit → GCM auth fails
	_, err = v.Open(context.Background(), ct, wrapped)
	assert.Error(t, err)
}

func TestVault_TamperedWrappedKeyRejected(t *testing.T) {
	v := newVault(t, newKEK(t))
	ct, wrapped, err := v.Seal(context.Background(), []byte("secret"))
	require.NoError(t, err)

	wrapped[len(wrapped)-1] ^= 0xFF
	_, err = v.Open(context.Background(), ct, wrapped)
	assert.Error(t, err)
}

func TestVault_OpenShortCiphertext(t *testing.T) {
	v := newVault(t, newKEK(t))
	_, wrapped, err := v.Seal(context.Background(), []byte("secret"))
	require.NoError(t, err)

	_, err = v.Open(context.Background(), []byte{0x01}, wrapped)
	assert.Error(t, err)
}

func TestLocalKeyProvider_RejectsBadKEKLength(t *testing.T) {
	_, err := vault.NewLocalKeyProvider(make([]byte, 16))
	assert.Error(t, err)
}

func TestLocalKeyProvider_FromEnv(t *testing.T) {
	t.Setenv("GLEIPNIR_KEY_ENCRYPTION_KEY", "24ckN78VPMEKDbrgVMEZUwtn5vIswCulIphK+666KWI=")
	p, err := vault.NewLocalKeyProviderFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "local", p.KeyID())
}

func TestLocalKeyProvider_FromEnvMissing(t *testing.T) {
	t.Setenv("GLEIPNIR_KEY_ENCRYPTION_KEY", "")
	_, err := vault.NewLocalKeyProviderFromEnv()
	assert.Error(t, err)
}
