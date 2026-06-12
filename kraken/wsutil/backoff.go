package wsutil

import (
	"context"
	"time"

	"github.com/spf13/viper"
)

const defaultReconnectMultiplier = 2

/*
Backoff carries context-aware reconnect delay settings.
*/
type Backoff struct {
	Initial    time.Duration
	Max        time.Duration
	Multiplier float64
}

func NewBackoffFromConfig() Backoff {
	backoff := Backoff{
		Initial:    viper.GetDuration("market.ws_reconnect_initial"),
		Max:        viper.GetDuration("market.ws_reconnect_max"),
		Multiplier: viper.GetFloat64("market.ws_reconnect_multiplier"),
	}

	if backoff.Initial <= 0 {
		backoff.Initial = time.Second
	}

	if backoff.Max <= 0 {
		backoff.Max = backoff.Initial
	}

	if backoff.Multiplier <= 1 {
		backoff.Multiplier = defaultReconnectMultiplier
	}

	return backoff
}

func (backoff Backoff) Delay(attempt uint64) time.Duration {
	delay := backoff.Initial

	for attemptIndex := uint64(0); attemptIndex < attempt; attemptIndex++ {
		nextDelay := time.Duration(float64(delay) * backoff.Multiplier)

		if nextDelay >= backoff.Max {
			return backoff.Max
		}

		delay = nextDelay
	}

	if delay > backoff.Max {
		return backoff.Max
	}

	return delay
}

func (backoff Backoff) Wait(ctx context.Context, attempt uint64) error {
	return Wait(ctx, backoff.Delay(attempt))
}

func NonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}

	return ctx
}

func Wait(ctx context.Context, delay time.Duration) error {
	ctx = NonNilContext(ctx)

	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
