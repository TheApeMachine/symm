package correlation

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Correlation perspective, reduced to a per-symbol price-motion
pipeline. It is ONLY a nomagique pipeline: the price return is windowed,
tracked by its own adaptive baseline, and scored by how far the current move
stands from the symbol's own established behavior. No cross-symbol cohort is
required for a symbol to establish its own relation to itself.
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
				statistic.Velocity,
				nomagique.Relay(statistic.SymbolVelocityDelta, nomagique.SampleValue),
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
	return string(types.SourceCorrelation)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceCorrelation
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

				for ticker := range symbol.MarketTickers(types.SourceCorrelation) {
					input := nomagique.Frame{}
					input.Put(nmtypes.Quantity, ticker.Last.Float64())
					input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
					input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))

					output, err := signal.number(symbol.Symbol, input)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							"correlation: number step failed for "+symbol.Symbol,
							err,
						))
						continue
					}

					symbol.Measurements.Push(nmtypes.NewMeasurement(
						uuid.NewString(),
						signal.Name(),
						ticker.Timestamp.UnixNano(),
						ticker.Timestamp.UnixNano(),
					).AddMetrics(
						nmtypes.NewMetric("price_velocity", output.MustGet(statistic.SymbolVelocityDelta), nmtypes.Descriptor{
							Unit:      nmtypes.UnitRate,
							Timescale: nmtypes.TimescalePerSecond,
						}),
						nmtypes.NewMetric("motion_baseline", output.MustGet(statistic.SymbolBaselineValue), nmtypes.Descriptor{
							Unit:      nmtypes.UnitRate,
							Timescale: nmtypes.TimescalePerSecond,
						}),
						nmtypes.NewMetric("motion_zscore", output.MustGet(statistic.SymbolZScore), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("motion_deviation", output.MustGet(statistic.SymbolDeviation), nmtypes.Descriptor{
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
