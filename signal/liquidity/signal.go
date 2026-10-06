package liquidity

import (
	"context"
	"errors"
	"sync"

	"github.com/theapemachine/errnie"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Signal is the touch-liquidity measuring instrument. It holds no logic of its
own: its entire behavior is one nomagique pipeline per symbol, a
transport.Parallel of stage groups. Group i receives the data.Adapter bound to
states[i], whose mapping binds the group's native primitive names to liquidity
domain names. Every state of a symbol shares one output map, so a domain fact
published by one group is read by the groups after it. The pipeline is driven
by trade frames only; the touch quote is read exclusively from the injected
BookSource and is the only envelope translation. Nothing about the touch is
ever read from the arriving measurement.
*/
type Signal struct {
	*runtime.System
	books     broker.BookSource
	pipelines sync.Map
	metrics   [][4]string
}

type symbolPipeline struct {
	output   data.Map[float64]
	envelope data.Map[float64]
	states   []*data.State
	pipeline core.Primitive
}

/*
NewSignal composes the touch-liquidity instrument. The BookSource is required:
it is the sole authority for the best bid, best ask, and their displayed
quantities. A missing BookSource is a wiring fault and halts the instrument.
*/
func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books: books,
		// {published label, output key, unit, timescale}
		metrics: [][4]string{
			{"best_bid_price", "bid", string(data.UnitPrice), string(data.TimescaleInstantaneous)},
			{"best_ask_price", "ask", string(data.UnitPrice), string(data.TimescaleInstantaneous)},
			{"touch_quantity:bid", "bid_qty", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"touch_quantity:ask", "ask_qty", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"touch_notional:bid", "touch_notional:bid", string(data.UnitNotional), string(data.TimescaleInstantaneous)},
			{"touch_notional:ask", "touch_notional:ask", string(data.UnitNotional), string(data.TimescaleInstantaneous)},
			{"midpoint", "midpoint", string(data.UnitPrice), string(data.TimescaleInstantaneous)},
			{"spread", "spread", string(data.UnitSpread), string(data.TimescaleInstantaneous)},
			{"relative_spread", "relative_spread", string(data.UnitRelativeSpread), string(data.TimescaleInstantaneous)},
			{"two_sided_touch_notional", "two_sided_touch_notional", string(data.UnitNotional), string(data.TimescaleInstantaneous)},
			{"touch_notional_imbalance", "touch_notional_imbalance", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"touch_notional_baseline:bid", "touch_notional_baseline:bid", string(data.UnitNotional), string(data.TimescaleRollingWindow)},
			{"touch_notional_baseline:ask", "touch_notional_baseline:ask", string(data.UnitNotional), string(data.TimescaleRollingWindow)},
			{"relative_spread_baseline", "relative_spread_baseline", string(data.UnitRelativeSpread), string(data.TimescaleRollingWindow)},
			{"depth_ratio:bid", "depth_ratio:bid", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"depth_ratio:ask", "depth_ratio:ask", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"spread_ratio", "spread_ratio", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"depth_divergence:bid", "depth_divergence:bid", string(data.UnitNotional), string(data.TimescaleRollingWindow)},
			{"depth_divergence:ask", "depth_divergence:ask", string(data.UnitNotional), string(data.TimescaleRollingWindow)},
			{"spread_divergence", "spread_divergence", string(data.UnitRelativeSpread), string(data.TimescaleRollingWindow)},
			{"depth_noise_scale:bid", "depth_noise_scale:bid", string(data.UnitNotional), string(data.TimescaleRollingWindow)},
			{"depth_noise_scale:ask", "depth_noise_scale:ask", string(data.UnitNotional), string(data.TimescaleRollingWindow)},
			{"spread_noise_scale", "spread_noise_scale", string(data.UnitRelativeSpread), string(data.TimescaleRollingWindow)},
			{"depth_zscore:bid", "depth_zscore:bid", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"depth_zscore:ask", "depth_zscore:ask", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"spread_zscore", "spread_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"divergence_velocity:bid", "divergence_velocity:bid", string(data.UnitVelocity), string(data.TimescaleInstantaneous)},
			{"divergence_velocity:ask", "divergence_velocity:ask", string(data.UnitVelocity), string(data.TimescaleInstantaneous)},
			{"spread_divergence_velocity", "spread_divergence_velocity", string(data.UnitVelocity), string(data.TimescaleInstantaneous)},
			{"historical_path_distance", "historical_path_distance", string(data.UnitDistance), string(data.TimescaleRollingWindow)},
			{"historical_path_percentile", "historical_path_percentile", string(data.UnitPercent), string(data.TimescaleRollingWindow)},
		},
	}

	signal.System = runtime.NewSystem(ctx, "liquidity", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[liquidity] book manager is required", err))
	}

	return signal
}

func (signal *Signal) pipelineFor(symbol string) *symbolPipeline {
	if existing, ok := signal.pipelines.Load(symbol); ok {
		return existing.(*symbolPipeline)
	}

	output := data.NewOutputMap()

	pipe := &symbolPipeline{
		output:   output,
		envelope: data.NewOutputMap(),
		states: []*data.State{
			// 0-1: Displayed touch notional per side.
			data.NewState(data.NewMap("left", "bid", "right", "bid_qty", "multiply", "touch_notional:bid"), output),
			data.NewState(data.NewMap("left", "ask", "right", "ask_qty", "multiply", "touch_notional:ask"), output),
			// 2-4: Touch cost geometry.
			data.NewState(data.NewMap("left", "ask", "right", "bid", "subtract", "spread"), output),
			data.NewState(data.NewMap("left", "bid", "right", "ask", "weight", "half", "mix", "midpoint"), output),
			data.NewState(data.NewMap("left", "spread", "right", "midpoint", "divide", "relative_spread"), output),
			// 5-7: Total and net touch notional, scale-free imbalance.
			data.NewState(data.NewMap("left", "touch_notional:bid", "right", "touch_notional:ask", "add", "touch_notional"), output),
			data.NewState(data.NewMap("left", "touch_notional:bid", "right", "touch_notional:ask", "subtract", "net_touch_notional"), output),
			data.NewState(data.NewMap("left", "net_touch_notional", "right", "touch_notional", "divide", "touch_notional_imbalance"), output),
			// 8-10: Two-sided notional, min(b, a) = (b + a - |b - a|) / 2.
			data.NewState(data.NewMap("value", "net_touch_notional", "absolute", "net_touch_notional:absolute"), output),
			data.NewState(data.NewMap("left", "touch_notional", "right", "net_touch_notional:absolute", "subtract", "two_sided_touch_notional:double"), output),
			data.NewState(data.NewMap("left", "two_sided_touch_notional:double", "right", "half", "multiply", "two_sided_touch_notional"), output),
			// 11-16: Bid depth against its own causal baseline.
			data.NewState(data.NewMap("value", "touch_notional:bid", "center", "touch_notional_baseline:bid", "scale", "depth_noise_scale:bid"), output),
			data.NewState(data.NewMap("left", "touch_notional:bid", "right", "touch_notional_baseline:bid", "subtract", "depth_divergence:bid"), output),
			data.NewState(data.NewMap("left", "touch_notional:bid", "right", "touch_notional_baseline:bid", "divide", "depth_ratio:bid"), output),
			data.NewState(data.NewMap("left", "depth_divergence:bid", "right", "depth_noise_scale:bid", "divide", "depth_zscore:bid"), output),
			data.NewState(data.NewMap("value", "depth_divergence:bid", "rate", "divergence_velocity:bid", "defined", "divergence_velocity:bid:defined"), output),
			// 16-20: Ask depth against its own causal baseline.
			data.NewState(data.NewMap("value", "touch_notional:ask", "center", "touch_notional_baseline:ask", "scale", "depth_noise_scale:ask"), output),
			data.NewState(data.NewMap("left", "touch_notional:ask", "right", "touch_notional_baseline:ask", "subtract", "depth_divergence:ask"), output),
			data.NewState(data.NewMap("left", "touch_notional:ask", "right", "touch_notional_baseline:ask", "divide", "depth_ratio:ask"), output),
			data.NewState(data.NewMap("left", "depth_divergence:ask", "right", "depth_noise_scale:ask", "divide", "depth_zscore:ask"), output),
			data.NewState(data.NewMap("value", "depth_divergence:ask", "rate", "divergence_velocity:ask", "defined", "divergence_velocity:ask:defined"), output),
			// 21-25: Relative spread against its own causal baseline.
			data.NewState(data.NewMap("value", "relative_spread", "center", "relative_spread_baseline", "scale", "spread_noise_scale"), output),
			data.NewState(data.NewMap("left", "relative_spread", "right", "relative_spread_baseline", "subtract", "spread_divergence"), output),
			data.NewState(data.NewMap("left", "relative_spread", "right", "relative_spread_baseline", "divide", "spread_ratio"), output),
			data.NewState(data.NewMap("left", "spread_divergence", "right", "spread_noise_scale", "divide", "spread_zscore"), output),
			data.NewState(data.NewMap("value", "spread_divergence", "rate", "spread_divergence_velocity", "defined", "spread_divergence_velocity:defined"), output),
		},
		pipeline: transport.NewParallel(
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(calculus.NewMix()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(calculus.NewAbsolute()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(temporal.NewVelocity()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(temporal.NewVelocity()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(temporal.NewVelocity()),
		),
	}

	actual, _ := signal.pipelines.LoadOrStore(symbol, pipe)
	return actual.(*symbolPipeline)
}

/*
Step reads the symbol's touch from the BookSource on each trade frame, binds
it to one adapter per stage group, runs the symbol's pipeline, and writes the
published liquidity facts into a fresh Measurement allocated from the signal's
own arena. The trade itself contributes only identity and venue time. A book
that is momentarily absent or one-sided yields no measurement (as in
depthflow). A present touch with a non-finite or non-positive price or
quantity (broker.InvalidTouch) or a crossed or locked touch
(broker.CrossedTouch) is corrupt book state and halts the signal. Facts a group left
unwritten (a ratio or z-score against an undefined baseline or scale, a
velocity without a prior observation) are omitted.
*/
func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[liquidity] book manager is required", nil))
		return nil
	}

	pipe := signal.pipelineFor(prior.Label)

	clear(pipe.output.Values)
	clear(pipe.envelope.Values)

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil {
			return
		}

		bid := book.BestBid()
		ask := book.BestAsk()

		if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil ||
			bid.Quantity == nil || ask.Quantity == nil {
			return
		}

		pipe.envelope.Values["bid"] = kraken.Float64(bid.Price)
		pipe.envelope.Values["bid_qty"] = kraken.Float64(bid.Quantity)
		pipe.envelope.Values["ask"] = kraken.Float64(ask.Price)
		pipe.envelope.Values["ask_qty"] = kraken.Float64(ask.Quantity)
	})

	// An absent or one-sided book is momentary: drop the event. Once the
	// touch is present, impossible values are corrupt book state and halt.
	if _, held := pipe.envelope.Values["bid"]; !held {
		return nil
	}

	touch := map[string]float64{}

	for _, key := range []string{"bid", "ask", "bid_qty", "ask_qty"} {
		touch[key] = pipe.envelope.Values[key]
	}

	for _, value := range touch {
		if !broker.ValidTouchValue(value) {
			signal.Error(broker.InvalidTouch("liquidity", prior.Label, touch))
			return nil
		}
	}

	if pipe.envelope.Values["ask"] <= pipe.envelope.Values["bid"] {
		// BookSource already withholds pending and checksum-diverging books,
		// so a crossed or locked touch here is corrupt book state, never a
		// market fact. It halts (Internal) like a missing BookSource, but its
		// Conflict cause and message name the symbol and touch so the two
		// faults are never confused.
		signal.Error(broker.CrossedTouch("liquidity", prior.Label, pipe.envelope.Values["bid"], pipe.envelope.Values["ask"]))
		return nil
	}

	pipe.envelope.Values["half"] = 0.5
	pipe.envelope.Values["at"] = float64(prior.At.UnixNano())

	publisher := data.NewAdapter(prior, data.NewState(data.NewMap(), pipe.output))

	for range publisher.Next(data.NewValue(pipe.envelope)) {
	}

	adapters := make([]*data.Adapter, len(pipe.states))

	for index, state := range pipe.states {
		adapters[index] = data.NewAdapter(prior, state)
	}

	for range pipe.pipeline.Next(data.NewValue(adapters...)) {
	}

	if err := errors.Join(publisher.Error(), pipe.pipeline.Error()); err != nil {
		signal.Error(err)
		return nil
	}

	out := data.NewMeasurement(
		prior.Epoch, prior.Label, signal.Name(), prior.SeqIdx, prior.Tick,
	)
	out.Peers(prior)
	out.At = prior.At
	out.From = prior.At

	metrics := make([]*data.Metric, 0, len(signal.metrics))

	for _, metric := range signal.metrics {
		value := pipe.output.Values[metric[1]]

		metrics = append(metrics, data.NewMetric(
			metric[0], value, data.Unit(metric[2]), data.Timescale(metric[3]),
		))
	}

	return out.Write(metrics...)
}
