package kraken

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/bytedance/sonic"
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
NewAuthenticatedREST returns a spot REST client signed with the API keys of
the process's credential role (UseCredentials) and the process-wide nonce generator. A generator that cannot be built
(unreadable or corrupt high-water file) is an error, never a fall back to the
SDK's own nonce: that would race the shared generator, and every private call
and Level3 token fetch depends on nonces staying strictly increasing.
*/
func NewAuthenticatedREST() (*spot.REST, error) {
	keys, err := processKeys()

	if err != nil {
		return nil, err
	}

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
	restClient.PublicKey = keys.key
	restClient.PrivateKey = keys.secret
	restClient.Nonce = nonce.Next

	return restClient, nil
}

/*
SignPrivate builds the JSON body and API-Key / API-Sign for one Kraken spot
private REST call. requestURI is the path plus query, the same string Kraken
hashes. The nonce is the process-wide generator unless body already carries one.
*/
func SignPrivate(requestURI string, body any) (key, signature string, signed []byte, err error) {
	keys, err := processKeys()

	if err != nil {
		return "", "", nil, err
	}

	nonceGen, err := ProcessAuthNonce()

	if err != nil {
		return "", "", nil, errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.auth] process auth nonce unavailable",
			err,
		))
	}

	if nonceGen == nil {
		return "", "", nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[kraken.auth] process auth nonce missing",
			nil,
		))
	}

	if err := nonceGen.Err(); err != nil {
		return "", "", nil, errnie.Error(errnie.Err(
			errnie.IO,
			"[kraken.auth] auth nonce high-water not persisted",
			err,
		))
	}

	signed, nonce, err := bodyWithNonce(body, nonceGen.Next)

	if err != nil {
		return "", "", nil, err
	}

	signature, err = spot.Sign(keys.secret, requestURI, nonce, io.NopCloser(bytes.NewReader(signed)))

	if err != nil {
		return "", "", nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[kraken.auth] private request sign failed",
			err,
		))
	}

	return keys.key, signature, signed, nil
}

func bodyWithNonce(body any, next func() string) ([]byte, string, error) {
	payload := map[string]any{}

	if body != nil {
		encoded, err := sonic.Marshal(body)

		if err != nil {
			return nil, "", errnie.Err(
				errnie.UnprocessableContent,
				"[kraken.auth] private body is not JSON",
				err,
			)
		}

		if !bytes.Equal(encoded, []byte("null")) {
			if err := sonic.Unmarshal(encoded, &payload); err != nil {
				return nil, "", errnie.Err(
					errnie.UnprocessableContent,
					"[kraken.auth] private body must be a JSON object",
					err,
				)
			}
		}
	}

	nonce := fmt.Sprint(payload["nonce"])

	if nonce == "" || nonce == "<nil>" {
		nonce = next()
		payload["nonce"] = nonce
	}

	signed, err := sonic.Marshal(payload)

	if err != nil {
		return nil, "", errnie.Err(
			errnie.UnprocessableContent,
			"[kraken.auth] private body is not JSON",
			err,
		)
	}

	return signed, nonce, nil
}

/*
Token returns a websockets authentication token young enough to subscribe
with: the cached token while it is within TokenReuse of issue, otherwise a
fresh one fetched through authenticated REST. Reconnect hooks therefore get a
token the venue will accept, not the one issued at process start. A failed or
empty fetch is returned at once: Token never retries; the websocket redial
loop (network.WebsocketClient's reconnect hook backoff) owns retrying.
*/
func (auth *Auth) Token(ctx context.Context) (string, error) {
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

	if err := ctx.Err(); err != nil {
		return "", errnie.Error(errnie.Err(errnie.IO, "[kraken.auth] token fetch cancelled", err))
	}

	token, err := fetch()

	if err != nil {
		return "", errnie.Error(errnie.Err(errnie.IO, "[kraken.auth] websockets token fetch failed", err))
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
