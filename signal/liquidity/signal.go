package liquidity

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the liquidity measuring instrument. It composes its market entities
in its constructor and exposes the canonical signal structure: Constructor,
Name, Error, Step, Close. It satisfies nomagique/runtime.Node[*types.Envelope],
writing its projected Measurement into the envelope's Liquidity field.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	status *runtime.Status

	ticker *Ticker
}

// NewSignal composes the Ticker (touch) entity. Full-book morphology is added
// as a further entity in a later pass.
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		status: runtime.NewStatus(),
		ticker: NewTicker(),
	}
}

func (signal *Signal) Name() string { return "liquidity" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	if signal.err != nil {
		errnie.Error(signal.Close())
		return nil
	}

	/*
		A signal observes exactly the envelope kind it consumes. Stepping on any
		other kind hands the estimator a zero-valued observation, which it
		correctly rejects — and that rejection becomes a Measurement carrying an
		Err. data.Lift discards the WHOLE frame on the first failed measurement,
		so one signal stepped out of turn erased every other signal's metrics
		from the same envelope, and no advisor could ever assemble a complete
		feature group.
	*/
	if envelope.TypeID != types.EnvelopeTicker {
		return envelope
	}

	measurement := signal.ticker.Step(envelope.TickerData)

	if measurement != nil {
		signal.err = measurement.Err
	}

	envelope.Liquidity = measurement

	return envelope
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.ticker.Close()
}
