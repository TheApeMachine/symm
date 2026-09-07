package toxicity

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// newTradeGraph retains only this symbol's explicit Primitive estimators.
func newTradeGraph() core.Primitive {
	return transport.NewPipe(store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewGet("touchFillBidFrac"), equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow())), store.NewKey("bid")), transport.NewPipe(store.NewGet("touchFillAskFrac"), equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow())), store.NewKey("ask"))), store.NewRecord(transport.NewPipe(),
		transport.NewPipe(logic.NewGate(store.NewGet("bidSupported"), store.NewGet("bid"), store.NewGet("ask")), store.NewKey("evidence")),
		transport.NewPipe(equation.NewAny(store.NewGet("bidSupported"), store.NewGet("askSupported")), store.NewKey("quality_defined"))))
}

// tradeProjection declares the source's exact metric labels and evidence.
func tradeProjection() *data.Projection {
	p := &data.Projection{Source: "toxicity", Metrics: []data.MetricProjection{{Label: "bracket_trade_quantity", Path: []string{"bracketQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "matched_touch_trade_quantity:bid", Path: []string{"matchedBidQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "matched_touch_trade_quantity:ask", Path: []string{"matchedAskQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_fill_quantity:bid", Path: []string{"touchFillBidQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_fill_quantity:ask", Path: []string{"touchFillAskQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_fill_fraction:bid", Path: []string{"touchFillBidFrac"}, Unit: data.Unit("dimensionless"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_fill_fraction:ask", Path: []string{"touchFillAskFrac"}, Unit: data.Unit("dimensionless"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_fill_rate:bid", Path: []string{"touchFillBidRate"}, Unit: data.Unit("per_second"), Timescale: data.Timescale("per_second"), Defined: []string{"hasRate"}},
		{Label: "touch_fill_rate:ask", Path: []string{"touchFillAskRate"}, Unit: data.Unit("per_second"), Timescale: data.Timescale("per_second"), Defined: []string{"hasRate"}},
		{Label: "fill_fraction_baseline:bid", Path: []string{"bid", "baseline"}, Defined: []string{"bid", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "fill_fraction_divergence:bid", Path: []string{"bid", "residual"}, Defined: []string{"bid", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "fill_fraction_zscore:bid", Path: []string{"bid", "zscore"}, Defined: []string{"bid", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "fill_fraction_baseline:ask", Path: []string{"ask", "baseline"}, Defined: []string{"ask", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "fill_fraction_divergence:ask", Path: []string{"ask", "residual"}, Defined: []string{"ask", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "fill_fraction_zscore:ask", Path: []string{"ask", "zscore"}, Defined: []string{"ask", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous}}}
	p.Facts = []data.FactProjection{
		{Name: data.MetadataSupport, Path: []string{"evidence", "count"}},
		{Name: data.MetadataDivergence, Path: []string{"evidence", "residual"}, Defined: []string{"quality_defined"}},
		{Name: data.MetadataNoiseVariance, Path: []string{"evidence", "variance"}, Defined: []string{"quality_defined"}},
	}
	return p
}
