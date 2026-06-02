package app

import (
	"os"
	"strings"
)

// envClientSecrets reads a connector's OAuth app credentials from the
// environment: GLEIPNIR_OAUTH_<SLUG>_CLIENT_ID / _CLIENT_SECRET (slug upper-cased).
type envClientSecrets struct{}

func NewEnvClientSecrets() ClientSecrets { return envClientSecrets{} }

func (envClientSecrets) ClientCredentials(slug string) (string, string, bool) {
	up := strings.ToUpper(slug)
	id := os.Getenv("GLEIPNIR_OAUTH_" + up + "_CLIENT_ID")
	secret := os.Getenv("GLEIPNIR_OAUTH_" + up + "_CLIENT_SECRET")
	if id == "" && secret == "" {
		return "", "", false
	}
	return id, secret, true
}
