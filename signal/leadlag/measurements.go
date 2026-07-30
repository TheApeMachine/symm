package leadlag

import (
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
measureFrame ingests prices, refreshes the anchor, and scores each follower row.
*/
func (signal *Signal) measureFrame(
	tickers []kraken.TickerData,
	crossSection *types.CrossSection,
) []*types.Measurement {
	rows := tickers
	out := make([]*types.Measurement, 0, len(rows))
	uiOut := datura.NewMap(
		"measurements", make([]*types.Measurement, 0),
	)

	anchor, _ := crossSection.Leadership()

	if anchor == "" {
		signal.section.ClearAnchor()
	}

	if anchor != "" {
		signal.section.SetAnchor(anchor)
	}

	for _, row := range rows {
		if row.Timestamp.IsZero() || row.Symbol == "" || row.Last == nil {
			continue
		}

		lastPrice := row.Last.Float64()

		if lastPrice <= 0 {
			continue
		}

		signal.section.ObservePrice(row.Symbol, lastPrice, row.Timestamp)
	}

	for _, row := range rows {
		if row.Timestamp.IsZero() || row.Symbol == "" || row.Last == nil {
			continue
		}

		if row.Last.Float64() <= 0 {
			continue
		}

		if signal.section.AnchorSymbol() == "" {
			out = append(out, signal.provisional(row.Symbol, row.Timestamp))
			continue
		}

		features := signal.section.Features(row.Symbol)

		if features.Price <= 0 {
			continue
		}

		out = append(out, signal.score(row.Symbol, row.Timestamp, features))

		if row.Symbol == types.Focus() {
			uiOut["measurements"] = append(
				uiOut["measurements"].([]*types.Measurement), out[len(out)-1],
			)
		}
	}

	if len(uiOut["measurements"].([]*types.Measurement)) > 0 {
		utils.Publish(signal.ui, uiOut)
	}

	return out
}

type correlationSelection struct {
	correlation              float64
	signedCorrelation        float64
	signedContempCorrelation float64
	signedLagCorrelation     float64
	lagFraction              float64
	lagBars                  int
	lagDirection             float64
}

/*
selectCorrelations derives lag and contemporaneous correlation evidence.
*/
func (signal *Signal) selectCorrelations(
	features LagFeatures,
) correlationSelection {
	selected := correlationSelection{}
	dynamicMax := 0

	if features.LagOK && features.SampleCount > 0 {
		dynamicMax = signal.section.maxLagBars(features.SampleCount)

		if dynamicMax > 0 {
			selected.lagFraction = math.Abs(float64(features.LagBars)) / float64(dynamicMax)
		}

		selected.signedLagCorrelation = features.LagCorr
		selected.lagBars = features.LagBars

		if features.LagBars != 0 {
			selected.lagDirection = math.Copysign(1, float64(features.LagBars))
		}
	}

	if features.ContempOK {
		selected.signedContempCorrelation = features.ContempCorr
	}

	lagCorrelation := math.Abs(selected.signedLagCorrelation)
	contempCorrelation := math.Abs(selected.signedContempCorrelation)
	selected.correlation = min(math.Max(contempCorrelation, lagCorrelation), 1)
	lagDominates := dominanceFraction(lagCorrelation, contempCorrelation, features.SampleCount)
	selected.signedCorrelation = min(max(
		selected.signedContempCorrelation+lagDominates*(selected.signedLagCorrelation-selected.signedContempCorrelation),
		-1,
	), 1)

	return selected
}

func dominanceFraction(lagCorrelation, contempCorrelation float64, sampleCount int) float64 {
	diff := lagCorrelation - contempCorrelation

	if diff <= 0 {
		return 0
	}

	tolerance := correlationTolerance(sampleCount)

	if tolerance <= 0 {
		return 0
	}

	return min(1, diff/tolerance)
}

func correlationTolerance(sampleCount int) float64 {
	if sampleCount <= 0 {
		return 0
	}

	return 1 / math.Sqrt(float64(sampleCount))
}

/*
sampleSupportFraction scales evidence by resolved short-window depth.
*/
func sampleSupportFraction(sampleCount int) float64 {
	if sampleCount <= 0 {
		return 0
	}

	shortWindow, _, _ := windowsFromCount(sampleCount)

	if shortWindow <= 0 {
		return 0
	}

	return min(1, float64(sampleCount)/float64(shortWindow))
}

type evidenceWeights struct {
	inefficient float64
	syncScore   float64
	decoupled   float64
	stall       float64
	strength    float64
}

/*
weightEvidence combines correlation selection with stall and anchor context.
*/
func weightEvidence(
	features LagFeatures,
	selected correlationSelection,
	sampleSupport float64,
) evidenceWeights {
	anchorActive := 0.0

	if features.MoveMoved ||
		(features.StallMargin > 0 && selected.lagFraction > 0) ||
		features.ContempOK ||
		features.LagOK {
		anchorActive = 1
	}

	stallDamp := 1.0

	if features.MoveMoved {
		stallDamp = 0
	}

	stallMargin := math.Min(1, math.Max(0, features.StallMargin))
	noLag := 1 - selected.lagFraction
	uncorrelated := 1 - selected.correlation
	lagCorrelation := math.Abs(selected.signedLagCorrelation)
	contempCorrelation := math.Abs(selected.signedContempCorrelation)
	lagEvidence := lagCorrelation * selected.lagFraction
	syncEvidence := contempCorrelation * noLag
	decoupledEvidence := uncorrelated * (1 - stallMargin)
	stallEvidence := stallMargin * uncorrelated * noLag * stallDamp
	inefficient := sampleSupport * anchorActive * lagEvidence * (1 - stallMargin)
	syncScore := sampleSupport * anchorActive * syncEvidence * (1 - stallMargin)
	decoupled := sampleSupport * anchorActive * decoupledEvidence
	stall := sampleSupport * anchorActive * stallEvidence

	return evidenceWeights{
		inefficient: inefficient,
		syncScore:   syncScore,
		decoupled:   decoupled,
		stall:       stall,
		strength:    max(max(inefficient, syncScore), max(decoupled, stall)),
	}
}

/*
buildScoreMeasurement materializes the lead-lag metric bundle for one row.
*/
func buildScoreMeasurement(
	symbol string,
	anchor string,
	at time.Time,
	selected correlationSelection,
	sampleSupport float64,
	weights evidenceWeights,
) *types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}
	type reading struct {
		metric types.MetricType
		raw    float64
	}
	readings := []reading{
		{types.MetricCorrelation, selected.correlation},
		{types.MetricSignedCorrelation, selected.signedCorrelation},
		{types.MetricSignedContempCorrelation, selected.signedContempCorrelation},
		{types.MetricSignedLagCorrelation, selected.signedLagCorrelation},
		{types.MetricLagFraction, selected.lagFraction},
		{types.MetricSampleSupport, sampleSupport},
		{types.MetricInefficient, weights.inefficient},
		{types.MetricSync, weights.syncScore},
		{types.MetricDecoupled, weights.decoupled},
		{types.MetricStall, weights.stall},
		{types.MetricStrength, weights.strength},
	}

	// Signed lead-lag direction against the live anchor: +1 anchor leads this
	// follower, -1 this follower leads the anchor. Peer names the counterpart
	// so the analyzer can draw a directed Leads/Lags edge between their nodes.
	// Idle / self-anchor ticks still emit Raw 0 so the metric identity is stable.
	peer := ""

	if anchor != "" && anchor != symbol {
		peer = anchor
	}

	measurement := &types.Measurement{
		Source:   types.SourceLeadLag,
		Symbol:   symbol,
		Peer:     peer,
		At:       at,
		Validity: validity,
		Metrics:  make(map[string]types.MetricSample, len(readings)+1),
	}

	for _, item := range readings {
		measurement.Metrics[types.MetricKey(item.metric, types.SideNone)] = types.MetricSample{
			Raw:  item.raw,
			Unit: types.UnitDimensionless,
		}
	}

	measurement.Metrics[types.MetricKey(types.MetricSignedLagDirection, types.SideNone)] = types.MetricSample{
		Raw:        selected.lagDirection,
		Normalized: types.NormalizeFinite(selected.lagDirection),
		Unit:       types.UnitDimensionless,
	}

	return measurement
}

/*
score converts one follower's lag features into the published lead-lag
measurement bundle for the current tick.
*/
func (signal *Signal) score(
	symbol string,
	at time.Time,
	features LagFeatures,
) *types.Measurement {
	selected := signal.selectCorrelations(features)
	sampleSupport := sampleSupportFraction(features.SampleCount)
	weights := weightEvidence(features, selected, sampleSupport)
	anchor := signal.section.AnchorSymbol()

	return buildScoreMeasurement(
		symbol, anchor, at, selected, sampleSupport, weights,
	)
}

/*
provisional publishes zero-valued lead-lag evidence while the cohort has no
live leader, so quote ticks still surface an observation instead of silence.
*/
func (signal *Signal) provisional(
	symbol string,
	at time.Time,
) *types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityProvisional,
		Readiness: types.ReadinessObservation,
		Reason:    "no cross-section leader",
	}
	measurement := &types.Measurement{
		Source:   types.SourceLeadLag,
		Symbol:   symbol,
		At:       at,
		Validity: validity,
		Metrics:  make(map[string]types.MetricSample, 12),
	}

	measurement.Metrics[types.MetricKey(types.MetricCorrelation, types.SideNone)] = types.MetricSample{Raw: 0, Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricSignedCorrelation, types.SideNone)] = types.MetricSample{Raw: 0, Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricSignedContempCorrelation, types.SideNone)] = types.MetricSample{Raw: 0, Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricSignedLagCorrelation, types.SideNone)] = types.MetricSample{Raw: 0, Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricLagFraction, types.SideNone)] = types.MetricSample{Raw: 0, Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricSampleSupport, types.SideNone)] = types.MetricSample{Raw: 0, Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricInefficient, types.SideNone)] = types.MetricSample{Raw: 0, Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricSync, types.SideNone)] = types.MetricSample{Raw: 0, Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricDecoupled, types.SideNone)] = types.MetricSample{Raw: 0, Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStall, types.SideNone)] = types.MetricSample{Raw: 0, Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStrength, types.SideNone)] = types.MetricSample{Raw: 0, Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricSignedLagDirection, types.SideNone)] = types.MetricSample{Raw: 0, Unit: types.UnitDimensionless}

	return measurement
}
