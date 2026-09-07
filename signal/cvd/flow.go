package cvd

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

// newFlowGraph owns the cumulative execution arithmetic for one symbol. Each
// event advances each accumulator once; later expressions read the result
// record, never re-enter an accumulator through an alias.
func newFlowGraph() core.Primitive {
	field := func(name string, expression core.Primitive) core.Primitive {
		return transport.NewPipe(expression, store.NewKey(name))
	}
	get := func(name string) core.Primitive { return store.NewGet(name) }
	zero := func() core.Primitive { return store.NewConstant(core.From(0.0)) }
	accumulate := func(source core.Primitive, buy bool) core.Primitive {
		return transport.NewPipe(logic.NewGate(get("buy"),
			logic.NewGate(store.NewConstant(core.From(buy)), source, zero()),
			logic.NewGate(store.NewConstant(core.From(buy)), zero(), source)), equation.NewCumulativeSum())
	}
	rates := []core.Primitive{}
	for _, item := range [][2]string{{"trade_rate", "trade_count"}, {"gross_notional_rate", "gross_notional"}, {"net_notional_rate", "net_notional"}, {"buy_notional_rate", "aggressive_notional:buy"}, {"sell_notional_rate", "aggressive_notional:sell"}} {
		rates = append(rates, field(item[0], equation.NewRatio[float64](get(item[1]), get("elapsed"))))
	}
	velocity := temporal.NewVelocity(get("net_notional_rate"), get("at"))
	return transport.NewPipe(
		store.NewRecord(transport.NewPipe(), field("notional", equation.NewProduct[float64](get("price"), get("quantity")))),
		store.NewRecord(transport.NewPipe(),
			field("executed_quantity:buy", accumulate(get("quantity"), true)),
			field("executed_quantity:sell", accumulate(get("quantity"), false)),
			field("aggressive_notional:buy", accumulate(get("notional"), true)),
			field("aggressive_notional:sell", accumulate(get("notional"), false)),
			field("trade_count:buy", accumulate(store.NewConstant(core.From(1.0)), true)),
			field("trade_count:sell", accumulate(store.NewConstant(core.From(1.0)), false))),
		store.NewRecord(transport.NewPipe(),
			field("trade_count", equation.NewSum[float64](get("trade_count:buy"), get("trade_count:sell"))),
			field("gross_notional", equation.NewSum[float64](get("aggressive_notional:buy"), get("aggressive_notional:sell"))),
			field("net_notional", equation.NewDifference[float64](get("aggressive_notional:buy"), get("aggressive_notional:sell"))),
			field("gross_executed_quantity", equation.NewSum[float64](get("executed_quantity:buy"), get("executed_quantity:sell"))),
			field("net_executed_quantity", equation.NewDifference[float64](get("executed_quantity:buy"), get("executed_quantity:sell"))),
			field("elapsed", equation.NewElapsed(get("from"), get("at"))),
			field("cvd_epoch_from", transport.NewPipe(get("from"), calculus.NewConvert[int64, float64](), equation.NewRatio[float64](transport.NewPipe(), store.NewConstant(core.From(1e9)))))),
		store.NewRecord(transport.NewPipe(),
			field("cumulative_volume_delta", get("net_executed_quantity")),
			field("cumulative_notional_delta", get("net_notional")),
			field("signed_count_fraction", equation.NewRatio[float64](equation.NewDifference[float64](get("trade_count:buy"), get("trade_count:sell")), get("trade_count"))),
			field("mean_trade_notional", equation.NewRatio[float64](get("gross_notional"), get("trade_count"))),
			field("signed_net_fraction", equation.NewRatio[float64](get("net_notional"), get("gross_notional"))),
			field("rate_defined", equation.NewGreater[float64](get("elapsed"), zero())),
			field("quote_defined", get("quoted"))),
		store.NewRecord(transport.NewPipe(),
			field("fraction", transport.NewPipe(get("signed_net_fraction"), equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow())))),
			logic.NewGate(get("rate_defined"), store.NewRecord(rates...), store.NewRecord()),
			logic.NewGate(get("quote_defined"), store.NewRecord(field("midpoint_log_return", transport.NewPipe(equation.NewRatio[float64](get("midpoint"), get("prior_mid")), calculus.NewLog(transport.NewIO(core.From(0.0)))))), store.NewRecord())),
		store.NewRecord(transport.NewPipe(),
			field("net_velocity", logic.NewGate(get("rate_defined"), velocity, store.NewConstant(core.Record(map[string]any{"defined": false})))),
			field("response_defined", equation.NewAll(get("quote_defined"), equation.NewGreater[float64](transport.NewPipe(get("net_notional"), calculus.NewAbsolute(transport.NewIO(core.From(0.0)))), zero()))),
			logic.NewGate(get("quote_defined"), store.NewRecord(field("flow_aligned_midpoint_return", equation.NewProduct[float64](get("midpoint_log_return"), transport.NewPipe(get("net_notional"), calculus.NewSign(transport.NewIO(core.From(0.0))))))), store.NewRecord())),
		store.NewRecord(transport.NewPipe(), logic.NewGate(get("response_defined"), store.NewRecord(field("midpoint_response_per_net_notional", equation.NewRatio[float64](get("midpoint_log_return"), get("net_notional")))), store.NewRecord())),
	)
}

// flowProjection declares the serialized metrics and evidence; a missing
// prerequisite omits a reading rather than fabricating a zero or evaluating 0/0.
func flowProjection() *data.Projection {
	p := &data.Projection{Source: "cvd"}
	for _, name := range []string{"trade_count", "trade_count:buy", "trade_count:sell", "executed_quantity:buy", "executed_quantity:sell", "gross_executed_quantity", "net_executed_quantity", "cumulative_volume_delta"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"aggressive_notional:buy", "aggressive_notional:sell", "gross_notional", "net_notional", "mean_trade_notional", "cumulative_notional_delta"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"signed_count_fraction", "signed_net_fraction"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"trade_rate", "gross_notional_rate", "net_notional_rate", "buy_notional_rate", "sell_notional_rate"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Defined: []string{"rate_defined"}, Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond})
	}
	p.Metrics = append(p.Metrics, data.MetricProjection{Label: "cvd_epoch_from", Path: []string{"cvd_epoch_from"}, Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous})
	for _, item := range [][2]string{{"baseline", "baseline"}, {"divergence", "residual"}, {"zscore", "zscore"}} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: "signed_net_fraction_" + item[0], Path: []string{"fraction", item[1]}, Defined: []string{"fraction", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"midpoint_log_return", "flow_aligned_midpoint_return"} {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: name, Path: []string{name}, Defined: []string{"quote_defined"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous})
	}
	p.Metrics = append(p.Metrics,
		data.MetricProjection{Label: "midpoint_response_per_net_notional", Path: []string{"midpoint_response_per_net_notional"}, Defined: []string{"response_defined"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		data.MetricProjection{Label: "net_notional_rate_velocity", Path: []string{"net_velocity", "rate"}, Defined: []string{"net_velocity", "defined"}, Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond})
	p.Facts = []data.FactProjection{
		{Name: data.MetadataSupport, Path: []string{"fraction", "count"}},
		{Name: data.MetadataDivergence, Path: []string{"fraction", "residual"}, Defined: []string{"fraction", "has_prior"}},
		{Name: data.MetadataNoiseVariance, Path: []string{"fraction", "variance"}, Defined: []string{"fraction", "variance_defined"}},
	}
	return p
}
