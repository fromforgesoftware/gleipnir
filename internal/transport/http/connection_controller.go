package http

import (
	"context"
	"net/http"

	"github.com/fromforgesoftware/go-kit/application/repository"
	"github.com/fromforgesoftware/go-kit/openapi"
	"github.com/fromforgesoftware/go-kit/search/query"
	kitrest "github.com/fromforgesoftware/go-kit/transport/rest"

	"github.com/fromforgesoftware/gleipnir/internal/api"
	"github.com/fromforgesoftware/gleipnir/internal/app"
	"github.com/fromforgesoftware/gleipnir/internal/domain"
)

// ConnectionController exposes /api/connections (CRUD) plus a sub-resource
// /api/connections/{id}/credentials that seals and stores the connection's
// secret material.
type ConnectionController struct {
	connections app.ConnectionUsecase
	tokens      *app.TokenUsecase
}

func NewConnectionController(connections app.ConnectionUsecase, tokens *app.TokenUsecase) kitrest.Controller {
	return &ConnectionController{connections: connections, tokens: tokens}
}

func (c *ConnectionController) Routes(r kitrest.Router) {
	r.Route("/api/connections", func(r kitrest.Router) {
		r.Post("", kitrest.NewJsonApiCreateHandler(
			c.connections, api.ConnectionFromDTO, api.ConnectionToDTO,
			kitrest.HandlerWithOpenAPI(
				openapi.Summary("Create a connection"),
				openapi.Description("Authorizes an owner's instance of a connector. owner is required."),
				openapi.Tags("connections"), openapi.Errors(400),
			),
		))
		r.Get("", kitrest.NewJsonApiListHandler(
			c.connections, api.ConnectionToDTO,
			kitrest.HandlerWithOpenAPI(
				openapi.Summary("List connections"),
				openapi.Description("Filter with filter[owner] and filter[status]."),
				openapi.Tags("connections"),
			),
		))
		r.Route("/{id}", func(r kitrest.Router) {
			r.Get("", kitrest.NewJsonApiGetHandler(
				c.connections, api.ConnectionToDTO, []query.ParseOpt{},
				kitrest.HandlerWithOpenAPI(openapi.Summary("Get a connection"), openapi.Tags("connections"), openapi.Errors(404)),
			))
			r.Delete("", kitrest.NewJsonApiDeleteHandler(
				c.connections, repository.DeleteTypeHard,
				kitrest.HandlerWithOpenAPI(openapi.Summary("Delete a connection"), openapi.Tags("connections"), openapi.Errors(404)),
			))
			r.Post("/credentials", kitrest.NewJsonApiCommandHandler(
				c.storeCredential, c.decodeStoreCredential, api.CredentialToDTO,
				kitrest.HandlerWithOpenAPI(
					openapi.Summary("Store a connection's credential"),
					openapi.Description("Seals the provided secret into the vault; the plaintext is never persisted or returned."),
					openapi.Tags("connections"), openapi.Errors(400, 404),
				),
			))
		})
	})
}

type storeCredentialCommand struct {
	ConnectionID string
	Kind         domain.CredentialKind
	Secret       app.Secret
}

func (c *ConnectionController) storeCredential(ctx context.Context, cmd storeCredentialCommand) (domain.Credential, error) {
	return c.tokens.StoreCredential(ctx, cmd.ConnectionID, cmd.Kind, cmd.Secret)
}

func (c *ConnectionController) decodeStoreCredential(req *http.Request) (storeCredentialCommand, error) {
	body, err := kitrest.UnmarshalPayloadFromRequest[*api.CredentialInputDTO](req)
	if err != nil {
		return storeCredentialCommand{}, err
	}
	return storeCredentialCommand{
		ConnectionID: req.PathValue("id"),
		Kind:         domain.CredentialKind(body.RKind),
		Secret: app.Secret{
			AccessToken:  body.RAccessToken,
			RefreshToken: body.RRefreshToken,
			APIKey:       body.RAPIKey,
			APISecret:    body.RAPISecret,
			ExpiresAt:    body.RExpiresAt,
		},
	}, nil
}
