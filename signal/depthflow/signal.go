package depthflow

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

var (
	SymbolTouchImbalance   = nomagique.MustIntern("depthflow/touch_imbalance")
	SymbolDeepImbalance    = nomagique.MustIntern("depthflow/deep_imbalance")
	SymbolSpoofScore       = nomagique.MustIntern("depthflow/spoof_score")
	SymbolLoadedScore      = nomagique.MustIntern("depthflow/loaded_score")
	SymbolThinScore        = nomagique.MustIntern("depthflow/thin_score")
)

/*
depthflowPipeline composes the real book-flow classification:

  - Touch imbalance and deep-book (decay-weighted) imbalance are computed in
    parallel.
  - The book notional is remembered through the adaptive baseline.
  - Spoof is the sign disagreement between touch and deep imbalance, loaded is
    a strong aligned imbalance, and thinning is notional falling below its own
    baseline.
*/
func depthflowPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Fork(
			nomagique.Pipe(
				calculus.Difference,
				nomagique.Relay(calculus.SymbolResult, SymbolTouchImbalance),
			),
			nomagique.Pipe(
				calculus.Decay,
				calculus.Difference,
				nomagique.Relay(calculus.SymbolResult, SymbolDeepImbalance),
			),
		),
		nomagique.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),
		nomagique.Fork(
			nomagique.Fork(
				nomagique.Pipe(
					calculus.Product,
					calculus.Positive,
					calculus.Squash,
					nomagique.Relay(calculus.SymbolResult, SymbolSpoofScore),
				),
				nomagique.Pipe(
					statistic.ZScore,
					calculus.Squash,
					nomagique.Relay(calculus.SymbolResult, SymbolLoadedScore),
				),
			),
			nomagique.Pipe(
				statistic.Lift,
				calculus.Inverse,
				nomagique.Relay(calculus.SymbolResult, SymbolThinScore),
			),
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
		number: nomagique.NewNumber[string](depthflowPipeline()),
	}

	signal.run()
	return signal
}

func (signal *Signal) Name() string { return string(types.SourceDepthFlow) }
func (signal *Signal) Type() types.SourceType { return types.SourceDepthFlow }

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

				for ticker := range symbol.MarketTickers(types.SourceDepthFlow) {
					input := nomagique.Frame{}
					input.Put(nmtypes.AlphaQuantity, ticker.BidQty)
					input.Put(nmtypes.BetaQuantity, ticker.AskQty)
					input.Put(calculus.SymbolLeft, ticker.BidQty)
					input.Put(calculus.SymbolRight, ticker.AskQty)
					input.Put(calculus.SymbolLevel, ticker.BidQty+ticker.AskQty)
					input.Put(calculus.SymbolClock, 0)
					input.Put(statistic.SymbolBaseline, ticker.BidQty+ticker.AskQty)
					input.Put(nmtypes.Quantity, ticker.BidQty+ticker.AskQty)
					input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
					input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))
					input.Put(statistic.SymbolDispersionHalflife, 30.0)

					output, err := signal.number(symbol.Symbol, input)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							"depthflow: failed for "+symbol.Symbol,
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
						nmtypes.NewMetric("touch_imbalance", output.MustGet(SymbolTouchImbalance), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("deep_imbalance", output.MustGet(SymbolDeepImbalance), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("spoof_score", output.MustGet(SymbolSpoofScore), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("loaded_score", output.MustGet(SymbolLoadedScore), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("thin_score", output.MustGet(SymbolThinScore), nmtypes.Descriptor{
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
