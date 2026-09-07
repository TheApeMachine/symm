package toxicity

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// newLevel3Graph retains only this symbol's explicit Primitive estimators.
func newLevel3Graph() core.Primitive {
	return store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewGet("withFracBid"), equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow())), store.NewKey("withdraw_bid")), transport.NewPipe(store.NewGet("withFracAsk"), equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow())), store.NewKey("withdraw_ask")), transport.NewPipe(store.NewGet("retreatFracBid"), equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow())), store.NewKey("retreat_bid")), transport.NewPipe(store.NewGet("retreatFracAsk"), equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow())), store.NewKey("retreat_ask")))
}

// level3Projection declares the source's exact metric labels and evidence.
func level3Projection() *data.Projection {
	p := &data.Projection{Source: "toxicity", Metrics: []data.MetricProjection{{Label: "best_price:bid", Path: []string{"curBid"}, Unit: data.Unit("rate"), Timescale: data.Timescale("instantaneous")},
		{Label: "best_price:ask", Path: []string{"curAsk"}, Unit: data.Unit("rate"), Timescale: data.Timescale("instantaneous")},
		{Label: "previous_best_price:bid", Path: []string{"prevBid"}, Unit: data.Unit("rate"), Timescale: data.Timescale("instantaneous")},
		{Label: "previous_best_price:ask", Path: []string{"prevAsk"}, Unit: data.Unit("rate"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_quantity:bid", Path: []string{"curBidQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_quantity:ask", Path: []string{"curAskQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "previous_touch_quantity:bid", Path: []string{"prevBidQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "previous_touch_quantity:ask", Path: []string{"prevAskQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "unfilled_residual_quantity:bid", Path: []string{"unfilledBid"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "unfilled_residual_quantity:ask", Path: []string{"unfilledAsk"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_price_log_change:bid", Path: []string{"logChangeBid"}, Unit: data.Unit("dimensionless"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_price_log_change:ask", Path: []string{"logChangeAsk"}, Unit: data.Unit("dimensionless"), Timescale: data.Timescale("instantaneous")},
		{Label: "retreated_quantity:bid", Path: []string{"retreatedBid"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "retreated_quantity:ask", Path: []string{"retreatedAsk"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "net_withdrawn_quantity:bid", Path: []string{"withdrawnBid"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "net_withdrawn_quantity:ask", Path: []string{"withdrawnAsk"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "net_replenished_quantity:bid", Path: []string{"replenishedBid"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "net_replenished_quantity:ask", Path: []string{"replenishedAsk"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "retreat_fraction:bid", Path: []string{"retreatFracBid"}, Unit: data.Unit("dimensionless"), Timescale: data.Timescale("instantaneous")},
		{Label: "retreat_fraction:ask", Path: []string{"retreatFracAsk"}, Unit: data.Unit("dimensionless"), Timescale: data.Timescale("instantaneous")},
		{Label: "net_withdrawal_fraction:bid", Path: []string{"withFracBid"}, Unit: data.Unit("dimensionless"), Timescale: data.Timescale("instantaneous")},
		{Label: "net_withdrawal_fraction:ask", Path: []string{"withFracAsk"}, Unit: data.Unit("dimensionless"), Timescale: data.Timescale("instantaneous")},
		{Label: "net_replenishment_fraction:bid", Path: []string{"repFracBid"}, Unit: data.Unit("dimensionless"), Timescale: data.Timescale("instantaneous")},
		{Label: "net_replenishment_fraction:ask", Path: []string{"repFracAsk"}, Unit: data.Unit("dimensionless"), Timescale: data.Timescale("instantaneous")},
		{Label: "retreat_rate:bid", Path: []string{"retreatRateBid"}, Unit: data.Unit("per_second"), Timescale: data.Timescale("per_second"), Defined: []string{"hasRate"}},
		{Label: "net_withdrawal_rate:bid", Path: []string{"withRateBid"}, Unit: data.Unit("per_second"), Timescale: data.Timescale("per_second"), Defined: []string{"hasRate"}},
		{Label: "net_replenishment_rate:bid", Path: []string{"repRateBid"}, Unit: data.Unit("per_second"), Timescale: data.Timescale("per_second"), Defined: []string{"hasRate"}},
		{Label: "retreat_rate:ask", Path: []string{"retreatRateAsk"}, Unit: data.Unit("per_second"), Timescale: data.Timescale("per_second"), Defined: []string{"hasRate"}},
		{Label: "net_withdrawal_rate:ask", Path: []string{"withRateAsk"}, Unit: data.Unit("per_second"), Timescale: data.Timescale("per_second"), Defined: []string{"hasRate"}},
		{Label: "net_replenishment_rate:ask", Path: []string{"repRateAsk"}, Unit: data.Unit("per_second"), Timescale: data.Timescale("per_second"), Defined: []string{"hasRate"}},
		{Label: "withdrawal_fraction_baseline:bid", Path: []string{"withdraw_bid", "mean"}, Defined: []string{"withdraw_bid", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "withdrawal_fraction_divergence:bid", Path: []string{"withdraw_bid", "residual"}, Defined: []string{"withdraw_bid", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "withdrawal_fraction_zscore:bid", Path: []string{"withdraw_bid", "zscore"}, Defined: []string{"withdraw_bid", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "withdrawal_fraction_baseline:ask", Path: []string{"withdraw_ask", "mean"}, Defined: []string{"withdraw_ask", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "withdrawal_fraction_divergence:ask", Path: []string{"withdraw_ask", "residual"}, Defined: []string{"withdraw_ask", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "withdrawal_fraction_zscore:ask", Path: []string{"withdraw_ask", "zscore"}, Defined: []string{"withdraw_ask", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "retreat_fraction_baseline:bid", Path: []string{"retreat_bid", "mean"}, Defined: []string{"retreat_bid", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "retreat_fraction_divergence:bid", Path: []string{"retreat_bid", "residual"}, Defined: []string{"retreat_bid", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "retreat_fraction_zscore:bid", Path: []string{"retreat_bid", "zscore"}, Defined: []string{"retreat_bid", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "retreat_fraction_baseline:ask", Path: []string{"retreat_ask", "mean"}, Defined: []string{"retreat_ask", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "retreat_fraction_divergence:ask", Path: []string{"retreat_ask", "residual"}, Defined: []string{"retreat_ask", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "retreat_fraction_zscore:ask", Path: []string{"retreat_ask", "zscore"}, Defined: []string{"retreat_ask", "has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous}}}
	return p
}
