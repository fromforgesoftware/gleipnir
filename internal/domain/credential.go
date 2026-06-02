package domain

import (
	"time"

	"github.com/fromforgesoftware/go-kit/resource"
)

// ResourceTypeCredential is the JSON:API type for credentials (never exposed
// with plaintext — admin inspect/rotate only).
const ResourceTypeCredential resource.Type = "credentials"

// CredentialKind is the shape of the stored secret.
type CredentialKind string

const (
	CredentialKindAPIKey      CredentialKind = "API_KEY"
	CredentialKindOAuthTokens CredentialKind = "OAUTH_TOKENS"
)

// Credential is a connection's secret material at rest: envelope-encrypted
// (Ciphertext under a per-connection data key, itself wrapped by a KMS master
// key in WrappedKey). Plaintext exists only transiently in the usecase during
// encrypt/decrypt — it is never held on the aggregate or returned by the API.
type Credential interface {
	resource.Resource
	Connection() resource.Identifier
	Kind() CredentialKind
	Ciphertext() []byte
	WrappedKey() []byte
	// KeyID names the wrapping key that sealed WrappedKey, so a KEK rotation
	// can find and re-wrap credentials sealed under the retired key.
	KeyID() string
	ExpiresAt() *time.Time
}

type credential struct {
	resource.Resource

	connectionID string
	kind         CredentialKind
	ciphertext   []byte
	wrappedKey   []byte
	keyID        string
	expiresAt    *time.Time
}

type CredentialOption func(*credential)

func WithCredentialID(id string) CredentialOption {
	return func(c *credential) { c.Resource = resource.Update(c.Resource, resource.WithID(id)) }
}
func WithCredentialKeyID(keyID string) CredentialOption {
	return func(c *credential) { c.keyID = keyID }
}
func WithCredentialExpiresAt(t *time.Time) CredentialOption {
	return func(c *credential) { c.expiresAt = t }
}

// NewCredential builds an encrypted-credential aggregate. connectionID/kind and
// the ciphertext + wrapped data key are mandatory.
func NewCredential(connectionID string, kind CredentialKind, ciphertext, wrappedKey []byte, opts ...CredentialOption) Credential {
	c := &credential{
		Resource:     resource.New(resource.WithType(ResourceTypeCredential)),
		connectionID: connectionID,
		kind:         kind,
		ciphertext:   ciphertext,
		wrappedKey:   wrappedKey,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *credential) Connection() resource.Identifier {
	return resource.NewIdentifier(c.connectionID, ResourceTypeConnection)
}
func (c *credential) Kind() CredentialKind  { return c.kind }
func (c *credential) Ciphertext() []byte    { return c.ciphertext }
func (c *credential) WrappedKey() []byte    { return c.wrappedKey }
func (c *credential) KeyID() string         { return c.keyID }
func (c *credential) ExpiresAt() *time.Time { return c.expiresAt }
