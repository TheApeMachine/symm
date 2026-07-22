package sentiment

import (
	"context"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/types"
)

/*
Signal measures global market conviction from breadth and leadership
performance. Categories belong in logic; this signal emits numerical scores only.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	ui     chan []byte
}

/*
NewSignal creates sentiment measurement state for central market cuts so every
tick can compare breadth with current leadership.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ui:     ui,
	}
}

/*
Publish sends one compact datura frame per symbol with nested metrics so the UI
receives one observation map rather than one envelope per metric.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.ForPublish(measurements),
	}.Marshal():
	default:
	}
}

/*
Interest requires the ticker stream; sentiment reads global breadth and
leadership from the cross-sectional quote surface.
*/
func (signal *Signal) Interest() types.StreamInterest {
	return types.StreamTicker
}

/*
Measure returns typed measurements for the cut, or an error when the
cut cannot be measured honestly.
*/
func (signal *Signal) Measure(thesis *types.Thesis) ([]*types.Measurement, error) {
	return signal.Calculate(thesis.Market())
}

/*
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	frame *types.MarketFrame,
) ([]*types.Measurement, error) {
	tickers := frame.Tickers
	out := make([]*types.Measurement, 0, len(tickers)*9)

	if frame.CrossSection == nil {
		return out, nil
	}

	leader, leadershipThreshold := frame.CrossSection.Leadership()
	breadth := frame.CrossSection.Breadth()
	cohortSize := 0
	leaderChange := 0.0
	totalChange := 0.0
	spreadUncertainty := 0.0
	positiveDisplacements := 0
	negativeDisplacements := 0
	minimumDisplacement := math.Inf(1)

	frame.CrossSection.Metrics.Range(func(_, value any) bool {
		metric := value.(types.SymbolMetric)
		absoluteChange := math.Abs(metric.LatestChange)
		displacement := absoluteChange - metric.RelativeSpread
		cohortSize++
		totalChange += absoluteChange
		spreadUncertainty += metric.RelativeSpread

		if metric.Symbol == leader {
			leaderChange = metric.LatestChange
		}

		if displacement <= 0 {
			return true
		}

		minimumDisplacement = math.Min(minimumDisplacement, displacement)

		if metric.LatestChange > 0 {
			positiveDisplacements++
		}

		if metric.LatestChange < 0 {
			negativeDisplacements++
		}

		return true
	})

	surgeScore := 0.0
	slumpScore := 0.0

	if cohortSize > 0 && positiveDisplacements == cohortSize && leadershipThreshold > 0 {
		surgeScore = minimumDisplacement / leadershipThreshold
	}

	if cohortSize > 0 && negativeDisplacements == cohortSize && leadershipThreshold > 0 {
		slumpScore = minimumDisplacement / leadershipThreshold
	}

	leaderMagnitude := math.Abs(leaderChange)
	divergenceScore := 0.0

	if leaderChange > 0 {
		peerChange := totalChange - leaderMagnitude
		dominance := leaderMagnitude - peerChange - spreadUncertainty

		if dominance > 0 {
			divergenceScore = dominance / leaderMagnitude
		}
	}

	if divergenceScore > 0 {
		surgeScore = 0
	}

	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}

	if cohortSize < 2 {
		validity.State = types.ValidityProvisional
		validity.Reason = "peer return cohort unavailable"
	}

	for _, row := range tickers {
		change := row.ChangePct / 100
		leaderStrength := 0.0
		leaderEvidence := 0.0
		relativeLead := 0.0
		divergentScore := 0.0
		isLeader := leader == row.Symbol && leaderMagnitude > 0

		if isLeader {
			leaderStrength = leaderMagnitude
			leaderEvidence = (leaderMagnitude - leadershipThreshold) / leaderMagnitude
			relativeLead = leaderMagnitude / totalChange
			divergentScore = divergenceScore
		}

		strength := math.Max(surgeScore, math.Max(divergentScore, slumpScore))
		specs := []struct {
			metric types.MetricType
			raw    float64
		}{
			{types.MetricChange, change},
			{types.MetricBreadth, breadth},
			{types.MetricLeaderStrength, leaderStrength},
			{types.MetricLeaderEvidence, leaderEvidence},
			{types.MetricRelativeLead, relativeLead},
			{types.MetricSurgeScore, surgeScore},
			{types.MetricDivergentScore, divergentScore},
			{types.MetricSlumpScore, slumpScore},
			{types.MetricStrength, strength},
		}

		for _, spec := range specs {
			out = append(out, &types.Measurement{
				Source:   types.SourceSentiment,
				Metric:   spec.metric,
				Stream:   types.Sentiment,
				Symbol:   row.Symbol,
				At:       row.Timestamp,
				Unit:     types.UnitDimensionless,
				Raw:      spec.raw,
				Validity: validity,
			})
		}
	}

	return out, nil
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
