package depthflow

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var (
	SymbolTouchImbalance = algo.SymbolTouchImbalance
	SymbolDeepImbalance  = algo.SymbolDeepImbalance
	SymbolSpoofScore     = algo.SymbolSpoofScore
	SymbolLoadedScore    = algo.SymbolLoadedScore
	SymbolThinScore      = algo.SymbolThinScore
	SymbolNeutralScore   = algo.SymbolNeutralScore
	SymbolSeparation     = algo.SymbolSeparation
)

/*
depthflowPipeline is the pure nomagique Depthflow primitive.
*/
func depthflowPipeline() nomagique.Primitive {
	return algo.Depthflow()
}

type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	thesis *types.Thesis
	number *nomagique.Number[string]
	work   *transport.Consumer[*types.Symbol]
	pool   *types.SymbolPool
}

func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](depthflowPipeline()),
		pool:   types.NewSymbolPool(types.ShardWorkers()),
	}
	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)
	thesis.Work(types.SourceDepthFlow).Register(signal.work)

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceDepthFlow) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceDepthFlow }

func (signal *Signal) consume() {
	go func() {
		defer func() {
			if err := signal.pool.Error(); err != nil {
				signal.err = err
			}

			signal.thesis.Fail(signal.err)
		}()

		for symbol := range signal.thesis.Work(types.SourceDepthFlow).Drain(signal.work, nil) {
			select {
			case <-signal.ctx.Done():
				signal.pool.CaptureError(signal.ctx.Err())
				return
			default:
			}

			if symbol == nil {
				continue
			}

			symbolName := symbol.Symbol

			signal.pool.Submit(symbolName, func() {
				if err := signal.consumeSymbol(symbol); err != nil {
					signal.pool.CaptureError(errnie.Error(errnie.Err(
						errnie.Validation,
						"depthflow: failed for "+symbolName,
						err,
					)))
				}
			})
		}
	}()
}

func (signal *Signal) consumeSymbol(symbol *types.Symbol) error {
	for frame := range symbol.MarketLevel3(
		symbol.Level3Consumers[types.Level3ConsumerDepthFlow],
	) {
		touchBid, touchAsk := frameTouch(frame)
		deepBid, deepAsk := frameDeep(frame)

		input := nomagique.Frame{}
		input.Put(algo.SymbolTouchBidQty, touchBid)
		input.Put(algo.SymbolTouchAskQty, touchAsk)
		input.Put(algo.SymbolDeepBidQty, deepBid)
		input.Put(algo.SymbolDeepAskQty, deepAsk)
		input.Put(nmtypes.EventTimeSec, float64(frame.Timestamp.Unix()))
		input.Put(nmtypes.EventTimeNsec, float64(frame.Timestamp.Nanosecond()))

		output, err := signal.number.Step(symbol.Symbol, input)

		if err != nil {
			return err
		}

		measurement := nmtypes.NewMeasurement(
			uuid.NewString(),
			signal.Name(),
			frame.Timestamp.UnixNano(),
			frame.Timestamp.UnixNano(),
		).AddMetrics(
			nmtypes.NewMetric("touch_imbalance", output.MustGet(SymbolTouchImbalance), nmtypes.Descriptor{
				Unit:      nmtypes.UnitDimensionless,
				Timescale: nmtypes.TimescaleInstantaneous,
			}),
			nmtypes.NewMetric("deep_imbalance", output.MustGet(SymbolDeepImbalance), nmtypes.Descriptor{
				Unit:      nmtypes.UnitDimensionless,
				Timescale: nmtypes.TimescaleInstantaneous,
			}),
			nmtypes.NewNormalizedMetric("spoof_score", output.MustGet(SymbolSpoofScore), output.MustGet(SymbolSpoofScore), nmtypes.Descriptor{
				Unit:      nmtypes.UnitDimensionless,
				Timescale: nmtypes.TimescaleInstantaneous,
			}),
			nmtypes.NewNormalizedMetric("loaded_score", output.MustGet(SymbolLoadedScore), output.MustGet(SymbolLoadedScore), nmtypes.Descriptor{
				Unit:      nmtypes.UnitDimensionless,
				Timescale: nmtypes.TimescaleInstantaneous,
			}),
			nmtypes.NewNormalizedMetric("thin_score", output.MustGet(SymbolThinScore), output.MustGet(SymbolThinScore), nmtypes.Descriptor{
				Unit:      nmtypes.UnitDimensionless,
				Timescale: nmtypes.TimescaleInstantaneous,
			}),
		)

		measurement.StampQuality(
			output.MustGet(SymbolSeparation),
			output.MustGet(nomagique.SampleCount),
		)

		symbol.AppendMeasurement(measurement)
	}

	return nil
}

func frameTouch(frame kraken.Level3Data) (float64, float64) {
	bestBid, bestAsk := 0.0, 0.0

	for _, order := range frame.Bids {
		if order.OrderQty == nil {
			continue
		}

		if order.OrderQty.Float64() > bestBid {
			bestBid = order.OrderQty.Float64()
		}
	}

	for _, order := range frame.Asks {
		if order.OrderQty == nil {
			continue
		}

		if order.OrderQty.Float64() > bestAsk {
			bestAsk = order.OrderQty.Float64()
		}
	}

	return bestBid, bestAsk
}

func frameDeep(frame kraken.Level3Data) (float64, float64) {
	bidTotal, askTotal := 0.0, 0.0

	for _, order := range frame.Bids {
		if order.OrderQty != nil {
			bidTotal += order.OrderQty.Float64()
		}
	}

	for _, order := range frame.Asks {
		if order.OrderQty != nil {
			askTotal += order.OrderQty.Float64()
		}
	}

	return bidTotal, askTotal
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if signal.pool != nil {
		signal.pool.Close()
	}

	return nil
}
