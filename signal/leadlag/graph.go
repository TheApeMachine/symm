package leadlag

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
)

// A pipeline belongs to an ordered symbol pair. Undefined searches do not
// advance its histories; observations from another peer cannot train them.
type pipeline struct {
	histories  [3]*adaptive.Baseline
	velocities [2]temporal.Velocity
}

const (
	lagHistory = iota
	gainHistory
	correlationHistory
)

func newPipeline() *pipeline {
	return &pipeline{histories: [3]*adaptive.Baseline{
		adaptive.NewBaseline(adaptive.NewWindow()),
		adaptive.NewBaseline(adaptive.NewWindow()),
		adaptive.NewBaseline(adaptive.NewWindow()),
	}}
}

var historyPaths = [3][1]string{{"x"}, {"absolute_gain"}, {"correlation"}}

/* Observe advances only the causal state owned by this ordered pair. */
func (pipeline *pipeline) Observe(pair map[string]core.Primitive, at int64) error {
	decoder := core.NewDecoder(pair)
	values := [3]float64{
		core.Decode[float64](decoder, historyPaths[lagHistory][:]...),
		core.Decode[float64](decoder, historyPaths[gainHistory][:]...),
		core.Decode[float64](decoder, historyPaths[correlationHistory][:]...),
	}

	if err := decoder.Error(); err != nil {
		return err
	}

	for index, value := range values {
		pipeline.histories[index].Observe(value)
	}
	pipeline.velocities[lagHistory].Observe(values[lagHistory], at)
	pipeline.velocities[gainHistory].Observe(values[gainHistory], at)
	return nil
}

/* Fields exposes the selected pair's state once at the measurement boundary. */
func (pipeline *pipeline) Fields(pair map[string]core.Primitive) map[string]core.Primitive {
	return map[string]core.Primitive{
		"pair":                core.From(pair),
		"lag_history":         core.From(pipeline.histories[lagHistory].Reading.Fields()),
		"gain_history":        core.From(pipeline.histories[gainHistory].Reading.Fields()),
		"correlation_history": core.From(pipeline.histories[correlationHistory].Reading.Fields()),
		"lag_velocity":        &pipeline.velocities[lagHistory].Reading,
		"gain_velocity":       &pipeline.velocities[gainHistory].Reading,
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
