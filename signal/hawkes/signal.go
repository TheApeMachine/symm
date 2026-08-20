package hawkes

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/statistic"
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
	number nomagique.Number[string]
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

func (signal *Signal) Run() error {
	for signal.err == nil {
		symbol, available := signal.thesis.Work(types.SourceHawkes).WaitPop(
			signal.ctx,
			string(types.SourceHawkes),
		)

		if !available {
			return signal.ctx.Err()
		}

		if symbol == nil {
			continue
		}

		for trade := range symbol.MarketTrades(types.SourceHawkes) {
			input := nomagique.Frame{}
			input.Put(algo.SymbolMark, markForSide(trade.Side))
			input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
			input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))

			output, err := signal.number(symbol.Symbol, input)

			if err != nil {
				signal.err = errnie.Error(errnie.Err(
					errnie.Validation,
					"hawkes: number step failed for "+symbol.Symbol,
					err,
				))
				break
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
				nmtypes.NewMetric(types.MetricKey(types.MetricExcitationAmplitude, types.SideBuyToBuy), output.MustGet(statistic.SymbolAlphaAA), nmtypes.Descriptor{
					Unit:      nmtypes.UnitInverseSecond,
					Timescale: nmtypes.TimescalePerSecond,
				}),
				nmtypes.NewMetric(types.MetricKey(types.MetricDecayRate, types.SideNone), output.MustGet(statistic.SymbolBeta), nmtypes.Descriptor{
					Unit:      nmtypes.UnitInverseSecond,
					Timescale: nmtypes.TimescalePerSecond,
				}),
				nmtypes.NewMetric(types.MetricKey(types.MetricSpectralRadius, types.SideNone), output.MustGet(statistic.SymbolSpectralRadius), nmtypes.Descriptor{
					Unit:      nmtypes.UnitDimensionless,
					Timescale: nmtypes.TimescaleInstantaneous,
				}),
				nmtypes.NewMetric(types.MetricKey(types.MetricTotalDescendants, types.SideBuy), output.MustGet(statistic.SymbolDescendantsAlpha), nmtypes.Descriptor{
					Unit:      nmtypes.UnitCount,
					Timescale: nmtypes.TimescaleInstantaneous,
				}),
			))
		}
	}

	return signal.err
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
