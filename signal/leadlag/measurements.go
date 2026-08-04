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
) []*types.Measurement {
	rows := tickers
	section := signal.section

	measurements := make([]*types.Measurement, 0)
	out := make([]*types.Measurement, 0)

	anchor := section.CausalAnchor()

	if anchor == "" {
		section.ClearAnchor()
	}

	if anchor != "" {
		section.SetAnchor(anchor)
	}

	changed := make(map[string]struct{}, len(rows))

	for _, row := range rows {
		if row.Timestamp.IsZero() || row.Symbol == "" || row.Last == nil {
			continue
		}

		lastPrice := row.Last.Float64()

		if lastPrice <= 0 {
			continue
		}

		if section.ObservePrice(row.Symbol, lastPrice, row.Timestamp) {
			changed[row.Symbol] = struct{}{}
		}
	}

	for _, row := range rows {
		if row.Timestamp.IsZero() || row.Symbol == "" || row.Last == nil {
			continue
		}

		if row.Last.Float64() <= 0 {
			continue
		}

		if _, exists := changed[row.Symbol]; !exists {
			continue
		}

		if section.AnchorSymbol() == "" {
			out = append(out, signal.provisional(row.Symbol, row.Timestamp))
			continue
		}

		features := section.Features(row.Symbol)

		if features.Price <= 0 {
			continue
		}

		measurements = append(measurements, signal.score(row.Symbol, row.Timestamp, features))

		if row.Symbol == types.Focus() {
			out = append(out, measurements[len(measurements)-1])
		}
	}

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap(
			"measurements", out,
		))
	}

	return measurements
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
	section := signal.section
	selected := correlationSelection{}
	dynamicMax := 0

	if features.LagOK && features.SampleCount > 0 {
		dynamicMax = section.maxLagBars(features.SampleCount)
		effectiveSupport := features.SampleCount - int(math.Abs(float64(features.LagBars))) - 1
		searches := dynamicMax * 2
		significance := lagSearchThreshold(effectiveSupport, searches)

		if dynamicMax > 0 && features.LagBars > 0 && features.LagCorr > significance {
			selected.lagFraction = math.Abs(float64(features.LagBars)) / float64(dynamicMax)
			selected.signedLagCorrelation = features.LagCorr
			selected.lagBars = features.LagBars
			selected.lagDirection = 1
		}
	}

	if features.ContempOK {
		selected.signedContempCorrelation = features.ContempCorr
	}

	selected.signedCorrelation = selected.signedContempCorrelation

	if selected.signedLagCorrelation > math.Max(0, selected.signedContempCorrelation) {
		selected.signedCorrelation = selected.signedLagCorrelation
	}

	selected.correlation = math.Abs(selected.signedCorrelation)

	return selected
}

func lagSearchThreshold(effectiveSupport, searches int) float64 {
	if effectiveSupport <= 0 || searches <= 1 {
		return 0
	}

	return math.Sqrt(
		2 * math.Log(float64(searches)) / float64(effectiveSupport),
	)
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

	if features.MoveReady {
		anchorActive = 1
	}

	stallDamp := 1.0

	if features.MoveMoved {
		stallDamp = 0
	}

	stallMargin := math.Min(1, math.Max(0, features.StallMargin))
	noLag := 1 - selected.lagFraction
	positiveCorrelation := math.Max(0, selected.signedCorrelation)
	uncorrelated := 1 - positiveCorrelation
	lagCorrelation := math.Max(0, selected.signedLagCorrelation)
	contempCorrelation := math.Max(0, selected.signedContempCorrelation)
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
	evidenceCount int,
	selected correlationSelection,
	sampleSupport float64,
	weights evidenceWeights,
) *types.Measurement {
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
		Validity: types.ObservationValidity(evidenceCount),
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
	evidenceCount := features.SampleCount

	if evidenceCount == 0 {
		evidenceCount = signal.section.PriceSampleCount(symbol)
	}

	return buildScoreMeasurement(
		symbol, anchor, at, evidenceCount, selected, sampleSupport, weights,
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
