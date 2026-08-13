// Package client is the consumer-facing SDK for Gleipnir's gRPC surface. Other forge services dial
// Gleipnir to vend a connection's current provider secret; returned gRPC status codes are mapped to kit
// apierrors.
package client

import (
	"context"
	"time"

	"google.golang.org/grpc"

	apierrors "github.com/fromforgesoftware/go-kit/errors"
	gleipnirv1 "github.com/fromforgesoftware/gleipnir/pkg/api/gleipnir/v1"
)

// Secret is the SDK-facing shape of a vended credential.
//
// Which fields are populated depends on the connector's auth type: an OAuth connector fills AccessToken,
// an API-key connector fills APIKey and APISecret, and a venue whose login needs more than that fills
// Fields. Refresh tokens are never returned — they stay sealed inside Gleipnir, which is what keeps a
// compromised caller unable to mint new sessions.
type Secret struct {
	// AccessToken is the OAuth bearer token, empty for API-key connectors.
	AccessToken string
	// APIKey and APISecret are the key pair, empty for OAuth connectors.
	APIKey    string
	APISecret string
	// Fields carries the credential material that does not fit a key and a secret — an account login, a
	// device id, comp ids. The names are the connector's own; Gleipnir does not interpret them.
	Fields map[string]string
	// Connector is the connection's connector slug, which is how a caller holding only a connection id
	// knows which adapter to build.
	Connector string
	// ExpiresAt is when the vended secret expires, or the zero time when the connector does not say.
	// API-key credentials generally do not expire.
	ExpiresAt time.Time
}

// Field returns one of the extra credential fields, and whether it was present.
//
// Distinguishing absent from empty matters for the venues that use this: a device id that is present but
// empty is a misconfiguration worth reporting, while one that is absent may be a connector that does not
// use device ids at all.
func (s Secret) Field(name string) (string, bool) {
	v, ok := s.Fields[name]
	return v, ok
}

// Client wraps Gleipnir's gRPC surface with kit error mapping.
type Client struct {
	tokens gleipnirv1.TokenServiceClient
}

func New(conn grpc.ClientConnInterface) *Client {
	return &Client{tokens: gleipnirv1.NewTokenServiceClient(conn)}
}

// NewFromServiceClient is the seam tests use to inject a fake gRPC client.
func NewFromServiceClient(c gleipnirv1.TokenServiceClient) *Client {
	return &Client{tokens: c}
}

// Vend returns the connection's current secret, refreshed if it was close to expiry.
//
// The owner is the opaque owner key the CALLER asserts, and it is checked against the connection's own
// owner: a mismatch is a not-found, never a secret. That is the whole trust boundary of this call — it is
// service-to-service, so there is no user token to derive access from, and a caller that could vend any
// connection by id could vend everyone's.
func (c *Client) Vend(ctx context.Context, owner, connectionID string) (Secret, error) {
	resp, err := c.tokens.Vend(ctx, &gleipnirv1.VendRequest{
		Owner:        owner,
		ConnectionId: connectionID,
	})
	if err != nil {
		return Secret{}, apierrors.FromGRPCError(err)
	}

	out := Secret{
		AccessToken: resp.GetAccessToken(),
		APIKey:      resp.GetApiKey(),
		APISecret:   resp.GetApiSecret(),
		Fields:      resp.GetFields(),
		Connector:   resp.GetConnector(),
	}
	if ts := resp.GetExpiresAt(); ts != nil {
		out.ExpiresAt = ts.AsTime()
	}
	return out, nil
}
