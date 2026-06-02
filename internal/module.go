// Package internal wires Gleipnir's components into a single fx module that
// cmd/server composes alongside the kit's defaults.
package internal

import (
	"context"
	"os"
	"time"

	"go.uber.org/fx"

	"github.com/fromforgesoftware/go-kit/httpclient"
	"github.com/fromforgesoftware/go-kit/monitoring/logger"
	"github.com/fromforgesoftware/go-kit/ratelimit"
	kitgrpc "github.com/fromforgesoftware/go-kit/transport/grpc"
	kitrest "github.com/fromforgesoftware/go-kit/transport/rest"

	"github.com/fromforgesoftware/gleipnir/internal/app"
	"github.com/fromforgesoftware/gleipnir/internal/db"
	gleipnirgrpc "github.com/fromforgesoftware/gleipnir/internal/transport/grpc"
	gleipnirhttp "github.com/fromforgesoftware/gleipnir/internal/transport/http"
	"github.com/fromforgesoftware/gleipnir/internal/vault"
)

const Version = "0.1.0"

// refreshInterval is how often the refresh-ahead cron runs; refreshBatch caps
// each tick's claim of soon-to-expire credentials.
const (
	refreshInterval = 1 * time.Minute
	refreshBatch    = 100
)

func FxModule() fx.Option {
	return fx.Module("gleipnir",
		cryptoFxModule(),
		registryFxModule(),
		repositoriesFxModule(),
		outboundFxModule(),
		usecasesFxModule(),
		transportFxModule(),
	)
}

// newVault selects the KEK provider: a Cloud KMS crypto key when
// GLEIPNIR_KMS_KEY is set (production), otherwise the local env KEK (dev). The
// KMS client connection is closed on shutdown.
func newVault(lc fx.Lifecycle) (app.Sealer, error) {
	if keyName := os.Getenv("GLEIPNIR_KMS_KEY"); keyName != "" {
		provider, err := vault.NewGCPKMSKeyProvider(context.Background(), keyName)
		if err != nil {
			return nil, err
		}
		lc.Append(fx.Hook{OnStop: func(context.Context) error { return provider.Close() }})
		return vault.New(provider), nil
	}
	provider, err := vault.NewLocalKeyProviderFromEnv()
	if err != nil {
		return nil, err
	}
	return vault.New(provider), nil
}

func cryptoFxModule() fx.Option {
	return fx.Module("gleipnir:crypto", fx.Provide(newVault))
}

func registryFxModule() fx.Option {
	return fx.Module("gleipnir:registry",
		fx.Provide(
			app.NewDefaultConnectorRegistry,
			app.NewConnectorCatalog,
		),
	)
}

func repositoriesFxModule() fx.Option {
	return fx.Module("gleipnir:repositories",
		fx.Provide(
			fx.Annotate(db.NewConnectionRepository,
				fx.As(new(app.ConnectionRepository)), fx.As(new(app.ConnectionStore))),
			fx.Annotate(db.NewCredentialRepository, fx.As(new(app.CredentialStore))),
			fx.Annotate(db.NewOAuthStateRepository, fx.As(new(app.OAuthStateStore))),
		),
	)
}

func newTokenUsecase(
	conns app.ConnectionStore,
	creds app.CredentialStore,
	sealer app.Sealer,
	registry app.ConnectorRegistry,
	exchanger app.TokenExchanger,
	notifier app.ReauthNotifier,
) *app.TokenUsecase {
	return app.NewTokenUsecase(conns, creds, sealer, registry, exchanger, notifier)
}

func newHTTPClient() *httpclient.Client {
	return httpclient.New(httpclient.WithTimeout(15*time.Second), httpclient.WithRetries(2))
}

func newLimiter() ratelimit.Limiter {
	return ratelimit.New(ratelimit.NewInMemoryStore())
}

// newExchanger builds the HTTP exchanger that serves both the refresh
// (TokenExchanger) and authorization_code (CodeExchanger) grants, so both share
// the per-connector rate limiter.
func newExchanger(client *httpclient.Client, limiter ratelimit.Limiter, secrets app.ClientSecrets) *app.HTTPExchanger {
	return app.NewHTTPExchanger(client, limiter, secrets)
}

func outboundFxModule() fx.Option {
	return fx.Module("gleipnir:outbound",
		fx.Provide(
			newHTTPClient,
			newLimiter,
			app.NewEnvClientSecrets,
			app.NewEnvRedirectAllowlist,
			fx.Annotate(newExchanger,
				fx.As(new(app.TokenExchanger)), fx.As(new(app.CodeExchanger))),
		),
	)
}

func usecasesFxModule() fx.Option {
	return fx.Module("gleipnir:usecases",
		fx.Provide(
			fx.Annotate(app.NewLogNotifier, fx.As(new(app.ReauthNotifier))),
			app.NewConnectionUsecase,
			newTokenUsecase,
			newOAuthUsecase,
		),
	)
}

func newOAuthUsecase(
	conns app.ConnectionStore,
	states app.OAuthStateStore,
	registry app.ConnectorRegistry,
	exchanger app.CodeExchanger,
	tokens *app.TokenUsecase,
	secrets app.ClientSecrets,
	redirects app.RedirectAllowlist,
) *app.OAuthUsecase {
	return app.NewOAuthUsecase(conns, states, registry, exchanger, tokens, secrets, redirects)
}

func transportFxModule() fx.Option {
	return fx.Module("gleipnir:transport",
		kitrest.NewFxMiddleware(kitrest.NewGatewayMiddleware),
		kitrest.NewFxController(gleipnirhttp.NewConnectorController),
		kitrest.NewFxController(gleipnirhttp.NewConnectionController),
		kitrest.NewFxController(gleipnirhttp.NewOAuthController),
		kitgrpc.NewFxController(gleipnirgrpc.NewGleipnirController),
		fx.Invoke(registerRefreshCron),
	)
}

// registerRefreshCron runs the refresh-ahead loop for the life of the process,
// keeping near-expiry tokens warm so the vend hot path rarely refreshes inline.
func registerRefreshCron(lc fx.Lifecycle, tokens *app.TokenUsecase) {
	ctx, cancel := context.WithCancel(context.Background())
	log := logger.New()
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go runRefreshCron(ctx, tokens, log)
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	})
}

func runRefreshCron(ctx context.Context, tokens *app.TokenUsecase, log logger.Logger) {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := tokens.RefreshDue(ctx, refreshBatch); err != nil {
				log.ErrorContext(ctx, "refresh-ahead failed", "error", err)
			}
		}
	}
}
