package liquidity

import (
	"context"
	"runtime"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Liquidity perspective. It is ONLY a living nomagique Number: one
self-adapting numeric unit per symbol that maps incoming order-book depth into
statistical truth. State is isolated per symbol so streams never smear each
other's windows or event clocks.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	thesis *types.Thesis
	number nomagique.Number[string]
}

/*
NewSignal constructs the liquidity signal as one living nomagique.Number and
starts it in its own goroutine. Nothing else is owned by the signal.
*/
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
				statistic.ExtractDepth,
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

	go signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceLiquidity)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceLiquidity
}

func (signal *Signal) run() {
	for {
		select {
		case <-signal.ctx.Done():
			return
		default:
		}

		if !signal.pending() {
			// Nothing queued for this signal; yield before polling again.
			runtime.Gosched()
			continue
		}

		signal.thesis.Symbols.Range(func(_ any, value any) bool {
				symbol, valid := value.(*types.Symbol)

				if !valid || symbol == nil {
					return true
				}

				for ticker := range symbol.MarketTickers(types.SourceLiquidity) {
					if ticker.Bid == nil || ticker.Ask == nil {
						continue
					}

					input := nomagique.Frame{}
					input.Put(nmtypes.AlphaPrice, ticker.Bid.Float64())
					input.Put(nmtypes.BetaPrice, ticker.Ask.Float64())
					input.Put(nmtypes.AlphaQuantity, ticker.BidQty)
					input.Put(nmtypes.BetaQuantity, ticker.AskQty)
					input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
					input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))
					input.Put(statistic.SymbolDispersionHalflife, 30.0)

					output, err := signal.number(symbol.Symbol, input)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Validation,
							"liquidity: number step failed for "+symbol.Symbol,
							err,
						))
						continue
					}

					measurement := nmtypes.NewMeasurement(
						uuid.NewString(),
						signal.Name(),
						ticker.Timestamp.UnixNano(),
						ticker.Timestamp.UnixNano(),
					).AddMetrics(
						nmtypes.NewMetric("executable_touch_depth", output.MustGet(nmtypes.Quantity), nmtypes.Descriptor{
							Unit:      nmtypes.UnitPrice,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("depth_baseline", output.MustGet(statistic.SymbolBaselineValue), nmtypes.Descriptor{
							Unit:      nmtypes.UnitPrice,
							Timescale: nmtypes.TimescalePerSecond,
						}),
						nmtypes.NewMetric("depth_zscore", output.MustGet(statistic.SymbolZScore), nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("depth_deviation", output.MustGet(statistic.SymbolDeviation), nmtypes.Descriptor{
							Unit:      nmtypes.UnitPercent,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("depth_stability", output.MustGet(statistic.SymbolBaselineStability), nmtypes.Descriptor{
							Unit:      nmtypes.UnitPercent,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
						nmtypes.NewMetric("effective_window", output.MustGet(nmtypes.Span), nmtypes.Descriptor{
							Unit:      nmtypes.UnitCount,
							Timescale: nmtypes.TimescaleInstantaneous,
						}),
					)

					symbol.Measurements.Push(measurement)
				}

				return true
			})
	}
}

/*
pending reports whether any symbol queues a Tickers frame, so the
run loop can yield without draining empty input.
*/
func (signal *Signal) pending() bool {
	if signal.thesis == nil {
		return false
	}

	hasWork := false

	signal.thesis.Symbols.Range(func(_ any, value any) bool {
		symbol, valid := value.(*types.Symbol)

		if !valid || symbol == nil {
			return true
		}

		if symbol.HasTickers() {
			hasWork = true

			return false
		}

		return true
	})

	return hasWork
}
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
