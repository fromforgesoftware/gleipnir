// Package grpc holds Gleipnir's gRPC controllers.
package grpc

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	kitgrpc "github.com/fromforgesoftware/go-kit/transport/grpc"

	"github.com/fromforgesoftware/gleipnir/internal/app"
	gleipnirv1 "github.com/fromforgesoftware/gleipnir/pkg/api/gleipnir/v1"
)

// gleipnirController serves the S2S TokenService: vending a connection's
// current secret to a trusted backend caller.
type gleipnirController struct {
	tokens *app.TokenUsecase
}

func NewGleipnirController(tokens *app.TokenUsecase) kitgrpc.Controller {
	return &gleipnirController{tokens: tokens}
}

func (c *gleipnirController) SD() kitgrpc.ServiceDesc {
	return &gleipnirv1.TokenService_ServiceDesc
}

func (c *gleipnirController) Vend(ctx context.Context, req *gleipnirv1.VendRequest) (*gleipnirv1.VendResponse, error) {
	secret, err := c.tokens.Vend(ctx, req.GetOwner(), req.GetConnectionId())
	if err != nil {
		return nil, err
	}
	resp := &gleipnirv1.VendResponse{
		AccessToken: secret.AccessToken,
		ApiKey:      secret.APIKey,
		ApiSecret:   secret.APISecret,
	}
	if secret.ExpiresAt != nil {
		resp.ExpiresAt = timestamppb.New(*secret.ExpiresAt)
	}
	return resp, nil
}
