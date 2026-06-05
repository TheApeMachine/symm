package private

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/symm/kraken/public"
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
	return newTokenProvider(ctx, apiKey, apiSecret, false)
}

/*
NewLiveTokenProvider caches tokens against the live Kraken REST API even when
trading.model is paper.
*/
func NewLiveTokenProvider(ctx context.Context, apiKey, apiSecret string) (*TokenProvider, error) {
	return newTokenProvider(ctx, apiKey, apiSecret, true)
}

func newTokenProvider(
	ctx context.Context,
	apiKey, apiSecret string,
	liveREST bool,
) (*TokenProvider, error) {
	ctx, cancel := context.WithCancel(ctx)

	var (
		rest *Rest
		err  error
	)

	if liveREST {
		rest, err = NewLiveRest(ctx, apiKey, apiSecret, public.EndpointWebSocketsToken)
	} else {
		rest, err = NewRest(ctx, apiKey, apiSecret, public.EndpointWebSocketsToken)
	}

	if err != nil {
		cancel()
		return nil, fmt.Errorf("kraken/private: token provider rest: %w", err)
	}

	provider := &TokenProvider{
		ctx:    ctx,
		cancel: cancel,
		rest:   rest,
		token:  "",
		until:  time.Time{},
	}

	return provider, nil
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
