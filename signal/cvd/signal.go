package cvd

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var (
	SymbolNetFlow     = nomagique.MustIntern("cvd/net_flow")
	SymbolImpactRatio = nomagique.MustIntern("cvd/impact_ratio")
	SymbolAbsorption  = nomagique.MustIntern("cvd/absorption")
)

/*
cvdPipeline is a pure composition of calculus, temporal, and logic primitives.
The signal owns no calculation: the recipe expresses signed aggressor flow,
impact efficiency, and absorption entirely from shared atomic reducers.
*/
func cvdPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Fork(
			calculus.Difference,
			calculus.Sum,
		),
		nomagique.Relay(calculus.SymbolResult, SymbolNetFlow),
		nomagique.Relay(SymbolNetFlow, nomagique.SampleValue),
		nomagique.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),
		nomagique.Fork(
			statistic.ZScore,
			statistic.Deviation,
		),
		nomagique.Fork(
			nomagique.Pipe(
				calculus.Ratio,
				calculus.Inverse,
				nomagique.Relay(calculus.SymbolResult, SymbolAbsorption),
			),
			logic.Gate,
		),
	)
}

type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	thesis *types.Thesis
	number nomagique.Number[string]
}

func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](cvdPipeline()),
	}

	signal.run()
	return signal
}

func (signal *Signal) Name() string { return string(types.SourceCVD) }
func (signal *Signal) Type() types.SourceType { return types.SourceCVD }

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
					input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
					input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))
					input.Put(statistic.SymbolDispersionHalflife, 30.0)

					if trade.Side == "sell" {
						input.Put(nmtypes.AlphaQuantity, 0)
						input.Put(nmtypes.BetaQuantity, notional)
					} else {
						input.Put(nmtypes.AlphaQuantity, notional)
						input.Put(nmtypes.BetaQuantity, 0)
					}

					output, err := signal.number(symbol.Symbol, input)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							"cvd: failed for "+symbol.Symbol,
							err,
						))
						continue
					}

					measurement := nmtypes.NewMeasurement(
						uuid.NewString(),
						signal.Name(),
						trade.Timestamp.UnixNano(),
						trade.Timestamp.UnixNano(),
					).AddMetrics(
						nmtypes.NewMetric("net_flow", output.MustGet(SymbolNetFlow), nmtypes.Descriptor{
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
						nmtypes.NewMetric("absorption", output.MustGet(SymbolAbsorption), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
					)

					symbol.Measurements.Push(measurement)
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
