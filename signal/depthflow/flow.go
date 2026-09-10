package depthflow

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
DepthInput is one message's mutation facts, not a reconstructed book.
*/
type DepthInput struct {
	ObservedBid, ObservedAsk, AddBid, AddAsk   float64
	ModifyBid, ModifyAsk, DeleteBid, DeleteAsk float64
	MutationBid, MutationAsk, Elapsed          float64
}

/*
Depth owns the two explicitly configured estimators for one symbol.
*/
type Depth struct {
	core.Base[DepthInput, data.ProjectionInput]
	imbalance *adaptive.Baseline
	rate      *adaptive.Baseline
}

func newDepthGraph() *Depth {
	return &Depth{
		imbalance: adaptive.NewBaseline(adaptive.NewWindow()),
		rate:      adaptive.NewBaseline(adaptive.NewWindow()),
	}
}

func (op *Depth) Next(
	in iter.Seq[core.Primitive[DepthInput, DepthInput]],
) iter.Seq[core.Primitive[data.ProjectionInput, data.ProjectionInput]] {
	return func(yield func(core.Primitive[data.ProjectionInput, data.ProjectionInput]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(op.observe(arriving.Read()))) {
				return
			}
		}
	}
}

func (op *Depth) observe(input DepthInput) data.ProjectionInput {
	observed := input.ObservedBid + input.ObservedAsk
	observedDiff := input.ObservedBid - input.ObservedAsk
	mutations := input.MutationBid + input.MutationAsk
	mutationDiff := input.MutationBid - input.MutationAsk
	values := map[string]float64{
		"observed_notional:bid":         input.ObservedBid,
		"observed_notional:ask":         input.ObservedAsk,
		"observed_notional":             observed,
		"observed_notional_diff":        observedDiff,
		"add_notional:bid":              input.AddBid,
		"add_notional:ask":              input.AddAsk,
		"modify_remaining_notional:bid": input.ModifyBid,
		"modify_remaining_notional:ask": input.ModifyAsk,
		"delete_count:bid":              input.DeleteBid,
		"delete_count:ask":              input.DeleteAsk,
		"mutation_count:bid":            input.MutationBid,
		"mutation_count:ask":            input.MutationAsk,
		"mutation_count":                mutations,
		"mutation_count_diff":           mutationDiff,
	}
	flags := map[string]bool{
		"observed_defined": observed > 0,
		"mutation_defined": mutations > 0,
		"rate_defined":     input.Elapsed > 0,
	}

	if observed > 0 {
		values["observed_notional_imbalance"] = observedDiff / observed
		reading := op.imbalance.Observe(observedDiff / observed)
		putBaseline(values, flags, "imbalance", reading)
	}

	if mutations > 0 {
		values["mutation_activity_imbalance"] = mutationDiff / mutations
	}

	if input.Elapsed > 0 {
		values["observed_notional_rate"] = observed / input.Elapsed
		reading := op.rate.Observe(observed / input.Elapsed)
		putBaseline(values, flags, "rate", reading)
	}

	return data.ProjectionInput{Values: values, Flags: flags}
}

func putBaseline(values map[string]float64, flags map[string]bool, prefix string, reading adaptive.BaselineReading) {
	flags[prefix+"_has_prior"] = reading.HasPrior
	flags[prefix+"_variance_defined"] = reading.VarianceDefined
	values[prefix+"_baseline"] = reading.Baseline
	values[prefix+"_residual"] = reading.Residual
	values[prefix+"_zscore"] = reading.ZScore
	values[prefix+"_count"] = reading.Count
	values[prefix+"_variance"] = reading.Variance
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
			p.Metrics = append(p.Metrics, data.MetricProjection{
				Label:   "observed_notional_" + prefix + "_" + item[0],
				Path:    []string{prefix + "_" + item[1]},
				Defined: []string{prefix + "_has_prior"},
				Unit:    metricUnit, Timescale: scale,
			})
		}
	}
	p.Facts = []data.FactProjection{
		{Name: data.MetadataSupport, Path: []string{"imbalance_count"}},
		{Name: data.MetadataDivergence, Path: []string{"imbalance_residual"}, Defined: []string{"imbalance_has_prior"}},
		{Name: data.MetadataNoiseVariance, Path: []string{"imbalance_variance"}, Defined: []string{"imbalance_variance_defined"}},
	}
	return p
}
