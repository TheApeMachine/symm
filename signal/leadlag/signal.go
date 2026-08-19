package leadlag

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
Signal is the LeadLag perspective, reduced to a per-symbol temporal-motion
pipeline: price velocity against the symbol's own adaptive baseline tells
whether the symbol is leading or lagging its own established rhythm. It is
ONLY a nomagique pipeline.
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
				statistic.ZScore,
			),
		),
	}

	signal.run()
	return signal
}

func (signal *Signal) Name() string {
	return string(types.SourceLeadLag)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceLeadLag
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

				for ticker := range symbol.MarketTickers(types.SourceLeadLag) {
					input := nomagique.Frame{}
					input.Put(nmtypes.Quantity, ticker.Last.Float64())
					input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
					input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))

					output, err := signal.number(symbol.Symbol, input)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							"leadlag: number step failed for "+symbol.Symbol,
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
						nmtypes.NewMetric("lead_score", output.MustGet(statistic.SymbolZScore), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("lead_baseline", output.MustGet(statistic.SymbolBaselineValue), nmtypes.Descriptor{
							Unit:      nmtypes.UnitRate,
							Timescale: nmtypes.TimescalePerSecond,
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
