package kraken

import (
	"os"
	"strings"
	"sync/atomic"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

/*
Credential roles. Kraken checks nonces per API key, so processes that sign at
the same time (the collector and the main system) each use their own key
pair, named per role in kraken.credentials.
*/
const (
	RoleCollector = "collector"
	RoleMain      = "main"
)

/*
credentials is the key pair of this process's role, resolved once at startup
by UseCredentials.
*/
type credentials struct {
	role   string
	key    string
	secret string
}

var processCredentials atomic.Pointer[credentials]

/*
UseCredentials resolves the API key pair for role from the environment
variables named by kraken.credentials.<role>.key_env and .secret_env. The
config holds variable names only, never values. A missing name or an unset or
empty variable is an error: the process must not start signing with some
other role's key and desync that key's nonces.
*/
func UseCredentials(role string) error {
	key, err := credentialEnv(role, "key_env")

	if err != nil {
		return err
	}

	secret, err := credentialEnv(role, "secret_env")

	if err != nil {
		return err
	}

	processCredentials.Store(&credentials{role: role, key: key, secret: secret})

	return nil
}

func credentialEnv(role, field string) (string, error) {
	name := strings.TrimSpace(viper.GetString("kraken.credentials." + role + "." + field))

	if name == "" {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"[kraken.credentials] kraken.credentials."+role+"."+field+" is not configured",
			nil,
		))
	}

	value, ok := os.LookupEnv(name)

	if !ok || strings.TrimSpace(value) == "" {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"[kraken.credentials] environment variable "+name+" ("+role+" "+field+") is not set",
			nil,
		))
	}

	return value, nil
}

/*
processKeys returns the resolved key pair, or an error when UseCredentials
has not run.
*/
func processKeys() (*credentials, error) {
	resolved := processCredentials.Load()

	if resolved == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[kraken.credentials] no credential role selected (UseCredentials)",
			nil,
		))
	}

	return resolved, nil
}
