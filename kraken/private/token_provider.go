package private

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
)

const tokenRefreshLead = 30 * time.Second

/*
TokenProvider caches Kraken WebSocket auth tokens for market-data channels (L3)
without opening a live trading session.
*/
type TokenProvider struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	rest   *Rest
	token  string
	until  time.Time
}

/*
NewTokenProvider builds a token cache from API credentials.
*/
func NewTokenProvider(ctx context.Context, apiKey, apiSecret string) (*TokenProvider, error) {
	ctx, cancel := context.WithCancel(ctx)

	provider := &TokenProvider{
		ctx:    ctx,
		cancel: cancel,
		token:  "",
		until:  time.Time{},
	}

	provider.rest = errnie.Does(func() (*Rest, error) {
		return NewRest(ctx, apiKey, apiSecret, EndpointWebSocketsToken)
	}).Or(func(err error) {
		provider.err = errnie.Error(err)
	}).Value()

	return provider, errnie.Error(errnie.Require(map[string]any{
		"rest":   provider.rest,
		"token":  provider.token,
		"until":  provider.until,
		"ctx":    provider.ctx,
		"cancel": provider.cancel,
	}))
}

/*
Token returns a valid WebSocket token, refreshing when near expiry.
*/
func (provider *TokenProvider) Token(ctx context.Context) (string, error) {
	if provider.token != "" && time.Now().Before(
		provider.until.Add(-tokenRefreshLead),
	) {
		return provider.token, nil
	}

	token, expires, err := provider.rest.WebSocketToken(ctx)

	if err != nil {
		return "", err
	}

	provider.token = token
	provider.until = time.Now().Add(expires)

	return provider.token, nil
}
