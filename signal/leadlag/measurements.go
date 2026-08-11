package leadlag

import (
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

/*
measureFrame ingests prices, refreshes the anchor, and scores each follower row.
*/
func (signal *Signal) measureFrame(
	tickers []kraken.TickerData,
) []*types.Measurement {
	section := signal.section
	measurements := make([]*types.Measurement, 0)
	var focused []*types.Measurement

	anchor := section.CausalAnchor()

	if anchor == "" {
		section.ClearAnchor()
	}

	if anchor != "" {
		section.SetAnchor(anchor)
	}

	rowBatches := make(map[string][]kraken.TickerData)
	symbols := make([]string, 0)
	seenSymbols := make(map[string]struct{})

	for _, row := range tickers {
		if row.Timestamp.IsZero() || row.Symbol == "" || row.Last == nil {
			continue
		}

		lastPrice := row.Last.Float64()

		if lastPrice <= 0 {
			continue
		}

		if _, exists := seenSymbols[row.Symbol]; !exists {
			seenSymbols[row.Symbol] = struct{}{}
			symbols = append(symbols, row.Symbol)
		}

		rowBatches[row.Symbol] = append(rowBatches[row.Symbol], row)
	}
	sort.Strings(symbols)
	changedRows := make([][]kraken.TickerData, len(symbols))
	results := make([][]*types.Measurement, len(symbols))
	publish := make([][]*types.Measurement, len(symbols))

	group, _ := errgroup.WithContext(signal.ctx)

	for index, symbol := range symbols {
		resultIndex := index
		symbolRows := rowBatches[symbol]
		sort.SliceStable(symbolRows, func(leftIndex, rightIndex int) bool {
			return symbolRows[leftIndex].Timestamp.Before(symbolRows[rightIndex].Timestamp)
		})

		group.Go(func() error {
			for _, row := range symbolRows {
				if section.ObservePrice(row.Symbol, row.Last.Float64(), row.Timestamp) {
					changedRows[resultIndex] = append(changedRows[resultIndex], row)
				}
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"leadlag: parallel ingestion failed",
			err,
		))
		return measurements
	}

	for index, symbol := range symbols {
		resultIndex := index
		symbolRows := changedRows[index]

		if len(symbolRows) == 0 {
			continue
		}

		group.Go(func() error {
			for _, row := range symbolRows {
				features := section.Features(symbol)

				if features.Price <= 0 {
					continue
				}

				measurement := signal.score(symbol, row.Timestamp, features)
				measurement.PutMetric(
					types.MetricLastPrice,
					types.SideNone,
					types.MetricSample{
						Raw:  row.Last.Float64(),
						Unit: types.UnitQuoteCurrency,
					},
				)
				results[resultIndex] = append(results[resultIndex], measurement)

				if symbol == types.Focus() {
					publish[resultIndex] = append(publish[resultIndex], measurement)
				}
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"leadlag: parallel measurement failed",
			err,
		))
		return measurements
	}

	for index := range symbols {
		measurements = append(measurements, results[index]...)

		if symbols[index] == types.Focus() {
			focused = publish[index]
		}
	}

	if len(focused) > 0 {
		utils.Publish(signal.ui, datura.NewMap(
			"measurements", focused,
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
sampleSupportFraction scales evidence by resolved short-window return depth.
One price establishes an origin but contributes no return observation.
*/
func sampleSupportFraction(sampleCount int) float64 {
	if sampleCount <= 0 {
		return 0
	}

	shortWindow, _, _ := windowsFromCount(sampleCount)

	if shortWindow <= 0 {
		return 0
	}

	returnCount := sampleCount - 1

	if returnCount <= 0 {
		return 0
	}

	return min(1, float64(returnCount)/float64(shortWindow))
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
		ID:      uuid.NewString(),
		Source:  types.SourceLeadLag,
		Symbol:  symbol,
		Peer:    peer,
		At:      at,
		Metrics: make(map[string]types.MetricSample, len(readings)+2),
	}

	for _, item := range readings {
		sample := types.MetricSample{
			Raw:  item.raw,
			Unit: types.UnitDimensionless,
		}

		sample.Normalized = normalizedLeadLag(item.metric, item.raw)

		if sample.Normalized == nil {
			panic("leadlag: score outside its defined metric domain")
		}

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

	if direction.Normalized == nil {
		panic("leadlag: direction outside its nominal domain")
	}

	measurement.Metrics[types.MetricKey(
		types.MetricSignedLagDirection,
		types.SideNone,
	)] = direction

	snr, snrReady := types.MeasurementSignalNoiseRatio(
		types.SourceLeadLag,
		measurement.Metrics,
	)
	snrSample := types.MetricSample{
		Raw:  snr,
		Unit: types.UnitDimensionless,
	}

	if snrReady && sampleSupport > 0 {
		snrSample.Normalized = &snr
	}

	measurement.PutMetric(types.MetricSNR, types.SideNone, snrSample)

	return measurement
}

/*
normalizedLeadLag validates the domains established by the lead-lag equations:
correlations and direction are signed, while support, lag fraction, evidence
weights, and strength are bounded fractions.
*/
func normalizedLeadLag(metric types.MetricType, raw float64) *float64 {
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
	default:
		if raw < 0 || raw > 1 {
			return nil
		}
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

	measurement := buildScoreMeasurement(
		symbol, anchor, at, selected, sampleSupport, weights,
	)

	if !features.ObservedFrom.IsZero() {
		if features.ObservedFrom.After(at) {
			panic("leadlag: observation interval runs backward")
		}

		measurement.ObservedFrom = features.ObservedFrom
		measurement.Horizon = at.Sub(features.ObservedFrom)
	}

	if measurement.Peer == "" {
		return measurement
	}

	if features.PeerPrice <= 0 || features.PeerAt.IsZero() ||
		features.PeerFrom.IsZero() ||
		features.PeerFrom.After(features.PeerAt) {
		panic("leadlag: peer observation is incomplete")
	}

	measurement.PeerAt = features.PeerAt
	measurement.PeerObservedFrom = features.PeerFrom
	measurement.PutMetric(
		types.MetricPeerLastPrice,
		types.SideNone,
		types.MetricSample{
			Raw:  features.PeerPrice,
			Unit: types.UnitQuoteCurrency,
		},
	)

	return measurement
}
