package depthflow

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// newDepthGraph consumes one message's mutation facts, not a reconstructed
// book. Only its two explicitly configured estimators retain numerical state.
func newDepthGraph() core.Primitive {
	field := func(name string, p core.Primitive) core.Primitive { return transport.NewPipe(p, store.NewKey(name)) }
	get := func(name string) core.Primitive { return store.NewGet(name) }
	zero := func() core.Primitive { return store.NewConstant(core.From(0.0)) }
	emptyEstimate := func() core.Primitive {
		return store.NewConstant(core.Record(map[string]any{"has_prior": false, "count": 0.0, "variance_defined": false}))
	}
	return transport.NewPipe(
		store.NewRecord(transport.NewPipe(),
			field("observed_notional", equation.NewSum[float64](get("observed_notional:bid"), get("observed_notional:ask"))),
			field("observed_notional_diff", equation.NewDifference[float64](get("observed_notional:bid"), get("observed_notional:ask"))),
			field("mutation_count", equation.NewSum[float64](get("mutation_count:bid"), get("mutation_count:ask"))),
			field("mutation_count_diff", equation.NewDifference[float64](get("mutation_count:bid"), get("mutation_count:ask")))),
		store.NewRecord(transport.NewPipe(),
			field("observed_defined", equation.NewGreater[float64](get("observed_notional"), zero())),
			field("mutation_defined", equation.NewGreater[float64](get("mutation_count"), zero())),
			field("rate_defined", equation.NewGreater[float64](get("elapsed"), zero()))),
		store.NewRecord(transport.NewPipe(),
			logic.NewGate(get("observed_defined"), store.NewRecord(field("observed_notional_imbalance", equation.NewRatio[float64](get("observed_notional_diff"), get("observed_notional")))), store.NewRecord()),
			logic.NewGate(get("mutation_defined"), store.NewRecord(field("mutation_activity_imbalance", equation.NewRatio[float64](get("mutation_count_diff"), get("mutation_count")))), store.NewRecord()),
			logic.NewGate(get("rate_defined"), store.NewRecord(field("observed_notional_rate", equation.NewRatio[float64](get("observed_notional"), get("elapsed")))), store.NewRecord())),
		store.NewRecord(transport.NewPipe(),
			field("imbalance", logic.NewGate(get("observed_defined"), transport.NewPipe(get("observed_notional_imbalance"), equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow()))), emptyEstimate())),
			field("rate", logic.NewGate(get("rate_defined"), transport.NewPipe(get("observed_notional_rate"), equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow()))), emptyEstimate()))),
	)
}

func depthProjection() *data.Projection {
	p := &data.Projection{Source: "depthflow"}
	for _, name := range []string{"observed_notional:bid", "observed_notional:ask", "observed_notional", "observed_notional_diff", "add_notional:bid", "add_notional:ask", "modify_remaining_notional:bid", "modify_remaining_notional:ask"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"delete_count:bid", "delete_count:ask", "mutation_count:bid", "mutation_count:ask", "mutation_count", "mutation_count_diff"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous})
	}
	p.Metrics = append(p.Metrics,
		data.MetricProjection{Label: "mutation_activity_imbalance", Path: []string{"mutation_activity_imbalance"}, Defined: []string{"mutation_defined"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		data.MetricProjection{Label: "observed_notional_imbalance", Path: []string{"observed_notional_imbalance"}, Defined: []string{"observed_defined"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		data.MetricProjection{Label: "observed_notional_rate", Path: []string{"observed_notional_rate"}, Defined: []string{"rate_defined"}, Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond})
	for _, prefix := range []string{"imbalance", "rate"} {
		unit, scale := data.UnitDimensionless, data.TimescaleInstantaneous
		if prefix == "rate" {
			unit, scale = data.UnitPerSecond, data.TimescalePerSecond
		}
		for _, item := range [][2]string{{"baseline", "baseline"}, {"divergence", "residual"}, {"zscore", "zscore"}} {
			metricUnit := unit
			if item[0] == "zscore" {
				metricUnit = data.UnitDimensionless
			}
			p.Metrics = append(p.Metrics, data.MetricProjection{Label: "observed_notional_" + prefix + "_" + item[0], Path: []string{prefix, item[1]}, Defined: []string{prefix, "has_prior"}, Unit: metricUnit, Timescale: scale})
		}
	}
	p.Facts = []data.FactProjection{
		{Name: data.MetadataSupport, Path: []string{"imbalance", "count"}},
		{Name: data.MetadataDivergence, Path: []string{"imbalance", "residual"}, Defined: []string{"imbalance", "has_prior"}},
		{Name: data.MetadataNoiseVariance, Path: []string{"imbalance", "variance"}, Defined: []string{"imbalance", "variance_defined"}},
	}
	return p
}
