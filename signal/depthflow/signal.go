package depthflow

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

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
Signal is the displayed-depth flow instrument. It holds no logic of its own:
its entire behavior is two nomagique pipelines per symbol, each a
transport.Parallel of stage groups over one shared output map. The book levels
are the only envelope translation — folded into per-side displayed notional
and per-level added and removed notional against the previous book, which the
adapter cannot carry as maps. The level pipeline runs on every observation;
the flow pipeline, which divides by elapsed time and the reference notional,
runs only once a previous book exists and venue time has advanced. Facts
accumulate in the output map and are written once.
*/
type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
	books     broker.BookSource
	pipelines sync.Map
	metrics   [][5]string
}

type symbolPipeline struct {
	output       data.Map[float64]
	envelope     data.Map[float64]
	levelStates  []*data.State
	level        core.Primitive
	flowStates   []*data.State
	flow         core.Primitive
	prevBids     map[float64]float64
	prevAsks     map[float64]float64
	currBids     map[float64]float64
	currAsks     map[float64]float64
	hasPrev      bool
	prevNotional float64
	prevAt       time.Time
}

/*
NewSignal composes the displayed-depth flow instrument. The BookSource
supplies the aggregated levels whose mutation is measured.
*/
func NewSignal(ctx context.Context, arena *data.ArenaOwner, books broker.BookSource) *Signal {
	signal := &Signal{
		arena: arena,
		books: books,
		// {published label, output key, unit, timescale, gate key}
		// A non-empty gate key publishes the metric only while that output is non-zero.
		metrics: [][5]string{
			{"book_notional:bid", "notional:bid", string(data.UnitNotional), string(data.TimescaleInstantaneous), ""},
			{"book_notional:ask", "notional:ask", string(data.UnitNotional), string(data.TimescaleInstantaneous), ""},
			{"book_notional", "book_notional", string(data.UnitNotional), string(data.TimescaleInstantaneous), ""},
			{"observed_notional:bid", "notional:bid", string(data.UnitNotional), string(data.TimescaleInstantaneous), ""},
			{"observed_notional:ask", "notional:ask", string(data.UnitNotional), string(data.TimescaleInstantaneous), ""},
			{"observed_notional", "book_notional", string(data.UnitNotional), string(data.TimescaleInstantaneous), ""},
			{"book_imbalance", "book_imbalance", string(data.UnitRatio), string(data.TimescaleInstantaneous), ""},
			{"observed_notional_imbalance", "book_imbalance", string(data.UnitRatio), string(data.TimescaleInstantaneous), ""},
			{"touch_imbalance", "touch_imbalance", string(data.UnitRatio), string(data.TimescaleInstantaneous), ""},
			{"imbalance_resolution_gap", "imbalance_resolution_gap", string(data.UnitRatio), string(data.TimescaleInstantaneous), ""},
			{"imbalance_resolution_distance", "imbalance_resolution_distance", string(data.UnitRatio), string(data.TimescaleInstantaneous), ""},
			{"added_notional:bid", "added:bid", string(data.UnitNotional), string(data.TimescaleInstantaneous), "flow:defined"},
			{"removed_notional:bid", "removed:bid", string(data.UnitNotional), string(data.TimescaleInstantaneous), "flow:defined"},
			{"net_displayed_flow:bid", "net_displayed_flow:bid", string(data.UnitNotional), string(data.TimescaleInstantaneous), "flow:defined"},
			{"added_notional:ask", "added:ask", string(data.UnitNotional), string(data.TimescaleInstantaneous), "flow:defined"},
			{"removed_notional:ask", "removed:ask", string(data.UnitNotional), string(data.TimescaleInstantaneous), "flow:defined"},
			{"net_displayed_flow:ask", "net_displayed_flow:ask", string(data.UnitNotional), string(data.TimescaleInstantaneous), "flow:defined"},
			{"flow_activity_imbalance", "flow_activity_imbalance", string(data.UnitRatio), string(data.TimescaleInstantaneous), "flow:defined"},
			{"book_imbalance_baseline", "book_imbalance_baseline", string(data.UnitRatio), string(data.TimescaleRollingWindow), ""},
			{"book_imbalance_divergence", "book_imbalance_divergence", string(data.UnitRatio), string(data.TimescaleRollingWindow), ""},
			{"book_imbalance_zscore", "book_imbalance_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow), ""},
			{"resolution_gap_baseline", "resolution_gap_baseline", string(data.UnitRatio), string(data.TimescaleRollingWindow), ""},
			{"resolution_gap_divergence", "resolution_gap_divergence", string(data.UnitRatio), string(data.TimescaleRollingWindow), ""},
			{"resolution_gap_zscore", "resolution_gap_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow), ""},
			{"book_imbalance_velocity", "book_imbalance_velocity", string(data.UnitVelocity), string(data.TimescaleInstantaneous), "book_imbalance_velocity:defined"},
			{"resolution_gap_velocity", "resolution_gap_velocity", string(data.UnitVelocity), string(data.TimescaleInstantaneous), "resolution_gap_velocity:defined"},
			{"added_notional_rate:bid", "added_notional_rate:bid", string(data.UnitNotionalRate), string(data.TimescalePerSecond), ""},
			{"added_notional_rate:ask", "added_notional_rate:ask", string(data.UnitNotionalRate), string(data.TimescalePerSecond), ""},
			{"removed_notional_rate:bid", "removed_notional_rate:bid", string(data.UnitNotionalRate), string(data.TimescalePerSecond), ""},
			{"removed_notional_rate:ask", "removed_notional_rate:ask", string(data.UnitNotionalRate), string(data.TimescalePerSecond), ""},
			{"net_displayed_flow_rate:bid", "net_displayed_flow_rate:bid", string(data.UnitNotionalRate), string(data.TimescalePerSecond), ""},
			{"net_displayed_flow_rate:ask", "net_displayed_flow_rate:ask", string(data.UnitNotionalRate), string(data.TimescalePerSecond), ""},
			{"book_turnover_rate", "book_turnover_rate", string(data.UnitRate), string(data.TimescalePerSecond), ""},
			{"net_book_change_rate", "net_book_change_rate", string(data.UnitRate), string(data.TimescalePerSecond), ""},
			{"signed_net_displayed_flow_rate", "signed_net_displayed_flow_rate", string(data.UnitRate), string(data.TimescalePerSecond), ""},
			{"turnover_baseline", "turnover_baseline", string(data.UnitRate), string(data.TimescaleRollingWindow), ""},
			{"turnover_divergence", "turnover_divergence", string(data.UnitRate), string(data.TimescaleRollingWindow), ""},
			{"turnover_zscore", "turnover_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow), ""},
			{"turnover_ratio", "turnover_ratio", string(data.UnitRatio), string(data.TimescaleRollingWindow), ""},
			{"net_book_change_rate_baseline", "net_book_change_rate_baseline", string(data.UnitRate), string(data.TimescaleRollingWindow), ""},
			{"net_book_change_rate_divergence", "net_book_change_rate_divergence", string(data.UnitRate), string(data.TimescaleRollingWindow), ""},
			{"net_book_change_rate_zscore", "net_book_change_rate_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow), ""},
			{"signed_net_displayed_flow_rate_baseline", "signed_net_displayed_flow_rate_baseline", string(data.UnitRate), string(data.TimescaleRollingWindow), ""},
			{"signed_net_displayed_flow_rate_divergence", "signed_net_displayed_flow_rate_divergence", string(data.UnitRate), string(data.TimescaleRollingWindow), ""},
			{"signed_net_displayed_flow_rate_zscore", "signed_net_displayed_flow_rate_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow), ""},
		},
	}

	signal.System = runtime.NewSystem(ctx, "depthflow", signal)
	return signal
}

/*
Arena exposes the signal's ArenaOwner to the runtime Consumer.
*/
func (signal *Signal) Arena() *data.ArenaOwner {
	return signal.arena
}

func (signal *Signal) pipelineFor(symbol string) *symbolPipeline {
	if existing, ok := signal.pipelines.Load(symbol); ok {
		return existing.(*symbolPipeline)
	}

	output := data.NewOutputMap()

	pipe := &symbolPipeline{
		output:   output,
		envelope: data.NewOutputMap(),
		prevBids: make(map[float64]float64),
		prevAsks: make(map[float64]float64),
		currBids: make(map[float64]float64),
		currAsks: make(map[float64]float64),
		levelStates: []*data.State{
			// 0-2: Displayed book notional and its scale-free imbalance.
			data.NewState(data.NewMap("left", "notional:bid", "right", "notional:ask", "add", "book_notional"), output),
			data.NewState(data.NewMap("left", "notional:bid", "right", "notional:ask", "subtract", "net_book_notional"), output),
			data.NewState(data.NewMap("left", "net_book_notional", "right", "book_notional", "divide", "book_imbalance"), output),
			// 3-5: Touch notional and its imbalance.
			data.NewState(data.NewMap("left", "touch_notional:bid", "right", "touch_notional:ask", "add", "touch_notional"), output),
			data.NewState(data.NewMap("left", "touch_notional:bid", "right", "touch_notional:ask", "subtract", "net_touch_notional"), output),
			data.NewState(data.NewMap("left", "net_touch_notional", "right", "touch_notional", "divide", "touch_imbalance"), output),
			// 6-8: How far the touch imbalance has yet to resolve into the book.
			data.NewState(data.NewMap("left", "touch_imbalance", "right", "book_imbalance", "subtract", "imbalance_resolution_gap"), output),
			data.NewState(data.NewMap("left", "imbalance_resolution_gap", "right", "zero", "subtract", "imbalance_resolution_distance"), output),
			data.NewState(data.NewMap("value", "imbalance_resolution_distance", "absolute", "imbalance_resolution_distance"), output),
			// 9-11: Net displayed flow per side and signed across sides.
			data.NewState(data.NewMap("left", "added:bid", "right", "removed:bid", "subtract", "net_displayed_flow:bid"), output),
			data.NewState(data.NewMap("left", "added:ask", "right", "removed:ask", "subtract", "net_displayed_flow:ask"), output),
			data.NewState(data.NewMap("left", "net_displayed_flow:bid", "right", "net_displayed_flow:ask", "subtract", "signed_net_displayed_flow"), output),
			// 12-17: Gross flow activity and the scale-free flow imbalance.
			data.NewState(data.NewMap("left", "net_displayed_flow:bid", "right", "zero", "subtract", "gross_displayed_flow:bid"), output),
			data.NewState(data.NewMap("value", "gross_displayed_flow:bid", "absolute", "gross_displayed_flow:bid"), output),
			data.NewState(data.NewMap("left", "net_displayed_flow:ask", "right", "zero", "subtract", "gross_displayed_flow:ask"), output),
			data.NewState(data.NewMap("value", "gross_displayed_flow:ask", "absolute", "gross_displayed_flow:ask"), output),
			data.NewState(data.NewMap("left", "gross_displayed_flow:bid", "right", "gross_displayed_flow:ask", "add", "gross_displayed_flow"), output),
			data.NewState(data.NewMap("left", "signed_net_displayed_flow", "right", "gross_displayed_flow", "divide", "flow_activity_imbalance"), output),
			// 18-20: Book imbalance against its own causal baseline.
			data.NewState(data.NewMap("value", "book_imbalance", "center", "book_imbalance_baseline", "scale", "book_imbalance_noise_scale"), output),
			data.NewState(data.NewMap("left", "book_imbalance", "right", "book_imbalance_baseline", "subtract", "book_imbalance_divergence"), output),
			data.NewState(data.NewMap("left", "book_imbalance_divergence", "right", "book_imbalance_noise_scale", "divide", "book_imbalance_zscore"), output),
			// 21-23: Resolution gap against its own causal baseline.
			data.NewState(data.NewMap("value", "imbalance_resolution_gap", "center", "resolution_gap_baseline", "scale", "resolution_gap_noise_scale"), output),
			data.NewState(data.NewMap("left", "imbalance_resolution_gap", "right", "resolution_gap_baseline", "subtract", "resolution_gap_divergence"), output),
			data.NewState(data.NewMap("left", "resolution_gap_divergence", "right", "resolution_gap_noise_scale", "divide", "resolution_gap_zscore"), output),
			// 24-25: Imbalance and gap velocities.
			data.NewState(data.NewMap("value", "book_imbalance", "rate", "book_imbalance_velocity", "defined", "book_imbalance_velocity:defined"), output),
			data.NewState(data.NewMap("value", "imbalance_resolution_gap", "rate", "resolution_gap_velocity", "defined", "resolution_gap_velocity:defined"), output),
		},
		level: transport.NewParallel(
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(calculus.NewAbsolute()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(calculus.NewAbsolute()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(calculus.NewAbsolute()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(temporal.NewVelocity()),
			transport.NewStages(temporal.NewVelocity()),
		),
		flowStates: []*data.State{
			// 0-3: Elapsed venue time and the reference notional exposure.
			data.NewState(data.NewMap("from", "prev_at", "to", "at", "elapsed", "elapsed"), output),
			data.NewState(data.NewMap("left", "book_notional", "right", "prev_notional", "add", "reference_notional:double"), output),
			data.NewState(data.NewMap("left", "reference_notional:double", "right", "half", "multiply", "reference_notional"), output),
			data.NewState(data.NewMap("left", "reference_notional", "right", "elapsed", "multiply", "reference_exposure"), output),
			// 4-9: Per-side notional rates.
			data.NewState(data.NewMap("left", "added:bid", "right", "elapsed", "divide", "added_notional_rate:bid"), output),
			data.NewState(data.NewMap("left", "added:ask", "right", "elapsed", "divide", "added_notional_rate:ask"), output),
			data.NewState(data.NewMap("left", "removed:bid", "right", "elapsed", "divide", "removed_notional_rate:bid"), output),
			data.NewState(data.NewMap("left", "removed:ask", "right", "elapsed", "divide", "removed_notional_rate:ask"), output),
			data.NewState(data.NewMap("left", "net_displayed_flow:bid", "right", "elapsed", "divide", "net_displayed_flow_rate:bid"), output),
			data.NewState(data.NewMap("left", "net_displayed_flow:ask", "right", "elapsed", "divide", "net_displayed_flow_rate:ask"), output),
			// 10-16: Turnover, net change, and signed flow per unit exposure.
			data.NewState(data.NewMap("left", "added:bid", "right", "removed:bid", "add", "book_activity:bid"), output),
			data.NewState(data.NewMap("left", "added:ask", "right", "removed:ask", "add", "book_activity:ask"), output),
			data.NewState(data.NewMap("left", "book_activity:bid", "right", "book_activity:ask", "add", "book_activity"), output),
			data.NewState(data.NewMap("left", "book_activity", "right", "reference_exposure", "divide", "book_turnover_rate"), output),
			data.NewState(data.NewMap("left", "book_notional", "right", "prev_notional", "subtract", "net_book_change"), output),
			data.NewState(data.NewMap("left", "net_book_change", "right", "reference_exposure", "divide", "net_book_change_rate"), output),
			data.NewState(data.NewMap("left", "signed_net_displayed_flow", "right", "reference_exposure", "divide", "signed_net_displayed_flow_rate"), output),
			// 17-20: Turnover against its own causal baseline.
			data.NewState(data.NewMap("value", "book_turnover_rate", "center", "turnover_baseline", "scale", "turnover_noise_scale"), output),
			data.NewState(data.NewMap("left", "book_turnover_rate", "right", "turnover_baseline", "subtract", "turnover_divergence"), output),
			data.NewState(data.NewMap("left", "turnover_divergence", "right", "turnover_noise_scale", "divide", "turnover_zscore"), output),
			data.NewState(data.NewMap("left", "book_turnover_rate", "right", "turnover_baseline", "divide", "turnover_ratio"), output),
			// 21-23: Net book change rate against its own causal baseline.
			data.NewState(data.NewMap("value", "net_book_change_rate", "center", "net_book_change_rate_baseline", "scale", "net_book_change_rate_noise_scale"), output),
			data.NewState(data.NewMap("left", "net_book_change_rate", "right", "net_book_change_rate_baseline", "subtract", "net_book_change_rate_divergence"), output),
			data.NewState(data.NewMap("left", "net_book_change_rate_divergence", "right", "net_book_change_rate_noise_scale", "divide", "net_book_change_rate_zscore"), output),
			// 24-26: Signed displayed flow rate against its own causal baseline.
			data.NewState(data.NewMap("value", "signed_net_displayed_flow_rate", "center", "signed_net_displayed_flow_rate_baseline", "scale", "signed_net_displayed_flow_rate_noise_scale"), output),
			data.NewState(data.NewMap("left", "signed_net_displayed_flow_rate", "right", "signed_net_displayed_flow_rate_baseline", "subtract", "signed_net_displayed_flow_rate_divergence"), output),
			data.NewState(data.NewMap("left", "signed_net_displayed_flow_rate_divergence", "right", "signed_net_displayed_flow_rate_noise_scale", "divide", "signed_net_displayed_flow_rate_zscore"), output),
		},
		flow: transport.NewParallel(
			transport.NewStages(temporal.NewElapsed()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
		),
	}

	actual, _ := signal.pipelines.LoadOrStore(symbol, pipe)
	return actual.(*symbolPipeline)
}

/*
Step folds the shared book's levels into per-side displayed and touch
notional plus per-level added and removed notional against the symbol's
previous book, drives the level pipeline and (once a previous book exists and
venue time has advanced) the flow pipeline, and writes the published
depth-flow facts into a fresh Measurement allocated from the signal's own
arena. An absent, empty, or crossed book yields no measurement: invalid
geometry is never fabricated into zero depth. Level-diff facts are omitted on
a symbol's first observation, and rates are omitted until time advances.
*/
func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" || signal.books == nil {
		return nil
	}

	pipe := signal.pipelineFor(prior.Label)

	clear(pipe.output.Values)
	clear(pipe.envelope.Values)
	clear(pipe.currBids)
	clear(pipe.currAsks)

	var bidNotional, askNotional, touchBid, touchAsk float64
	var addedBid, removedBid, addedAsk, removedAsk float64
	ok := false

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil {
			return
		}

		bid := book.BestBid()
		ask := book.BestAsk()

		if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil {
			return
		}

		if kraken.Float64(ask.Price) <= kraken.Float64(bid.Price) {
			return
		}

		for cursor, count := bid, 0; cursor != nil && count < 100; cursor, count = cursor.Lower, count+1 {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			price := kraken.Float64(cursor.Price)
			qty := kraken.Float64(cursor.Quantity)

			if price <= 0 || qty <= 0 || math.IsNaN(price) || math.IsNaN(qty) || math.IsInf(price, 0) || math.IsInf(qty, 0) {
				continue
			}

			if len(pipe.currBids) == 0 {
				touchBid = price * qty
			}

			pipe.currBids[price] = qty
			bidNotional += price * qty
		}

		for cursor, count := ask, 0; cursor != nil && count < 100; cursor, count = cursor.Higher, count+1 {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			price := kraken.Float64(cursor.Price)
			qty := kraken.Float64(cursor.Quantity)

			if price <= 0 || qty <= 0 || math.IsNaN(price) || math.IsNaN(qty) || math.IsInf(price, 0) || math.IsInf(qty, 0) {
				continue
			}

			if len(pipe.currAsks) == 0 {
				touchAsk = price * qty
			}

			pipe.currAsks[price] = qty
			askNotional += price * qty
		}

		ok = len(pipe.currBids) > 0 && len(pipe.currAsks) > 0
	})

	if !ok {
		return nil
	}

	// Level-diff fold: displayed quantity that appeared or vanished at each
	// price since the previous book, in notional.
	for price, qty := range pipe.currBids {
		delta := price * (qty - pipe.prevBids[price])
		addedBid += math.Max(delta, 0)
		removedBid += math.Max(-delta, 0)
	}

	for price, qty := range pipe.prevBids {
		if _, held := pipe.currBids[price]; !held {
			removedBid += price * qty
		}
	}

	for price, qty := range pipe.currAsks {
		delta := price * (qty - pipe.prevAsks[price])
		addedAsk += math.Max(delta, 0)
		removedAsk += math.Max(-delta, 0)
	}

	for price, qty := range pipe.prevAsks {
		if _, held := pipe.currAsks[price]; !held {
			removedAsk += price * qty
		}
	}

	hadPrev := pipe.hasPrev
	prevAt := pipe.prevAt
	flowReady := hadPrev && prior.At.After(prevAt)

	pipe.envelope.Values["notional:bid"] = bidNotional
	pipe.envelope.Values["notional:ask"] = askNotional
	pipe.envelope.Values["touch_notional:bid"] = touchBid
	pipe.envelope.Values["touch_notional:ask"] = touchAsk
	pipe.envelope.Values["added:bid"] = addedBid
	pipe.envelope.Values["removed:bid"] = removedBid
	pipe.envelope.Values["added:ask"] = addedAsk
	pipe.envelope.Values["removed:ask"] = removedAsk
	pipe.envelope.Values["prev_notional"] = pipe.prevNotional
	pipe.envelope.Values["prev_at"] = float64(prevAt.UnixNano())
	pipe.envelope.Values["at"] = float64(prior.At.UnixNano())
	pipe.envelope.Values["zero"] = 0
	pipe.envelope.Values["half"] = 0.5
	pipe.envelope.Values["flow:defined"] = 0

	if hadPrev {
		pipe.envelope.Values["flow:defined"] = 1
	}

	publisher := data.NewAdapter(prior, data.NewState(data.NewMap(), pipe.output))

	for range publisher.Next(data.NewValue(pipe.envelope)) {
	}

	levelAdapters := make([]*data.Adapter, len(pipe.levelStates))

	for index, state := range pipe.levelStates {
		levelAdapters[index] = data.NewAdapter(prior, state)
	}

	for range pipe.level.Next(data.NewValue(levelAdapters...)) {
	}

	if flowReady {
		flowAdapters := make([]*data.Adapter, len(pipe.flowStates))

		for index, state := range pipe.flowStates {
			flowAdapters[index] = data.NewAdapter(prior, state)
		}

		for range pipe.flow.Next(data.NewValue(flowAdapters...)) {
		}
	}

	if err := errors.Join(publisher.Error(), pipe.level.Error(), pipe.flow.Error()); err != nil {
		signal.Error(err)
		return nil
	}

	pipe.prevBids, pipe.currBids = pipe.currBids, pipe.prevBids
	pipe.prevAsks, pipe.currAsks = pipe.currAsks, pipe.prevAsks
	pipe.prevNotional = pipe.output.Values["book_notional"]
	pipe.prevAt = prior.At
	pipe.hasPrev = true

	out := signal.arena.NewMeasurement(
		prior.Epoch, prior.Label, signal.Name(), prior.SeqIdx, prior.Tick, []*data.Measurement{prior},
	)
	out.Epoch = prior.Epoch
	out.Label = prior.Label
	out.Source = signal.Name()
	out.SeqIdx = prior.SeqIdx
	out.Tick = prior.Tick
	out.At = prior.At
	out.From = prior.At

	if flowReady {
		out.From = prevAt
	}

	metrics := make([]data.Metric, 0, len(signal.metrics))

	for _, metric := range signal.metrics {
		value, held := pipe.output.Values[metric[1]]

		if !held {
			continue
		}

		if metric[4] != "" && pipe.output.Values[metric[4]] == 0 {
			continue
		}

		metrics = append(metrics, data.NewMetric(
			metric[0], value, data.Unit(metric[2]), data.Timescale(metric[3]),
		))
	}

	return out.Write(metrics...)
}
