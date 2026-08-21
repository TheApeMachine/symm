package depthflow

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var (
	SymbolTouchImbalance = nomagique.MustIntern("depthflow/touch_imbalance")
	SymbolDeepImbalance  = nomagique.MustIntern("depthflow/deep_imbalance")
	SymbolSpoofScore     = nomagique.MustIntern("depthflow/spoof_score")
	SymbolLoadedScore    = nomagique.MustIntern("depthflow/loaded_score")
	SymbolThinScore      = nomagique.MustIntern("depthflow/thin_score")
)

/*
depthflowPipeline is slot-aligned with the calculus atoms:

  - TouchImbalance is the Difference of best bid/ask quantity.
  - DeepImbalance is the Difference of the distance-decayed resting book, whose
    decay remainder is the book's own depth share (not a zero clock).
  - Spoof is the positive product of the two imbalances' disagreement, loaded
    is a squashed z-score of that alignment, and thinning is the inverse lift
    of the total notional below its own baseline.
*/
func depthflowPipeline() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Fork(
			nomagique.Pipe(
				calculus.Difference,
				nomagique.Relay(calculus.SymbolResult, SymbolTouchImbalance),
			),
			nomagique.Pipe(
				calculus.Decay,
				calculus.Difference,
				nomagique.Relay(calculus.SymbolResult, SymbolDeepImbalance),
			),
		),
		nomagique.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),
		nomagique.Fork(
			nomagique.Fork(
				nomagique.Pipe(
					calculus.Product,
					calculus.Positive,
					calculus.Squash,
					nomagique.Relay(calculus.SymbolResult, SymbolSpoofScore),
				),
				nomagique.Pipe(
					statistic.ZScore,
					calculus.Squash,
					nomagique.Relay(calculus.SymbolResult, SymbolLoadedScore),
				),
			),
			nomagique.Pipe(
				statistic.Lift,
				calculus.Inverse,
				nomagique.Relay(calculus.SymbolResult, SymbolThinScore),
			),
		),
	)
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
		touch := frameTouch(frame)
		deep := frameDeep(frame)
		total := touch + deep

		input := nomagique.Frame{}
		input.Put(calculus.SymbolLeft, touch)
		input.Put(calculus.SymbolRight, deep)
		input.Put(calculus.SymbolLevel, deep)

		if total > 0 {
			input.Put(calculus.SymbolClock, touch/total)
		}

		input.Put(nomagique.SampleValue, total)
		input.Put(statistic.SymbolBaseline, total)
		input.Put(calculus.SymbolValue, touch)
		input.Put(calculus.SymbolScale, total)
		input.Put(nmtypes.EventTimeSec, float64(frame.Timestamp.Unix()))
		input.Put(nmtypes.EventTimeNsec, float64(frame.Timestamp.Nanosecond()))
		input.Put(statistic.SymbolDispersionHalflife, 30.0)

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

		/*
			Depthflow emits three hypothesis scores, not a competing pair,
			so its honest separation is zero until the pipeline exposes a
			winning-vs-competitor margin. Maturity still marks evidence.
		*/
		measurement.StampQuality(0, output.MustGet(nomagique.SampleCount))

		symbol.AppendMeasurement(measurement)
	}

	return nil
}

/*
frameTouch returns the best level's resting quantity on the heavier side, and
frameDeep returns the total resting quantity across all other levels — the L3
depth the touch sits on top of. Both are real book quantities, so the decay
clock (touch share) genuinely discounts the deep book by its distance share.
*/
func frameTouch(frame kraken.Level3Data) float64 {
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

	if bestBid > bestAsk {
		return bestBid
	}

	return bestAsk
}

func frameDeep(frame kraken.Level3Data) float64 {
	total := 0.0

	for _, orders := range [][]kraken.Level3Order{frame.Bids, frame.Asks} {
		for _, order := range orders {
			if order.OrderQty != nil {
				total += order.OrderQty.Float64()
			}
		}
	}

	return total
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
