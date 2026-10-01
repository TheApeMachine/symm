package kraken

import (
	"os"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
)

type Auth struct {
	token atomic.Pointer[string]
}

func NewAuth() *Auth {
	return &Auth{}
}

/*
Token returns the cached Kraken websockets authentication token,
or fetches a fresh token using authenticated REST when uninitialized.
*/
func (auth *Auth) Token() (string, error) {
	if auth == nil {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"[kraken.auth] nil auth instance",
			nil,
		))
	}

	if ptr := auth.token.Load(); ptr != nil && *ptr != "" {
		return *ptr, nil
	}

	restClient := spot.NewREST()
	restClient.PublicKey = os.Getenv("KRAKEN_API_KEY")
	restClient.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

	if nonce, err := ProcessAuthNonce(); err == nil && nonce != nil {
		restClient.Nonce = nonce.Next
	}

	tokenRes, err := restClient.GetWebSocketsToken()
	if err != nil {
		return "", errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.auth] failed to retrieve websockets token",
			err,
		))
	}

	if tokenRes == nil || tokenRes.Result.Token == "" {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"[kraken.auth] empty websockets token received",
			nil,
		))
	}

	token := tokenRes.Result.Token
	auth.token.Store(&token)
	return token, nil
}
