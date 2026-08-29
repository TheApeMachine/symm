package correlation

import (
	"context"

	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the asynchronous price-path correlation instrument. It composes its
per-symbol Ticker entity in its constructor and exposes the canonical signal
structure: Constructor, Name, Error, Step, Close. It satisfies
nomagique/runtime.Node[*types.Envelope], reading the envelope's TickerData and
writing its projected Measurement into the envelope's Correlation field.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	status *runtime.Status
	ticker *Ticker
}

/*
NewSignal composes the Ticker entity.
*/
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		status: runtime.NewStatus(),
		ticker: NewTicker(),
	}
}

func (signal *Signal) Name() string { return "correlation" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	envelope.Correlation = signal.ticker.Step(envelope.TickerData)

	return envelope
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.ticker.Close()
}
