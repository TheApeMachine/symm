package hawkes

import (
	"context"
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
	"github.com/theapemachine/symm/nomagique/transport"
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

	trade *Trade
}

// NewSignal composes the Trade (arrival-dynamics) entity.
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
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

	/*
		A signal observes exactly the envelope kind it consumes. Stepping on any
		other kind hands the estimator a zero-valued observation, which it
		correctly rejects — and that rejection becomes a Measurement carrying an
		Err. data.Lift discards the WHOLE frame on the first failed measurement,
		so one signal stepped out of turn erased every other signal's metrics
		from the same envelope, and no advisor could ever assemble a complete
		feature group.
	*/
	if envelope.TypeID != types.EnvelopeTrade {
		return envelope
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

type Trade struct {
	process *nmhawkes.Bivariate
}

func NewTrade() *Trade {
	return &Trade{
		process: nmhawkes.NewBivariate(),
	}
}

func (trade *Trade) Step(observation kraken.TradeData) *data.Measurement[float64] {
	if observation.Side != "buy" && observation.Side != "sell" {
		return &data.Measurement[float64]{
			Err: fmt.Errorf(
				"hawkes: unsupported trade side %q", observation.Side,
			),
		}
	}

	measurementEval := transport.NewEvaluate(trade.process)
	var measurement *data.Measurement[float64]

	for out := range measurementEval.Next(transport.NewValues(nmhawkes.Event{
		Key:  observation.Symbol,
		At:   observation.Timestamp.UnixNano(),
		Mark: markForSide(observation.Side),
	}).Next(nil)) {
		measurement = *(**data.Measurement[float64])(out)
	}

	err := measurementEval.Error()

	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}

	return measurement
}

func (trade *Trade) Close() error { return nil }

func markForSide(side string) float64 {
	if side == "buy" {
		return 1
	}

	return -1
}
