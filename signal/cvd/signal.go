package cvd

import (
	"context"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Signal is the CVD executed-flow measuring instrument. It holds no state and no
logic of its own: its entire behavior is composed of nomagique pipelines per symbol.
*/
type Signal struct {
	*runtime.System
	arena    *data.ArenaOwner
	pipeline *nomagique.Number
}

func NewSignal(ctx context.Context, arena *data.ArenaOwner) *Signal {
	signal := &Signal{
		arena: arena,
		pipeline: nomagique.NewNumber(
			transport.NewParallel(
				nomagique.NewNumber(
					// Branch 0: Temporal Velocity of Gross Notional Rate
					arithmetic.NewMultiply(),
					statistic.NewSum(),
					temporal.NewVelocity(),
				),
				nomagique.NewNumber(
					// Branch 1: Adaptive Baseline of Gross Notional Rate
					arithmetic.NewMultiply(),
					statistic.NewSum(),
					adaptive.NewBaseline(adaptive.NewWindow()),
				),
				nomagique.NewNumber(
					// Branch 2: Adaptive Baseline of Midpoint Return Rate
					statistic.NewSum(),
					adaptive.NewBaseline(adaptive.NewWindow()),
				),
				nomagique.NewNumber(
					// Branch 3: 3-Channel Joint SNR
					statistic.NewSum(),
					statistic.NewJoint(3),
				),
				nomagique.NewNumber(
					// Branch 4: Buy Quantity
					statistic.NewSum(),
					statistic.NewRegressionAccumulator(2),
				),
				nomagique.NewNumber(
					// Branch 5: Sell Quantity
					statistic.NewSum(),
					statistic.NewRegressionAccumulator(2),
				),
				nomagique.NewNumber(
					// Branch 6: Buy Notional
					statistic.NewSum(),
				),
				nomagique.NewNumber(
					// Branch 7: Sell Notional
					statistic.NewSum(),
				),
				nomagique.NewNumber(
					// Branch 8: Buy count
					statistic.NewSum(),
				),
				nomagique.NewNumber(
					// Branch 9: Sell count
					statistic.NewSum(),
				),
			),
		),
	}
	signal.System = runtime.NewSystem(ctx, "cvd", signal)
	return signal
}

/*
Step reads trade data from the prior measurement and writes CVD metrics
into a fresh measurement allocated from its own arena.
*/
func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	result := data.Read[[]float64](
		signal.pipeline.Next(data.NewValue(
			data.NewAdapter(prior, data.NewMap[string]("price", "whatever")),
			data.NewAdapter(prior, data.NewMap("price", "whatever")),
			data.NewAdapter(prior, data.NewMap("price", "whatever")),
			data.NewAdapter(prior, data.NewMap("price", "whatever")),
			data.NewAdapter(prior, data.NewMap("price", "whatever")),
			data.NewAdapter(prior, data.NewMap("price", "whatever")),
			data.NewAdapter(prior, data.NewMap("price", "whatever")),
			data.NewAdapter(prior, data.NewMap("price", "whatever")),
			data.NewAdapter(prior, data.NewMap("price", "whatever")),
		)),
	)

	return out
}
