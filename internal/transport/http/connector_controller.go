// Package http holds Gleipnir's JSON:API controllers.
package http

import (
	"github.com/fromforgesoftware/go-kit/openapi"
	kitrest "github.com/fromforgesoftware/go-kit/transport/rest"

	"github.com/fromforgesoftware/gleipnir/internal/api"
	"github.com/fromforgesoftware/gleipnir/internal/app"
)

// ConnectorController exposes the read-only provider catalog at /api/connectors.
type ConnectorController struct {
	catalog *app.ConnectorCatalog
}

func NewConnectorController(catalog *app.ConnectorCatalog) kitrest.Controller {
	return &ConnectorController{catalog: catalog}
}

func (c *ConnectorController) Routes(r kitrest.Router) {
	r.Route("/api/connectors", func(r kitrest.Router) {
		r.Get("", kitrest.NewJsonApiListHandler(
			c.catalog, api.ConnectorToDTO,
			kitrest.HandlerWithOpenAPI(
				openapi.Summary("List connectors"),
				openapi.Description("The built-in catalog of supported providers and their auth type."),
				openapi.Tags("connectors"),
			),
		))
	})
}
