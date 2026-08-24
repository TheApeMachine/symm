package hawkes

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
hawkesPipeline composes the Hawkes point process and hypothesis separation.
*/
func hawkesPipeline() nmtypes.Primitive {
	return nmtypes.Pipe(
		algo.Hawkes(),
		nmtypes.Relay(algo.SymbolLambdaAlpha, nmtypes.AlphaQuantity),
		nmtypes.Relay(algo.SymbolLambdaBeta, nmtypes.BetaQuantity),
		statistic.Separation,
	)
}

/*
Signal measures the buy/sell trade-arrival process as a bivariate Hawkes
process. It is ONLY a nomagique Number: one self-adapting numeric unit per
symbol that maps each signed trade mark into fitted self- and cross-exciting
intensities.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	thesis       *types.Thesis
	number       *nomagique.Number[string]
	measurements *runtime.Channel[*nmtypes.Measurement]
}

/*
NewSignal constructs the Hawkes pipeline.
*/
func NewSignal(
	ctx context.Context,
	thesis *types.Thesis,
	bus *runtime.Workspace,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](hawkesPipeline()),
	}
	signal.measurements = runtime.ChannelOf[*nmtypes.Measurement](
		bus, types.ChannelMeasurements,
		func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
	)
	runtime.ChannelOf[kraken.TradeData](
		bus, types.ChannelTrades,
		func(trade kraken.TradeData) string { return trade.Symbol },
	).Subscribe(signal.Name(), signal.Step)

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

// Step processes one ready symbol cut. The transport workspace preserves
// order for this symbol while allowing every other symbol to advance.
func (signal *Signal) Step(trade kraken.TradeData) error {
	input := nmtypes.Frame{}
	input.Put(algo.SymbolMark, markForSide(trade.Side))
	input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))

	output, err := signal.number.Step(trade.Symbol, input)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"hawkes: number step failed for "+trade.Symbol,
			err,
		))
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

	measurement := nmtypes.NewMeasurement(
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
	)
	measurement.StampQuality(
		output.MustGet(statistic.SymbolSeparation),
		output.MustGet(algo.SymbolEventCount),
	)

	types.PublishMeasurement(signal.thesis, signal.measurements, trade.Symbol, measurement)
	return nil
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
