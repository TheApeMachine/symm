package liquidity

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/runtime"
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
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	thesis       *types.Thesis
	number       *nomagique.Number[string]
	measurements *runtime.Channel[*nmtypes.Measurement]
}

/*
NewSignal constructs the liquidity signal as one living nomagique.Number.
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
		number: nomagique.NewNumber[string](
			nmtypes.Pipe(
				statistic.ExtractDepth,
				nmtypes.Configure(
					statistic.Baseline,
					nmtypes.Span,
					temporal.Window,
				),
				nmtypes.Fork(
					statistic.ZScore,
					statistic.Deviation,
				),
			),
		),
	}
	signal.measurements = runtime.ChannelOf[*nmtypes.Measurement](
		bus, types.ChannelMeasurements,
		func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
	)
	runtime.ChannelOf[kraken.TickerData](
		bus, types.ChannelTickers,
		func(ticker kraken.TickerData) string { return ticker.Symbol },
	).Subscribe(signal.Name(), signal.Step)

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

// Step processes one ready symbol cut. The transport workspace preserves
// order for this symbol while allowing every other symbol to advance.
func (signal *Signal) Step(ticker kraken.TickerData) error {
	if ticker.Bid == nil || ticker.Ask == nil {
		return nil
	}

	input := nmtypes.Frame{}
	input.Put(nmtypes.AlphaPrice, ticker.Bid.Float64())
	input.Put(nmtypes.BetaPrice, ticker.Ask.Float64())
	input.Put(nmtypes.AlphaQuantity, ticker.BidQty)
	input.Put(nmtypes.BetaQuantity, ticker.AskQty)
	input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))

	output, err := signal.number.Step(ticker.Symbol, input)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"liquidity: number step failed for "+ticker.Symbol,
			err,
		))
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
		nmtypes.NewMetric("effective_window", output.MustGet(statistic.SymbolBaselineWindow), nmtypes.Descriptor{
			Unit:      nmtypes.UnitCount,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
	)
	measurement.StampQuality(
		statistic.StandardSeparation(output.MustGet(statistic.SymbolZScore)),
		output.MustGet(nmtypes.SampleCount),
	)

	types.PublishMeasurement(signal.thesis, signal.measurements, ticker.Symbol, measurement)
	return nil
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
