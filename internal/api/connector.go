// Package api holds Gleipnir's JSON:API DTOs and their domain mappers.
package api

import (
	"github.com/fromforgesoftware/go-kit/resource"

	"github.com/fromforgesoftware/gleipnir/internal/domain"
)

const ResourceTypeConnector resource.Type = "connectors"

// ConnectorDTO is the read-only wire shape of a provider definition.
type ConnectorDTO struct {
	resource.RestDTO

	RName       string   `jsonapi:"attr,name,omitempty"`
	RAuthType   string   `jsonapi:"attr,authType,omitempty"`
	RAuthURL    string   `jsonapi:"attr,authUrl,omitempty"`
	RTokenURL   string   `jsonapi:"attr,tokenUrl,omitempty"`
	RScopes     []string `jsonapi:"attr,scopes,omitempty"`
	RRateLimit  int      `jsonapi:"attr,rateLimit,omitempty"`
	RRateWindow string   `jsonapi:"attr,rateWindow,omitempty"`
}

func ConnectorToDTO(c domain.Connector) *ConnectorDTO {
	dto := &ConnectorDTO{
		RName:       c.Name,
		RAuthType:   string(c.AuthType),
		RAuthURL:    c.AuthURL,
		RTokenURL:   c.TokenURL,
		RScopes:     c.Scopes,
		RRateLimit:  c.Rate.Limit,
		RRateWindow: c.Rate.Window.String(),
	}
	dto.RID = c.Slug
	dto.RType = ResourceTypeConnector
	return dto
}
