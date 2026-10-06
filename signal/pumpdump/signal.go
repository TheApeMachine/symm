package pumpdump

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
Signal is the volume-clocked tape activity instrument (legacy name pumpdump).
It holds no logic of its own: its entire behavior is five nomagique pipelines
per symbol, each a transport.Parallel of stage groups over one shared output
map. The tape pipeline runs on every valid trade and accumulates the open
volume bar against a target fixed from the causal quantity baseline when the
bar opens. The interval pipeline runs once a previous trade exists; the touch
pipeline whenever an executable, uncrossed touch is available; the bar
pipeline when the bar closes; the response pipeline when a closed bar has a
valid touch at both ends. The trade, the touch (from the trade, or the shared
book), and the open bar's running totals are the only envelope translation.
Facts accumulate in the output map and are written once.
*/
type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
	books     broker.BookSource
	pipelines sync.Map
	metrics   [][5]string
}

type symbolPipeline struct {
	output         data.Map[float64]
	envelope       data.Map[float64]
	tapeStates     []*data.State
	tape           core.Primitive
	intervalStates []*data.State
	interval       core.Primitive
	touchStates    []*data.State
	touch          core.Primitive
	barStates      []*data.State
	bar            core.Primitive
	responseStates []*data.State
	response       core.Primitive
	hasPrev        bool
	prevAt         time.Time
	barStart       time.Time
	barQuantity    float64
	barNotional    float64
	barTradeCount  float64
	barTarget      float64
	barFromMid     float64
}

/*
NewSignal composes the volume-clocked activity instrument. The BookSource
supplies the touch when the arriving trade does not carry it.
*/
func NewSignal(ctx context.Context, arena *data.ArenaOwner, books broker.BookSource) *Signal {
	signal := &Signal{
		arena: arena,
		books: books,
		// {published label, output key, unit, timescale, gate key}
		// A non-empty gate key publishes the metric only while that output is positive.
		metrics: [][5]string{
			{"trade_price", "price", string(data.UnitPrice), string(data.TimescaleTick), ""},
			{"trade_quantity", "qty", string(data.UnitQuantity), string(data.TimescaleTick), ""},
			{"trade_notional", "trade_notional", string(data.UnitNotional), string(data.TimescaleTick), ""},
			{"trade_interval_seconds", "trade_interval_seconds", string(data.UnitSecond), string(data.TimescaleTick), "trade_interval_seconds"},
			{"volume_bar_target_quantity", "volume_bar_target_quantity", string(data.UnitQuantity), string(data.TimescaleVolumeBar), "completed_bars"},
			{"volume_bar_quantity", "volume_bar_quantity", string(data.UnitQuantity), string(data.TimescaleVolumeBar), "completed_bars"},
			{"volume_bar_notional", "volume_bar_notional", string(data.UnitNotional), string(data.TimescaleVolumeBar), "completed_bars"},
			{"volume_bar_trade_count", "volume_bar_trade_count", string(data.UnitCount), string(data.TimescaleVolumeBar), "completed_bars"},
			{"volume_bar_duration", "volume_bar_duration", string(data.UnitDuration), string(data.TimescaleVolumeBar), "completed_bars"},
			{"volume_rate", "volume_rate", string(data.UnitVolumeRate), string(data.TimescalePerSecond), "completed_bars"},
			{"notional_rate", "notional_rate", string(data.UnitNotionalRate), string(data.TimescalePerSecond), "completed_bars"},
			{"trade_rate", "trade_rate", string(data.UnitTradeRate), string(data.TimescalePerSecond), "completed_bars"},
			{"completed_bars", "completed_bars", string(data.UnitCount), string(data.TimescaleSession), "completed_bars"},
			{"notional_rate_baseline", "notional_rate_baseline", string(data.UnitNotionalRate), string(data.TimescaleRollingWindow), "notional_rate_noise_scale"},
			{"notional_rate_ratio", "notional_rate_ratio", string(data.UnitRatio), string(data.TimescaleRollingWindow), "notional_rate_noise_scale"},
			{"notional_rate_divergence", "notional_rate_divergence", string(data.UnitLogReturn), string(data.TimescaleRollingWindow), "notional_rate_noise_scale"},
			{"notional_rate_zscore", "notional_rate_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow), "notional_rate_noise_scale"},
			{"notional_rate_velocity", "notional_rate_velocity", string(data.UnitVelocity), string(data.TimescalePerSecond), "notional_rate_velocity:defined"},
			{"best_bid", "bid", string(data.UnitPrice), string(data.TimescaleTick), "relative_spread"},
			{"best_ask", "ask", string(data.UnitPrice), string(data.TimescaleTick), "relative_spread"},
			{"midpoint", "midpoint", string(data.UnitPrice), string(data.TimescaleTick), "relative_spread"},
			{"spread", "spread", string(data.UnitSpread), string(data.TimescaleTick), "relative_spread"},
			{"relative_spread", "relative_spread", string(data.UnitRelativeSpread), string(data.TimescaleTick), "relative_spread"},
			{"relative_spread_baseline", "relative_spread_baseline", string(data.UnitRelativeSpread), string(data.TimescaleRollingWindow), "spread_noise_scale"},
			{"spread_ratio", "spread_ratio", string(data.UnitRatio), string(data.TimescaleRollingWindow), "spread_noise_scale"},
			{"spread_divergence", "spread_divergence", string(data.UnitLogReturn), string(data.TimescaleRollingWindow), "spread_noise_scale"},
			{"spread_zscore", "spread_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow), "spread_noise_scale"},
			{"spread_divergence_velocity", "spread_divergence_velocity", string(data.UnitVelocity), string(data.TimescalePerSecond), "spread_divergence_velocity:defined"},
			{"midpoint:from", "midpoint:from", string(data.UnitPrice), string(data.TimescaleVolumeBar), "midpoint_ratio"},
			{"midpoint:at", "midpoint", string(data.UnitPrice), string(data.TimescaleVolumeBar), "midpoint_ratio"},
			{"midpoint_log_return", "midpoint_log_return", string(data.UnitLogReturn), string(data.TimescaleVolumeBar), "midpoint_ratio"},
			{"midpoint_return_rate", "midpoint_return_rate", string(data.UnitVelocity), string(data.TimescalePerSecond), "midpoint_ratio"},
			{"positive_midpoint_return", "positive_midpoint_return", string(data.UnitLogReturn), string(data.TimescaleVolumeBar), "midpoint_ratio"},
			{"negative_midpoint_return", "negative_midpoint_return", string(data.UnitLogReturn), string(data.TimescaleVolumeBar), "midpoint_ratio"},
			{"midpoint_return_baseline", "midpoint_return_baseline", string(data.UnitLogReturn), string(data.TimescaleRollingWindow), "midpoint_return_noise_scale"},
			{"midpoint_return_divergence", "midpoint_return_divergence", string(data.UnitLogReturn), string(data.TimescaleRollingWindow), "midpoint_return_noise_scale"},
			{"midpoint_return_zscore", "midpoint_return_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow), "midpoint_return_noise_scale"},
			{"midpoint_return_velocity", "midpoint_return_velocity", string(data.UnitVelocity), string(data.TimescalePerSecond), "midpoint_return_velocity:defined"},
		},
	}

	signal.System = runtime.NewSystem(ctx, "pumpdump", signal)
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
		tapeStates: []*data.State{
			// 0: Trade notional.
			data.NewState(data.NewMap("left", "price", "right", "qty", "multiply", "trade_notional"), output),
			// 1-4: Open bar running totals, closing trade included.
			data.NewState(data.NewMap("left", "bar_quantity:prior", "right", "qty", "add", "volume_bar_quantity"), output),
			data.NewState(data.NewMap("left", "bar_notional:prior", "right", "trade_notional", "add", "volume_bar_notional"), output),
			data.NewState(data.NewMap("left", "bar_trade_count:prior", "right", "one", "add", "volume_bar_trade_count"), output),
			data.NewState(data.NewMap("from", "bar_start", "to", "at", "elapsed", "volume_bar_duration"), output),
			// 5: Causal trade-quantity baseline (prior mean; the first trade bootstraps itself).
			data.NewState(data.NewMap("value", "qty", "center", "trade_quantity_baseline", "scale", "trade_quantity_noise_scale"), output),
			// 6-10: Target fixed when the bar opens: bar_open = 1 - sign(prior count)².
			data.NewState(data.NewMap("left", "bar_trade_count:prior", "right", "one", "multiply", "bar_occupied"), output),
			data.NewState(data.NewMap("value", "bar_occupied", "sign", "bar_occupied"), output),
			data.NewState(data.NewMap("left", "bar_occupied", "right", "bar_occupied", "multiply", "bar_occupied:square"), output),
			data.NewState(data.NewMap("left", "one", "right", "bar_occupied:square", "subtract", "bar_open"), output),
			data.NewState(data.NewMap("left", "bar_target:prior", "right", "trade_quantity_baseline", "weight", "bar_open", "mix", "volume_bar_target_quantity"), output),
		},
		tape: transport.NewParallel(
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(temporal.NewElapsed()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(calculus.NewSign()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(calculus.NewMix()),
		),
		intervalStates: []*data.State{
			// 0: Venue time since the previous trade.
			data.NewState(data.NewMap("from", "prev_at", "to", "at", "elapsed", "trade_interval_seconds"), output),
		},
		interval: transport.NewParallel(
			transport.NewStages(temporal.NewElapsed()),
		),
		touchStates: []*data.State{
			// 0-2: Touch geometry, relative spread = spread / midpoint.
			data.NewState(data.NewMap("left", "ask", "right", "bid", "subtract", "spread"), output),
			data.NewState(data.NewMap("left", "bid", "right", "ask", "weight", "half", "mix", "midpoint"), output),
			data.NewState(data.NewMap("left", "spread", "right", "midpoint", "divide", "relative_spread"), output),
			// 3-10: Log relative spread against its own causal baseline.
			// Unary primitives answer in place, so each operand is copied (x · 1) first.
			data.NewState(data.NewMap("left", "relative_spread", "right", "one", "multiply", "log_relative_spread"), output),
			data.NewState(data.NewMap("value", "log_relative_spread", "log", "log_relative_spread"), output),
			data.NewState(data.NewMap("value", "log_relative_spread", "center", "log_relative_spread_baseline", "scale", "spread_noise_scale"), output),
			data.NewState(data.NewMap("left", "log_relative_spread_baseline", "right", "one", "multiply", "relative_spread_baseline"), output),
			data.NewState(data.NewMap("value", "relative_spread_baseline", "exp", "relative_spread_baseline"), output),
			data.NewState(data.NewMap("left", "log_relative_spread", "right", "log_relative_spread_baseline", "subtract", "spread_divergence"), output),
			data.NewState(data.NewMap("left", "relative_spread", "right", "relative_spread_baseline", "divide", "spread_ratio"), output),
			data.NewState(data.NewMap("left", "spread_divergence", "right", "spread_noise_scale", "divide", "spread_zscore"), output),
			// 11: Spread divergence velocity.
			data.NewState(data.NewMap("value", "spread_divergence", "rate", "spread_divergence_velocity", "defined", "spread_divergence_velocity:defined"), output),
		},
		touch: transport.NewParallel(
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(calculus.NewMix()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(calculus.NewLog()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(calculus.NewExp()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(temporal.NewVelocity()),
		),
		barStates: []*data.State{
			// 0-2: Completed-bar activity rates.
			data.NewState(data.NewMap("left", "volume_bar_quantity", "right", "volume_bar_duration", "divide", "volume_rate"), output),
			data.NewState(data.NewMap("left", "volume_bar_notional", "right", "volume_bar_duration", "divide", "notional_rate"), output),
			data.NewState(data.NewMap("left", "volume_bar_trade_count", "right", "volume_bar_duration", "divide", "trade_rate"), output),
			// 3-10: Log notional rate against its own causal baseline.
			data.NewState(data.NewMap("left", "notional_rate", "right", "one", "multiply", "log_notional_rate"), output),
			data.NewState(data.NewMap("value", "log_notional_rate", "log", "log_notional_rate"), output),
			data.NewState(data.NewMap("value", "log_notional_rate", "center", "log_notional_rate_baseline", "scale", "notional_rate_noise_scale"), output),
			data.NewState(data.NewMap("left", "log_notional_rate_baseline", "right", "one", "multiply", "notional_rate_baseline"), output),
			data.NewState(data.NewMap("value", "notional_rate_baseline", "exp", "notional_rate_baseline"), output),
			data.NewState(data.NewMap("left", "log_notional_rate", "right", "log_notional_rate_baseline", "subtract", "notional_rate_divergence"), output),
			data.NewState(data.NewMap("left", "notional_rate", "right", "notional_rate_baseline", "divide", "notional_rate_ratio"), output),
			data.NewState(data.NewMap("left", "notional_rate_divergence", "right", "notional_rate_noise_scale", "divide", "notional_rate_zscore"), output),
			// 11: Log notional rate velocity.
			data.NewState(data.NewMap("value", "log_notional_rate", "rate", "notional_rate_velocity", "defined", "notional_rate_velocity:defined"), output),
			// 12: Completed bar count.
			data.NewState(data.NewMap("value", "one", "sum", "completed_bars"), output),
		},
		bar: transport.NewParallel(
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(calculus.NewLog()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(calculus.NewExp()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(temporal.NewVelocity()),
			transport.NewStages(statistic.NewSum()),
		),
		responseStates: []*data.State{
			// 0-3: Midpoint log return over the bar and its rate.
			data.NewState(data.NewMap("left", "midpoint", "right", "midpoint:from", "divide", "midpoint_ratio"), output),
			data.NewState(data.NewMap("left", "midpoint_ratio", "right", "one", "multiply", "midpoint_log_return"), output),
			data.NewState(data.NewMap("value", "midpoint_log_return", "log", "midpoint_log_return"), output),
			data.NewState(data.NewMap("left", "midpoint_log_return", "right", "volume_bar_duration", "divide", "midpoint_return_rate"), output),
			// 4-9: Exact decomposition r = r⁺ - r⁻, r⁺ = (|r| + r) / 2, r⁻ = (|r| - r) / 2.
			data.NewState(data.NewMap("left", "midpoint_log_return", "right", "one", "multiply", "midpoint_log_return:absolute"), output),
			data.NewState(data.NewMap("value", "midpoint_log_return:absolute", "absolute", "midpoint_log_return:absolute"), output),
			data.NewState(data.NewMap("left", "midpoint_log_return:absolute", "right", "midpoint_log_return", "add", "positive_midpoint_return:double"), output),
			data.NewState(data.NewMap("left", "positive_midpoint_return:double", "right", "half", "multiply", "positive_midpoint_return"), output),
			data.NewState(data.NewMap("left", "midpoint_log_return:absolute", "right", "midpoint_log_return", "subtract", "negative_midpoint_return:double"), output),
			data.NewState(data.NewMap("left", "negative_midpoint_return:double", "right", "half", "multiply", "negative_midpoint_return"), output),
			// 10-12: Signed return against its own additive causal baseline.
			data.NewState(data.NewMap("value", "midpoint_log_return", "center", "midpoint_return_baseline", "scale", "midpoint_return_noise_scale"), output),
			data.NewState(data.NewMap("left", "midpoint_log_return", "right", "midpoint_return_baseline", "subtract", "midpoint_return_divergence"), output),
			data.NewState(data.NewMap("left", "midpoint_return_divergence", "right", "midpoint_return_noise_scale", "divide", "midpoint_return_zscore"), output),
			// 13: Midpoint return velocity.
			data.NewState(data.NewMap("value", "midpoint_log_return", "rate", "midpoint_return_velocity", "defined", "midpoint_return_velocity:defined"), output),
		},
		response: transport.NewParallel(
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(calculus.NewLog()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(calculus.NewAbsolute()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(temporal.NewVelocity()),
		),
	}

	actual, _ := signal.pipelines.LoadOrStore(symbol, pipe)
	return actual.(*symbolPipeline)
}

/*
Step binds the prior trade, the active touch, and the open bar's running
totals to the symbol's pipelines and writes the published activity facts into
a fresh Measurement allocated from the signal's own arena. A trade without a
positive, finite price and quantity yields no measurement. A trade without an
executable, uncrossed touch still advances tape accounting, but every
touch-dependent fact is omitted. Bar facts are published only on the trade
that closes a bar (accumulated quantity at or above the target, positive
duration); an open bar is never reported as a zero-rate bar. Baseline-relative
facts are omitted until their causal noise scale is defined and positive.
*/
func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	pipe := signal.pipelineFor(prior.Label)

	clear(pipe.output.Values)
	clear(pipe.envelope.Values)

	for _, key := range []string{"price", "qty", "bid", "ask"} {
		if entry := data.Pull(prior.Read(key)); entry.Err == nil && entry.Metric.Label != "" {
			pipe.envelope.Values[key] = entry.Metric.Raw
		}
	}

	for _, key := range []string{"price", "qty"} {
		value, held := pipe.envelope.Values[key]

		if !held || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil
		}
	}

	if signal.books != nil {
		signal.books.Book(prior.Label, func(book *spotbook.Book) {
			if book == nil {
				return
			}

			if _, held := pipe.envelope.Values["bid"]; !held {
				if best := book.BestBid(); best != nil && best.Price != nil {
					pipe.envelope.Values["bid"] = kraken.Float64(best.Price)
				}
			}

			if _, held := pipe.envelope.Values["ask"]; !held {
				if best := book.BestAsk(); best != nil && best.Price != nil {
					pipe.envelope.Values["ask"] = kraken.Float64(best.Price)
				}
			}
		})
	}

	bid, bidHeld := pipe.envelope.Values["bid"]
	ask, askHeld := pipe.envelope.Values["ask"]
	touchValid := bidHeld && askHeld &&
		bid > 0 && !math.IsInf(bid, 0) && !math.IsNaN(bid) &&
		ask > bid && !math.IsInf(ask, 0) && !math.IsNaN(ask)

	if !touchValid {
		errnie.Warn(signal.Name() + ": absent, crossed, or locked touch; touch facts omitted")
		delete(pipe.envelope.Values, "bid")
		delete(pipe.envelope.Values, "ask")
	}

	hadPrev := pipe.hasPrev
	prevAt := pipe.prevAt

	if !hadPrev {
		pipe.barStart = prior.At
	}

	barStart := pipe.barStart

	pipe.envelope.Values["at"] = float64(prior.At.UnixNano())
	pipe.envelope.Values["bar_start"] = float64(barStart.UnixNano())
	pipe.envelope.Values["bar_quantity:prior"] = pipe.barQuantity
	pipe.envelope.Values["bar_notional:prior"] = pipe.barNotional
	pipe.envelope.Values["bar_trade_count:prior"] = pipe.barTradeCount
	pipe.envelope.Values["bar_target:prior"] = pipe.barTarget
	pipe.envelope.Values["one"] = 1
	pipe.envelope.Values["half"] = 0.5

	if hadPrev {
		pipe.envelope.Values["prev_at"] = float64(prevAt.UnixNano())
	}

	if pipe.barFromMid > 0 {
		pipe.envelope.Values["midpoint:from"] = pipe.barFromMid
	}

	publisher := data.NewAdapter(prior, data.NewState(data.NewMap(), pipe.output))

	for range publisher.Next(data.NewValue(pipe.envelope)) {
	}

	tapeAdapters := make([]*data.Adapter, len(pipe.tapeStates))

	for index, state := range pipe.tapeStates {
		tapeAdapters[index] = data.NewAdapter(prior, state)
	}

	for range pipe.tape.Next(data.NewValue(tapeAdapters...)) {
	}

	if hadPrev {
		intervalAdapters := make([]*data.Adapter, len(pipe.intervalStates))

		for index, state := range pipe.intervalStates {
			intervalAdapters[index] = data.NewAdapter(prior, state)
		}

		for range pipe.interval.Next(data.NewValue(intervalAdapters...)) {
		}
	}

	if touchValid {
		touchAdapters := make([]*data.Adapter, len(pipe.touchStates))

		for index, state := range pipe.touchStates {
			touchAdapters[index] = data.NewAdapter(prior, state)
		}

		for range pipe.touch.Next(data.NewValue(touchAdapters...)) {
		}
	}

	closed := pipe.output.Values["volume_bar_duration"] > 0 &&
		pipe.output.Values["volume_bar_quantity"] >= pipe.output.Values["volume_bar_target_quantity"]

	if closed {
		barAdapters := make([]*data.Adapter, len(pipe.barStates))

		for index, state := range pipe.barStates {
			barAdapters[index] = data.NewAdapter(prior, state)
		}

		for range pipe.bar.Next(data.NewValue(barAdapters...)) {
		}
	}

	if closed && touchValid && pipe.barFromMid > 0 {
		responseAdapters := make([]*data.Adapter, len(pipe.responseStates))

		for index, state := range pipe.responseStates {
			responseAdapters[index] = data.NewAdapter(prior, state)
		}

		for range pipe.response.Next(data.NewValue(responseAdapters...)) {
		}
	}

	if err := errors.Join(
		publisher.Error(), pipe.tape.Error(), pipe.interval.Error(),
		pipe.touch.Error(), pipe.bar.Error(), pipe.response.Error(),
	); err != nil {
		signal.Error(err)
		return nil
	}

	midpoint := pipe.output.Values["midpoint"]

	if closed {
		pipe.barQuantity = 0
		pipe.barNotional = 0
		pipe.barTradeCount = 0
		pipe.barStart = prior.At
		pipe.barFromMid = midpoint
	} else {
		pipe.barQuantity = pipe.output.Values["volume_bar_quantity"]
		pipe.barNotional = pipe.output.Values["volume_bar_notional"]
		pipe.barTradeCount = pipe.output.Values["volume_bar_trade_count"]

		if pipe.barFromMid <= 0 {
			pipe.barFromMid = midpoint
		}
	}

	pipe.barTarget = pipe.output.Values["volume_bar_target_quantity"]
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

	if closed {
		out.From = barStart
	}

	metrics := make([]data.Metric, 0, len(signal.metrics))

	for _, metric := range signal.metrics {
		value, held := pipe.output.Values[metric[1]]

		if !held || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}

		if metric[4] != "" && !(pipe.output.Values[metric[4]] > 0) {
			continue
		}

		metrics = append(metrics, data.NewMetric(
			metric[0], value, data.Unit(metric[2]), data.Timescale(metric[3]),
		))
	}

	return out.Write(metrics...)
}
