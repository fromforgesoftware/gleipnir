package api

import (
	"time"

	"github.com/fromforgesoftware/go-kit/resource"

	"github.com/fromforgesoftware/gleipnir/internal/domain"
)

const ResourceTypeCredential resource.Type = "credentials"

// CredentialInputDTO is the write-only intake for storing a connection's
// secret. It is never returned with plaintext; reads use CredentialDTO.
type CredentialInputDTO struct {
	resource.RestDTO

	RKind         string     `jsonapi:"attr,kind,omitempty"`
	RAccessToken  string     `jsonapi:"attr,accessToken,omitempty"`
	RRefreshToken string     `jsonapi:"attr,refreshToken,omitempty"`
	RAPIKey       string     `jsonapi:"attr,apiKey,omitempty"`
	RAPISecret    string     `jsonapi:"attr,apiSecret,omitempty"`
	RExpiresAt    *time.Time `jsonapi:"attr,expiresAt,omitempty"`
}

// CredentialDTO is the read-safe wire shape: metadata only, never plaintext.
type CredentialDTO struct {
	resource.RestDTO

	RKind       string                    `jsonapi:"attr,kind,omitempty"`
	RKeyID      string                    `jsonapi:"attr,keyId,omitempty"`
	RExpiresAt  *time.Time                `jsonapi:"attr,expiresAt,omitempty"`
	RConnection *resource.RelationshipDTO `jsonapi:"rel,connection,omitempty"`
}

func CredentialToDTO(c domain.Credential) *CredentialDTO {
	if c == nil {
		return nil
	}
	dto := &CredentialDTO{
		RestDTO:     resource.ToRestDTO(c),
		RKind:       string(c.Kind()),
		RKeyID:      c.KeyID(),
		RExpiresAt:  c.ExpiresAt(),
		RConnection: resource.RelationshipToDTO(resource.RelFromIdentifier(c.Connection())),
	}
	dto.RType = ResourceTypeCredential
	return dto
}
