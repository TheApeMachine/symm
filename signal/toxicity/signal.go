package toxicity

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Toxicity perspective: touch-liquidity honesty conditioned per
symbol. It is ONLY a nomagique pipeline — each accepted order frame's net
retreating quantity (cancelled minus filled) feeds an adaptive baseline whose
deviation reports how far the current book honesty stands from the symbol's
own established behavior.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	thesis *types.Thesis
	number nomagique.Number[string]
}

func NewSignal(
	ctx context.Context,
	thesis *types.Thesis,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](
			nomagique.Pipe(
				temporal.Window,
				statistic.Baseline,
				statistic.Deviation,
			),
		),
	}

	signal.run()
	return signal
}

func (signal *Signal) Name() string {
	return string(types.SourceToxicity)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceToxicity
}

func (signal *Signal) run() {
	for {
		select {
		case <-signal.ctx.Done():
			return
		default:
			signal.thesis.Symbols.Range(func(_ any, value any) bool {
				symbol, valid := value.(*types.Symbol)

				if !valid || symbol == nil {
					return true
				}

				for frame := range symbol.MarketLevel3(types.SourceToxicity) {
					input := nomagique.Frame{}
					input.Put(nmtypes.Quantity, honesty(frame))
					input.Put(nmtypes.EventTimeSec, float64(frame.Timestamp.Unix()))
					input.Put(nmtypes.EventTimeNsec, float64(frame.Timestamp.Nanosecond()))

					output, err := signal.number(symbol.Symbol, input)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							"toxicity: number step failed for "+symbol.Symbol,
							err,
						))
						continue
					}

					symbol.Measurements.Push(nmtypes.NewMeasurement(
						uuid.NewString(),
						signal.Name(),
						frame.Timestamp.UnixNano(),
						frame.Timestamp.UnixNano(),
					).AddMetrics(
						nmtypes.NewMetric("honesty_deviation", output.MustGet(statistic.SymbolDeviation), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("honesty_baseline", output.MustGet(statistic.SymbolBaselineValue), nmtypes.Descriptor{
							Unit:      nmtypes.UnitBaseCurrency,
							Timescale: nmtypes.TimescalePerSecond,
						}),
					))
				}

				return true
			})
		}
	}
}

func honesty(frame kraken.Level3Data) float64 {
	filled := 0.0
	retreated := 0.0

	for _, orders := range [][]kraken.Level3Order{frame.Bids, frame.Asks} {
		for _, order := range orders {
			if order.OrderQty == nil {
				continue
			}

			quantity := order.OrderQty.Float64()

			switch order.Event {
			case "fill":
				filled += quantity
			case "delete":
				retreated += quantity
			}
		}
	}

	return retreated - filled
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
