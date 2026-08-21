package liquidity

import (
	"context"
	"math"
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
	number *nomagique.Number[string]
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

				output, err := signal.number.Step(symbol.Symbol, input)

				if err != nil {
					signal.err = errnie.Error(errnie.Err(
						errnie.Validation,
						"liquidity: number step failed for "+symbol.Symbol,
						err,
					))
					break
				}

				separation := liquiditySeparation(output.MustGet(statistic.SymbolZScore))

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
					nmtypes.NewNormalizedMetric(
						string(types.MetricHypothesisSeparation),
						separation,
						separation,
						nmtypes.Descriptor{
							Unit:      nmtypes.UnitDimensionless,
							Timescale: nmtypes.TimescaleInstantaneous,
						},
					),
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

/*
liquiditySeparation turns the executable-touch-depth z-score into the margin
the "depth has moved" hypothesis holds over the "depth sits at baseline" null.
The z-score is already self-standardizing (residual over its own dispersion
estimate). Mapping its magnitude through the standard-normal CDF as the mass
inside ±z, 2·Φ(|x|) − 1, yields a value that starts near zero at the baseline
and rises toward one as the reading separates from it. This is real statistical
distance, not an arbitrary rescale, and it stays bounded in [0, 1] so the
reading can be painted as a percentage.
*/
func liquiditySeparation(zScore float64) float64 {
	return standardNormalMass(math.Abs(zScore))
}

/*
standardNormalMass is the probability mass inside ±z of a standard normal:
2·Φ(z) − 1, where Φ(z) = (1 + erf(z/√2))/2.
*/
func standardNormalMass(z float64) float64 {
	accumulated := 0.5 * (1 + math.Erf(z/math.Sqrt2))
	mass := 2*accumulated - 1

	if mass > 0 {
		return mass
	}

	return 0
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
