package derivatives

import (
	"context"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmderivatives "github.com/theapemachine/symm/nomagique/derivatives"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Trade is the liquidation-notional accounting instrument. It holds no state
and no logic of its own: its entire behavior is one nomagique pipeline over
the measurement itself — every stage writes its facts into the measurement
where it computes them, and the workload's register owns the measurement's
lifetime.
*/
type Trade struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewTrade(ctx context.Context) *Trade {
	return &Trade{
		System: runtime.NewSystem(ctx, "derivatives:trade"),
		pipeline: nomagique.NewNumber(
			nmderivatives.NewTradeGate(),
			nmderivatives.NewLiquidation(),
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
Register returns the pre-allocated measurement every futures trade flows
through: every metric the instrument can produce is declared, none valued.
*/
func (trade *Trade) Register() *data.Measurement[float64] {
	return data.NewMeasurement("derivatives", map[string]data.Metric[float64]{
		"liquidation_notional:buy": data.NewMetric[float64](
			"liquidation_notional:buy", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"liquidation_notional:sell": data.NewMetric[float64](
			"liquidation_notional:sell", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"gross_liquidation_notional": data.NewMetric[float64](
			"gross_liquidation_notional", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"net_liquidation_notional": data.NewMetric[float64](
			"net_liquidation_notional", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"gross_derivative_trade_notional": data.NewMetric[float64](
			"gross_derivative_trade_notional", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"liquidation_signed_fraction": data.NewMetric[float64](
			"liquidation_signed_fraction", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"liquidation_share": data.NewMetric[float64](
			"liquidation_share", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"liquidation_notional_rate": data.NewMetric[float64](
			"liquidation_notional_rate", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
		"liquidation_share_velocity": data.NewMetric[float64](
			"liquidation_share_velocity", data.UnitPerSecond, data.TimescaleInstantaneous, 0, 1,
		),
	})
}
