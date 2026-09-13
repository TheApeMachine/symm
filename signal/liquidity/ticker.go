package liquidity

import (
	"context"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmliquidity "github.com/theapemachine/symm/nomagique/liquidity"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the asynchronous touch-liquidity instrument. It holds no state and
no logic of its own: its entire behavior is one nomagique pipeline over the
measurement itself — every stage writes its facts into the measurement where
it computes them, and the workload's register owns the measurement's lifetime.
*/
type Ticker struct {
	*runtime.System
	pipeline core.Primitive
	ID       int
}

func NewTicker(ctx context.Context) *Ticker {
	return &Ticker{
		System: runtime.NewSystem(ctx, "liquidity:ticker"),
		pipeline: nomagique.NewNumber(
			nmliquidity.NewGate(),
			nmliquidity.NewTouch(),
			data.NewFinalizer[float64](),
		),
	}
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (ticker *Ticker) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	return data.Read[*data.Measurement[float64]](ticker.pipeline.Next(transport.NewOne(unsafe.Pointer(&m)).Next(nil)))
}

/*
Register returns the pre-allocated measurement every liquidity tick flows
through: every metric the instrument can produce is declared, none valued.
The touch quote and displayed quantities are the feed's facts the stages
consume; the rest are written where they are computed.
*/
func (ticker *Ticker) Register() *data.Measurement[float64] {
	return data.NewMeasurement("liquidity", map[string]data.Metric[float64]{
		"bid": data.NewMetric[float64](
			"bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"ask": data.NewMetric[float64](
			"ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"bid_qty": data.NewMetric[float64](
			"bid_qty", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"ask_qty": data.NewMetric[float64](
			"ask_qty", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"best_bid_price": data.NewMetric[float64](
			"best_bid_price", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"best_ask_price": data.NewMetric[float64](
			"best_ask_price", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"midpoint": data.NewMetric[float64](
			"midpoint", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"spread": data.NewMetric[float64](
			"spread", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"touch_quantity:bid": data.NewMetric[float64](
			"touch_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"touch_quantity:ask": data.NewMetric[float64](
			"touch_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 1,
		),
		"touch_notional:bid": data.NewMetric[float64](
			"touch_notional:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"touch_notional:ask": data.NewMetric[float64](
			"touch_notional:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"two_sided_touch_notional": data.NewMetric[float64](
			"two_sided_touch_notional", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"relative_spread": data.NewMetric[float64](
			"relative_spread", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"touch_notional_imbalance": data.NewMetric[float64](
			"touch_notional_imbalance", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"touch_notional_baseline:bid": data.NewMetric[float64](
			"touch_notional_baseline:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"depth_ratio:bid": data.NewMetric[float64](
			"depth_ratio:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"depth_divergence:bid": data.NewMetric[float64](
			"depth_divergence:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"depth_noise_scale:bid": data.NewMetric[float64](
			"depth_noise_scale:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"depth_zscore:bid": data.NewMetric[float64](
			"depth_zscore:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"divergence_velocity:bid": data.NewMetric[float64](
			"divergence_velocity:bid", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"divergence_velocity_snr:bid": data.NewMetric[float64](
			"divergence_velocity_snr:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"touch_notional_baseline:ask": data.NewMetric[float64](
			"touch_notional_baseline:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1,
		),
		"depth_ratio:ask": data.NewMetric[float64](
			"depth_ratio:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"depth_divergence:ask": data.NewMetric[float64](
			"depth_divergence:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"depth_noise_scale:ask": data.NewMetric[float64](
			"depth_noise_scale:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"depth_zscore:ask": data.NewMetric[float64](
			"depth_zscore:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"divergence_velocity:ask": data.NewMetric[float64](
			"divergence_velocity:ask", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"divergence_velocity_snr:ask": data.NewMetric[float64](
			"divergence_velocity_snr:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"relative_spread_baseline": data.NewMetric[float64](
			"relative_spread_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"spread_ratio": data.NewMetric[float64](
			"spread_ratio", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"spread_divergence": data.NewMetric[float64](
			"spread_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"spread_noise_scale": data.NewMetric[float64](
			"spread_noise_scale", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"spread_zscore": data.NewMetric[float64](
			"spread_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
		"spread_divergence_velocity": data.NewMetric[float64](
			"spread_divergence_velocity", data.UnitPerSecond, data.TimescalePerSecond, 0, 1,
		),
		"spread_divergence_velocity_snr": data.NewMetric[float64](
			"spread_divergence_velocity_snr", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		),
	})
}
