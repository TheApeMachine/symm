package cvd

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the CVD perspective: signed aggressor flow conditioned per symbol.
It is ONLY a nomagique pipeline composing the shared math primitives — the
difference of the two flow channels, an adaptive window tracking its own
baseline, and the departure from that baseline.
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
				calculus.Difference,
				nomagique.Relay(calculus.SymbolResult, nomagique.SampleValue),
				nomagique.Configure(
					statistic.Baseline,
					nmtypes.Span,
					temporal.Window,
				),
				nomagique.Fork(
					statistic.ZScore,
					statistic.Deviation,
				),
			),
		),
	}

	signal.run()
	return signal
}

func (signal *Signal) Name() string {
	return string(types.SourceCVD)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceCVD
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

				for trade := range symbol.MarketTrades(types.SourceCVD) {
					notional := trade.Price.Float64() * trade.Qty

					input := nomagique.Frame{}
					input.Put(nmtypes.AlphaQuantity, notional)
					input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
					input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))

					if trade.Side == "sell" {
						input.Put(nmtypes.BetaQuantity, notional)
					} else {
						input.Put(nmtypes.AlphaQuantity, notional)
						input.Put(nmtypes.BetaQuantity, 0)
					}

					output, err := signal.number(symbol.Symbol, input)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							"cvd: number step failed for "+symbol.Symbol,
							err,
						))
						continue
					}

					symbol.Measurements.Push(nmtypes.NewMeasurement(
						uuid.NewString(),
						signal.Name(),
						trade.Timestamp.UnixNano(),
						trade.Timestamp.UnixNano(),
					).AddMetrics(
						nmtypes.NewMetric("flow_imbalance", output.MustGet(nmtypes.Quantity), nmtypes.Descriptor{
							Unit:      nmtypes.UnitQuoteCurrency,
							Timescale: nmtypes.TimescalePerTick,
						}),
						nmtypes.NewMetric("flow_baseline", output.MustGet(statistic.SymbolBaselineValue), nmtypes.Descriptor{
							Unit:      nmtypes.UnitQuoteCurrency,
							Timescale: nmtypes.TimescalePerSecond,
						}),
						nmtypes.NewMetric("flow_zscore", output.MustGet(statistic.SymbolZScore), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("flow_deviation", output.MustGet(statistic.SymbolDeviation), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
					))
				}

				return true
			})
		}
	}
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
