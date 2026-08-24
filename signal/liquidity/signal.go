package liquidity

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Signal is the liquidity measuring instrument. It composes its market entities
in its constructor and exposes the canonical signal structure: Constructor,
Name, Error, Step, Close.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	ticker *Ticker
}

// NewSignal composes the Ticker (touch) entity. Full-book morphology is added
// as a further entity in a later pass.
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(),
	}
}

func (signal *Signal) Name() string { return "liquidity" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(ticker kraken.TickerData) *data.Measurement[float64] {
	return signal.ticker.Step(ticker)
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.ticker.Close()
}
