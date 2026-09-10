package leadlag

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
)

type pairObservation struct {
	Lag, AbsoluteGain, Correlation float64
}

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

func (pipeline *pipeline) Observe(pair pairObservation, at int64) {
	values := [3]float64{pair.Lag, pair.AbsoluteGain, pair.Correlation}
	for index, value := range values {
		pipeline.histories[index].Observe(value)
	}
	pipeline.velocities[lagHistory].Observe(values[lagHistory], at)
	pipeline.velocities[gainHistory].Observe(values[gainHistory], at)
}

func (pipeline *pipeline) project(pair nmcorrelation.LeadLagReading, fisher nmcorrelation.FisherReading, resolution, spanSeconds float64) data.ProjectionInput {
	lag := pipeline.histories[lagHistory].Reading
	gain := pipeline.histories[gainHistory].Reading
	corr := pipeline.histories[correlationHistory].Reading
	lagVel := pipeline.velocities[lagHistory].Reading
	gainVel := pipeline.velocities[gainHistory].Reading
	values := map[string]float64{
		"contemporaneous_correlation":   pair.Contemporaneous,
		"best_lag_correlation":          pair.Correlation,
		"absolute_correlation_gain":     pair.AbsoluteGain,
		"lag_fraction":                  pair.LagFraction,
		"best_lag_index":                pair.LagIndex,
		"reference_return_count":        pair.Observations,
		"measured_return_count":         pair.Observations,
		"overlap_pair_count":            pair.Support,
		"effective_sample_count":        pair.Support,
		"search_count":                  pair.SearchCount,
		"best_lag_seconds":              pair.X,
		"lag_search_resolution_seconds": resolution,
		"lag_search_span":               spanSeconds,
		"lag_peak_prominence":           pair.Prominence,
		"lag_peak_curvature":            pair.Curvature,
		"correlation_p_value":           fisher.PValue,
		"search_adjusted_p_value":       fisher.SearchAdjustedPValue,
		"lag_baseline_seconds":          lag.Baseline,
		"lag_divergence_seconds":        lag.Residual,
		"lag_noise_scale_seconds":       lag.Dispersion,
		"lag_zscore":                    lag.ZScore,
		"lag_velocity":                  lagVel.Rate,
		"correlation_gain_baseline":     gain.Baseline,
		"correlation_gain_zscore":       gain.ZScore,
		"correlation_gain_velocity":     gainVel.Rate,
		"best_lag_correlation_baseline": corr.Baseline,
		"best_lag_correlation_zscore":   corr.ZScore,
		"correlation_history_count":     corr.Count,
		"correlation_history_residual":  corr.Residual,
		"correlation_history_variance":  corr.Variance,
	}
	flags := map[string]bool{
		"shape_defined":                        pair.ShapeDefined,
		"fisher_defined":                       fisher.Defined,
		"lag_history_variance_defined":         lag.VarianceDefined,
		"lag_velocity_defined":                 lagVel.Defined,
		"gain_velocity_defined":                gainVel.Defined,
		"correlation_history_has_prior":        corr.HasPrior,
		"correlation_history_variance_defined": corr.VarianceDefined,
	}
	return data.ProjectionInput{Values: values, Flags: flags}
}

func lagProjection() *data.Projection {
	p := &data.Projection{Source: "leadlag"}
	add := func(label string, unit data.Unit, defined ...string) {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: label, Path: []string{label}, Unit: unit, Timescale: data.TimescaleInstantaneous, Defined: defined})
	}
	for _, name := range []string{"contemporaneous_correlation", "best_lag_correlation", "absolute_correlation_gain", "lag_fraction"} {
		add(name, data.UnitDimensionless)
	}
	for _, name := range []string{"best_lag_index", "reference_return_count", "measured_return_count", "overlap_pair_count", "effective_sample_count", "search_count"} {
		add(name, data.UnitCount)
	}
	add("best_lag_seconds", data.UnitSecond)
	add("lag_search_resolution_seconds", data.UnitSecond)
	add("lag_search_span", data.UnitSecond)
	add("lag_peak_prominence", data.UnitDimensionless, "shape_defined")
	add("lag_peak_curvature", data.UnitPerSecond, "shape_defined")
	add("correlation_p_value", data.UnitDimensionless, "fisher_defined")
	add("search_adjusted_p_value", data.UnitDimensionless, "fisher_defined")
	add("lag_baseline_seconds", data.UnitSecond)
	add("lag_divergence_seconds", data.UnitSecond)
	add("lag_noise_scale_seconds", data.UnitSecond, "lag_history_variance_defined")
	add("lag_zscore", data.UnitDimensionless)
	add("lag_velocity", data.UnitPerSecond, "lag_velocity_defined")
	add("correlation_gain_baseline", data.UnitDimensionless)
	add("correlation_gain_zscore", data.UnitDimensionless)
	add("correlation_gain_velocity", data.UnitPerSecond, "gain_velocity_defined")
	add("best_lag_correlation_baseline", data.UnitDimensionless)
	add("best_lag_correlation_zscore", data.UnitDimensionless)
	p.Facts = []data.FactProjection{
		{Name: data.MetadataSupport, Path: []string{"correlation_history_count"}},
		{Name: data.MetadataDivergence, Path: []string{"correlation_history_residual"}, Defined: []string{"correlation_history_has_prior"}},
		{Name: data.MetadataNoiseVariance, Path: []string{"correlation_history_variance"}, Defined: []string{"correlation_history_variance_defined"}},
	}
	return p
}
