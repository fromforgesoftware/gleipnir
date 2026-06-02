package api

import (
	"time"

	"github.com/fromforgesoftware/go-kit/resource"

	"github.com/fromforgesoftware/gleipnir/internal/domain"
)

const ResourceTypeConnection resource.Type = "connections"

// ConnectionDTO is the wire shape of a connection. Owner is an opaque key the
// caller supplies on create and Gleipnir scopes by.
type ConnectionDTO struct {
	resource.RestDTO

	ROwner     string     `jsonapi:"attr,owner,omitempty"`
	RConnector string     `jsonapi:"attr,connector,omitempty"`
	RStatus    string     `jsonapi:"attr,status,omitempty"`
	RScopes    []string   `jsonapi:"attr,scopes,omitempty"`
	RExpiresAt *time.Time `jsonapi:"attr,expiresAt,omitempty"`
	RCreatedAt time.Time  `jsonapi:"attr,createdAt,omitempty"`
	RUpdatedAt time.Time  `jsonapi:"attr,updatedAt,omitempty"`
}

func ConnectionToDTO(c domain.Connection) *ConnectionDTO {
	if c == nil {
		return nil
	}
	dto := &ConnectionDTO{
		RestDTO:    resource.ToRestDTO(c),
		ROwner:     c.Owner(),
		RConnector: c.Connector(),
		RStatus:    string(c.Status()),
		RScopes:    c.Scopes(),
		RExpiresAt: c.ExpiresAt(),
		RCreatedAt: c.CreatedAt(),
		RUpdatedAt: c.UpdatedAt(),
	}
	dto.RType = ResourceTypeConnection
	return dto
}

func ConnectionFromDTO(dto *ConnectionDTO) domain.Connection {
	if dto == nil {
		return nil
	}
	opts := []domain.ConnectionOption{}
	if dto.RStatus != "" {
		opts = append(opts, domain.WithConnectionStatus(domain.ConnectionStatus(dto.RStatus)))
	}
	if len(dto.RScopes) > 0 {
		opts = append(opts, domain.WithConnectionScopes(dto.RScopes))
	}
	if dto.RExpiresAt != nil {
		opts = append(opts, domain.WithConnectionExpiresAt(dto.RExpiresAt))
	}
	return domain.NewConnection(dto.ROwner, dto.RConnector, opts...)
}
