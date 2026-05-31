package private

import (
	"context"
	"sync"
	"time"
)

const tokenRefreshLead = 30 * time.Second

/*
TokenProvider caches Kraken WebSocket auth tokens for market-data channels (L3)
without opening a live trading session.
*/
type TokenProvider struct {
	rest  *Rest
	mu    sync.Mutex
	token string
	until time.Time
}

/*
NewTokenProvider builds a token cache from API credentials.
*/
func NewTokenProvider(apiKey, apiSecret string) (*TokenProvider, error) {
	rest, err := NewRest(apiKey, apiSecret)

	if err != nil {
		return nil, err
	}

	return &TokenProvider{rest: rest}, nil
}

/*
Token returns a valid WebSocket token, refreshing when near expiry.
*/
func (provider *TokenProvider) Token(ctx context.Context) (string, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()

	if provider.token != "" && time.Now().Before(provider.until.Add(-tokenRefreshLead)) {
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
