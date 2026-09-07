package liquidity

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/equation/joint"
	"github.com/theapemachine/symm/nomagique/equation/linear"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// newLiquidityGraph retains one joint model and three event-time regressions
// per symbol. The configured log-moment view keeps noise absent until the prior
// sample actually supports it. Velocity remains OLS, not a message difference.
func newLiquidityGraph() core.Primitive {
	field := func(name string, node core.Primitive) core.Primitive {
		return transport.NewPipe(node, store.NewKey(name))
	}
	get := func(name string) core.Primitive { return store.NewGet(name) }
	moments := []core.Primitive{transport.NewPipe()}
	velocities := []core.Primitive{transport.NewPipe()}

	for index, name := range []string{"bid", "ask", "spread"} {
		channel := name + "_moments"
		moments = append(moments, field(channel, transport.NewPipe(
			get("channels"), collection.NewAt[core.Primitive](store.NewConstant(core.From(float64(index)))),
		)))
		velocities = append(velocities, field(name+"_velocity", logic.NewGate(
			transport.NewPipe(get(channel), get("has_prior")),
			transport.NewPipe(
				store.NewRecord(field("at", get("at")), field("value", transport.NewPipe(get(channel), get("residual")))),
				linear.NewLocalRegression(),
			),
			store.NewConstant(core.Record(map[string]any{"slope_defined": false, "snr_defined": false})),
		)))
	}
	return transport.NewPipe(
		store.NewRecord(transport.NewPipe(),
			field("touch_notional:bid", equation.NewProduct[float64](get("best_bid_price"), get("touch_quantity:bid"))),
			field("touch_notional:ask", equation.NewProduct[float64](get("best_ask_price"), get("touch_quantity:ask"))),
			field("midpoint", equation.NewRatio[float64](equation.NewSum[float64](get("best_bid_price"), get("best_ask_price")), store.NewConstant(core.From(2.0)))),
			field("spread", equation.NewDifference[float64](get("best_ask_price"), get("best_bid_price")))),
		store.NewRecord(transport.NewPipe(),
			field("relative_spread", equation.NewRatio[float64](get("spread"), get("midpoint"))),
			field("two_sided_touch_notional", equation.NewMinimum(get("touch_notional:bid"), get("touch_notional:ask"))),
			field("touch_notional_imbalance", equation.NewRatio[float64](equation.NewDifference[float64](get("touch_notional:bid"), get("touch_notional:ask")), equation.NewSum[float64](get("touch_notional:bid"), get("touch_notional:ask"))))),
		store.NewRecord(transport.NewPipe(), field("values", transport.NewPipe(
			transport.NewFan(transport.NewPipe(), transport.NewIO(get("touch_notional:bid"), get("touch_notional:ask"), get("relative_spread"))),
			transport.NewMap(calculus.NewLog(transport.NewIO(core.From(0.0)))), transport.NewCollect[float64]()))),
		joint.NewEstimator(transport.NewIO(algo.NewWelford(), algo.NewWelford(), algo.NewWelford())),
		store.NewRecord(moments...),
		store.NewRecord(velocities...),
	)
}

func liquidityProjection() *data.Projection {
	p := &data.Projection{Source: "liquidity"}
	for _, name := range []string{"best_bid_price", "best_ask_price", "touch_notional:bid", "touch_notional:ask", "midpoint", "spread", "two_sided_touch_notional"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"touch_quantity:bid", "touch_quantity:ask"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"relative_spread", "touch_notional_imbalance"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous})
	}
	for _, side := range []string{"bid", "ask", "spread"} {
		names := []string{"touch_notional_baseline:" + side, "depth_ratio:" + side, "depth_divergence:" + side, "depth_noise_scale:" + side, "depth_zscore:" + side, "divergence_velocity:" + side, "divergence_velocity_snr:" + side}
		if side == "spread" {
			names = []string{"relative_spread_baseline", "spread_ratio", "spread_divergence", "spread_noise_scale", "spread_zscore", "spread_divergence_velocity", "spread_divergence_velocity_snr"}
		}
		for index, key := range []string{"baseline", "ratio", "residual", "noise", "zscore"} {
			gate := "has_prior"
			if index >= 3 {
				gate = "noise_defined"
			}
			unit := data.UnitDimensionless
			if index == 0 && side != "spread" {
				unit = data.UnitRate
			}
			p.Metrics = append(p.Metrics, data.MetricProjection{Label: names[index], Path: []string{side + "_moments", key}, Defined: []string{side + "_moments", gate}, Unit: unit, Timescale: data.TimescaleInstantaneous})
		}
		p.Metrics = append(p.Metrics,
			data.MetricProjection{Label: names[5], Path: []string{side + "_velocity", "slope"}, Defined: []string{side + "_velocity", "slope_defined"}, Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.MetricProjection{Label: names[6], Path: []string{side + "_velocity", "snr"}, Defined: []string{side + "_velocity", "snr_defined"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous})
	}
	p.Facts = []data.FactProjection{
		{Name: data.MetadataSupport, Path: []string{"bid_moments", "count"}},
		{Name: data.MetadataMahalanobisSNR, Path: []string{"snr"}, Defined: []string{"snr_defined"}},
	}
	return p
}
