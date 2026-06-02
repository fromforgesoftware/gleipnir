package app

import (
	"os"
	"strings"
)

// RedirectAllowlist decides whether a caller-supplied redirect_uri is one
// Gleipnir is willing to send to a provider and accept back on callback. An
// open redirect (reflecting any URI the caller passes) would let an attacker
// have the authorization code delivered to a host they control, so the URI is
// matched exactly against a configured allowlist.
type RedirectAllowlist interface {
	// Allowed reports whether uri is on the allowlist (exact match).
	Allowed(uri string) bool
}

// envRedirectAllowlist is the deployment allowlist, read once from
// GLEIPNIR_OAUTH_REDIRECT_URIS (comma-separated, exact-match). An empty
// allowlist denies everything — fail closed — so a misconfigured deployment
// cannot become an open redirect.
type envRedirectAllowlist struct {
	allowed map[string]struct{}
}

// NewEnvRedirectAllowlist reads the allowlist from the environment.
func NewEnvRedirectAllowlist() RedirectAllowlist {
	return NewRedirectAllowlist(splitURIs(os.Getenv("GLEIPNIR_OAUTH_REDIRECT_URIS"))...)
}

// NewRedirectAllowlist builds an allowlist from explicit URIs (tests / wiring).
func NewRedirectAllowlist(uris ...string) RedirectAllowlist {
	set := make(map[string]struct{}, len(uris))
	for _, u := range uris {
		if u = strings.TrimSpace(u); u != "" {
			set[u] = struct{}{}
		}
	}
	return envRedirectAllowlist{allowed: set}
}

func (a envRedirectAllowlist) Allowed(uri string) bool {
	_, ok := a.allowed[uri]
	return ok
}

func splitURIs(csv string) []string {
	if csv == "" {
		return nil
	}
	return strings.Split(csv, ",")
}
