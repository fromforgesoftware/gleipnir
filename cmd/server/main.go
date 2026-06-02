// Command server boots the Gleipnir integration service: the kit's REST gateway
// (OpenAPI 3.1) plus a gRPC server exposing TokenService.
package main

import (
	"github.com/fromforgesoftware/go-kit/app"
	"github.com/fromforgesoftware/go-kit/openapi"
	"github.com/fromforgesoftware/go-kit/persistence/gormdb/gormpg"
	kitgrpc "github.com/fromforgesoftware/go-kit/transport/grpc"

	"github.com/fromforgesoftware/gleipnir/internal"
)

func main() {
	app.Run(
		app.WithName("gleipnir"),
		app.WithVersion(internal.Version),
		app.WithOpenAPI(
			openapi.SpecTitle("Gleipnir"),
			openapi.SpecVersion(internal.Version),
			openapi.SpecDescription("Forge broker/exchange integration hub: connections, sealed credentials, and S2S token vending."),
		),
		gormpg.FxModule(),
		kitgrpc.FxModule(),
		internal.FxModule(),
	)
}
