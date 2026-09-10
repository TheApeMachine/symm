package correlation

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/calculus"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

type pairResult struct {
	dependence nmcorrelation.DependenceReading
	fisher     nmcorrelation.FisherReading
}

type pipeline struct {
	pairwise   *nmcorrelation.Dependence
	fisher     *nmcorrelation.Fisher
	cohort     *nmcorrelation.Cohort
	history    *nmcorrelation.FisherEstimator
	relative   *adaptive.Baseline
	corrVel    *temporal.Velocity
	energyVel  *temporal.Velocity
	projection *data.Projection
}

func newPipeline() *pipeline {
	return &pipeline{
		pairwise:   nmcorrelation.NewDependence(algo.NewHayashiYoshida()),
		fisher:     nmcorrelation.NewFisher(),
		cohort:     nmcorrelation.NewCohort(calculus.NewAtanh[float64]()),
		history:    nmcorrelation.NewFisherEstimator(equation.NewWelford()),
		relative:   adaptive.NewBaseline(adaptive.NewWindow()),
		corrVel:    temporal.NewVelocity(),
		energyVel:  temporal.NewVelocity(),
		projection: correlationProjection(),
	}
}

func (built *pipeline) pair(left, right []equation.Price) (pairResult, error) {
	dependence, err := transport.Evaluate(built.pairwise, transport.Values(equation.LagProfileInput{Left: left, Right: right}))
	if err != nil {
		return pairResult{}, err
	}
	fisher, err := transport.Evaluate(built.fisher, transport.Values(nmcorrelation.FisherSample{
		Correlation: dependence.Correlation, Support: dependence.Support,
	}))
	if err != nil {
		return pairResult{}, err
	}
	return pairResult{dependence: dependence, fisher: fisher}, nil
}

func (built *pipeline) fold(peers []nmcorrelation.Peer) (nmcorrelation.CohortSummary, error) {
	return transport.Evaluate(built.cohort, transport.Values(peers...))
}

func (built *pipeline) advance(pair pairResult, cohort nmcorrelation.CohortSummary, at int64) (data.ProjectionInput, error) {
	relative := 0.0
	if cohort.PeerEnergyRate > 0 {
		relative = pair.dependence.LeftEnergyRate / cohort.PeerEnergyRate
	}
	history, err := transport.Evaluate(built.history, transport.Values(cohort.SignedCorrelation))
	if err != nil {
		return data.ProjectionInput{}, err
	}
	relativeHistory := built.relative.Observe(relative)
	corrVel := built.corrVel.Observe(cohort.SignedCorrelation, at)
	energyVel := built.energyVel.Observe(relative, at)
	values := map[string]float64{
		"signed_correlation":                pair.dependence.Correlation,
		"absolute_correlation":              abs(pair.dependence.Correlation),
		"cohort_signed_correlation":         cohort.SignedCorrelation,
		"cohort_absolute_correlation":       cohort.AbsoluteCorrelation,
		"covariance":                        pair.dependence.Covariance,
		"return_energy:reference":           pair.dependence.RightEnergy,
		"return_energy:measured":            pair.dependence.LeftEnergy,
		"return_energy_rate:reference":      pair.dependence.RightEnergyRate,
		"return_energy_rate:measured":       pair.dependence.LeftEnergyRate,
		"focal_return_energy_rate":          pair.dependence.LeftEnergyRate,
		"overlap_density":                   pair.dependence.OverlapDensity,
		"peer_return_energy_rate":           cohort.PeerEnergyRate,
		"supported_return_count:measured":   pair.dependence.LeftReturns,
		"supported_return_count:reference":  pair.dependence.RightReturns,
		"overlap_pair_count":                pair.dependence.Support,
		"effective_sample_count":            pair.dependence.Support,
		"shared_time":                       pair.dependence.SharedTime,
		"correlation_p_value":               pair.fisher.PValue,
		"correlation_standard_error_fisher": pair.fisher.StandardError,
		"cohort_peer_count":                 cohort.Peers,
		"cohort_effective_peer_count":       cohort.EffectivePeers,
		"cohort_correlation_dispersion":     cohort.Dispersion,
		"relative_return_energy":            relative,
		"relative_cohort_return_energy":     relative,
		"correlation_baseline":              history.Baseline,
		"correlation_divergence":            history.Divergence,
		"correlation_zscore":                history.ZScore,
		"correlation_velocity":              corrVel.Rate,
		"relative_return_energy_baseline":   relativeHistory.Baseline,
		"relative_return_energy_divergence": relativeHistory.Residual,
		"relative_return_energy_zscore":     relativeHistory.ZScore,
		"relative_return_energy_velocity":   energyVel.Rate,
		"support":                           history.Count,
		"history_variance":                  history.Variance,
	}
	if history.Defined {
		values["support"] = history.Count
	}
	flags := map[string]bool{
		"fisher_defined":               pair.fisher.Defined,
		"cohort_fisher_defined":        cohort.FisherDefined,
		"history_defined":              history.Defined,
		"prior_available":              history.Defined && history.HasPrior,
		"noise_available":              history.Defined && history.VarianceDefined,
		"correlation_velocity_defined": corrVel.Defined,
		"energy_velocity_defined":      energyVel.Defined,
	}
	return data.ProjectionInput{Values: values, Flags: flags}, nil
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func correlationProjection() *data.Projection {
	p := &data.Projection{Source: "correlation"}
	add := func(label string, unit data.Unit, defined ...string) {
		p.Metrics = append(p.Metrics, data.MetricProjection{Label: label, Path: []string{label}, Defined: defined, Unit: unit, Timescale: data.TimescaleInstantaneous})
	}
	for _, name := range []string{"signed_correlation", "absolute_correlation", "cohort_signed_correlation", "cohort_absolute_correlation"} {
		add(name, data.UnitDimensionless)
	}
	for _, name := range []string{"covariance", "return_energy:reference", "return_energy:measured"} {
		add(name, data.UnitNat)
	}
	for _, name := range []string{"return_energy_rate:reference", "return_energy_rate:measured", "focal_return_energy_rate", "overlap_density", "peer_return_energy_rate"} {
		add(name, data.UnitPerSecond)
	}
	for _, name := range []string{"supported_return_count:measured", "supported_return_count:reference", "overlap_pair_count", "effective_sample_count", "cohort_peer_count", "cohort_effective_peer_count"} {
		add(name, data.UnitCount)
	}
	add("shared_time", data.UnitSecond)
	add("correlation_p_value", data.UnitDimensionless, "fisher_defined")
	add("correlation_standard_error_fisher", data.UnitDimensionless, "fisher_defined")
	add("cohort_correlation_dispersion", data.UnitNat, "cohort_fisher_defined")
	add("relative_return_energy", data.UnitDimensionless)
	add("relative_cohort_return_energy", data.UnitDimensionless)
	add("correlation_baseline", data.UnitDimensionless, "history_defined")
	add("correlation_divergence", data.UnitNat, "prior_available")
	add("correlation_zscore", data.UnitDimensionless, "prior_available")
	add("correlation_velocity", data.UnitPerSecond, "correlation_velocity_defined")
	add("relative_return_energy_baseline", data.UnitDimensionless)
	add("relative_return_energy_divergence", data.UnitDimensionless)
	add("relative_return_energy_zscore", data.UnitDimensionless)
	add("relative_return_energy_velocity", data.UnitPerSecond, "energy_velocity_defined")
	p.Facts = []data.FactProjection{
		{Name: data.MetadataSupport, Path: []string{"support"}},
		{Name: data.MetadataDivergence, Path: []string{"correlation_divergence"}, Defined: []string{"prior_available"}},
		{Name: data.MetadataNoiseVariance, Path: []string{"history_variance"}, Defined: []string{"noise_available"}},
	}
	return p
}
