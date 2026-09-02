package hawkes

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Hawkes arrival-dynamics measuring instrument. It composes its
market entities in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close. It satisfies
nomagique/runtime.Node[*types.Envelope], writing its projected Measurement
into the envelope's Hawkes field — the manifold stage's forcing term.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	status *runtime.Status

	trade *Trade
}

// NewSignal composes the Trade (arrival-dynamics) entity.
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		status: runtime.NewStatus(),
		trade:  NewTrade(),
	}
}

func (signal *Signal) Name() string { return "hawkes" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	if signal.err != nil {
		errnie.Error(signal.Close())
		return nil
	}

	envelope.Hawkes = signal.trade.Step(envelope.TradeData)

	return envelope
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.trade.Close()
}
