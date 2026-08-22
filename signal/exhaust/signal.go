package exhaust

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var (
	SymbolMechanical = algo.SymbolMechanical
	SymbolFragile    = algo.SymbolFragile
	SymbolThermal    = algo.SymbolThermal
	SymbolReversal   = algo.SymbolReversal
	SymbolUrgency    = algo.SymbolUrgency
)

/*
exhaustPipeline is the pure nomagique Exhaust primitive.
*/
func exhaustPipeline() nomagique.Primitive {
	return algo.Exhaust()
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
				side := 1.0

				if trade.Side == "sell" {
					side = -1.0
				}

				input := nomagique.Frame{}
				input.Put(algo.SymbolVolume, trade.Qty)
				input.Put(algo.SymbolAggressorSide, side)
				input.Put(algo.SymbolPriceDelta, trade.Price.Float64())
				input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
				input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))

				output, err := signal.number.Step(symbol.Symbol, input)

				if err != nil {
					signal.err = errnie.Error(errnie.Err(
						errnie.Validation,
						"exhaust: failed for "+symbol.Symbol,
						err,
					))
					break
				}

				measurement := nmtypes.NewMeasurement(
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
				)

				measurement.StampQuality(0, output.MustGet(nomagique.SampleCount))

				symbol.AppendMeasurement(measurement)
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
