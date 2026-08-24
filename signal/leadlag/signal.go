package leadlag

import (
	"context"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Signal is the asynchronous price-path lead-lag instrument. It composes its
per-symbol Ticker entity in its constructor and exposes the canonical signal
structure: Constructor, Name, Error, Step, Close.
*/
type Signal struct {
	ctx       context.Context
	cancel    context.CancelFunc
	err       error
	workspace *runtime.Workspace
	ticker    *Ticker
}

/*
NewSignal composes the Ticker entity and retains the shared-object workspace
pool for any future cross-signal shared state. The lead-lag README defines no
downstream cross-symbol snapshot, so no shared object is registered here.
*/
func NewSignal(ctx context.Context, workspace *runtime.Workspace) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:       ctx,
		cancel:    cancel,
		workspace: workspace,
		ticker:    NewTicker(),
	}
}

func (signal *Signal) Name() string { return "leadlag" }

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
