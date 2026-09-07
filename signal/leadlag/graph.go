package leadlag

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

// A pipeline belongs to an ordered symbol pair. Undefined searches do not
// advance its histories; observations from another peer cannot train them.
type pipeline struct {
	progress   core.Primitive
	projection *data.Projection
}

func newPipeline() *pipeline {
	path := func(names ...string) core.Primitive {
		nodes := make([]core.Primitive, len(names))
		for i, name := range names {
			nodes[i] = store.NewGet(name)
		}
		return transport.NewPipe(nodes...)
	}
	field := func(name string, node core.Primitive) core.Primitive {
		return transport.NewPipe(node, store.NewKey(name))
	}
	return &pipeline{
		progress: transport.NewPipe(
			store.NewRecord(transport.NewPipe(),
				field("fisher", transport.NewPipe(path("pair"), nmcorrelation.NewFisher())),
				field("lag_history", transport.NewPipe(path("pair", "x"), adaptive.NewBaseline(adaptive.NewWindow()))),
				field("gain_history", transport.NewPipe(path("pair", "absolute_gain"), adaptive.NewBaseline(adaptive.NewWindow()))),
				field("correlation_history", transport.NewPipe(path("pair", "correlation"), adaptive.NewBaseline(adaptive.NewWindow()))),
				field("lag_velocity", temporal.NewVelocity(path("pair", "x"), path("at"))),
				field("gain_velocity", temporal.NewVelocity(path("pair", "absolute_gain"), path("at"))),
				field("resolution", equation.NewProduct[float64](path("pair", "spacing"), store.NewConstant(core.From(1e-9))))),
			store.NewRecord(transport.NewPipe(), field("span_seconds", equation.NewProduct[float64](path("pair", "span"), path("resolution")))),
		), projection: lagProjection(),
	}
}

func lagProjection() *data.Projection {
	p := &data.Projection{Source: "leadlag"}
	add := func(label string, path []string, unit data.Unit, defined ...string) {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: label, Path: path, Unit: unit, Timescale: data.TimescaleInstantaneous, Defined: defined})
	}
	for _, item := range [][2]string{{"contemporaneous_correlation", "contemporaneous"}, {"best_lag_correlation", "correlation"}, {"absolute_correlation_gain", "absolute_gain"}, {"lag_fraction", "lag_fraction"}} {
		add(item[0], []string{"pair", item[1]}, data.UnitDimensionless)
	}
	for _, item := range [][2]string{{"best_lag_index", "lag_index"}, {"reference_return_count", "left_returns"}, {"measured_return_count", "right_returns"}, {"overlap_pair_count", "support"}, {"effective_sample_count", "support"}, {"search_count", "search_count"}} {
		add(item[0], []string{"pair", item[1]}, data.UnitCount)
	}
	add("best_lag_seconds", []string{"pair", "x"}, data.UnitSecond)
	add("lag_search_resolution_seconds", []string{"resolution"}, data.UnitSecond)
	add("lag_search_span", []string{"span_seconds"}, data.UnitSecond)
	add("lag_peak_prominence", []string{"pair", "prominence"}, data.UnitDimensionless, "pair", "shape_defined")
	add("lag_peak_curvature", []string{"pair", "curvature"}, data.UnitPerSecond, "pair", "shape_defined")
	add("correlation_p_value", []string{"fisher", "p_value"}, data.UnitDimensionless, "fisher", "defined")
	add("search_adjusted_p_value", []string{"fisher", "search_adjusted_p_value"}, data.UnitDimensionless, "fisher", "defined")
	add("lag_baseline_seconds", []string{"lag_history", "baseline"}, data.UnitSecond)
	add("lag_divergence_seconds", []string{"lag_history", "residual"}, data.UnitSecond)
	add("lag_noise_scale_seconds", []string{"lag_history", "dispersion"}, data.UnitSecond, "lag_history", "variance_defined")
	add("lag_zscore", []string{"lag_history", "zscore"}, data.UnitDimensionless)
	add("lag_velocity", []string{"lag_velocity", "rate"}, data.UnitPerSecond, "lag_velocity", "defined")
	add("correlation_gain_baseline", []string{"gain_history", "baseline"}, data.UnitDimensionless)
	add("correlation_gain_zscore", []string{"gain_history", "zscore"}, data.UnitDimensionless)
	add("correlation_gain_velocity", []string{"gain_velocity", "rate"}, data.UnitPerSecond, "gain_velocity", "defined")
	add("best_lag_correlation_baseline", []string{"correlation_history", "baseline"}, data.UnitDimensionless)
	add("best_lag_correlation_zscore", []string{"correlation_history", "zscore"}, data.UnitDimensionless)
	p.Facts = []data.FactProjection{
		{Name: data.MetadataSupport, Path: []string{"correlation_history", "count"}},
		{Name: data.MetadataDivergence, Path: []string{"correlation_history", "residual"}, Defined: []string{"correlation_history", "has_prior"}},
		{Name: data.MetadataNoiseVariance, Path: []string{"correlation_history", "variance"}, Defined: []string{"correlation_history", "variance_defined"}},
	}
	return p
}
