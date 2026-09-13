package cvd

import (
	"context"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmcvd "github.com/theapemachine/symm/nomagique/cvd"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Trade is the CVD executed-flow measuring instrument. It holds no state and no
logic of its own: its entire behavior is one nomagique pipeline over the
measurement itself — every stage writes its facts into the measurement where
it computes them, and the workload's register owns the measurement's lifetime.
*/
type Trade struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewTrade(ctx context.Context) *Trade {
	return &Trade{
		System: runtime.NewSystem(ctx, "cvd:trade"),
		pipeline: nomagique.NewNumber(
			nmcvd.NewGate(),
			nmcvd.NewQuantity(),
			nmcvd.NewNotional(),
			nmcvd.NewRates(),
			data.NewFinalizer[float64](),
		),
	}
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (trade *Trade) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	return data.Read[*data.Measurement[float64]](trade.pipeline.Next(transport.NewOne(unsafe.Pointer(&m)).Next(nil)))
}

/*
Register returns the pre-allocated measurement every trade flows through:
every metric the instrument can produce is declared, none valued.
*/
func (trade *Trade) Register() *data.Measurement[float64] {
	return data.NewMeasurement("cvd", map[string]data.Metric[float64]{
		"trade_count": data.NewMetric[float64](
			"trade_count", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"trade_count:buy": data.NewMetric[float64](
			"trade_count:buy", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"trade_count:sell": data.NewMetric[float64](
			"trade_count:sell", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"executed_quantity:buy": data.NewMetric[float64](
			"executed_quantity:buy", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"executed_quantity:sell": data.NewMetric[float64](
			"executed_quantity:sell", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"gross_executed_quantity": data.NewMetric[float64](
			"gross_executed_quantity", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"net_executed_quantity": data.NewMetric[float64](
			"net_executed_quantity", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"cumulative_volume_delta": data.NewMetric[float64](
			"cumulative_volume_delta", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"aggressive_notional:buy": data.NewMetric[float64](
			"aggressive_notional:buy", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"aggressive_notional:sell": data.NewMetric[float64](
			"aggressive_notional:sell", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"gross_notional": data.NewMetric[float64](
			"gross_notional", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"net_notional": data.NewMetric[float64](
			"net_notional", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"mean_trade_notional": data.NewMetric[float64](
			"mean_trade_notional", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"cumulative_notional_delta": data.NewMetric[float64](
			"cumulative_notional_delta", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"trade_rate": data.NewMetric[float64](
			"trade_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"gross_notional_rate": data.NewMetric[float64](
			"gross_notional_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"net_notional_rate": data.NewMetric[float64](
			"net_notional_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"buy_notional_rate": data.NewMetric[float64](
			"buy_notional_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"sell_notional_rate": data.NewMetric[float64](
			"sell_notional_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"net_notional_rate_velocity": data.NewMetric[float64](
			"net_notional_rate_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_count_fraction": data.NewMetric[float64](
			"signed_count_fraction", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_net_fraction": data.NewMetric[float64](
			"signed_net_fraction", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"cvd_epoch_from": data.NewMetric[float64](
			"cvd_epoch_from", data.UnitSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_net_fraction_baseline": data.NewMetric[float64](
			"signed_net_fraction_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_net_fraction_divergence": data.NewMetric[float64](
			"signed_net_fraction_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"signed_net_fraction_zscore": data.NewMetric[float64](
			"signed_net_fraction_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
	})
}
