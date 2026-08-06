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
		Source:  types.SourceLeadLag,
		Symbol:  symbol,
		Peer:    peer,
		At:      at,
		Metrics: make(map[string]types.MetricSample, len(readings)+1),
	}

	for _, item := range readings {
		sample := types.MetricSample{
			Raw:  item.raw,
			Unit: types.UnitDimensionless,
		}

		sample.Normalized = normalizedLeadLag(item.metric, item.raw)

		measurement.Metrics[types.MetricKey(item.metric, types.SideNone)] = sample
	}

	direction := types.MetricSample{
		Raw:  selected.lagDirection,
		Unit: types.UnitDimensionless,
	}

	direction.Normalized = normalizedLeadLag(
		types.MetricSignedLagDirection,
		selected.lagDirection,
	)

	measurement.Metrics[types.MetricKey(
		types.MetricSignedLagDirection,
		types.SideNone,
	)] = direction

	return measurement
}

/*
normalizedLeadLag validates the domains established by the lead-lag equations:
correlations and direction are signed, while support, lag fraction, evidence
weights, and strength are bounded fractions.
*/
func normalizedLeadLag(metric types.MetricType, raw float64) *float64 {
	if math.IsNaN(raw) || math.IsInf(raw, 0) {
		return nil
	}

	switch metric {
	case types.MetricSignedCorrelation,
		types.MetricSignedContempCorrelation,
		types.MetricSignedLagCorrelation:
		if raw < -1 || raw > 1 {
			return nil
		}
	case types.MetricSignedLagDirection:
		if raw != -1 && raw != 0 && raw != 1 {
			return nil
		}
	case types.MetricCorrelation,
		types.MetricLagFraction,
		types.MetricSampleSupport,
		types.MetricInefficient,
		types.MetricSync,
		types.MetricDecoupled,
		types.MetricStall,
		types.MetricStrength:
		if raw < 0 || raw > 1 {
			return nil
		}
	default:
		return nil
	}

	value := raw

	return &value
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
	measurement := &types.Measurement{
		Source: types.SourceLeadLag,
		Symbol: symbol,
		At:     at,
		Metrics: map[string]types.MetricSample{
			types.MetricKey(types.MetricCorrelation, types.SideNone): {
				Raw:  0,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSignedCorrelation, types.SideNone): {
				Raw:  0,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSignedContempCorrelation, types.SideNone): {
				Raw:  0,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSignedLagCorrelation, types.SideNone): {
				Raw:  0,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricLagFraction, types.SideNone): {
				Raw:  0,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSampleSupport, types.SideNone): {
				Raw:  0,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricInefficient, types.SideNone): {
				Raw:  0,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSync, types.SideNone): {
				Raw:  0,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricDecoupled, types.SideNone): {
				Raw:  0,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricStall, types.SideNone): {
				Raw:  0,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricStrength, types.SideNone): {
				Raw:  0,
				Unit: types.UnitDimensionless,
			},
			types.MetricKey(types.MetricSignedLagDirection, types.SideNone): {
				Raw:  0,
				Unit: types.UnitDimensionless,
			},
		},
	}

	return measurement
}
