package exhaust

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var (
	SymbolMechanical = algo.SymbolMechanical
	SymbolFragile    = algo.SymbolFragile
	SymbolThermal    = algo.SymbolThermal
	SymbolReversal   = algo.SymbolReversal
	SymbolUrgency    = algo.SymbolUrgency
)

/*
exhaustPipeline is the pure nomagique Exhaust primitive.
*/
func exhaustPipeline() nmtypes.Primitive {
	return algo.Exhaust()
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
		number: nomagique.NewNumber[string](exhaustPipeline()),
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

func (signal *Signal) Name() string           { return string(types.SourceExhaustion) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceExhaustion }

// Step processes one ready symbol cut. The transport workspace preserves
// order for this symbol while allowing every other symbol to advance.
func (signal *Signal) Step(trade kraken.TradeData) error {
	side := 1.0

	if trade.Side == "sell" {
		side = -1.0
	}

	input := nmtypes.Frame{}
	input.Put(algo.SymbolVolume, trade.Qty)
	input.Put(algo.SymbolAggressorSide, side)
	input.Put(algo.SymbolPriceDelta, trade.Price.Float64())
	input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))

	output, err := signal.number.Step(trade.Symbol, input)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"exhaust: failed for "+trade.Symbol,
			err,
		))
	}

	measurement := nmtypes.NewMeasurement(
		uuid.NewString(),
		signal.Name(),
		trade.Timestamp.UnixNano(),
		trade.Timestamp.UnixNano(),
	).AddMetrics(
		nmtypes.NewMetric("mechanical", output.MustGet(SymbolMechanical), nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric("fragile", output.MustGet(SymbolFragile), nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric("thermal", output.MustGet(SymbolThermal), nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric("reversal", output.MustGet(SymbolReversal), nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
		nmtypes.NewMetric("urgency", output.MustGet(SymbolUrgency), nmtypes.Descriptor{
			Unit:      nmtypes.UnitDimensionless,
			Timescale: nmtypes.TimescaleInstantaneous,
		}),
	)

	measurement.StampQuality(0, output.MustGet(nmtypes.SampleCount))

	types.PublishMeasurement(signal.thesis, signal.measurements, trade.Symbol, measurement)
	return nil
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
