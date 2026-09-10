package toxicity

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

type TradeInput struct {
	BracketQty, MatchedBidQty, MatchedAskQty float64
	TouchFillBidQty, TouchFillAskQty         float64
	TouchFillBidFrac, TouchFillAskFrac       float64
	TouchFillBidRate, TouchFillAskRate       float64
	HasRate, BidSupported, AskSupported      bool
}

type TradeGraph struct {
	core.Base[TradeInput, data.ProjectionInput]
	bid *adaptive.Baseline
	ask *adaptive.Baseline
}

func newTradeGraph() *TradeGraph {
	return &TradeGraph{
		bid: adaptive.NewBaseline(adaptive.NewWindow()),
		ask: adaptive.NewBaseline(adaptive.NewWindow()),
	}
}

func (op *TradeGraph) Next(
	in iter.Seq[core.Primitive[TradeInput, TradeInput]],
) iter.Seq[core.Primitive[data.ProjectionInput, data.ProjectionInput]] {
	return func(yield func(core.Primitive[data.ProjectionInput, data.ProjectionInput]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(op.observe(arriving.Read()))) {
				return
			}
		}
	}
}

func (op *TradeGraph) observe(input TradeInput) data.ProjectionInput {
	bid := op.bid.Observe(input.TouchFillBidFrac)
	ask := op.ask.Observe(input.TouchFillAskFrac)
	evidence := ask

	if input.BidSupported {
		evidence = bid
	}

	values := map[string]float64{
		"bracketQty":                   input.BracketQty,
		"matchedBidQty":                input.MatchedBidQty,
		"matchedAskQty":                input.MatchedAskQty,
		"touchFillBidQty":              input.TouchFillBidQty,
		"touchFillAskQty":              input.TouchFillAskQty,
		"touchFillBidFrac":             input.TouchFillBidFrac,
		"touchFillAskFrac":             input.TouchFillAskFrac,
		"touchFillBidRate":             input.TouchFillBidRate,
		"touchFillAskRate":             input.TouchFillAskRate,
		"fill_fraction_baseline:bid":   bid.Baseline,
		"fill_fraction_divergence:bid": bid.Residual,
		"fill_fraction_zscore:bid":     bid.ZScore,
		"fill_fraction_baseline:ask":   ask.Baseline,
		"fill_fraction_divergence:ask": ask.Residual,
		"fill_fraction_zscore:ask":     ask.ZScore,
		"evidence_count":               evidence.Count,
		"evidence_residual":            evidence.Residual,
		"evidence_variance":            evidence.Variance,
	}
	flags := map[string]bool{
		"hasRate":         input.HasRate,
		"bid_has_prior":   bid.HasPrior,
		"ask_has_prior":   ask.HasPrior,
		"quality_defined": input.BidSupported || input.AskSupported,
	}

	return data.ProjectionInput{Values: values, Flags: flags}
}

func tradeProjection() *data.Projection {
	p := &data.Projection{Source: "toxicity", Metrics: []data.MetricProjection{
		{Label: "bracket_trade_quantity", Path: []string{"bracketQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "matched_touch_trade_quantity:bid", Path: []string{"matchedBidQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "matched_touch_trade_quantity:ask", Path: []string{"matchedAskQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_fill_quantity:bid", Path: []string{"touchFillBidQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_fill_quantity:ask", Path: []string{"touchFillAskQty"}, Unit: data.Unit("count"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_fill_fraction:bid", Path: []string{"touchFillBidFrac"}, Unit: data.Unit("dimensionless"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_fill_fraction:ask", Path: []string{"touchFillAskFrac"}, Unit: data.Unit("dimensionless"), Timescale: data.Timescale("instantaneous")},
		{Label: "touch_fill_rate:bid", Path: []string{"touchFillBidRate"}, Unit: data.Unit("per_second"), Timescale: data.Timescale("per_second"), Defined: []string{"hasRate"}},
		{Label: "touch_fill_rate:ask", Path: []string{"touchFillAskRate"}, Unit: data.Unit("per_second"), Timescale: data.Timescale("per_second"), Defined: []string{"hasRate"}},
		{Label: "fill_fraction_baseline:bid", Path: []string{"fill_fraction_baseline:bid"}, Defined: []string{"bid_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "fill_fraction_divergence:bid", Path: []string{"fill_fraction_divergence:bid"}, Defined: []string{"bid_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "fill_fraction_zscore:bid", Path: []string{"fill_fraction_zscore:bid"}, Defined: []string{"bid_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "fill_fraction_baseline:ask", Path: []string{"fill_fraction_baseline:ask"}, Defined: []string{"ask_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "fill_fraction_divergence:ask", Path: []string{"fill_fraction_divergence:ask"}, Defined: []string{"ask_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		{Label: "fill_fraction_zscore:ask", Path: []string{"fill_fraction_zscore:ask"}, Defined: []string{"ask_has_prior"}, Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
	}}
	p.Facts = []data.FactProjection{
		{Name: data.MetadataSupport, Path: []string{"evidence_count"}},
		{Name: data.MetadataDivergence, Path: []string{"evidence_residual"}, Defined: []string{"quality_defined"}},
		{Name: data.MetadataNoiseVariance, Path: []string{"evidence_variance"}, Defined: []string{"quality_defined"}},
	}
	return p
}
