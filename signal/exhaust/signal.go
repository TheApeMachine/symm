package exhaust

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Signal is the microstructure support-state measuring instrument. It composes
its market entity in its constructor and exposes the canonical signal
structure: Constructor, Name, Error, Step, Close.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	ticker *Ticker
}

/*
NewSignal composes the Ticker (touch) entity. No workspace is required: the
ticker entity consumes only the ticker stream.
*/
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(),
	}
}

func (signal *Signal) Name() string { return "exhaust" }

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
