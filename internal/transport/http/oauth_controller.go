package http

import (
	"context"
	"net/http"

	"github.com/fromforgesoftware/go-kit/openapi"
	"github.com/fromforgesoftware/go-kit/resource"
	kitrest "github.com/fromforgesoftware/go-kit/transport/rest"

	"github.com/fromforgesoftware/gleipnir/internal/api"
	"github.com/fromforgesoftware/gleipnir/internal/app"
	"github.com/fromforgesoftware/gleipnir/internal/domain"
)

// OAuthController exposes the OAuth2 authorization_code flow as sub-resources of
// a connection:
//
//	POST /api/connections/{id}/oauth/authorize → provider redirect URL (+ state/PKCE)
//	POST /api/connections/{id}/oauth/callback  → exchange code, seal tokens
//
// Tokens are sealed into the vault on callback and never returned; the callback
// response carries only the authorized connection's id/status.
type OAuthController struct {
	oauth *app.OAuthUsecase
}

func NewOAuthController(oauth *app.OAuthUsecase) kitrest.Controller {
	return &OAuthController{oauth: oauth}
}

func (c *OAuthController) Routes(r kitrest.Router) {
	r.Route("/api/connections/{id}/oauth", func(r kitrest.Router) {
		r.Post("/authorize", kitrest.NewJsonApiCommandHandler(
			c.authorize, c.decodeAuthorize, identityDTO[*api.OAuthAuthorizationDTO],
			kitrest.HandlerWithSuccessStatus(http.StatusOK),
			kitrest.HandlerWithOpenAPI(
				openapi.Summary("Start an OAuth2 authorization"),
				openapi.Description("Builds the provider authorize URL with a single-use CSRF state (and PKCE challenge where supported). redirectUri must be on the configured allowlist."),
				openapi.Tags("oauth"), openapi.Errors(400, 404),
			),
		))
		r.Post("/callback", kitrest.NewJsonApiCommandHandler(
			c.callback, c.decodeCallback, identityDTO[*api.OAuthCallbackDTO],
			kitrest.HandlerWithSuccessStatus(http.StatusOK),
			kitrest.HandlerWithOpenAPI(
				openapi.Summary("Complete an OAuth2 authorization"),
				openapi.Description("Validates the state, exchanges the code at the provider, and seals the resulting tokens. Tokens are never returned."),
				openapi.Tags("oauth"), openapi.Errors(400, 404),
			),
		))
	})
}

// identityDTO is the encoder for handlers whose command already produces the
// wire DTO.
func identityDTO[T resource.Resource](dto T) T { return dto }

type authorizeCommand struct {
	Owner        string
	ConnectionID string
	RedirectURI  string
}

func (c *OAuthController) authorize(ctx context.Context, cmd authorizeCommand) (*api.OAuthAuthorizationDTO, error) {
	url, err := c.oauth.AuthorizeStart(ctx, cmd.Owner, cmd.ConnectionID, cmd.RedirectURI)
	if err != nil {
		return nil, err
	}
	dto := &api.OAuthAuthorizationDTO{RRedirectURL: url}
	dto.RID = cmd.ConnectionID
	dto.RType = api.ResourceTypeOAuthAuthorization
	return dto, nil
}

func (c *OAuthController) decodeAuthorize(req *http.Request) (authorizeCommand, error) {
	body, err := kitrest.UnmarshalPayloadFromRequest[*api.OAuthAuthorizeInputDTO](req)
	if err != nil {
		return authorizeCommand{}, err
	}
	return authorizeCommand{
		Owner:        req.URL.Query().Get("owner"),
		ConnectionID: req.PathValue("id"),
		RedirectURI:  body.RRedirectURI,
	}, nil
}

type callbackCommand struct {
	State string
	Code  string
}

func (c *OAuthController) callback(ctx context.Context, cmd callbackCommand) (*api.OAuthCallbackDTO, error) {
	res, err := c.oauth.Callback(ctx, cmd.State, cmd.Code)
	if err != nil {
		return nil, err
	}
	dto := &api.OAuthCallbackDTO{
		RConnectionID: res.Connection.ID(),
		RStatus:       string(res.Connection.Status()),
	}
	dto.RID = res.Connection.ID()
	dto.RType = api.ResourceTypeOAuthCallback
	if res.Credential != nil {
		dto.RCredential = resource.RelationshipToDTO(
			resource.RelFromIdentifier(resource.NewIdentifier(res.Credential.ID(), domain.ResourceTypeCredential)))
	}
	return dto, nil
}

func (c *OAuthController) decodeCallback(req *http.Request) (callbackCommand, error) {
	body, err := kitrest.UnmarshalPayloadFromRequest[*api.OAuthCallbackInputDTO](req)
	if err != nil {
		return callbackCommand{}, err
	}
	return callbackCommand{State: body.RState, Code: body.RCode}, nil
}
