package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fromforgesoftware/gleipnir/internal/app"
)

func TestRedirectAllowlist_AllowAndDeny(t *testing.T) {
	al := app.NewRedirectAllowlist(
		"https://app.example.com/oauth/callback",
		" https://spaced.example.com/cb ", // trimmed
	)

	assert.True(t, al.Allowed("https://app.example.com/oauth/callback"))
	assert.True(t, al.Allowed("https://spaced.example.com/cb"))

	// Exact-match only: no prefix, suffix, host, or scheme slack.
	assert.False(t, al.Allowed("https://app.example.com/oauth/callback?x=1"))
	assert.False(t, al.Allowed("https://app.example.com/oauth/callback/"))
	assert.False(t, al.Allowed("http://app.example.com/oauth/callback"))
	assert.False(t, al.Allowed("https://evil.example.com/oauth/callback"))
	assert.False(t, al.Allowed(""))
}

func TestRedirectAllowlist_EmptyFailsClosed(t *testing.T) {
	al := app.NewRedirectAllowlist()
	assert.False(t, al.Allowed("https://app.example.com/cb"), "empty allowlist must deny everything")
}
