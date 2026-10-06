package cvd

import (
	"context"
	"errors"
	"time"

	"github.com/theapemachine/errnie"

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
Signal is the CVD executed-flow measuring instrument. It holds no logic of its
own: its entire behavior is one nomagique pipeline, a transport.Parallel of
stage groups. Group i receives the data.Adapter bound to states[i], whose
mapping binds the group's native primitive names to CVD domain names. Every
state shares one output map, so a domain fact published by one group is read
by the groups after it. The trade side is the only envelope translation: the
"side" metadata becomes the "buy" and "sell" indicators.
*/
type Signal struct {
	*runtime.System
	arena    *data.ArenaOwner
	output   data.Map[float64]
	envelope data.Map[float64]
	states   []*data.State
	pipeline core.Primitive
	metrics  [][4]string
}

func NewSignal(ctx context.Context, arena *data.ArenaOwner) *Signal {
	output := data.NewOutputMap()

	signal := &Signal{
		arena:    arena,
		output:   output,
		envelope: data.NewOutputMap(),
		states: []*data.State{
			// 0: Per-trade notional.
			data.NewState(data.NewMap("left", "price", "right", "qty", "multiply", "notional"), output),
			// 1-2: Executed quantity per aggressor side.
			data.NewState(data.NewMap("left", "qty", "right", "buy", "value", "executed_quantity:buy:increment", "sum", "executed_quantity:buy"), output),
			data.NewState(data.NewMap("left", "qty", "right", "sell", "value", "executed_quantity:sell:increment", "sum", "executed_quantity:sell"), output),
			// 3-4: Aggressive notional per aggressor side.
			data.NewState(data.NewMap("left", "notional", "right", "buy", "value", "aggressive_notional:buy:increment", "sum", "aggressive_notional:buy"), output),
			data.NewState(data.NewMap("left", "notional", "right", "sell", "value", "aggressive_notional:sell:increment", "sum", "aggressive_notional:sell"), output),
			// 5-6: Trade count per aggressor side.
			data.NewState(data.NewMap("value", "buy", "sum", "trade_count:buy"), output),
			data.NewState(data.NewMap("value", "sell", "sum", "trade_count:sell"), output),
			// 7-9: Trade count and its signed fraction.
			data.NewState(data.NewMap("left", "trade_count:buy", "right", "trade_count:sell", "add", "trade_count"), output),
			data.NewState(data.NewMap("left", "trade_count:buy", "right", "trade_count:sell", "subtract", "net_trade_count"), output),
			data.NewState(data.NewMap("left", "net_trade_count", "right", "trade_count", "divide", "signed_count_fraction"), output),
			// 10-11: Gross and net executed quantity.
			data.NewState(data.NewMap("left", "executed_quantity:buy", "right", "executed_quantity:sell", "add", "gross_executed_quantity"), output),
			data.NewState(data.NewMap("left", "executed_quantity:buy", "right", "executed_quantity:sell", "subtract", "net_executed_quantity"), output),
			// 12-15: Gross and net notional, signed net fraction, mean trade notional.
			data.NewState(data.NewMap("left", "aggressive_notional:buy", "right", "aggressive_notional:sell", "add", "gross_notional"), output),
			data.NewState(data.NewMap("left", "aggressive_notional:buy", "right", "aggressive_notional:sell", "subtract", "net_notional"), output),
			data.NewState(data.NewMap("left", "net_notional", "right", "gross_notional", "divide", "signed_net_fraction"), output),
			data.NewState(data.NewMap("left", "gross_notional", "right", "trade_count", "divide", "mean_trade_notional"), output),
			// 16-17: CVD epoch origin and the elapsed span since it.
			data.NewState(data.NewMap("value", "cvd_epoch_from", "min", "cvd_epoch_from"), output),
			data.NewState(data.NewMap("from", "cvd_epoch_from", "to", "at", "elapsed", "cvd_elapsed"), output),
			// 18-22: Rates over the elapsed span; undefined while the span is zero.
			data.NewState(data.NewMap("left", "trade_count", "right", "cvd_elapsed", "divide", "trade_rate"), output),
			data.NewState(data.NewMap("left", "gross_notional", "right", "cvd_elapsed", "divide", "gross_notional_rate"), output),
			data.NewState(data.NewMap("left", "net_notional", "right", "cvd_elapsed", "divide", "net_notional_rate"), output),
			data.NewState(data.NewMap("left", "aggressive_notional:buy", "right", "cvd_elapsed", "divide", "buy_notional_rate"), output),
			data.NewState(data.NewMap("left", "aggressive_notional:sell", "right", "cvd_elapsed", "divide", "sell_notional_rate"), output),
			// 23-25: Causal baseline of the signed net fraction, divergence, z-score.
			data.NewState(data.NewMap("value", "signed_net_fraction", "center", "signed_net_fraction_baseline", "scale", "signed_net_fraction_scale"), output),
			data.NewState(data.NewMap("left", "signed_net_fraction", "right", "signed_net_fraction_baseline", "subtract", "signed_net_fraction_divergence"), output),
			data.NewState(data.NewMap("left", "signed_net_fraction_divergence", "right", "signed_net_fraction_scale", "divide", "signed_net_fraction_zscore"), output),
		},
		pipeline: transport.NewParallel(
			transport.NewStages(arithmetic.NewMultiply()),
			transport.NewStages(arithmetic.NewMultiply(), statistic.NewSum()),
			transport.NewStages(arithmetic.NewMultiply(), statistic.NewSum()),
			transport.NewStages(arithmetic.NewMultiply(), statistic.NewSum()),
			transport.NewStages(arithmetic.NewMultiply(), statistic.NewSum()),
			transport.NewStages(statistic.NewSum()),
			transport.NewStages(statistic.NewSum()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewAdd()),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(calculus.NewMinimum()),
			transport.NewStages(temporal.NewElapsed()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(arithmetic.NewDivide()),
			transport.NewStages(adaptive.NewBaseline(adaptive.NewWindow())),
			transport.NewStages(arithmetic.NewSubtract()),
			transport.NewStages(arithmetic.NewDivide()),
		),
		// {published label, output key, unit, timescale}
		metrics: [][4]string{
			{"trade_count", "trade_count", string(data.UnitCount), string(data.TimescaleEpoch)},
			{"trade_count:buy", "trade_count:buy", string(data.UnitCount), string(data.TimescaleEpoch)},
			{"trade_count:sell", "trade_count:sell", string(data.UnitCount), string(data.TimescaleEpoch)},
			{"signed_count_fraction", "signed_count_fraction", string(data.UnitRatio), string(data.TimescaleEpoch)},
			{"executed_quantity:buy", "executed_quantity:buy", string(data.UnitQuantity), string(data.TimescaleEpoch)},
			{"executed_quantity:sell", "executed_quantity:sell", string(data.UnitQuantity), string(data.TimescaleEpoch)},
			{"gross_executed_quantity", "gross_executed_quantity", string(data.UnitQuantity), string(data.TimescaleEpoch)},
			{"net_executed_quantity", "net_executed_quantity", string(data.UnitQuantity), string(data.TimescaleEpoch)},
			{"cumulative_volume_delta", "net_executed_quantity", string(data.UnitQuantity), string(data.TimescaleEpoch)},
			{"aggressive_notional:buy", "aggressive_notional:buy", string(data.UnitNotional), string(data.TimescaleEpoch)},
			{"aggressive_notional:sell", "aggressive_notional:sell", string(data.UnitNotional), string(data.TimescaleEpoch)},
			{"gross_notional", "gross_notional", string(data.UnitNotional), string(data.TimescaleEpoch)},
			{"net_notional", "net_notional", string(data.UnitNotional), string(data.TimescaleEpoch)},
			{"cumulative_notional_delta", "net_notional", string(data.UnitNotional), string(data.TimescaleEpoch)},
			{"signed_net_fraction", "signed_net_fraction", string(data.UnitRatio), string(data.TimescaleEpoch)},
			{"mean_trade_notional", "mean_trade_notional", string(data.UnitNotional), string(data.TimescaleEpoch)},
			{"cvd_epoch_from", "cvd_epoch_from", string(data.UnitNanosecond), string(data.TimescaleEpoch)},
			{"trade_rate", "trade_rate", string(data.UnitTradeRate), string(data.TimescalePerSecond)},
			{"gross_notional_rate", "gross_notional_rate", string(data.UnitNotionalRate), string(data.TimescalePerSecond)},
			{"net_notional_rate", "net_notional_rate", string(data.UnitNotionalRate), string(data.TimescalePerSecond)},
			{"buy_notional_rate", "buy_notional_rate", string(data.UnitNotionalRate), string(data.TimescalePerSecond)},
			{"sell_notional_rate", "sell_notional_rate", string(data.UnitNotionalRate), string(data.TimescalePerSecond)},
			{"signed_net_fraction_baseline", "signed_net_fraction_baseline", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"signed_net_fraction_divergence", "signed_net_fraction_divergence", string(data.UnitRatio), string(data.TimescaleRollingWindow)},
			{"signed_net_fraction_zscore", "signed_net_fraction_zscore", string(data.UnitZScore), string(data.TimescaleRollingWindow)},
		},
	}

	signal.System = runtime.NewSystem(ctx, "cvd", signal)
	return signal
}

/*
Arena exposes the signal's ArenaOwner to the runtime Consumer.
*/
func (signal *Signal) Arena() *data.ArenaOwner {
	return signal.arena
}

/*
Step binds the prior trade Measurement to one adapter per stage group, runs
the pipeline, and writes the published CVD facts into a fresh Measurement
allocated from the signal's own arena. Facts a group left unwritten (an
undefined rate or fraction) are omitted, never fabricated as zero.
*/
func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil {
		return nil
	}

	clear(signal.output.Values)
	clear(signal.envelope.Values)

	switch prior.Meta("side") {
	case "buy":
		signal.envelope.Values["buy"] = 1
		signal.envelope.Values["sell"] = 0
	case "sell":
		signal.envelope.Values["buy"] = 0
		signal.envelope.Values["sell"] = 1
	default:
		errnie.Warn(signal.Name() + ": trade without an explicit aggressor side; dropping event")
		return nil
	}

	signal.envelope.Values["at"] = float64(prior.At.UnixNano())
	signal.envelope.Values["cvd_epoch_from"] = float64(prior.At.UnixNano())

	publisher := data.NewAdapter(prior, data.NewState(data.NewMap(), signal.output))

	for range publisher.Next(data.NewValue(signal.envelope)) {
	}

	adapters := make([]*data.Adapter, len(signal.states))

	for index, state := range signal.states {
		adapters[index] = data.NewAdapter(prior, state)
	}

	for range signal.pipeline.Next(data.NewValue(adapters...)) {
	}

	if err := errors.Join(publisher.Error(), signal.pipeline.Error()); err != nil {
		signal.Error(err)
		return nil
	}

	out := signal.arena.NewMeasurement(
		prior.Epoch, prior.Label, signal.Name(), prior.SeqIdx, prior.Tick, nil,
	)

	out.At = prior.At
	out.From = prior.At

	metrics := make([]*data.Metric, 0, len(signal.metrics))

	for _, metric := range signal.metrics {
		value, held := signal.output.Values[metric[1]]

		if !held {
			continue
		}

		metrics = append(metrics, data.NewMetric(
			metric[0], value, data.Unit(metric[2]), data.Timescale(metric[3]),
		))
	}

	if epochFrom, held := signal.output.Values["cvd_epoch_from"]; held {
		out.From = time.Unix(0, int64(epochFrom)).UTC()
	}

	return out.Write(metrics...)
}
