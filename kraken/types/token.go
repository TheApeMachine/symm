package types

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

const tokenRefreshLead = 30 * time.Second

var (
	cachedToken atomic.Pointer[Token]
	tokenRest   TokenRest
)

/*
TokenRest fetches a Kraken authenticated websocket token.
*/
type TokenRest interface {
	WebSocketToken(context.Context, *Token) error
}

/*
BindTokenRest installs the REST client used by NewToken.
*/
func BindTokenRest(rest TokenRest) {
	tokenRest = rest
}

/*
Token is Kraken's short-lived authenticated websocket credential.
*/
type Token struct {
	Token   string `json:"token"`
	Expires int    `json:"expires"`
	until   time.Time
}

/*
NewToken returns the cached venue token string when it is still valid,
otherwise requests a fresh one from Kraken and caches it.
*/
func NewToken(ctx context.Context) (string, error) {
	if tokenRest == nil {
		return "", fmt.Errorf("types: token rest not bound")
	}

	now := time.Now()

	if token := cachedToken.Load(); token != nil && token.valid(now) {
		return token.Token, nil
	}

	token := &Token{}

	if err := tokenRest.WebSocketToken(ctx, token); err != nil {
		return "", err
	}

	if token.Token == "" {
		return "", fmt.Errorf("types: empty websockets token")
	}

	expires := time.Duration(token.Expires) * time.Second

	if expires <= 0 {
		expires = 15 * time.Minute
	}

	token.until = now.Add(expires)
	cachedToken.Store(token)

	return token.Token, nil
}

func (token *Token) valid(now time.Time) bool {
	if token.Token == "" {
		return false
	}

	if token.until.IsZero() {
		expires := time.Duration(token.Expires) * time.Second

		if expires <= 0 {
			expires = 15 * time.Minute
		}

		token.until = now.Add(expires)
	}

	return now.Before(token.until.Add(-tokenRefreshLead))
}
