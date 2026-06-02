package api

import (
	"github.com/fromforgesoftware/go-kit/resource"
)

// ResourceTypeOAuthAuthorization is the JSON:API type for the authorize-start
// response (the provider redirect URL).
const ResourceTypeOAuthAuthorization resource.Type = "oauthAuthorizations"

// ResourceTypeOAuthCallback is the JSON:API type for a completed callback
// exchange. It returns only the connection reference and status — never tokens.
const ResourceTypeOAuthCallback resource.Type = "oauthCallbacks"

// OAuthAuthorizeInputDTO is the authorize-start intake: the caller-supplied
// redirect_uri the provider should return to (validated against the allowlist).
type OAuthAuthorizeInputDTO struct {
	resource.RestDTO

	RRedirectURI string `jsonapi:"attr,redirectUri"`
}

// OAuthAuthorizationDTO carries the provider authorize URL to redirect to. It
// has no persistent identity; the type is set for JSON:API conformance.
type OAuthAuthorizationDTO struct {
	resource.RestDTO

	RRedirectURL string `jsonapi:"attr,redirectUrl,omitempty"`
}

// OAuthCallbackInputDTO is the callback intake: the provider-echoed state and
// authorization code.
type OAuthCallbackInputDTO struct {
	resource.RestDTO

	RState string `jsonapi:"attr,state"`
	RCode  string `jsonapi:"attr,code"`
}

// OAuthCallbackDTO is the read-safe callback result: the authorized
// connection's id and status, with the credential as a relationship. It never
// carries token material.
type OAuthCallbackDTO struct {
	resource.RestDTO

	RConnectionID string                    `jsonapi:"attr,connectionId,omitempty"`
	RStatus       string                    `jsonapi:"attr,status,omitempty"`
	RCredential   *resource.RelationshipDTO `jsonapi:"rel,credential,omitempty"`
}
