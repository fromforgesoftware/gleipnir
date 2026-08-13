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
	auth        kitrest.HTTPAuthenticator
}

func NewConnectionController(
	connections app.ConnectionUsecase, tokens *app.TokenUsecase, auth kitrest.HTTPAuthenticator,
) kitrest.Controller {
	return &ConnectionController{connections: connections, tokens: tokens, auth: auth}
}

func (c *ConnectionController) Routes(r kitrest.Router) {
	r.Route("/api/connections", func(r kitrest.Router) {
		// Authentication for the whole group, applied here rather than per route so a route added
		// later is protected by default. Nothing under /api/connections is public: even the list is
		// a statement about which venues a workspace trades with.
		//
		// This is not belt-and-braces over an existing check. The kit's GatewayMiddleware is also
		// installed, but it is a NO-OP unless FORGE_GATEWAY_SECRET is set, and it is not set in any
		// deployment of this service — a security middleware whose unconfigured default is "allow"
		// looked like protection and provided none.
		r.Use(kitrest.NewAuthMiddleware(c.auth))
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
			// Enabling/disabling, so a UI toggle need not destroy the sealed credential to turn a
			// connection off. DELETE remains the destructive option and is a different verb for a
			// different intent.
			r.Patch("", kitrest.NewJsonApiCommandHandler(
				c.setStatus, c.decodeSetStatus, api.ConnectionToDTO,
				kitrest.HandlerWithOpenAPI(
					openapi.Summary("Enable or disable a connection"),
					openapi.Description("Sets status to ACTIVE or REVOKED, leaving the stored credential intact."),
					openapi.Tags("connections"), openapi.Errors(400, 401, 404),
				),
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
	// Ownership BEFORE sealing. StoreCredential takes a connection id and asks nothing about who is
	// calling, so without this an authenticated caller could overwrite the credential another
	// workspace's live strategy vends — pointing its orders at keys of the caller's choosing.
	if _, err := c.connections.OwnedConnection(ctx, cmd.ConnectionID); err != nil {
		return nil, err
	}
	return c.tokens.StoreCredential(ctx, cmd.ConnectionID, cmd.Kind, cmd.Secret)
}

type setStatusCommand struct {
	ConnectionID string
	Status       domain.ConnectionStatus
}

func (c *ConnectionController) setStatus(
	ctx context.Context, cmd setStatusCommand,
) (domain.Connection, error) {
	return c.connections.SetStatus(ctx, cmd.ConnectionID, cmd.Status)
}

func (c *ConnectionController) decodeSetStatus(req *http.Request) (setStatusCommand, error) {
	body, err := kitrest.UnmarshalPayloadFromRequest[*api.ConnectionDTO](req)
	if err != nil {
		return setStatusCommand{}, err
	}
	return setStatusCommand{
		ConnectionID: req.PathValue("id"),
		Status:       domain.ConnectionStatus(body.RStatus),
	}, nil
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
			Fields:       body.RFields,
		},
	}, nil
}
