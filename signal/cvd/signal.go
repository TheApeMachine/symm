package cvd

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var (
	// SymbolNetFlow is the instantaneous signed trade delta (Buy Volume - Sell Volume).
	SymbolNetFlow = nmtypes.MustIntern("cvd/net_flow")

	// SymbolCVD is the persistent running Cumulative Volume Delta.
	SymbolCVD = nmtypes.MustIntern("cvd/accumulated")

	// SymbolPriceDelta is the tick-to-tick trade price change (P_t - P_{t-1}).
	SymbolPriceDelta = nmtypes.MustIntern("cvd/price_delta")

	// SymbolAbsPriceDelta is the absolute magnitude of the price change |ΔP|.
	SymbolAbsPriceDelta = nmtypes.MustIntern("cvd/abs_price_delta")

	// SymbolAbsorption is the squashed ratio of volume absorbed at the touch.
	SymbolAbsorption = nmtypes.MustIntern("cvd/absorption")
)

/*
cvdPipeline constructs the pure nomagique reducer pipeline for CVD and Absorption:
 1. calculus.Difference: Computes signed notional delta (Buy Volume - Sell Volume).
 2. calculus.Accumulate: Accumulates signed delta into persistent running CVD in Frame state.
 3. temporal.Observer + calculus.Difference: Tracks tick-to-tick trade price movement (ΔP).
 4. statistic.Baseline & ZScore: Calculates adaptive volume baseline and flow anomaly z-score.
 5. calculus.Squash: Scales volume against baseline to detect absorption when price progress stalls.
*/
func cvdPipeline() nmtypes.Primitive {
	return nmtypes.Pipe(
		// 1. Calculate Signed Trade Delta: Buy Volume - Sell Volume
		nmtypes.Wire(
			calculus.Difference,
			nmtypes.In(nmtypes.AlphaQuantity, calculus.PortA), // Buy Volume
			nmtypes.In(nmtypes.BetaQuantity, calculus.PortB),  // Sell Volume
			nmtypes.Out(calculus.PortResult, SymbolNetFlow),
		),

		// 2. Accumulate delta into persistent running Cumulative Volume Delta (CVD)
		nmtypes.Wire(
			calculus.Accumulate,
			nmtypes.In(SymbolNetFlow, calculus.SymbolDelta),
			nmtypes.State(SymbolCVD, calculus.SymbolTotal),
			nmtypes.Out(calculus.PortResult, SymbolCVD),
		),

		// 3. Observe trade price and calculate tick-to-tick price change (ΔP)
		temporal.Observer(nmtypes.AlphaPrice),
		logic.If(
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(calculus.SymbolReady, logic.SymbolCondition),
				nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
			),
			nmtypes.Pipe(
				nmtypes.Wire(
					calculus.Difference,
					nmtypes.In(calculus.SymbolCurrent, calculus.PortA),
					nmtypes.In(calculus.SymbolPrevious, calculus.PortB),
					nmtypes.Out(calculus.PortResult, SymbolPriceDelta),
				),
				nmtypes.Wire(
					calculus.Absolute,
					nmtypes.In(SymbolPriceDelta, calculus.PortX),
					nmtypes.Out(calculus.PortResult, SymbolAbsPriceDelta),
				),
			),
			nmtypes.Assign(SymbolAbsPriceDelta, 0),
		),

		// 4. Adapt volume baseline over incoming trade volume (Quantity maps directly to SampleValue)
		nmtypes.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),

		// 5. Fork into statistical Z-Score, Deviation, and Absorption metrics
		nmtypes.ForkStrict(
			statistic.ZScore,
			statistic.Deviation,
			// Absorption: Volume squashed against the adaptive volume baseline
			nmtypes.Wire(
				calculus.Squash,
				nmtypes.In(nmtypes.Quantity, calculus.PortX),
				nmtypes.In(statistic.SymbolBaselineValue, calculus.SymbolScale),
				nmtypes.Out(calculus.PortResult, SymbolAbsorption),
			),
		),
	)
}

/*
Signal evaluates signed aggressor volume flow, cumulative volume delta (CVD),
and absorption over streaming market trades.
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
NewSignal initializes the CVD Signal reducer and subscribes to the trades channel.
*/
func NewSignal(ctx context.Context, thesis *types.Thesis, bus *runtime.Workspace) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](cvdPipeline()),
	}

	signal.measurements = runtime.ChannelOf(
		bus, types.ChannelMeasurements,
		func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
	)

	runtime.ChannelOf(
		bus, types.ChannelTrades,
		func(trade kraken.TradeData) string { return trade.Symbol },
	).Subscribe(signal.Name(), signal.Step)

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceCVD) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceCVD }

/*
Step processes a single trade observation through the CVD reducer and publishes
the resulting measurement downstream.
*/
func (signal *Signal) Step(trade kraken.TradeData) error {
	notional := trade.Price.Float64() * trade.Qty

	input := nmtypes.Frame{}
	input.Put(nmtypes.Quantity, notional)
	input.Put(nmtypes.AlphaPrice, trade.Price.Float64())
	input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))

	// Route buy vs. sell notional to Alpha and Beta channels
	if trade.Side == "sell" {
		input.Put(nmtypes.AlphaQuantity, 0)
		input.Put(nmtypes.BetaQuantity, notional)
	} else {
		input.Put(nmtypes.AlphaQuantity, notional)
		input.Put(nmtypes.BetaQuantity, 0)
	}

	// Advance the per-symbol nomagique reducer state
	output, err := signal.number.Step(trade.Symbol, input)
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cvd: failed for "+trade.Symbol,
			err,
		))
	}

	// Construct the standardized analytical measurement
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
		nmtypes.NewMetric("cvd", output.MustGet(SymbolCVD), nmtypes.Descriptor{
			Unit:      nmtypes.UnitQuoteCurrency,
			Timescale: nmtypes.TimescaleInstantaneous,
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

	// Stamp statistical quality and empirical maturity
	measurement.StampQuality(
		statistic.StandardSeparation(output.MustGet(statistic.SymbolZScore)),
		output.MustGet(nmtypes.SampleCount),
	)

	// Publish to the workspace measurement bus
	types.PublishMeasurement(signal.thesis, signal.measurements, trade.Symbol, measurement)
	return nil
}

/*
Close cancels the signal context and releases resources.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
