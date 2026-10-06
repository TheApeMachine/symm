package toxicity

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
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Signal is the toxic-flow instrument: touch disposition (retreats,
withdrawals, replenishments) and trade matching against the touch, one
measurement per trade. It holds no logic of its own: its entire behavior is
three nomagique pipelines per symbol, each a transport.Parallel of stage
groups over one shared output map. Every predicate is arithmetic over
calculus.Sign — at-touch is 1 - sign², a retreat is (sign² ∓ sign) / 2, and a
positive part is (x + |x|) / 2 — so no branch lives outside the primitives.
The match pipeline runs on every trade; the disposition pipeline once a
previous touch exists; the rate pipeline once venue time has also advanced.
The pipeline is driven by trade frames only: the trade supplies price,
quantity, and aggressor side; the touch quote is read exclusively from the
injected BookSource. The touch and the previous touch are the only envelope
translation. Facts accumulate in the output map and are
written once.
*/
type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
	books     broker.BookSource
	pipelines sync.Map
	metrics   [][4]string
}

type symbolPipeline struct {
	output            data.Map[float64]
	envelope          data.Map[float64]
	matchStates       []*data.State
	match             core.Primitive
	dispositionStates []*data.State
	disposition       core.Primitive
	rateStates        []*data.State
	rate              core.Primitive
	hasPrev           bool
	prevBid           float64
	prevAsk           float64
	prevBidQty        float64
	prevAskQty        float64
	prevAt            time.Time
}

/*
NewSignal composes the toxic-flow instrument. The BookSource is required: it
is the sole authority for the touch. A missing BookSource is a wiring fault
and halts the instrument.
*/
func NewSignal(ctx context.Context, arena *data.ArenaOwner, books broker.BookSource) *Signal {
	signal := &Signal{
		arena: arena,
		books: books,
		// {published label, output key, unit, timescale}
		metrics: [][4]string{
			{"best_price:bid", "bid", string(data.UnitPrice), string(data.TimescaleInstantaneous)},
			{"best_price:ask", "ask", string(data.UnitPrice), string(data.TimescaleInstantaneous)},
			{"touch_quantity:bid", "bid_qty", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"touch_quantity:ask", "ask_qty", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"unfilled_residual_quantity:bid", "bid_qty", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"unfilled_residual_quantity:ask", "ask_qty", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"bracket_trade_quantity", "bracket_trade_quantity", string(data.UnitQuantity), string(data.TimescaleSession)},
			{"matched_touch_trade_quantity:bid", "touch_fill_quantity:bid", string(data.UnitQuantity), string(data.TimescaleSession)},
			{"matched_touch_trade_quantity:ask", "touch_fill_quantity:ask", string(data.UnitQuantity), string(data.TimescaleSession)},
			{"touch_fill_quantity:bid", "touch_fill_quantity:bid", string(data.UnitQuantity), string(data.TimescaleSession)},
			{"touch_fill_quantity:ask", "touch_fill_quantity:ask", string(data.UnitQuantity), string(data.TimescaleSession)},
			{"touch_fill_fraction:bid", "touch_fill_fraction:bid", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"touch_fill_fraction:ask", "touch_fill_fraction:ask", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"fill_fraction_baseline:bid", "fill_fraction_baseline:bid", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"fill_fraction_baseline:ask", "fill_fraction_baseline:ask", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"fill_fraction_divergence:bid", "fill_fraction_divergence:bid", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"fill_fraction_divergence:ask", "fill_fraction_divergence:ask", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"fill_fraction_zscore:bid", "fill_fraction_zscore:bid", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"fill_fraction_zscore:ask", "fill_fraction_zscore:ask", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"fill_fraction_velocity:bid", "fill_fraction_velocity:bid", string(data.UnitVelocity), string(data.TimescaleInstantaneous)},
			{"fill_fraction_velocity:ask", "fill_fraction_velocity:ask", string(data.UnitVelocity), string(data.TimescaleInstantaneous)},
			{"previous_best_price:bid", "prev_bid", string(data.UnitPrice), string(data.TimescaleInstantaneous)},
			{"previous_best_price:ask", "prev_ask", string(data.UnitPrice), string(data.TimescaleInstantaneous)},
			{"previous_touch_quantity:bid", "prev_bid_qty", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"previous_touch_quantity:ask", "prev_ask_qty", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"touch_price_log_change:bid", "touch_price_log_change:bid", string(data.UnitLogReturn), string(data.TimescaleInstantaneous)},
			{"touch_price_log_change:ask", "touch_price_log_change:ask", string(data.UnitLogReturn), string(data.TimescaleInstantaneous)},
			{"retreated_quantity:bid", "retreated_quantity:bid", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"retreated_quantity:ask", "retreated_quantity:ask", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"retreat_fraction:bid", "retreat_fraction:bid", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"retreat_fraction:ask", "retreat_fraction:ask", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"retreat_rate:bid", "retreat_rate:bid", string(data.UnitRate), string(data.TimescalePerSecond)},
			{"retreat_rate:ask", "retreat_rate:ask", string(data.UnitRate), string(data.TimescalePerSecond)},
			{"net_withdrawn_quantity:bid", "net_withdrawn_quantity:bid", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"net_withdrawn_quantity:ask", "net_withdrawn_quantity:ask", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"net_withdrawal_fraction:bid", "net_withdrawal_fraction:bid", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"net_withdrawal_fraction:ask", "net_withdrawal_fraction:ask", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"net_withdrawal_rate:bid", "net_withdrawal_rate:bid", string(data.UnitRate), string(data.TimescalePerSecond)},
			{"net_withdrawal_rate:ask", "net_withdrawal_rate:ask", string(data.UnitRate), string(data.TimescalePerSecond)},
			{"net_replenished_quantity:bid", "net_replenished_quantity:bid", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"net_replenished_quantity:ask", "net_replenished_quantity:ask", string(data.UnitQuantity), string(data.TimescaleInstantaneous)},
			{"net_replenishment_fraction:bid", "net_replenishment_fraction:bid", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"net_replenishment_fraction:ask", "net_replenishment_fraction:ask", string(data.UnitRatio), string(data.TimescaleInstantaneous)},
			{"net_replenishment_rate:bid", "net_replenishment_rate:bid", string(data.UnitRate), string(data.TimescalePerSecond)},
			{"net_replenishment_rate:ask", "net_replenishment_rate:ask", string(data.UnitRate), string(data.TimescalePerSecond)},
			{"touch_fill_rate:bid", "touch_fill_rate:bid", string(data.UnitRate), string(data.TimescalePerSecond)},
			{"touch_fill_rate:ask", "touch_fill_rate:ask", string(data.UnitRate), string(data.TimescalePerSecond)},
			{"withdrawal_fraction_baseline:bid", "withdrawal_fraction_baseline:bid", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"withdrawal_fraction_baseline:ask", "withdrawal_fraction_baseline:ask", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"withdrawal_fraction_divergence:bid", "withdrawal_fraction_divergence:bid", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"withdrawal_fraction_divergence:ask", "withdrawal_fraction_divergence:ask", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"withdrawal_fraction_zscore:bid", "withdrawal_fraction_zscore:bid", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"withdrawal_fraction_zscore:ask", "withdrawal_fraction_zscore:ask", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"withdrawal_fraction_velocity:bid", "withdrawal_fraction_velocity:bid", string(data.UnitVelocity), string(data.TimescaleInstantaneous)},
			{"withdrawal_fraction_velocity:ask", "withdrawal_fraction_velocity:ask", string(data.UnitVelocity), string(data.TimescaleInstantaneous)},
			{"retreat_fraction_baseline:bid", "retreat_fraction_baseline:bid", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"retreat_fraction_baseline:ask", "retreat_fraction_baseline:ask", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"retreat_fraction_zscore:bid", "retreat_fraction_zscore:bid", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"retreat_fraction_zscore:ask", "retreat_fraction_zscore:ask", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
			{"replenishment_fraction_baseline:bid", "replenishment_fraction_baseline:bid", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"replenishment_fraction_baseline:ask", "replenishment_fraction_baseline:ask", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"historical_path_distance", "historical_path_distance", string(data.UnitDistance), string(data.TimescaleRollingWindow)},
			{"historical_path_percentile", "historical_path_percentile", string(data.UnitPercent), string(data.TimescaleRollingWindow)},
		},
	}

	signal.System = runtime.NewSystem(ctx, "toxicity", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[toxicity] book manager is required", err))
	}

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
		matchStates: []*data.State{
			// 0-6: Trade price against the bid: at the bid, and not below it.
			data.NewState(data.NewMap("left", "price", "right", "bid", "subtract", "bid_gap_sign"), output),
			data.NewState(data.NewMap("value", "bid_gap_sign", "sign", "bid_gap_sign"), output),
			data.NewState(data.NewMap("left", "bid_gap_sign", "right", "bid_gap_sign", "multiply", "bid_gap_magnitude"), output),
			data.NewState(data.NewMap("left", "one", "right", "bid_gap_magnitude", "subtract", "at_bid"), output),
			data.NewState(data.NewMap("left", "bid_gap_magnitude", "right", "bid_gap_sign", "subtract", "below_bid:double"), output),
			data.NewState(data.NewMap("left", "below_bid:double", "right", "half", "multiply", "below_bid"), output),
			data.NewState(data.NewMap("left", "one", "right", "below_bid", "subtract", "not_below_bid"), output),
			// 7-13: Trade price against the ask: at the ask, and not above it.
			data.NewState(data.NewMap("left", "ask", "right", "price", "subtract", "ask_gap_sign"), output),
			data.NewState(data.NewMap("value", "ask_gap_sign", "sign", "ask_gap_sign"), output),
			data.NewState(data.NewMap("left", "ask_gap_sign", "right", "ask_gap_sign", "multiply", "ask_gap_magnitude"), output),
			data.NewState(data.NewMap("left", "one", "right", "ask_gap_magnitude", "subtract", "at_ask"), output),
			data.NewState(data.NewMap("left", "ask_gap_magnitude", "right", "ask_gap_sign", "subtract", "above_ask:double"), output),
			data.NewState(data.NewMap("left", "above_ask:double", "right", "half", "multiply", "above_ask"), output),
			data.NewState(data.NewMap("left", "one", "right", "above_ask", "subtract", "not_above_ask"), output),
			// 14-16: Quantity traded inside the bracket [bid, ask].
			data.NewState(data.NewMap("left", "not_below_bid", "right", "not_above_ask", "multiply", "in_bracket"), output),
			data.NewState(data.NewMap("left", "in_bracket", "right", "qty", "multiply", "bracket_increment"), output),
			data.NewState(data.NewMap("value", "bracket_increment", "sum", "bracket_trade_quantity"), output),
			// 17-22: Aggressive sells filling the bid, aggressive buys filling the ask.
			data.NewState(data.NewMap("left", "sell", "right", "at_bid", "multiply", "bid_match"), output),
			data.NewState(data.NewMap("left", "bid_match", "right", "qty", "multiply", "bid_fill_increment"), output),
			data.NewState(data.NewMap("value", "bid_fill_increment", "sum", "touch_fill_quantity:bid"), output),
			data.NewState(data.NewMap("left", "buy", "right", "at_ask", "multiply", "ask_match"), output),
			data.NewState(data.NewMap("left", "ask_match", "right", "qty", "multiply", "ask_fill_increment"), output),
			data.NewState(data.NewMap("value", "ask_fill_increment", "sum", "touch_fill_quantity:ask"), output),
			// 23-26: Cumulative fill against the displayed touch, on the trade that matched.
			data.NewState(data.NewMap("left", "touch_fill_quantity:bid", "right", "bid_qty", "divide", "fill_ratio:bid"), output),
			data.NewState(data.NewMap("left", "bid_match", "right", "fill_ratio:bid", "multiply", "touch_fill_fraction:bid"), output),
			data.NewState(data.NewMap("left", "touch_fill_quantity:ask", "right", "ask_qty", "divide", "fill_ratio:ask"), output),
			data.NewState(data.NewMap("left", "ask_match", "right", "fill_ratio:ask", "multiply", "touch_fill_fraction:ask"), output),
			// 27-32: Fill fractions against their own causal baselines.
			data.NewState(data.NewMap("value", "touch_fill_fraction:bid", "center", "fill_fraction_baseline:bid", "scale", "fill_fraction_noise_scale:bid"), output),
			data.NewState(data.NewMap("left", "touch_fill_fraction:bid", "right", "fill_fraction_baseline:bid", "subtract", "fill_fraction_divergence:bid"), output),
			data.NewState(data.NewMap("left", "fill_fraction_divergence:bid", "right", "fill_fraction_noise_scale:bid", "divide", "fill_fraction_zscore:bid"), output),
			data.NewState(data.NewMap("value", "touch_fill_fraction:ask", "center", "fill_fraction_baseline:ask", "scale", "fill_fraction_noise_scale:ask"), output),
			data.NewState(data.NewMap("left", "touch_fill_fraction:ask", "right", "fill_fraction_baseline:ask", "subtract", "fill_fraction_divergence:ask"), output),
			data.NewState(data.NewMap("left", "fill_fraction_divergence:ask", "right", "fill_fraction_noise_scale:ask", "divide", "fill_fraction_zscore:ask"), output),
			// 33-34: Fill fraction velocities.
			data.NewState(data.NewMap("value", "touch_fill_fraction:bid", "rate", "fill_fraction_velocity:bid", "defined", "fill_fraction_velocity:bid:defined"), output),
			data.NewState(data.NewMap("value", "touch_fill_fraction:ask", "rate", "fill_fraction_velocity:ask", "defined", "fill_fraction_velocity:ask:defined"), output),
		},
		match: transport.NewParallel(
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(calculus.NewSign()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(calculus.NewSign()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(statistic.NewSum()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(statistic.NewSum()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(statistic.NewSum()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(temporal.NewVelocity()),
			transport.NewStages(temporal.NewVelocity()),
		),
		dispositionStates: []*data.State{
			// 0-3: Touch price log changes.
			data.NewState(data.NewMap("left", "bid", "right", "prev_bid", "divide", "touch_price_log_change:bid"), output),
			data.NewState(data.NewMap("value", "touch_price_log_change:bid", "log", "touch_price_log_change:bid"), output),
			data.NewState(data.NewMap("left", "ask", "right", "prev_ask", "divide", "touch_price_log_change:ask"), output),
			data.NewState(data.NewMap("value", "touch_price_log_change:ask", "log", "touch_price_log_change:ask"), output),
			// 4-9: Bid retreat, (sign² - sign) / 2 of the bid step.
			data.NewState(data.NewMap("left", "bid", "right", "prev_bid", "subtract", "bid_step_sign"), output),
			data.NewState(data.NewMap("value", "bid_step_sign", "sign", "bid_step_sign"), output),
			data.NewState(data.NewMap("left", "bid_step_sign", "right", "bid_step_sign", "multiply", "bid_step_magnitude"), output),
			data.NewState(data.NewMap("left", "bid_step_magnitude", "right", "bid_step_sign", "subtract", "retreat:bid:double"), output),
			data.NewState(data.NewMap("left", "retreat:bid:double", "right", "half", "multiply", "retreat_fraction:bid"), output),
			data.NewState(data.NewMap("left", "retreat_fraction:bid", "right", "prev_bid_qty", "multiply", "retreated_quantity:bid"), output),
			// 10-21: Held bid price: positive and negative parts of the quantity drop.
			data.NewState(data.NewMap("left", "one", "right", "bid_step_magnitude", "subtract", "bid_held"), output),
			data.NewState(data.NewMap("left", "prev_bid_qty", "right", "bid_qty", "subtract", "bid_qty_drop"), output),
			data.NewState(data.NewMap("left", "bid_qty_drop", "right", "zero", "subtract", "bid_qty_drop_magnitude"), output),
			data.NewState(data.NewMap("value", "bid_qty_drop_magnitude", "absolute", "bid_qty_drop_magnitude"), output),
			data.NewState(data.NewMap("left", "bid_qty_drop_magnitude", "right", "bid_qty_drop", "add", "withdrawn:bid:double"), output),
			data.NewState(data.NewMap("left", "withdrawn:bid:double", "right", "half", "multiply", "withdrawn:bid"), output),
			data.NewState(data.NewMap("left", "bid_held", "right", "withdrawn:bid", "multiply", "net_withdrawn_quantity:bid"), output),
			data.NewState(data.NewMap("left", "bid_qty_drop_magnitude", "right", "bid_qty_drop", "subtract", "replenished:bid:double"), output),
			data.NewState(data.NewMap("left", "replenished:bid:double", "right", "half", "multiply", "replenished:bid"), output),
			data.NewState(data.NewMap("left", "bid_held", "right", "replenished:bid", "multiply", "net_replenished_quantity:bid"), output),
			data.NewState(data.NewMap("left", "net_withdrawn_quantity:bid", "right", "prev_bid_qty", "divide", "net_withdrawal_fraction:bid"), output),
			data.NewState(data.NewMap("left", "net_replenished_quantity:bid", "right", "prev_bid_qty", "divide", "net_replenishment_fraction:bid"), output),
			// 22-27: Ask retreat, (sign² + sign) / 2 of the ask step.
			data.NewState(data.NewMap("left", "ask", "right", "prev_ask", "subtract", "ask_step_sign"), output),
			data.NewState(data.NewMap("value", "ask_step_sign", "sign", "ask_step_sign"), output),
			data.NewState(data.NewMap("left", "ask_step_sign", "right", "ask_step_sign", "multiply", "ask_step_magnitude"), output),
			data.NewState(data.NewMap("left", "ask_step_magnitude", "right", "ask_step_sign", "add", "retreat:ask:double"), output),
			data.NewState(data.NewMap("left", "retreat:ask:double", "right", "half", "multiply", "retreat_fraction:ask"), output),
			data.NewState(data.NewMap("left", "retreat_fraction:ask", "right", "prev_ask_qty", "multiply", "retreated_quantity:ask"), output),
			// 28-39: Held ask price: positive and negative parts of the quantity drop.
			data.NewState(data.NewMap("left", "one", "right", "ask_step_magnitude", "subtract", "ask_held"), output),
			data.NewState(data.NewMap("left", "prev_ask_qty", "right", "ask_qty", "subtract", "ask_qty_drop"), output),
			data.NewState(data.NewMap("left", "ask_qty_drop", "right", "zero", "subtract", "ask_qty_drop_magnitude"), output),
			data.NewState(data.NewMap("value", "ask_qty_drop_magnitude", "absolute", "ask_qty_drop_magnitude"), output),
			data.NewState(data.NewMap("left", "ask_qty_drop_magnitude", "right", "ask_qty_drop", "add", "withdrawn:ask:double"), output),
			data.NewState(data.NewMap("left", "withdrawn:ask:double", "right", "half", "multiply", "withdrawn:ask"), output),
			data.NewState(data.NewMap("left", "ask_held", "right", "withdrawn:ask", "multiply", "net_withdrawn_quantity:ask"), output),
			data.NewState(data.NewMap("left", "ask_qty_drop_magnitude", "right", "ask_qty_drop", "subtract", "replenished:ask:double"), output),
			data.NewState(data.NewMap("left", "replenished:ask:double", "right", "half", "multiply", "replenished:ask"), output),
			data.NewState(data.NewMap("left", "ask_held", "right", "replenished:ask", "multiply", "net_replenished_quantity:ask"), output),
			data.NewState(data.NewMap("left", "net_withdrawn_quantity:ask", "right", "prev_ask_qty", "divide", "net_withdrawal_fraction:ask"), output),
			data.NewState(data.NewMap("left", "net_replenished_quantity:ask", "right", "prev_ask_qty", "divide", "net_replenishment_fraction:ask"), output),
			// 40-45: Withdrawal fractions against their own causal baselines.
			data.NewState(data.NewMap("value", "net_withdrawal_fraction:bid", "center", "withdrawal_fraction_baseline:bid", "scale", "withdrawal_fraction_noise_scale:bid"), output),
			data.NewState(data.NewMap("left", "net_withdrawal_fraction:bid", "right", "withdrawal_fraction_baseline:bid", "subtract", "withdrawal_fraction_divergence:bid"), output),
			data.NewState(data.NewMap("left", "withdrawal_fraction_divergence:bid", "right", "withdrawal_fraction_noise_scale:bid", "divide", "withdrawal_fraction_zscore:bid"), output),
			data.NewState(data.NewMap("value", "net_withdrawal_fraction:ask", "center", "withdrawal_fraction_baseline:ask", "scale", "withdrawal_fraction_noise_scale:ask"), output),
			data.NewState(data.NewMap("left", "net_withdrawal_fraction:ask", "right", "withdrawal_fraction_baseline:ask", "subtract", "withdrawal_fraction_divergence:ask"), output),
			data.NewState(data.NewMap("left", "withdrawal_fraction_divergence:ask", "right", "withdrawal_fraction_noise_scale:ask", "divide", "withdrawal_fraction_zscore:ask"), output),
			// 46-51: Retreat fractions against their own causal baselines.
			data.NewState(data.NewMap("value", "retreat_fraction:bid", "center", "retreat_fraction_baseline:bid", "scale", "retreat_fraction_noise_scale:bid"), output),
			data.NewState(data.NewMap("left", "retreat_fraction:bid", "right", "retreat_fraction_baseline:bid", "subtract", "retreat_fraction_divergence:bid"), output),
			data.NewState(data.NewMap("left", "retreat_fraction_divergence:bid", "right", "retreat_fraction_noise_scale:bid", "divide", "retreat_fraction_zscore:bid"), output),
			data.NewState(data.NewMap("value", "retreat_fraction:ask", "center", "retreat_fraction_baseline:ask", "scale", "retreat_fraction_noise_scale:ask"), output),
			data.NewState(data.NewMap("left", "retreat_fraction:ask", "right", "retreat_fraction_baseline:ask", "subtract", "retreat_fraction_divergence:ask"), output),
			data.NewState(data.NewMap("left", "retreat_fraction_divergence:ask", "right", "retreat_fraction_noise_scale:ask", "divide", "retreat_fraction_zscore:ask"), output),
			// 52-53: Replenishment fraction baselines.
			data.NewState(data.NewMap("value", "net_replenishment_fraction:bid", "center", "replenishment_fraction_baseline:bid", "scale", "replenishment_fraction_noise_scale:bid"), output),
			data.NewState(data.NewMap("value", "net_replenishment_fraction:ask", "center", "replenishment_fraction_baseline:ask", "scale", "replenishment_fraction_noise_scale:ask"), output),
			// 54-55: Withdrawal fraction velocities.
			data.NewState(data.NewMap("value", "net_withdrawal_fraction:bid", "rate", "withdrawal_fraction_velocity:bid", "defined", "withdrawal_fraction_velocity:bid:defined"), output),
			data.NewState(data.NewMap("value", "net_withdrawal_fraction:ask", "rate", "withdrawal_fraction_velocity:ask", "defined", "withdrawal_fraction_velocity:ask:defined"), output),
		},
		disposition: transport.NewParallel(
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(calculus.NewLog()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(calculus.NewLog()),
			// bid retreat
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(calculus.NewSign()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewMultiply()),
			// bid held
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(calculus.NewAbsolute()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			// ask retreat
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(calculus.NewSign()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewMultiply()),
			// ask held
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(calculus.NewAbsolute()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			// withdrawal baselines
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			// retreat baselines
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			// replenishment baselines
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			// withdrawal velocities
			transport.NewStages(temporal.NewVelocity()),
			transport.NewStages(temporal.NewVelocity()),
		),
		rateStates: []*data.State{
			// 0: Elapsed venue time since the previous touch.
			data.NewState(data.NewMap("from", "prev_at", "to", "at", "elapsed", "elapsed"), output),
			// 1-4: Retreat rates, the retreated touch per elapsed second.
			data.NewState(data.NewMap("left", "prev_bid_qty", "right", "elapsed", "divide", "retreat_rate_scale:bid"), output),
			data.NewState(data.NewMap("left", "retreat_fraction:bid", "right", "retreat_rate_scale:bid", "multiply", "retreat_rate:bid"), output),
			data.NewState(data.NewMap("left", "prev_ask_qty", "right", "elapsed", "divide", "retreat_rate_scale:ask"), output),
			data.NewState(data.NewMap("left", "retreat_fraction:ask", "right", "retreat_rate_scale:ask", "multiply", "retreat_rate:ask"), output),
			// 5-8: Withdrawal and replenishment rates.
			data.NewState(data.NewMap("left", "net_withdrawn_quantity:bid", "right", "elapsed", "divide", "net_withdrawal_rate:bid"), output),
			data.NewState(data.NewMap("left", "net_withdrawn_quantity:ask", "right", "elapsed", "divide", "net_withdrawal_rate:ask"), output),
			data.NewState(data.NewMap("left", "net_replenished_quantity:bid", "right", "elapsed", "divide", "net_replenishment_rate:bid"), output),
			data.NewState(data.NewMap("left", "net_replenished_quantity:ask", "right", "elapsed", "divide", "net_replenishment_rate:ask"), output),
			// 9-10: Cumulative touch fill per elapsed second.
			data.NewState(data.NewMap("left", "touch_fill_quantity:bid", "right", "elapsed", "divide", "touch_fill_rate:bid"), output),
			data.NewState(data.NewMap("left", "touch_fill_quantity:ask", "right", "elapsed", "divide", "touch_fill_rate:ask"), output),
		},
		rate: transport.NewParallel(
			transport.NewStages(temporal.NewElapsed()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
		),
	}

	actual, _ := signal.pipelines.LoadOrStore(symbol, pipe)
	return actual.(*symbolPipeline)
}

/*
Step binds the prior trade and the symbol's touch, read from the BookSource,
to the symbol's pipelines and writes the published toxicity facts into a fresh
Measurement allocated from the signal's own arena. A trade frame that cannot
be read, or that lacks price or qty, is an error. A trade with a non-positive
or non-finite price or quantity yields no measurement. A book that is
momentarily absent or one-sided yields no measurement (as in depthflow). A
present touch with a non-finite or non-positive price or quantity
(broker.InvalidTouch) or a crossed or locked touch (broker.CrossedTouch) is
corrupt book state and halts the signal. Disposition facts are omitted on a
symbol's first trade, and rates are omitted until venue time advances. A trade
without an explicit aggressor side is bracketed but never matched to a touch.
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
		signal.Error(errnie.Err(errnie.Internal, "[toxicity] book manager is required", nil))
		return nil
	}

	pipe := signal.pipelineFor(prior.Label)

	clear(pipe.output.Values)
	clear(pipe.envelope.Values)

	for _, key := range []string{"price", "qty"} {
		value, err := tradeValue(prior, key)

		if err != nil {
			signal.Error(err)
			return nil
		}

		pipe.envelope.Values[key] = value
	}

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

	for _, key := range []string{"price", "qty"} {
		value := pipe.envelope.Values[key]

		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil
		}
	}

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
			signal.Error(broker.InvalidTouch("toxicity", prior.Label, touch))
			return nil
		}
	}

	if pipe.envelope.Values["ask"] <= pipe.envelope.Values["bid"] {
		// BookSource already withholds pending and checksum-diverging books,
		// so a crossed or locked touch here is corrupt book state, never a
		// market fact. It halts (Internal) like a missing BookSource, but its
		// Conflict cause and message name the symbol and touch so the two
		// faults are never confused.
		signal.Error(broker.CrossedTouch("toxicity", prior.Label, pipe.envelope.Values["bid"], pipe.envelope.Values["ask"]))
		return nil
	}

	pipe.envelope.Values["buy"] = 0
	pipe.envelope.Values["sell"] = 0

	switch prior.Meta("side") {
	case "buy":
		pipe.envelope.Values["buy"] = 1
	case "sell":
		pipe.envelope.Values["sell"] = 1
	}

	hadPrev := pipe.hasPrev
	prevAt := pipe.prevAt
	rateReady := hadPrev && prior.At.After(prevAt)

	pipe.envelope.Values["at"] = float64(prior.At.UnixNano())
	pipe.envelope.Values["zero"] = 0
	pipe.envelope.Values["one"] = 1
	pipe.envelope.Values["half"] = 0.5

	if hadPrev {
		pipe.envelope.Values["prev_bid"] = pipe.prevBid
		pipe.envelope.Values["prev_ask"] = pipe.prevAsk
		pipe.envelope.Values["prev_bid_qty"] = pipe.prevBidQty
		pipe.envelope.Values["prev_ask_qty"] = pipe.prevAskQty
		pipe.envelope.Values["prev_at"] = float64(prevAt.UnixNano())
	}

	publisher := data.NewAdapter(prior, data.NewState(data.NewMap(), pipe.output))

	for range publisher.Next(data.NewValue(pipe.envelope)) {
	}

	matchAdapters := make([]*data.Adapter, len(pipe.matchStates))

	for index, state := range pipe.matchStates {
		matchAdapters[index] = data.NewAdapter(prior, state)
	}

	for range pipe.match.Next(data.NewValue(matchAdapters...)) {
	}

	if hadPrev {
		dispositionAdapters := make([]*data.Adapter, len(pipe.dispositionStates))

		for index, state := range pipe.dispositionStates {
			dispositionAdapters[index] = data.NewAdapter(prior, state)
		}

		for range pipe.disposition.Next(data.NewValue(dispositionAdapters...)) {
		}
	}

	if rateReady {
		rateAdapters := make([]*data.Adapter, len(pipe.rateStates))

		for index, state := range pipe.rateStates {
			rateAdapters[index] = data.NewAdapter(prior, state)
		}

		for range pipe.rate.Next(data.NewValue(rateAdapters...)) {
		}
	}

	if err := errors.Join(
		publisher.Error(), pipe.match.Error(), pipe.disposition.Error(), pipe.rate.Error(),
	); err != nil {
		signal.Error(err)
		return nil
	}

	pipe.prevBid = pipe.envelope.Values["bid"]
	pipe.prevAsk = pipe.envelope.Values["ask"]
	pipe.prevBidQty = pipe.envelope.Values["bid_qty"]
	pipe.prevAskQty = pipe.envelope.Values["ask_qty"]
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

	if rateReady {
		out.From = prevAt
	}

	metrics := make([]*data.Metric, 0, len(signal.metrics))

	for _, metric := range signal.metrics {
		value := pipe.output.Values[metric[1]]

		metrics = append(metrics, data.NewMetric(
			metric[0], value, data.Unit(metric[2]), data.Timescale(metric[3]),
		))
	}

	return out.Write(metrics...)
}

/*
tradeValue reads one required trade field from a trade frame. A read failure
or an absent field is an error: every trade frame carries price and qty, so a
frame without them is broken upstream and must not be silently skipped.
*/
func tradeValue(prior *data.Measurement, key string) (float64, error) {
	entry := data.Pull(prior.Read(key))

	if entry != nil && entry.Err != nil {
		return 0, entry.Err
	}

	if entry == nil || entry.Metric == nil || entry.Metric.Label != key {
		return 0, errnie.Err(
			errnie.NotAcceptable, "[toxicity] trade frame is missing "+key, nil,
		)
	}

	return entry.Metric.Raw, nil
}
