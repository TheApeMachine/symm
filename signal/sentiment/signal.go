package sentiment

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Signal is the cross-sectional price-state instrument. It composes its market
entity in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	ticker *Ticker
}

/*
NewSignal composes the Ticker (per-symbol price-state) entity, which adopts or
creates the shared cross-section in the workspace pool.
*/
func NewSignal(ctx context.Context, workspace *runtime.Workspace) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(workspace),
	}
}

func (signal *Signal) Name() string { return "sentiment" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(ticker kraken.TickerData) *data.Measurement[float64] {
	measurement := signal.ticker.Step(ticker)

	if measurement != nil {
		signal.err = measurement.Err
	}

	return measurement
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.ticker.Close()
}
