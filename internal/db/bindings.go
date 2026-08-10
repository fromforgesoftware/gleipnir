package db

import (
	"github.com/fromforgesoftware/gleipnir/internal/app"
)

// Compile-time proof that each repository satisfies the interfaces fx binds it to.
//
// This file exists because of a real outage-shaped mistake: a method was added to
// app.ConnectionRepository and not to *connectionRepo. Everything passed — `go build`, `go vet`,
// `golangci-lint`, and the whole unit suite, because the tests use a fake that DID implement it.
// fx.As resolves interfaces by REFLECTION at startup, so the only thing that noticed was the
// container, in a crash loop, after the image was built, released and deployed.
//
// These assertions turn that into a build error. They cost nothing at runtime — the compiler
// discards them — and they must name every interface the fx annotations in internal/module.go bind
// each repository to. When one of those annotations gains an interface, add it here too.
var (
	_ app.ConnectionRepository = (*connectionRepo)(nil)
	_ app.ConnectionStore      = (*connectionRepo)(nil)
	_ app.CredentialStore      = (*credentialRepo)(nil)
	_ app.OAuthStateStore      = (*oauthStateRepo)(nil)
)
