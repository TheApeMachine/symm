package liquidity

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
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
	err    error
	clock  map[string]time.Time
	thesis *types.Thesis
	number nomagique.Number[string]
	work   *transport.Consumer[*types.Symbol]
}

/*
NewSignal constructs the liquidity signal as one living nomagique.Number.
*/
func NewSignal(
	ctx context.Context,
	thesis *types.Thesis,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		clock:  make(map[string]time.Time),
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
	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)
	thesis.Work(types.SourceLiquidity).Register(signal.work)

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceLiquidity)
}

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Type() types.SourceType {
	return types.SourceLiquidity
}

func (signal *Signal) consume() {
	go func() {
		defer func() {
			signal.thesis.Fail(signal.err)
		}()

		for symbol := range signal.thesis.Work(types.SourceLiquidity).Drain(signal.work, nil) {
			select {
			case <-signal.ctx.Done():
				signal.err = signal.ctx.Err()
				return
			default:
			}

			if symbol == nil {
				continue
			}

			for ticker := range symbol.MarketTickers(
				symbol.TickerConsumers[types.TickerConsumerLiquidity],
			) {
				if ticker.Bid == nil || ticker.Ask == nil {
					continue
				}

				eventTime := signal.eventTime(symbol.Symbol, ticker.Timestamp)
				input := nomagique.Frame{}
				input.Put(nmtypes.AlphaPrice, ticker.Bid.Float64())
				input.Put(nmtypes.BetaPrice, ticker.Ask.Float64())
				input.Put(nmtypes.AlphaQuantity, ticker.BidQty)
				input.Put(nmtypes.BetaQuantity, ticker.AskQty)
				input.Put(nmtypes.EventTimeSec, float64(eventTime.Unix()))
				input.Put(nmtypes.EventTimeNsec, float64(eventTime.Nanosecond()))
				input.Put(statistic.SymbolDispersionHalflife, 30.0)

				output, err := signal.number(symbol.Symbol, input)

				if err != nil {
					signal.err = errnie.Error(errnie.Err(
						errnie.Validation,
						"liquidity: number step failed for "+symbol.Symbol,
						err,
					))
					break
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

				symbol.AppendMeasurement(measurement)
			}
		}
	}()
}

func (signal *Signal) eventTime(symbol string, observedAt time.Time) time.Time {
	previousAt := signal.clock[symbol]

	if observedAt.Before(previousAt) {
		return previousAt
	}

	signal.clock[symbol] = observedAt

	return observedAt
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
