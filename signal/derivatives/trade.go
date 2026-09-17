package derivatives

import (
	"context"
	"iter"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/runtime"
	derivatives "github.com/theapemachine/symm/nomagique/statistic/derivatives"
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
	trade := &Trade{
		pipeline: nomagique.NewNumber(derivatives.
			NewTradeGate(), derivatives.
			NewLiquidation(), data.NewFinalizer[float64](),
		),
	}

	trade.System = runtime.NewSystem(ctx, "derivatives:trade", trade)
	return trade
}

/*
Next supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (trade *Trade) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
	inputs:
		for arriving := range in {
			measurement := *(**data.Measurement[float64])(arriving)

			if trade.Status() != runtime.READY {
				errnie.Warn(trade.Name() + ": Next called before READY; dropping event")
				if measurement != nil && !yield(unsafe.Pointer(&measurement)) {
					return
				}
				continue inputs
			}

			if measurement == nil {
				if measurement != nil && !yield(unsafe.Pointer(&measurement)) {
					return
				}
				continue inputs
			}

			if len(measurement.Peers) > 0 {
				peer := measurement.FindPeer(func(candidate *data.Measurement[float64]) bool {
					if candidate.Label == "" {
						return false
					}

					_, hasPrice := candidate.Metrics["price"]
					_, hasQty := candidate.Metrics["qty"]
					return hasPrice && hasQty
				})

				if peer == nil {
					continue inputs
				}

				measurement.Pull(peer, "price", "qty")
			}

			res := sequence.Read[*data.Measurement[float64]](trade.pipeline.Next(sequence.NewOne(unsafe.Pointer(&measurement)).Next(nil)))

			if res == nil {
				if measurement != nil && !yield(unsafe.Pointer(&measurement)) {
					return
				}
				continue inputs
			}

			if res != nil && !yield(unsafe.Pointer(&res)) {
				return
			}
			continue inputs

		}
	}
}

/*
Register returns the pre-allocated measurement every futures trade flows
through: every metric the instrument can produce is declared, none valued.
*/
func (trade *Trade) Register() *data.Measurement[float64] {
	m := data.NewMeasurement("derivatives", map[string]data.Metric[float64]{
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
	m.Metadata["peer-interest"] = "*"
	return m
}
