package kraken

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
)

/*
Token fetch backoff bounds. They are the websocket redial bounds
(network.WebsocketClient: one second doubling to thirty), so a token fetch
behind a reconnect retries on the same schedule as the socket it serves.
*/
var (
	TokenRetryFloor   = time.Second
	TokenRetryCeiling = 30 * time.Second
)

/*
permanentErrors are Kraken REST errors that retrying with the same
credentials cannot clear.
*/
var permanentErrors = []string{
	"EAPI:Invalid key",
	"EAPI:Invalid signature",
	"EAPI:Feature disabled",
	"EGeneral:Permission denied",
	"EGeneral:Invalid arguments",
}

/*
Permanent reports whether err is a Kraken REST error that retrying with the
same credentials cannot clear. Everything else, an invalid nonce, a rate
limit, a temporary lockout, an unavailable service or a network failure, is
transient.
*/
func Permanent(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()

	for _, code := range permanentErrors {
		if strings.Contains(message, code) {
			return true
		}
	}

	return false
}

/*
RetryToken calls fetch until it returns a token, a permanent error, or ctx
ends, waiting TokenRetryFloor doubling to TokenRetryCeiling between attempts.
Every failed attempt is logged as an error naming label and the attempt.
*/
func RetryToken(ctx context.Context, label string, fetch func() (string, error)) (string, error) {
	wait := TokenRetryFloor

	for attempt := 1; ; attempt++ {
		token, err := fetch()

		if err == nil && token == "" {
			err = errnie.Err(errnie.IO, "empty websockets token", nil)
		}

		if err == nil {
			if attempt > 1 {
				errnie.Info(fmt.Sprintf("[kraken.token] %s: token fetched on attempt %d", label, attempt))
			}

			return token, nil
		}

		if Permanent(err) {
			return "", errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				fmt.Sprintf("[kraken.token] %s: permanent token error on attempt %d", label, attempt),
				err,
			))
		}

		errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("[kraken.token] %s: token fetch failed on attempt %d, retrying in %s", label, attempt, wait),
			err,
		))

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}

		wait = min(2*wait, TokenRetryCeiling)
	}
}
