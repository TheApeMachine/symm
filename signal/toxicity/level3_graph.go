package toxicity

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

type Level3Input struct {
	CurBid, CurAsk, PrevBid, PrevAsk             float64
	CurBidQty, CurAskQty, PrevBidQty, PrevAskQty float64
	UnfilledBid, UnfilledAsk                     float64
	LogChangeBid, LogChangeAsk                   float64
	RetreatedBid, RetreatedAsk                   float64
	WithdrawnBid, WithdrawnAsk                   float64
	ReplenishedBid, ReplenishedAsk               float64
	RetreatFracBid, RetreatFracAsk               float64
	WithFracBid, WithFracAsk                     float64
	RepFracBid, RepFracAsk                       float64
	RetreatRateBid, RetreatRateAsk               float64
	WithRateBid, WithRateAsk                     float64
	RepRateBid, RepRateAsk                       float64
	HasRate                                      bool
}

type Level3Graph struct {
	err         error
	withdrawBid core.Primitive
	withdrawAsk core.Primitive
	retreatBid  core.Primitive
	retreatAsk  core.Primitive
}

func newLevel3Graph() *Level3Graph {
	return &Level3Graph{
		withdrawBid: adaptive.NewBaseline(adaptive.NewWindow()),
		withdrawAsk: adaptive.NewBaseline(adaptive.NewWindow()),
		retreatBid:  adaptive.NewBaseline(adaptive.NewWindow()),
		retreatAsk:  adaptive.NewBaseline(adaptive.NewWindow()),
	}
}

func (op *Level3Graph) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			projected := op.observe(*(*Level3Input)(arriving))

			if !yield(unsafe.Pointer(&projected)) {
				return
			}
		}
	}
}

func (op *Level3Graph) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}

func (op *Level3Graph) observe(input Level3Input) data.ProjectionInput {
	withdrawBid := baselineReading(op.withdrawBid, input.WithFracBid)
	withdrawAsk := baselineReading(op.withdrawAsk, input.WithFracAsk)
	retreatBid := baselineReading(op.retreatBid, input.RetreatFracBid)
	retreatAsk := baselineReading(op.retreatAsk, input.RetreatFracAsk)
	values := map[string]float64{
		"curBid": input.CurBid, "curAsk": input.CurAsk,
		"prevBid": input.PrevBid, "prevAsk": input.PrevAsk,
		"curBidQty": input.CurBidQty, "curAskQty": input.CurAskQty,
		"prevBidQty": input.PrevBidQty, "prevAskQty": input.PrevAskQty,
		"unfilledBid": input.UnfilledBid, "unfilledAsk": input.UnfilledAsk,
		"logChangeBid": input.LogChangeBid, "logChangeAsk": input.LogChangeAsk,
		"retreatedBid": input.RetreatedBid, "retreatedAsk": input.RetreatedAsk,
		"withdrawnBid": input.WithdrawnBid, "withdrawnAsk": input.WithdrawnAsk,
		"replenishedBid": input.ReplenishedBid, "replenishedAsk": input.ReplenishedAsk,
		"retreatFracBid": input.RetreatFracBid, "retreatFracAsk": input.RetreatFracAsk,
		"withFracBid": input.WithFracBid, "withFracAsk": input.WithFracAsk,
		"repFracBid": input.RepFracBid, "repFracAsk": input.RepFracAsk,
		"retreatRateBid": input.RetreatRateBid, "retreatRateAsk": input.RetreatRateAsk,
		"withRateBid": input.WithRateBid, "withRateAsk": input.WithRateAsk,
		"repRateBid": input.RepRateBid, "repRateAsk": input.RepRateAsk,
		"withdrawal_fraction_baseline:bid":   withdrawBid.Mean,
		"withdrawal_fraction_divergence:bid": withdrawBid.Residual,
		"withdrawal_fraction_zscore:bid":     withdrawBid.ZScore,
		"withdrawal_fraction_baseline:ask":   withdrawAsk.Mean,
		"withdrawal_fraction_divergence:ask": withdrawAsk.Residual,
		"withdrawal_fraction_zscore:ask":     withdrawAsk.ZScore,
		"retreat_fraction_baseline:bid":      retreatBid.Mean,
		"retreat_fraction_divergence:bid":    retreatBid.Residual,
		"retreat_fraction_zscore:bid":        retreatBid.ZScore,
		"retreat_fraction_baseline:ask":      retreatAsk.Mean,
		"retreat_fraction_divergence:ask":    retreatAsk.Residual,
		"retreat_fraction_zscore:ask":        retreatAsk.ZScore,
	}
	flags := map[string]bool{
		"hasRate":                input.HasRate,
		"withdraw_bid_has_prior": withdrawBid.HasPrior,
		"withdraw_ask_has_prior": withdrawAsk.HasPrior,
		"retreat_bid_has_prior":  retreatBid.HasPrior,
		"retreat_ask_has_prior":  retreatAsk.HasPrior,
	}

	return data.ProjectionInput{Values: values, Flags: flags}
}

func level3Projection() *data.Projection {
	p := &data.Projection{Source: "toxicity", Metrics: []data.MetricProjection{
		{Label: "best_price:bid", Path: []string{"curBid"}, Unit: data.Unit("rate"), Timescale: data.Timescale("instantaneous")},
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
		{Label: "withdrawal_fraction_baseline:bid", Path: []string{"withdrawal_fraction_baseline:bid"}, Defined: []string{"withdraw_bid_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "withdrawal_fraction_divergence:bid", Path: []string{"withdrawal_fraction_divergence:bid"}, Defined: []string{"withdraw_bid_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "withdrawal_fraction_zscore:bid", Path: []string{"withdrawal_fraction_zscore:bid"}, Defined: []string{"withdraw_bid_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "withdrawal_fraction_baseline:ask", Path: []string{"withdrawal_fraction_baseline:ask"}, Defined: []string{"withdraw_ask_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "withdrawal_fraction_divergence:ask", Path: []string{"withdrawal_fraction_divergence:ask"}, Defined: []string{"withdraw_ask_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "withdrawal_fraction_zscore:ask", Path: []string{"withdrawal_fraction_zscore:ask"}, Defined: []string{"withdraw_ask_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "retreat_fraction_baseline:bid", Path: []string{"retreat_fraction_baseline:bid"}, Defined: []string{"retreat_bid_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "retreat_fraction_divergence:bid", Path: []string{"retreat_fraction_divergence:bid"}, Defined: []string{"retreat_bid_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "retreat_fraction_zscore:bid", Path: []string{"retreat_fraction_zscore:bid"}, Defined: []string{"retreat_bid_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "retreat_fraction_baseline:ask", Path: []string{"retreat_fraction_baseline:ask"}, Defined: []string{"retreat_ask_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "retreat_fraction_divergence:ask", Path: []string{"retreat_fraction_divergence:ask"}, Defined: []string{"retreat_ask_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "retreat_fraction_zscore:ask", Path: []string{"retreat_fraction_zscore:ask"}, Defined: []string{"retreat_ask_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
	}}
	return p
}
