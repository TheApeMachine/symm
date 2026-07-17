package leadlag

import (
	"math"
	"time"

	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/types"
)

/*
score converts one follower's lag features into the published lead-lag
measurement bundle for the current tick.
*/
func (signal *Signal) score(
	symbol string,
	at time.Time,
	features LagFeatures,
) []*types.Measurement {
	lagFraction := 0.0
	lagCorrelation := 0.0
	contempCorrelation := 0.0
	signedLagCorrelation := 0.0
	signedContempCorrelation := 0.0

	if features.LagOK && features.SampleCount > 0 {
		dynamicMax := signal.section.maxLagBars(features.SampleCount)

		if dynamicMax > 0 {
			lagFraction = math.Abs(float64(features.LagBars)) / float64(dynamicMax)
		}

		signedLagCorrelation = features.LagCorr
		lagCorrelation = math.Abs(features.LagCorr)
	}

	if features.ContempOK {
		signedContempCorrelation = features.ContempCorr
		contempCorrelation = math.Abs(features.ContempCorr)
	}

	correlation := min(math.Max(contempCorrelation, lagCorrelation), 1)
	lagDominates := max(0, min(1, (lagCorrelation-contempCorrelation)*1e9))
	signedCorrelation := min(max(
		signedContempCorrelation+lagDominates*(signedLagCorrelation-signedContempCorrelation),
		-1,
	), 1)
	sampleSupport := 0.0

	if features.SampleCount > 0 {
		shortWindow, _, err := statistic.ResolveWindows(
			make([]float64, features.SampleCount),
			0,
			0,
		)

		if err == nil && shortWindow > 0 {
			sampleSupport = float64(features.SampleCount) / float64(shortWindow)
		}
	}

	anchorActive := 0.1

	if features.MoveMoved ||
		(features.StallMargin > 0 && lagFraction > 0) ||
		features.ContempOK ||
		features.LagOK {
		anchorActive = 1
	}

	stallDamp := 1.0

	if features.MoveMoved {
		stallDamp = 0
	}

	stallMargin := math.Min(1, math.Max(0, features.StallMargin))
	noLag := 1 - lagFraction
	uncorrelated := 1 - correlation
	lagEvidence := lagCorrelation * lagFraction
	syncEvidence := contempCorrelation * noLag
	decoupledEvidence := uncorrelated * (1 - stallMargin)
	stallEvidence := stallMargin * uncorrelated * noLag * stallDamp
	inefficient := sampleSupport * anchorActive * lagEvidence * (1 - stallMargin)
	syncScore := sampleSupport * anchorActive * syncEvidence * (1 - stallMargin)
	decoupled := sampleSupport * anchorActive * decoupledEvidence
	stall := sampleSupport * anchorActive * stallEvidence
	strength := max(max(inefficient, syncScore), max(decoupled, stall))
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}
	type reading struct {
		metric types.MetricType
		raw    float64
	}
	readings := []reading{
		{types.MetricCorrelation, correlation},
		{types.MetricSignedCorrelation, signedCorrelation},
		{types.MetricSignedContempCorrelation, signedContempCorrelation},
		{types.MetricSignedLagCorrelation, signedLagCorrelation},
		{types.MetricLagFraction, lagFraction},
		{types.MetricSampleSupport, sampleSupport},
		{types.MetricInefficient, inefficient},
		{types.MetricSync, syncScore},
		{types.MetricDecoupled, decoupled},
		{types.MetricStall, stall},
		{types.MetricStrength, strength},
	}
	measurements := make([]*types.Measurement, 0, len(readings))

	for _, item := range readings {
		measurements = append(measurements, &types.Measurement{
			Source:   types.SourceLeadLag,
			Metric:   item.metric,
			Stream:   types.LeadLag,
			Symbol:   symbol,
			At:       at,
			Unit:     types.UnitDimensionless,
			Raw:      item.raw,
			Validity: validity,
		})
	}

	return measurements
}

/*
provisional publishes zero-valued lead-lag evidence while the cohort has no
live leader, so quote ticks still surface an observation instead of silence.
*/
func (signal *Signal) provisional(
	symbol string,
	at time.Time,
) []*types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityProvisional,
		Readiness: types.ReadinessObservation,
		Reason:    "no cross-section leader",
	}
	metrics := []types.MetricType{
		types.MetricCorrelation,
		types.MetricSignedCorrelation,
		types.MetricSignedContempCorrelation,
		types.MetricSignedLagCorrelation,
		types.MetricLagFraction,
		types.MetricSampleSupport,
		types.MetricInefficient,
		types.MetricSync,
		types.MetricDecoupled,
		types.MetricStall,
		types.MetricStrength,
	}
	measurements := make([]*types.Measurement, 0, len(metrics))

	for _, metric := range metrics {
		measurements = append(measurements, &types.Measurement{
			Source:   types.SourceLeadLag,
			Metric:   metric,
			Stream:   types.LeadLag,
			Symbol:   symbol,
			At:       at,
			Unit:     types.UnitDimensionless,
			Raw:      0,
			Validity: validity,
		})
	}

	return measurements
}
