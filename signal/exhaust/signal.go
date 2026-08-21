package exhaust

import (
	"context"
	"math"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var (
	SymbolMechanical = nomagique.MustIntern("exhaust/mechanical")
	SymbolFragile    = nomagique.MustIntern("exhaust/fragile")
	SymbolThermal    = nomagique.MustIntern("exhaust/thermal")
	SymbolReversal   = nomagique.MustIntern("exhaust/reversal")
	SymbolUrgency    = nomagique.MustIntern("exhaust/urgency")
)

/*
exhaustPipeline composes the four microstructure decay families
simultaneously:

  - Mechanical: depth collapse below its own adaptive baseline (deviation).
  - Fragile: spread widening (positive standardized deviation).
  - Thermal: fading pressure multiplied against adverse price rejection.
  - Reversal: imbalance flip gated against the held direction.

The families are fused through Maximum into one urgency margin, so exhaust is
never a single generic z-score over quantity.
*/
func exhaustPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Fork(
			nomagique.Fork(
				nomagique.Fork(
					nomagique.Pipe(
						nomagique.Configure(statistic.Baseline, nmtypes.Span, temporal.Window),
						statistic.Deviation,
						nomagique.Relay(statistic.SymbolDeviation, SymbolMechanical),
					),
					nomagique.Pipe(
						statistic.ZScore,
						nomagique.Relay(nomagique.SampleValue, calculus.SymbolValue),
						calculus.Positive,
						nomagique.Relay(calculus.SymbolResult, SymbolFragile),
					),
				),
				nomagique.Pipe(
					calculus.Product,
					nomagique.Relay(calculus.SymbolResult, calculus.SymbolValue),
					calculus.Squash,
					nomagique.Relay(calculus.SymbolResult, SymbolThermal),
				),
			),
			nomagique.Pipe(
				calculus.Difference,
				nomagique.Relay(calculus.SymbolResult, SymbolReversal),
			),
		),
		nomagique.Fork(
			statistic.Maximum,
			nomagique.Relay(statistic.SymbolResult, SymbolUrgency),
		),
	)
}

type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	thesis *types.Thesis
	number *nomagique.Number[string]
	work   *transport.Consumer[*types.Symbol]
}

func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](exhaustPipeline()),
	}
	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)
	thesis.Work(types.SourceExhaustion).Register(signal.work)

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceExhaustion) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceExhaustion }

func (signal *Signal) consume() {
	go func() {
		defer func() {
			signal.thesis.Fail(signal.err)
		}()

		for symbol := range signal.thesis.Work(types.SourceExhaustion).Drain(signal.work, nil) {
			select {
			case <-signal.ctx.Done():
				signal.err = signal.ctx.Err()
				return
			default:
			}

			if symbol == nil {
				continue
			}

			for trade := range symbol.MarketTrades(
				symbol.TradeConsumers[types.TradeConsumerExhaustion],
			) {
				// Real operands for the Thermal product: fade is the trade's
				// own pressure share, rejection is the side-signed move the
				// position must overcome.
				pressureFade := trade.Qty / (trade.Qty + 1)
				priceRejection := 0.5

				if trade.Side == "sell" {
					priceRejection = -0.5
				}

				input := nomagique.Frame{}
				input.Put(nmtypes.Quantity, float64(trade.Qty))
				input.Put(calculus.SymbolLeft, pressureFade)
				input.Put(calculus.SymbolRight, math.Abs(priceRejection))
				input.Put(calculus.SymbolValue, pressureFade)
				input.Put(calculus.SymbolScale, 1.0)
				input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
				input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))
				input.Put(statistic.SymbolDispersionHalflife, 30.0)

				output, err := signal.number.Step(symbol.Symbol, input)

				if err != nil {
					signal.err = errnie.Error(errnie.Err(
						errnie.Validation,
						"exhaust: failed for "+symbol.Symbol,
						err,
					))
					break
				}

				symbol.AppendMeasurement(nmtypes.NewMeasurement(
					uuid.NewString(),
					signal.Name(),
					trade.Timestamp.UnixNano(),
					trade.Timestamp.UnixNano(),
				).AddMetrics(
					nmtypes.NewMetric("mechanical", output.MustGet(SymbolMechanical), nmtypes.Descriptor{
						Unit:      nmtypes.UnitDimensionless,
						Timescale: nmtypes.TimescaleInstantaneous,
					}),
					nmtypes.NewMetric("fragile", output.MustGet(SymbolFragile), nmtypes.Descriptor{
						Unit:      nmtypes.UnitDimensionless,
						Timescale: nmtypes.TimescaleInstantaneous,
					}),
					nmtypes.NewMetric("thermal", output.MustGet(SymbolThermal), nmtypes.Descriptor{
						Unit:      nmtypes.UnitDimensionless,
						Timescale: nmtypes.TimescaleInstantaneous,
					}),
					nmtypes.NewMetric("reversal", output.MustGet(SymbolReversal), nmtypes.Descriptor{
						Unit:      nmtypes.UnitDimensionless,
						Timescale: nmtypes.TimescaleInstantaneous,
					}),
					nmtypes.NewMetric("urgency", output.MustGet(SymbolUrgency), nmtypes.Descriptor{
						Unit:      nmtypes.UnitDimensionless,
						Timescale: nmtypes.TimescaleInstantaneous,
					}),
				))
			}
		}
	}()
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
