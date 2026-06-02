package vault

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cloud.google.com/go/kms/apiv1/kmspb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// fakeKMS simulates Cloud KMS: ciphertext is a reversible "enc:"-prefix of the
// plaintext, with crc32c checksums set the way the real service sets them.
type fakeKMS struct {
	failEncryptVerify bool
	corruptCipherCRC  bool
	corruptPlainCRC   bool
}

func (f *fakeKMS) Encrypt(_ context.Context, req *kmspb.EncryptRequest, _ ...gax.CallOption) (*kmspb.EncryptResponse, error) {
	ct := append([]byte("enc:"), req.Plaintext...)
	crc := int64(crc32c(ct))
	if f.corruptCipherCRC {
		crc++
	}
	return &kmspb.EncryptResponse{
		Ciphertext:              ct,
		CiphertextCrc32C:        wrapperspb.Int64(crc),
		VerifiedPlaintextCrc32C: !f.failEncryptVerify,
	}, nil
}

func (f *fakeKMS) Decrypt(_ context.Context, req *kmspb.DecryptRequest, _ ...gax.CallOption) (*kmspb.DecryptResponse, error) {
	pt := bytes.TrimPrefix(req.Ciphertext, []byte("enc:"))
	crc := int64(crc32c(pt))
	if f.corruptPlainCRC {
		crc++
	}
	return &kmspb.DecryptResponse{Plaintext: pt, PlaintextCrc32C: wrapperspb.Int64(crc)}, nil
}

func (f *fakeKMS) Close() error { return nil }

func TestGCPKMS_VaultRoundTrip(t *testing.T) {
	provider := &GCPKMSKeyProvider{client: &fakeKMS{}, keyName: "projects/p/locations/l/keyRings/r/cryptoKeys/k"}
	v := New(provider)

	secret := []byte("coinbase-refresh-token-abc")
	ct, wrapped, err := v.Seal(context.Background(), secret)
	require.NoError(t, err)
	assert.NotContains(t, string(wrapped), "coinbase-refresh-token", "DEK must be KMS-wrapped, not raw")

	got, err := v.Open(context.Background(), ct, wrapped)
	require.NoError(t, err)
	assert.Equal(t, secret, got)
	assert.Equal(t, "projects/p/locations/l/keyRings/r/cryptoKeys/k", provider.KeyID())
}

func TestGCPKMS_WrapRejectsUnverifiedRequest(t *testing.T) {
	provider := &GCPKMSKeyProvider{client: &fakeKMS{failEncryptVerify: true}, keyName: "k"}
	_, err := provider.Wrap(context.Background(), []byte("dek"))
	assert.Error(t, err)
}

func TestGCPKMS_WrapRejectsCorruptCiphertextCRC(t *testing.T) {
	provider := &GCPKMSKeyProvider{client: &fakeKMS{corruptCipherCRC: true}, keyName: "k"}
	_, err := provider.Wrap(context.Background(), []byte("dek"))
	assert.Error(t, err)
}

func TestGCPKMS_UnwrapRejectsCorruptPlaintextCRC(t *testing.T) {
	provider := &GCPKMSKeyProvider{client: &fakeKMS{corruptPlainCRC: true}, keyName: "k"}
	_, err := provider.Unwrap(context.Background(), []byte("enc:dek"))
	assert.Error(t, err)
}
