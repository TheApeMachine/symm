package cvd

import (
	"context"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Signal is the CVD executed-flow measuring instrument. It owns no market
mathematics. Every transformation is a nomagique Primitive and every
domain/native name translation is declared by Adapter state in Step.
*/
type Signal struct {
	*runtime.System
	arena    *data.ArenaOwner
	pipeline *nomagique.Number
}

func NewSignal(ctx context.Context, arena *data.ArenaOwner) *Signal {
	signal := &Signal{
		arena: arena,
		pipeline: nomagique.NewNumber(
			data.NewPartition(func() core.Primitive {
				return transport.NewParallel(
					nomagique.NewNumber(store.NewInitial(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewSubtract(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewAdd(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewSubtract(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewMultiply(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewMultiply(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewMultiply(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewMultiply(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewMultiply(), transport.NewDiscard()),
					nomagique.NewNumber(statistic.NewSum(), transport.NewDiscard()),
					nomagique.NewNumber(statistic.NewSum(), transport.NewDiscard()),
					nomagique.NewNumber(statistic.NewSum(), transport.NewDiscard()),
					nomagique.NewNumber(statistic.NewSum(), transport.NewDiscard()),
					nomagique.NewNumber(statistic.NewSum(), transport.NewDiscard()),
					nomagique.NewNumber(statistic.NewSum(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewAdd(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewSubtract(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewAdd(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewSubtract(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewSubtract(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewAdd(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewSubtract(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewSubtract(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(calculus.NewLog(), transport.NewDiscard()),
					nomagique.NewNumber(temporal.NewVelocity(), transport.NewDiscard()),
					nomagique.NewNumber(temporal.NewVelocity(), transport.NewDiscard()),
					nomagique.NewNumber(adaptive.NewBaseline(adaptive.NewWindow()), transport.NewDiscard()),
					nomagique.NewNumber(calculus.NewExp(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewSubtract(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(adaptive.NewBaseline(adaptive.NewWindow()), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewSubtract(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewAdd(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(store.NewInitial(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(calculus.NewLog(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(adaptive.NewBaseline(adaptive.NewWindow()), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewSubtract(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(calculus.NewSign(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewMultiply(), transport.NewDiscard()),
					nomagique.NewNumber(arithmetic.NewDivide(), transport.NewDiscard()),
					nomagique.NewNumber(statistic.NewJoint(3), transport.NewDiscard()),
					data.NewProject(
						arena,
						data.NewMetric("trade_count", 0, data.UnitCount, data.TimescaleRollingWindow),
						data.NewMetric("trade_count:buy", 0, data.UnitCount, data.TimescaleRollingWindow),
						data.NewMetric("trade_count:sell", 0, data.UnitCount, data.TimescaleRollingWindow),
						data.NewMetric("signed_count_fraction", 0, data.UnitRatio, data.TimescaleRollingWindow),
						data.NewMetric("executed_quantity:buy", 0, data.UnitQuantity, data.TimescaleRollingWindow),
						data.NewMetric("executed_quantity:sell", 0, data.UnitQuantity, data.TimescaleRollingWindow),
						data.NewMetric("gross_executed_quantity", 0, data.UnitQuantity, data.TimescaleRollingWindow),
						data.NewMetric("net_executed_quantity", 0, data.UnitQuantity, data.TimescaleRollingWindow),
						data.NewMetric("cumulative_volume_delta", 0, data.UnitQuantity, data.TimescaleEpoch),
						data.NewMetric("aggressive_notional:buy", 0, data.UnitNotional, data.TimescaleRollingWindow),
						data.NewMetric("aggressive_notional:sell", 0, data.UnitNotional, data.TimescaleRollingWindow),
						data.NewMetric("gross_notional", 0, data.UnitNotional, data.TimescaleRollingWindow),
						data.NewMetric("net_notional", 0, data.UnitNotional, data.TimescaleRollingWindow),
						data.NewMetric("cumulative_notional_delta", 0, data.UnitNotional, data.TimescaleEpoch),
						data.NewMetric("signed_net_fraction", 0, data.UnitRatio, data.TimescaleRollingWindow),
						data.NewMetric("mean_trade_notional", 0, data.UnitNotional, data.TimescaleRollingWindow),
						data.NewMetric("cvd_epoch_from", 0, data.UnitSecond, data.TimescaleEpoch),
						data.NewMetric("trade_rate", 0, data.UnitTradeRate, data.TimescalePerSecond),
						data.NewMetric("gross_notional_rate", 0, data.UnitNotionalRate, data.TimescalePerSecond),
						data.NewMetric("net_notional_rate", 0, data.UnitNotionalRate, data.TimescalePerSecond),
						data.NewMetric("buy_notional_rate", 0, data.UnitNotionalRate, data.TimescalePerSecond),
						data.NewMetric("sell_notional_rate", 0, data.UnitNotionalRate, data.TimescalePerSecond),
						data.NewMetric("response_midpoint:from", 0, data.UnitPrice, data.TimescaleRollingWindow),
						data.NewMetric("response_midpoint:at", 0, data.UnitPrice, data.TimescaleInstantaneous),
						data.NewMetric("midpoint_log_return", 0, data.UnitLogReturn, data.TimescaleRollingWindow),
						data.NewMetric("midpoint_return_rate", 0, data.UnitPerSecond, data.TimescalePerSecond),
						data.NewMetric("flow_aligned_midpoint_return", 0, data.UnitLogReturn, data.TimescaleRollingWindow),
						data.NewMetric("midpoint_response_per_net_notional", 0, data.UnitDimensionless, data.TimescaleRollingWindow),
						data.NewMetric("gross_notional_rate_baseline", 0, data.UnitNotionalRate, data.TimescaleRollingWindow),
						data.NewMetric("gross_notional_rate_ratio", 0, data.UnitRatio, data.TimescaleRollingWindow),
						data.NewMetric("gross_notional_rate_divergence", 0, data.UnitLogReturn, data.TimescaleRollingWindow),
						data.NewMetric("gross_notional_rate_zscore", 0, data.UnitZScore, data.TimescaleRollingWindow),
						data.NewMetric("signed_net_fraction_baseline", 0, data.UnitRatio, data.TimescaleRollingWindow),
						data.NewMetric("signed_net_fraction_divergence", 0, data.UnitRatio, data.TimescaleRollingWindow),
						data.NewMetric("signed_net_fraction_zscore", 0, data.UnitZScore, data.TimescaleRollingWindow),
						data.NewMetric("midpoint_return_rate_baseline", 0, data.UnitPerSecond, data.TimescaleRollingWindow),
						data.NewMetric("midpoint_return_rate_divergence", 0, data.UnitPerSecond, data.TimescaleRollingWindow),
						data.NewMetric("midpoint_return_rate_zscore", 0, data.UnitZScore, data.TimescaleRollingWindow),
						data.NewMetric("gross_notional_rate_velocity", 0, data.UnitVelocity, data.TimescalePerSecond),
						data.NewMetric("net_notional_rate_velocity", 0, data.UnitVelocity, data.TimescalePerSecond),
						data.NewMetric("SNR", 0, data.UnitSNR, data.TimescaleRollingWindow),
					),
				)
			}),
		),
	}

	signal.System = runtime.NewSystem(ctx, "cvd", signal)
	return signal
}

/*
Step binds one immutable ingress measurement to the CVD algebra.

Ingress supplies the numeric boundary facts:
price, qty, aggressor_sign (+1 buy / -1 sell), event_time (seconds), and,
when available, best_bid and best_ask.
*/
func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	values := data.NewOutputMap()
	values.Values["one"] = 1
	values.Values["two"] = 2

	result := data.Read[*data.Measurement](
		signal.pipeline.Next(data.NewValue(
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"value", "event_time",
				"initial", "cvd_epoch_from",
				"subsequent", "_event_time_subsequent",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"minuend", "event_time",
				"subtrahend", "cvd_epoch_from",
				"difference", "_elapsed_seconds",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"augend", "aggressor_sign",
				"addend", "one",
				"sum", "_buy_indicator_numerator",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "_buy_indicator_numerator",
				"divisor", "two",
				"quotient", "_buy_indicator",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"minuend", "one",
				"subtrahend", "aggressor_sign",
				"difference", "_sell_indicator_numerator",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "_sell_indicator_numerator",
				"divisor", "two",
				"quotient", "_sell_indicator",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"multiplicand", "price",
				"multiplier", "qty",
				"product", "_trade_notional",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"multiplicand", "qty",
				"multiplier", "_buy_indicator",
				"product", "_buy_quantity_increment",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"multiplicand", "qty",
				"multiplier", "_sell_indicator",
				"product", "_sell_quantity_increment",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"multiplicand", "_trade_notional",
				"multiplier", "_buy_indicator",
				"product", "_buy_notional_increment",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"multiplicand", "_trade_notional",
				"multiplier", "_sell_indicator",
				"product", "_sell_notional_increment",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"value", "_buy_indicator",
				"sum", "trade_count:buy",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"value", "_sell_indicator",
				"sum", "trade_count:sell",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"value", "_buy_quantity_increment",
				"sum", "executed_quantity:buy",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"value", "_sell_quantity_increment",
				"sum", "executed_quantity:sell",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"value", "_buy_notional_increment",
				"sum", "aggressive_notional:buy",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"value", "_sell_notional_increment",
				"sum", "aggressive_notional:sell",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"augend", "trade_count:buy",
				"addend", "trade_count:sell",
				"sum", "trade_count",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"minuend", "trade_count:buy",
				"subtrahend", "trade_count:sell",
				"difference", "_signed_count_delta",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "_signed_count_delta",
				"divisor", "trade_count",
				"quotient", "signed_count_fraction",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"augend", "executed_quantity:buy",
				"addend", "executed_quantity:sell",
				"sum", "gross_executed_quantity",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"minuend", "executed_quantity:buy",
				"subtrahend", "executed_quantity:sell",
				"difference", "net_executed_quantity",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"minuend", "executed_quantity:buy",
				"subtrahend", "executed_quantity:sell",
				"difference", "cumulative_volume_delta",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"augend", "aggressive_notional:buy",
				"addend", "aggressive_notional:sell",
				"sum", "gross_notional",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"minuend", "aggressive_notional:buy",
				"subtrahend", "aggressive_notional:sell",
				"difference", "net_notional",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"minuend", "aggressive_notional:buy",
				"subtrahend", "aggressive_notional:sell",
				"difference", "cumulative_notional_delta",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "net_notional",
				"divisor", "gross_notional",
				"quotient", "signed_net_fraction",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "gross_notional",
				"divisor", "trade_count",
				"quotient", "mean_trade_notional",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "trade_count",
				"divisor", "_elapsed_seconds",
				"quotient", "trade_rate",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "gross_notional",
				"divisor", "_elapsed_seconds",
				"quotient", "gross_notional_rate",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "net_notional",
				"divisor", "_elapsed_seconds",
				"quotient", "net_notional_rate",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "aggressive_notional:buy",
				"divisor", "_elapsed_seconds",
				"quotient", "buy_notional_rate",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "aggressive_notional:sell",
				"divisor", "_elapsed_seconds",
				"quotient", "sell_notional_rate",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"argument", "gross_notional_rate",
				"logarithm", "_gross_notional_rate_log",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"position", "_gross_notional_rate_log",
				"time", "event_time",
				"velocity", "gross_notional_rate_velocity",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"position", "net_notional_rate",
				"time", "event_time",
				"velocity", "net_notional_rate_velocity",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"value", "_gross_notional_rate_log",
				"center", "_gross_notional_rate_log_center",
				"scale", "_gross_notional_rate_log_scale",
				"capacity", "_gross_window_capacity",
				"observations", "_gross_window_observations",
				"shed_ratio", "_gross_window_shed_ratio",
				"variance", "_gross_window_variance",
				"recent_count", "_gross_window_recent_count",
				"prior_count", "_gross_window_prior_count",
				"bound", "_gross_window_bound",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"exponent", "_gross_notional_rate_log_center",
				"exponential", "gross_notional_rate_baseline",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "gross_notional_rate",
				"divisor", "gross_notional_rate_baseline",
				"quotient", "gross_notional_rate_ratio",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"minuend", "_gross_notional_rate_log",
				"subtrahend", "_gross_notional_rate_log_center",
				"difference", "gross_notional_rate_divergence",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "gross_notional_rate_divergence",
				"divisor", "_gross_notional_rate_log_scale",
				"quotient", "gross_notional_rate_zscore",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"value", "signed_net_fraction",
				"center", "signed_net_fraction_baseline",
				"scale", "_signed_net_fraction_scale",
				"capacity", "_fraction_window_capacity",
				"observations", "_fraction_window_observations",
				"shed_ratio", "_fraction_window_shed_ratio",
				"variance", "_fraction_window_variance",
				"recent_count", "_fraction_window_recent_count",
				"prior_count", "_fraction_window_prior_count",
				"bound", "_fraction_window_bound",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"minuend", "signed_net_fraction",
				"subtrahend", "signed_net_fraction_baseline",
				"difference", "signed_net_fraction_divergence",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "signed_net_fraction_divergence",
				"divisor", "_signed_net_fraction_scale",
				"quotient", "signed_net_fraction_zscore",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"augend", "best_bid",
				"addend", "best_ask",
				"sum", "_midpoint_sum",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "_midpoint_sum",
				"divisor", "two",
				"quotient", "response_midpoint:at",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"value", "response_midpoint:at",
				"initial", "response_midpoint:from",
				"subsequent", "_response_midpoint_subsequent",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "_response_midpoint_subsequent",
				"divisor", "response_midpoint:from",
				"quotient", "_midpoint_ratio",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"argument", "_midpoint_ratio",
				"logarithm", "midpoint_log_return",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "midpoint_log_return",
				"divisor", "_elapsed_seconds",
				"quotient", "midpoint_return_rate",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"value", "midpoint_return_rate",
				"center", "midpoint_return_rate_baseline",
				"scale", "_midpoint_return_rate_scale",
				"capacity", "_midpoint_window_capacity",
				"observations", "_midpoint_window_observations",
				"shed_ratio", "_midpoint_window_shed_ratio",
				"variance", "_midpoint_window_variance",
				"recent_count", "_midpoint_window_recent_count",
				"prior_count", "_midpoint_window_prior_count",
				"bound", "_midpoint_window_bound",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"minuend", "midpoint_return_rate",
				"subtrahend", "midpoint_return_rate_baseline",
				"difference", "midpoint_return_rate_divergence",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "midpoint_return_rate_divergence",
				"divisor", "_midpoint_return_rate_scale",
				"quotient", "midpoint_return_rate_zscore",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"argument", "net_notional",
				"sign", "_net_notional_sign",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"multiplicand", "_net_notional_sign",
				"multiplier", "midpoint_log_return",
				"product", "flow_aligned_midpoint_return",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"dividend", "midpoint_log_return",
				"divisor", "net_notional",
				"quotient", "midpoint_response_per_net_notional",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(
				"coordinate:0", "gross_notional_rate_divergence",
				"coordinate:1", "signed_net_fraction_divergence",
				"coordinate:2", "midpoint_return_rate_divergence",
				"snr", "SNR",
				"maturity", "_joint_maturity",
				"support", "_joint_support",
			), values)),
			data.NewAdapter(prior, data.NewState(data.NewMap(), values)),
		)),
	)

	return result
}
