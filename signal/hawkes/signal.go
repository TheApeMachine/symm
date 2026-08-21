package hawkes

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Signal measures the buy/sell trade-arrival process as a bivariate Hawkes
process. It is ONLY a nomagique Number: one self-adapting numeric unit per
symbol that maps each signed trade mark into fitted self- and cross-exciting
intensities.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	thesis *types.Thesis
	number *nomagique.Number[string]
	work   *transport.Consumer[*types.Symbol]
}

/*
NewSignal constructs the Hawkes pipeline.
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
			nomagique.Pipe(algo.Hawkes()),
		),
	}
	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)
	thesis.Work(types.SourceHawkes).Register(signal.work)

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceHawkes)
}

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Type() types.SourceType {
	return types.SourceHawkes
}

func (signal *Signal) consume() {
	go func() {
		defer func() {
			signal.thesis.Fail(signal.err)
		}()

		for symbol := range signal.thesis.Work(types.SourceHawkes).Drain(signal.work, nil) {
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
				symbol.TradeConsumers[types.TradeConsumerHawkes],
			) {
				input := nomagique.Frame{}
				input.Put(algo.SymbolMark, markForSide(trade.Side))
				input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
				input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))

				output, err := signal.number.Step(symbol.Symbol, input)

				if err != nil {
					signal.err = errnie.Error(errnie.Err(
						errnie.Validation,
						"hawkes: number step failed for "+symbol.Symbol,
						err,
					))
					break
				}

				beta := output.MustGet(statistic.SymbolBeta)
				buyExcitation := output.MustGet(statistic.SymbolAlphaAA)
				sellExcitation := output.MustGet(statistic.SymbolAlphaBB)
				spectralRadius := output.MustGet(statistic.SymbolSpectralRadius)
				buyMetric := nmtypes.NewMetric(types.MetricKey(types.MetricExcitationAmplitude, types.SideBuyToBuy), buyExcitation, nmtypes.Descriptor{
					Unit: nmtypes.UnitInverseSecond, Timescale: nmtypes.TimescalePerSecond,
				})
				sellMetric := nmtypes.NewMetric(types.MetricKey(types.MetricExcitationAmplitude, types.SideSellToSell), sellExcitation, nmtypes.Descriptor{
					Unit: nmtypes.UnitInverseSecond, Timescale: nmtypes.TimescalePerSecond,
				})

				if beta > 0 {
					buyMetric = nmtypes.NewNormalizedMetric(types.MetricKey(types.MetricExcitationAmplitude, types.SideBuyToBuy), buyExcitation, buyExcitation/beta, buyMetric.Descriptor)
					sellMetric = nmtypes.NewNormalizedMetric(types.MetricKey(types.MetricExcitationAmplitude, types.SideSellToSell), sellExcitation, sellExcitation/beta, sellMetric.Descriptor)
				}

				spectralMetric := nmtypes.NewMetric(types.MetricKey(types.MetricSpectralRadius, types.SideNone), spectralRadius, nmtypes.Descriptor{
					Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous,
				})

				if spectralRadius >= 0 && spectralRadius < 1 {
					spectralMetric = nmtypes.NewNormalizedMetric(types.MetricKey(types.MetricSpectralRadius, types.SideNone), spectralRadius, spectralRadius, spectralMetric.Descriptor)
				}

				symbol.AppendMeasurement(nmtypes.NewMeasurement(
					uuid.NewString(),
					signal.Name(),
					trade.Timestamp.UnixNano(),
					trade.Timestamp.UnixNano(),
				).AddMetrics(
					nmtypes.NewMetric(types.MetricKey(types.MetricEventCount, types.SideNone), output.MustGet(algo.SymbolEventCount), nmtypes.Descriptor{
						Unit:      nmtypes.UnitCount,
						Timescale: nmtypes.TimescalePerTick,
					}),
					nmtypes.NewMetric(types.MetricKey(types.MetricEventCount, types.SideBuy), output.MustGet(algo.SymbolAlphaEventCount), nmtypes.Descriptor{
						Unit:      nmtypes.UnitCount,
						Timescale: nmtypes.TimescalePerTick,
					}),
					nmtypes.NewMetric(types.MetricKey(types.MetricEventCount, types.SideSell), output.MustGet(algo.SymbolBetaEventCount), nmtypes.Descriptor{
						Unit:      nmtypes.UnitCount,
						Timescale: nmtypes.TimescalePerTick,
					}),
					nmtypes.NewMetric(types.MetricKey(types.MetricConditionalIntensity, types.SideBuy), output.MustGet(algo.SymbolLambdaAlpha), nmtypes.Descriptor{
						Unit:      nmtypes.UnitEventsPerSecond,
						Timescale: nmtypes.TimescalePerSecond,
					}),
					nmtypes.NewMetric(types.MetricKey(types.MetricConditionalIntensity, types.SideSell), output.MustGet(algo.SymbolLambdaBeta), nmtypes.Descriptor{
						Unit:      nmtypes.UnitEventsPerSecond,
						Timescale: nmtypes.TimescalePerSecond,
					}),
					nmtypes.NewMetric(types.MetricKey(types.MetricBaselineIntensity, types.SideBuy), output.MustGet(algo.SymbolMuAlpha), nmtypes.Descriptor{
						Unit:      nmtypes.UnitEventsPerSecond,
						Timescale: nmtypes.TimescalePerSecond,
					}),
					nmtypes.NewMetric(types.MetricKey(types.MetricBaselineIntensity, types.SideSell), output.MustGet(algo.SymbolMuBeta), nmtypes.Descriptor{
						Unit:      nmtypes.UnitEventsPerSecond,
						Timescale: nmtypes.TimescalePerSecond,
					}),
					buyMetric,
					sellMetric,
					nmtypes.NewMetric(types.MetricKey(types.MetricDecayRate, types.SideNone), beta, nmtypes.Descriptor{
						Unit:      nmtypes.UnitInverseSecond,
						Timescale: nmtypes.TimescalePerSecond,
					}),
					spectralMetric,
					nmtypes.NewMetric(types.MetricKey(types.MetricTotalDescendants, types.SideBuy), output.MustGet(statistic.SymbolDescendantsAlpha), nmtypes.Descriptor{
						Unit:      nmtypes.UnitCount,
						Timescale: nmtypes.TimescaleInstantaneous,
					}),
				))
			}
		}
	}()
}

/*
markForSide encodes one trade's aggressor side into the process mark: buy is
the positive (alpha) channel, sell the negative (beta) channel.
*/
func markForSide(side string) float64 {
	if side == "buy" {
		return 1
	}

	return -1
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
