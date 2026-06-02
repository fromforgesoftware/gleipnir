package vault

import (
	"context"
	"errors"
	"hash/crc32"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// kmsClient is the subset of the GCP KMS KeyManagementClient the provider uses,
// extracted so tests can substitute a fake.
type kmsClient interface {
	Encrypt(ctx context.Context, req *kmspb.EncryptRequest, opts ...gax.CallOption) (*kmspb.EncryptResponse, error)
	Decrypt(ctx context.Context, req *kmspb.DecryptRequest, opts ...gax.CallOption) (*kmspb.DecryptResponse, error)
	Close() error
}

// GCPKMSKeyProvider wraps DEKs with a Cloud KMS crypto key. The wrapping key
// never leaves the HSM-backed KMS; a stolen database yields only ciphertext and
// KMS-wrapped data keys, which require a live KMS Decrypt this provider's
// service identity alone is authorized to make. crc32c checksums guard the
// payloads in transit, as recommended by Google.
type GCPKMSKeyProvider struct {
	client  kmsClient
	keyName string
}

// NewGCPKMSKeyProvider dials Cloud KMS (Application Default Credentials) and
// targets keyName, the crypto key resource
// projects/P/locations/L/keyRings/R/cryptoKeys/K.
func NewGCPKMSKeyProvider(ctx context.Context, keyName string) (*GCPKMSKeyProvider, error) {
	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return nil, err
	}
	return &GCPKMSKeyProvider{client: client, keyName: keyName}, nil
}

func (p *GCPKMSKeyProvider) Wrap(ctx context.Context, dek []byte) ([]byte, error) {
	resp, err := p.client.Encrypt(ctx, &kmspb.EncryptRequest{
		Name:            p.keyName,
		Plaintext:       dek,
		PlaintextCrc32C: wrapperspb.Int64(int64(crc32c(dek))),
	})
	if err != nil {
		return nil, err
	}
	if !resp.VerifiedPlaintextCrc32C {
		return nil, errors.New("vault: KMS encrypt request corrupted in transit")
	}
	if resp.CiphertextCrc32C == nil || int64(crc32c(resp.Ciphertext)) != resp.CiphertextCrc32C.Value {
		return nil, errors.New("vault: KMS encrypt response corrupted in transit")
	}
	return resp.Ciphertext, nil
}

func (p *GCPKMSKeyProvider) Unwrap(ctx context.Context, wrapped []byte) ([]byte, error) {
	resp, err := p.client.Decrypt(ctx, &kmspb.DecryptRequest{
		Name:             p.keyName,
		Ciphertext:       wrapped,
		CiphertextCrc32C: wrapperspb.Int64(int64(crc32c(wrapped))),
	})
	if err != nil {
		return nil, err
	}
	if resp.PlaintextCrc32C == nil || int64(crc32c(resp.Plaintext)) != resp.PlaintextCrc32C.Value {
		return nil, errors.New("vault: KMS decrypt response corrupted in transit")
	}
	return resp.Plaintext, nil
}

// KeyID is the KMS crypto key resource name, recorded on each sealed credential
// so a key rotation can find and re-wrap what a retired key sealed.
func (p *GCPKMSKeyProvider) KeyID() string { return p.keyName }

// Close releases the KMS client connection.
func (p *GCPKMSKeyProvider) Close() error { return p.client.Close() }

var crc32cTable = crc32.MakeTable(crc32.Castagnoli)

func crc32c(b []byte) uint32 { return crc32.Checksum(b, crc32cTable) }
