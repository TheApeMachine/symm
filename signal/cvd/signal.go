package cvd

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
Signal is the CVD executed-flow measuring instrument. Mathematical state lives
inside one primitive pipeline per symbol; Signal owns only runtime wiring.
*/
type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
}

func NewSignal(ctx context.Context, arena *data.ArenaOwner) *Signal {
	signal := &Signal{arena: arena}
	signal.System = runtime.NewSystem(ctx, "cvd", signal)
	return signal
}

func (signal *Signal) pipelineFor(symbol string) core.Primitive {
	if existing, ok := signal.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		data.NewBind(data.NewMap("sequence", "SeqIdx", "at", "At")),
		temporal.NewCausalOrder(),

		data.NewBind(data.NewMap("value", "price")),
		logic.NewPositive(),
		data.NewBind(data.NewMap("value", "qty")),
		logic.NewPositive(),
		data.NewBind(data.NewMap("value", "side")),
		logic.NewUnitSign(),

		data.NewBind(data.NewMap("count", "trade_count")),
		statistic.NewCount(),

		data.NewBind(data.NewMap(
			"left", "price",
			"right", "qty",
			"product", "trade_notional",
		)),
		arithmetic.NewMultiply(),

		data.NewBind(data.NewMap(
			"left", "side",
			"right", "qty",
			"product", "signed_quantity",
		)),
		arithmetic.NewMultiply(),

		data.NewBind(data.NewMap(
			"left", "side",
			"right", "trade_notional",
			"product", "signed_notional",
		)),
		arithmetic.NewMultiply(),

		data.NewBind(data.NewMap(
			"value", "side",
			"positive", "buy_count_tick",
			"negative", "sell_count_tick",
		)),
		calculus.NewPolarize(),

		data.NewBind(data.NewMap("value", "buy_count_tick", "sum", "trade_count:buy")),
		statistic.NewSum(),
		data.NewBind(data.NewMap("value", "sell_count_tick", "sum", "trade_count:sell")),
		statistic.NewSum(),
		data.NewBind(data.NewMap(
			"left", "trade_count:buy",
			"right", "trade_count:sell",
			"difference", "signed_count_delta",
		)),
		arithmetic.NewSubtract(),
		data.NewBind(data.NewMap(
			"left", "signed_count_delta",
			"right", "trade_count",
			"quotient", "signed_count_fraction",
		)),
		arithmetic.NewDivide(),

		data.NewBind(data.NewMap(
			"value", "signed_quantity",
			"positive", "buy_quantity_tick",
			"negative", "sell_quantity_tick",
		)),
		calculus.NewPolarize(),
		data.NewBind(data.NewMap("value", "buy_quantity_tick", "sum", "executed_quantity:buy")),
		statistic.NewSum(),
		data.NewBind(data.NewMap("value", "sell_quantity_tick", "sum", "executed_quantity:sell")),
		statistic.NewSum(),
		data.NewBind(data.NewMap(
			"left", "executed_quantity:buy",
			"right", "executed_quantity:sell",
			"sum", "gross_executed_quantity",
		)),
		arithmetic.NewAdd(),
		data.NewBind(data.NewMap(
			"left", "executed_quantity:buy",
			"right", "executed_quantity:sell",
			"difference", "net_executed_quantity",
		)),
		arithmetic.NewSubtract(),

		data.NewBind(data.NewMap(
			"value", "signed_notional",
			"positive", "buy_notional_tick",
			"negative", "sell_notional_tick",
		)),
		calculus.NewPolarize(),
		data.NewBind(data.NewMap("value", "buy_notional_tick", "sum", "aggressive_notional:buy")),
		statistic.NewSum(),
		data.NewBind(data.NewMap("value", "sell_notional_tick", "sum", "aggressive_notional:sell")),
		statistic.NewSum(),
		data.NewBind(data.NewMap(
			"left", "aggressive_notional:buy",
			"right", "aggressive_notional:sell",
			"sum", "gross_notional",
		)),
		arithmetic.NewAdd(),
		data.NewBind(data.NewMap(
			"left", "aggressive_notional:buy",
			"right", "aggressive_notional:sell",
			"difference", "net_notional",
		)),
		arithmetic.NewSubtract(),
		data.NewBind(data.NewMap(
			"left", "net_notional",
			"right", "gross_notional",
			"quotient", "signed_net_fraction",
		)),
		arithmetic.NewDivide(),
		data.NewBind(data.NewMap(
			"left", "gross_notional",
			"right", "trade_count",
			"quotient", "mean_trade_notional",
		)),
		arithmetic.NewDivide(),

		data.NewBind(data.NewMap("value", "At", "origin", "cvd_epoch_from")),
		store.NewOrigin(),
		data.NewBind(data.NewMap(
			"left", "At",
			"right", "cvd_epoch_from",
			"difference", "elapsed",
		)),
		arithmetic.NewSubtract(),

		data.NewBind(data.NewMap("left", "trade_count", "right", "elapsed", "quotient", "trade_rate")),
		arithmetic.NewDivide(),
		data.NewBind(data.NewMap("left", "gross_notional", "right", "elapsed", "quotient", "gross_notional_rate")),
		arithmetic.NewDivide(),
		data.NewBind(data.NewMap("left", "net_notional", "right", "elapsed", "quotient", "net_notional_rate")),
		arithmetic.NewDivide(),
		data.NewBind(data.NewMap("left", "aggressive_notional:buy", "right", "elapsed", "quotient", "buy_notional_rate")),
		arithmetic.NewDivide(),
		data.NewBind(data.NewMap("left", "aggressive_notional:sell", "right", "elapsed", "quotient", "sell_notional_rate")),
		arithmetic.NewDivide(),

		data.NewBind(data.NewMap(
			"lower", "best_bid",
			"upper", "best_ask",
			"midpoint", "response_midpoint:at",
		)),
		arithmetic.NewMidpoint(),
		data.NewBind(data.NewMap("value", "response_midpoint:at", "origin", "response_midpoint:from")),
		store.NewOrigin(),
		data.NewBind(data.NewMap(
			"left", "response_midpoint:at",
			"right", "response_midpoint:from",
			"quotient", "midpoint_ratio",
		)),
		arithmetic.NewDivide(),
		data.NewBind(data.NewMap("value", "midpoint_ratio", "log", "midpoint_log_return")),
		calculus.NewLog(),
		data.NewBind(data.NewMap(
			"left", "midpoint_log_return",
			"right", "elapsed",
			"quotient", "midpoint_return_rate",
		)),
		arithmetic.NewDivide(),
		data.NewBind(data.NewMap("value", "net_notional", "sign", "net_notional_sign")),
		calculus.NewSign(),
		data.NewBind(data.NewMap(
			"left", "net_notional_sign",
			"right", "midpoint_log_return",
			"product", "flow_aligned_midpoint_return",
		)),
		arithmetic.NewMultiply(),
		data.NewBind(data.NewMap(
			"left", "midpoint_log_return",
			"right", "net_notional",
			"quotient", "midpoint_response_per_net_notional",
		)),
		arithmetic.NewDivide(),

		data.NewBind(data.NewMap("value", "gross_notional_rate", "log", "gross_notional_rate_log")),
		calculus.NewLog(),
		data.NewBind(data.NewMap(
			"value", "gross_notional_rate_log",
			"center", "gross_notional_rate_log_baseline",
			"scale", "gross_notional_rate_log_scale",
		)),
		adaptive.NewBaseline(adaptive.NewWindow()),
		data.NewBind(data.NewMap("value", "gross_notional_rate_log_baseline", "exp", "gross_notional_rate_baseline")),
		calculus.NewExp(),
		data.NewBind(data.NewMap(
			"left", "gross_notional_rate",
			"right", "gross_notional_rate_baseline",
			"quotient", "gross_notional_rate_ratio",
		)),
		arithmetic.NewDivide(),
		data.NewBind(data.NewMap("value", "gross_notional_rate_ratio", "log", "gross_notional_rate_divergence")),
		calculus.NewLog(),
		data.NewBind(data.NewMap(
			"left", "gross_notional_rate_divergence",
			"right", "gross_notional_rate_log_scale",
			"quotient", "gross_notional_rate_zscore",
		)),
		arithmetic.NewDivide(),

		data.NewBind(data.NewMap(
			"value", "signed_net_fraction",
			"center", "signed_net_fraction_baseline",
			"scale", "signed_net_fraction_scale",
		)),
		adaptive.NewBaseline(adaptive.NewWindow()),
		data.NewBind(data.NewMap(
			"left", "signed_net_fraction",
			"right", "signed_net_fraction_baseline",
			"difference", "signed_net_fraction_divergence",
		)),
		arithmetic.NewSubtract(),
		data.NewBind(data.NewMap(
			"left", "signed_net_fraction_divergence",
			"right", "signed_net_fraction_scale",
			"quotient", "signed_net_fraction_zscore",
		)),
		arithmetic.NewDivide(),

		data.NewBind(data.NewMap(
			"value", "midpoint_return_rate",
			"center", "midpoint_return_rate_baseline",
			"scale", "midpoint_return_rate_scale",
		)),
		adaptive.NewBaseline(adaptive.NewWindow()),
		data.NewBind(data.NewMap(
			"left", "midpoint_return_rate",
			"right", "midpoint_return_rate_baseline",
			"difference", "midpoint_return_rate_divergence",
		)),
		arithmetic.NewSubtract(),
		data.NewBind(data.NewMap(
			"left", "midpoint_return_rate_divergence",
			"right", "midpoint_return_rate_scale",
			"quotient", "midpoint_return_rate_zscore",
		)),
		arithmetic.NewDivide(),

		data.NewBind(data.NewMap(
			"first", "gross_notional_rate_divergence",
			"second", "signed_net_fraction_divergence",
			"third", "midpoint_return_rate_divergence",
			"snr", "SNR",
			"maturity", "Maturity",
		)),
		statistic.NewJointSNR3(),

		data.NewBind(data.NewMap(
			"value", "net_notional_rate",
			"at", "At",
			"slope", "net_notional_rate_velocity",
			"slope_snr", "net_notional_rate_velocity_snr",
		)),
		statistic.NewLocalRegression(),
		data.NewBind(data.NewMap(
			"value", "gross_notional_rate_log",
			"at", "At",
			"slope", "gross_notional_rate_velocity",
			"slope_snr", "gross_notional_rate_velocity_snr",
		)),
		statistic.NewLocalRegression(),

		data.NewProject(
			signal.arena,
			"cvd",
			data.NewMap(
				"trade_count", "trade_count",
				"trade_count:buy", "trade_count:buy",
				"trade_count:sell", "trade_count:sell",
				"signed_count_fraction", "signed_count_fraction",
				"executed_quantity:buy", "executed_quantity:buy",
				"executed_quantity:sell", "executed_quantity:sell",
				"gross_executed_quantity", "gross_executed_quantity",
				"net_executed_quantity", "net_executed_quantity",
				"aggressive_notional:buy", "aggressive_notional:buy",
				"aggressive_notional:sell", "aggressive_notional:sell",
				"gross_notional", "gross_notional",
				"net_notional", "net_notional",
				"signed_net_fraction", "signed_net_fraction",
				"mean_trade_notional", "mean_trade_notional",
				"trade_rate", "trade_rate",
				"gross_notional_rate", "gross_notional_rate",
				"net_notional_rate", "net_notional_rate",
				"buy_notional_rate", "buy_notional_rate",
				"sell_notional_rate", "sell_notional_rate",
				"cumulative_volume_delta", "net_executed_quantity",
				"cumulative_notional_delta", "net_notional",
				"cvd_epoch_from", "cvd_epoch_from",
				"response_midpoint:from", "response_midpoint:from",
				"response_midpoint:at", "response_midpoint:at",
				"midpoint_log_return", "midpoint_log_return",
				"midpoint_return_rate", "midpoint_return_rate",
				"flow_aligned_midpoint_return", "flow_aligned_midpoint_return",
				"midpoint_response_per_net_notional", "midpoint_response_per_net_notional",
				"gross_notional_rate_baseline", "gross_notional_rate_baseline",
				"gross_notional_rate_ratio", "gross_notional_rate_ratio",
				"gross_notional_rate_divergence", "gross_notional_rate_divergence",
				"gross_notional_rate_zscore", "gross_notional_rate_zscore",
				"signed_net_fraction_baseline", "signed_net_fraction_baseline",
				"signed_net_fraction_divergence", "signed_net_fraction_divergence",
				"signed_net_fraction_zscore", "signed_net_fraction_zscore",
				"midpoint_return_rate_baseline", "midpoint_return_rate_baseline",
				"midpoint_return_rate_divergence", "midpoint_return_rate_divergence",
				"midpoint_return_rate_zscore", "midpoint_return_rate_zscore",
				"net_notional_rate_velocity", "net_notional_rate_velocity",
				"gross_notional_rate_velocity", "gross_notional_rate_velocity",
				"SNR", "SNR",
				"Maturity", "Maturity",
			),
			map[string]data.Unit{
				"trade_count":                         data.UnitCount,
				"trade_count:buy":                     data.UnitCount,
				"trade_count:sell":                    data.UnitCount,
				"signed_count_fraction":               data.UnitRatio,
				"executed_quantity:buy":               data.UnitQuantity,
				"executed_quantity:sell":              data.UnitQuantity,
				"gross_executed_quantity":             data.UnitQuantity,
				"net_executed_quantity":               data.UnitQuantity,
				"aggressive_notional:buy":             data.UnitNotional,
				"aggressive_notional:sell":            data.UnitNotional,
				"gross_notional":                      data.UnitNotional,
				"net_notional":                        data.UnitNotional,
				"signed_net_fraction":                 data.UnitRatio,
				"mean_trade_notional":                 data.UnitNotional,
				"trade_rate":                          data.UnitTradeRate,
				"gross_notional_rate":                 data.UnitNotionalRate,
				"net_notional_rate":                   data.UnitNotionalRate,
				"buy_notional_rate":                   data.UnitNotionalRate,
				"sell_notional_rate":                  data.UnitNotionalRate,
				"cumulative_volume_delta":             data.UnitQuantity,
				"cumulative_notional_delta":           data.UnitNotional,
				"cvd_epoch_from":                      data.UnitSecond,
				"response_midpoint:from":              data.UnitPrice,
				"response_midpoint:at":                data.UnitPrice,
				"midpoint_log_return":                 data.UnitLogReturn,
				"midpoint_return_rate":                data.UnitPerSecond,
				"flow_aligned_midpoint_return":        data.UnitLogReturn,
				"midpoint_response_per_net_notional":  data.UnitInverseQuoteCurrency,
				"gross_notional_rate_baseline":        data.UnitNotionalRate,
				"gross_notional_rate_ratio":           data.UnitRatio,
				"gross_notional_rate_divergence":      data.UnitDimensionless,
				"gross_notional_rate_zscore":          data.UnitZScore,
				"signed_net_fraction_baseline":        data.UnitRatio,
				"signed_net_fraction_divergence":      data.UnitRatio,
				"signed_net_fraction_zscore":          data.UnitZScore,
				"midpoint_return_rate_baseline":       data.UnitPerSecond,
				"midpoint_return_rate_divergence":     data.UnitPerSecond,
				"midpoint_return_rate_zscore":         data.UnitZScore,
				"net_notional_rate_velocity":          data.UnitAcceleration,
				"gross_notional_rate_velocity":        data.UnitAcceleration,
				"SNR":                                 data.UnitSNR,
				"Maturity":                            data.UnitRatio,
			},
			map[string]data.Timescale{
				"trade_count":                         data.TimescaleEpoch,
				"trade_count:buy":                     data.TimescaleEpoch,
				"trade_count:sell":                    data.TimescaleEpoch,
				"signed_count_fraction":               data.TimescaleEpoch,
				"executed_quantity:buy":               data.TimescaleEpoch,
				"executed_quantity:sell":              data.TimescaleEpoch,
				"gross_executed_quantity":             data.TimescaleEpoch,
				"net_executed_quantity":               data.TimescaleEpoch,
				"aggressive_notional:buy":             data.TimescaleEpoch,
				"aggressive_notional:sell":            data.TimescaleEpoch,
				"gross_notional":                      data.TimescaleEpoch,
				"net_notional":                        data.TimescaleEpoch,
				"signed_net_fraction":                 data.TimescaleEpoch,
				"mean_trade_notional":                 data.TimescaleEpoch,
				"trade_rate":                          data.TimescalePerSecond,
				"gross_notional_rate":                 data.TimescalePerSecond,
				"net_notional_rate":                   data.TimescalePerSecond,
				"buy_notional_rate":                   data.TimescalePerSecond,
				"sell_notional_rate":                  data.TimescalePerSecond,
				"cumulative_volume_delta":             data.TimescaleEpoch,
				"cumulative_notional_delta":           data.TimescaleEpoch,
				"cvd_epoch_from":                      data.TimescaleEpoch,
				"response_midpoint:from":              data.TimescaleEpoch,
				"response_midpoint:at":                data.TimescaleInstantaneous,
				"midpoint_log_return":                 data.TimescaleEpoch,
				"midpoint_return_rate":                data.TimescalePerSecond,
				"flow_aligned_midpoint_return":        data.TimescaleEpoch,
				"midpoint_response_per_net_notional":  data.TimescaleEpoch,
				"gross_notional_rate_baseline":        data.TimescaleRollingWindow,
				"gross_notional_rate_ratio":           data.TimescaleRollingWindow,
				"gross_notional_rate_divergence":      data.TimescaleRollingWindow,
				"gross_notional_rate_zscore":          data.TimescaleRollingWindow,
				"signed_net_fraction_baseline":        data.TimescaleRollingWindow,
				"signed_net_fraction_divergence":      data.TimescaleRollingWindow,
				"signed_net_fraction_zscore":          data.TimescaleRollingWindow,
				"midpoint_return_rate_baseline":       data.TimescaleRollingWindow,
				"midpoint_return_rate_divergence":     data.TimescaleRollingWindow,
				"midpoint_return_rate_zscore":         data.TimescaleRollingWindow,
				"net_notional_rate_velocity":          data.TimescalePerSecond,
				"gross_notional_rate_velocity":        data.TimescalePerSecond,
				"SNR":                                 data.TimescaleRollingWindow,
				"Maturity":                            data.TimescaleRollingWindow,
			},
		),
	)

	actual, _ := signal.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

/*
Step binds the arriving WORM measurement to the symbol's primitive graph and
returns the projected CVD measurement.
*/
func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	adapter := data.NewAdapter(prior, data.NewState(data.NewMap()))

	return data.Read[*data.Measurement](
		signal.pipelineFor(prior.Label).Next(data.NewValue(adapter)),
	)
}
