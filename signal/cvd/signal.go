package cvd

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var (
	SymbolNetFlow    = nmtypes.MustIntern("cvd/net_flow")
	SymbolAbsorption = nmtypes.MustIntern("cvd/absorption")
)

/*
cvdPipeline is slot-aligned with the calculus atoms:

  - calculus.Difference reads left, right and writes result (the signed net
    flow).
  - The net flow is lifted into SampleValue for the adaptive baseline, window,
    z-score, and deviation stages.
  - Absorption is the non-negative squash of the same flow magnitude, so high
    aggressor flow with little price response reads as absorption.
*/
func cvdPipeline() nmtypes.Primitive {
	return nmtypes.Pipe(
		calculus.Difference,
		nmtypes.Relay(calculus.SymbolResult, SymbolNetFlow),
		nmtypes.Relay(SymbolNetFlow, nmtypes.SampleValue),
		nmtypes.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),
		nmtypes.Fork(
			statistic.ZScore,
			nmtypes.Fork(
				statistic.Deviation,
				nmtypes.Pipe(
					calculus.Positive,
					nmtypes.Relay(calculus.SymbolResult, calculus.SymbolValue),
					nmtypes.Relay(statistic.SymbolBaselineValue, calculus.SymbolScale),
					calculus.Squash,
					nmtypes.Relay(calculus.SymbolResult, SymbolAbsorption),
				),
			),
		),
	)
}

type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	thesis       *types.Thesis
	number       *nomagique.Number[string]
	measurements *runtime.Channel[*nmtypes.Measurement]
}

func NewSignal(ctx context.Context, thesis *types.Thesis, bus *runtime.Workspace) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](cvdPipeline()),
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

func (signal *Signal) Name() string           { return string(types.SourceCVD) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceCVD }

// Step processes one ready symbol cut. The transport workspace preserves
// order for this symbol while allowing every other symbol to advance.
func (signal *Signal) Step(trade kraken.TradeData) error {
	notional := trade.Price.Float64() * trade.Qty

	input := nmtypes.Frame{}
	input.Put(calculus.SymbolRight, notional)
	input.Put(calculus.SymbolLeft, 0)

	if trade.Side != "sell" {
		input.Put(calculus.SymbolLeft, notional)
		input.Put(calculus.SymbolRight, 0)
	}

	input.Put(calculus.SymbolValue, notional)
	input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))

	output, err := signal.number.Step(trade.Symbol, input)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cvd: failed for "+trade.Symbol,
			err,
		))
	}

	measurement := nmtypes.NewMeasurement(
		uuid.NewString(),
		signal.Name(),
		trade.Timestamp.UnixNano(),
		trade.Timestamp.UnixNano(),
	).AddMetrics(
		nmtypes.NewMetric("net", output.MustGet(SymbolNetFlow), nmtypes.Descriptor{
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
		nmtypes.NewNormalizedMetric("absorption", output.MustGet(SymbolAbsorption), output.MustGet(SymbolAbsorption), nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
	)
	measurement.StampQuality(
		statistic.StandardSeparation(output.MustGet(statistic.SymbolZScore)),
		output.MustGet(nmtypes.SampleCount),
	)

	types.PublishMeasurement(signal.thesis, signal.measurements, trade.Symbol, measurement)
	return nil
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
