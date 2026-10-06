package kraken

import (
	"os"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
)

/*
TokenReuse bounds how long one websockets token is presented for a new
private subscription. Kraken honours a token for establishing a subscription
only within 15 minutes of issue; an established connection keeps it, but a
reconnect after that window must present a fresh token or the venue rejects
the subscription and the private stream (executions, balances) goes dark.
*/
const TokenReuse = 10 * time.Minute

type Auth struct {
	mu       sync.Mutex
	token    string
	issuedAt time.Time
	now      func() time.Time
	fetch    func() (string, error)
}

func NewAuth() *Auth {
	return &Auth{now: time.Now, fetch: fetchWebSocketsToken}
}

/*
NewAuthenticatedREST returns a spot REST client signed with the process API
keys and the process-wide nonce generator. A generator that cannot be built
(unreadable or corrupt high-water file) is an error, never a fall back to the
SDK's own nonce: that would race the shared generator, and every private call
and Level3 token fetch depends on nonces staying strictly increasing.
*/
func NewAuthenticatedREST() (*spot.REST, error) {
	nonce, err := ProcessAuthNonce()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.auth] process auth nonce unavailable",
			err,
		))
	}

	if nonce == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[kraken.auth] process auth nonce missing",
			nil,
		))
	}

	if err := nonce.Err(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.auth] auth nonce high-water not persisted",
			err,
		))
	}

	restClient := spot.NewREST()
	restClient.PublicKey = os.Getenv("KRAKEN_API_KEY")
	restClient.PrivateKey = os.Getenv("KRAKEN_API_SECRET")
	restClient.Nonce = nonce.Next

	return restClient, nil
}

/*
Token returns a websockets authentication token young enough to subscribe
with: the cached token while it is within TokenReuse of issue, otherwise a
fresh one fetched through authenticated REST. Reconnect hooks therefore get a
token the venue will accept, not the one issued at process start.
*/
func (auth *Auth) Token() (string, error) {
	if auth == nil {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"[kraken.auth] nil auth instance",
			nil,
		))
	}

	auth.mu.Lock()
	defer auth.mu.Unlock()

	now := auth.now
	if now == nil {
		now = time.Now
	}

	if auth.token != "" && now().Sub(auth.issuedAt) < TokenReuse {
		return auth.token, nil
	}

	fetch := auth.fetch
	if fetch == nil {
		fetch = fetchWebSocketsToken
	}

	token, err := fetch()

	if err != nil {
		return "", err
	}

	if token == "" {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"[kraken.auth] empty websockets token received",
			nil,
		))
	}

	auth.token = token
	auth.issuedAt = now()

	return token, nil
}

func fetchWebSocketsToken() (string, error) {
	restClient, err := NewAuthenticatedREST()

	if err != nil {
		return "", err
	}

	tokenRes, err := restClient.GetWebSocketsToken()

	if err != nil {
		return "", errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.auth] failed to retrieve websockets token",
			err,
		))
	}

	if tokenRes == nil {
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"[kraken.auth] empty websockets token response",
			nil,
		))
	}

	return tokenRes.Result.Token, nil
}
