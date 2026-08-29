package sentiment

import (
	"context"

	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
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
	status *runtime.Status

	ticker *Ticker
}

/*
NewSignal composes the Ticker (per-symbol price-state) entity.
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

func (signal *Signal) Name() string { return "sentiment" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	measurement := signal.ticker.Step(envelope.TickerData)

	if measurement != nil {
		signal.err = measurement.Err
	}

	envelope.Sentiment = measurement

	return envelope
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.ticker.Close()
}
