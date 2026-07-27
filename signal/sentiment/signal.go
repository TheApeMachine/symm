package sentiment

import (
	"context"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal measures global market conviction from breadth and leadership
performance. Categories belong in logic; this signal emits numerical scores only.
*/
type Signal struct {
	*types.Actor
	thesis       *types.Thesis
	ctx          context.Context
	cancel       context.CancelFunc
	ui           chan []byte
	crossSection *types.CrossSection
}

/*
NewSignal creates sentiment measurement state for central market cuts so every
tick can compare breadth with current leadership.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		ui:           ui,
		crossSection: types.NewCrossSection(),
	}

	signal.Actor = types.NewActor(ctx, "sentiment", map[string]types.Handler{
		"ticker": {Topic: "thesis", Fn: signal.onTicker},
	})

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceSentiment)
}

/*
Initialize wires ticker ingress from Live. Sentiment is ticker-cross-section
only; book and trade floods must not fill unused buffers.
*/
func (signal *Signal) Initialize(live *types.Actor, thesis *types.Thesis) {
	signal.thesis = thesis
	signal.Actor.Initialize(
		types.Topic{Name: "ticker", Actor: live},
	)
}

func (signal *Signal) onTicker(message any) any {
	rows := message.(*kraken.Ticker).Data
	measurements, err := signal.Calculate(rows, nil, nil)

	if err != nil {
		errnie.Error(err)
		return types.SignalResult{Source: types.SourceSentiment, Status: types.SignalSkip}
	}

	if len(measurements) > 0 {
		signal.thesis.AppendMeasurements(measurements)
		return types.SignalResult{Source: types.SourceSentiment, Measurements: measurements, Status: types.SignalReady}
	}

	return types.SignalResult{Source: types.SourceSentiment, Status: types.SignalSkip}
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) ([]*types.Measurement, error) {
	if len(tickers) > 0 {
		signal.crossSection.Measure(tickers)
	}

	out := make([]*types.Measurement, 0, 64)
	var focusMeasurements []*types.Measurement

	if signal.crossSection == nil {
		return out, nil
	}

	leader, leadershipThreshold := signal.crossSection.Leadership()
	breadth := signal.crossSection.Breadth()
	cohortSize := 0
	leaderChange := 0.0
	totalChange := 0.0
	spreadUncertainty := 0.0
	positiveDisplacements := 0
	negativeDisplacements := 0
	minimumDisplacement := math.Inf(1)
	peers := make([]types.SymbolMetric, 0, 64)

	signal.crossSection.Metrics.Range(func(_, value any) bool {
		metric := value.(types.SymbolMetric)
		peers = append(peers, metric)
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

	// Emit the retained cohort, not only this message's rows, so a focused
	// major still receives frames when an alt ticker arrives (same pattern as
	// liquidity). Focus-gated WireMeasurements otherwise paints false STANDBY.
	for _, peer := range peers {
		change := peer.LatestChange
		leaderStrength := 0.0
		leaderEvidence := 0.0
		relativeLead := 0.0
		divergentScore := 0.0
		isLeader := leader == peer.Symbol && leaderMagnitude > 0

		if isLeader {
			leaderStrength = leaderMagnitude
			leaderEvidence = (leaderMagnitude - leadershipThreshold) / leaderMagnitude
			relativeLead = leaderMagnitude / totalChange
			divergentScore = divergenceScore
		}

		strength := math.Max(surgeScore, math.Max(divergentScore, slumpScore))
		measurement := &types.Measurement{
			Source:   types.SourceSentiment,
			Symbol:   peer.Symbol,
			At:       peer.At,
			Validity: validity,
			Metrics:  make(map[string]types.MetricSample, 9),
		}

		measurement.Metrics[types.MetricKey(types.MetricChange, types.SideNone)] = types.MetricSample{Raw: change, Unit: types.UnitDimensionless}
		measurement.Metrics[types.MetricKey(types.MetricBreadth, types.SideNone)] = types.MetricSample{Raw: breadth, Unit: types.UnitDimensionless}
		measurement.Metrics[types.MetricKey(types.MetricLeaderStrength, types.SideNone)] = types.MetricSample{Raw: leaderStrength, Unit: types.UnitDimensionless}
		measurement.Metrics[types.MetricKey(types.MetricLeaderEvidence, types.SideNone)] = types.MetricSample{Raw: leaderEvidence, Unit: types.UnitDimensionless}
		measurement.Metrics[types.MetricKey(types.MetricRelativeLead, types.SideNone)] = types.MetricSample{Raw: relativeLead, Unit: types.UnitDimensionless}
		measurement.Metrics[types.MetricKey(types.MetricSurgeScore, types.SideNone)] = types.MetricSample{Raw: surgeScore, Unit: types.UnitDimensionless}
		measurement.Metrics[types.MetricKey(types.MetricDivergentScore, types.SideNone)] = types.MetricSample{Raw: divergentScore, Unit: types.UnitDimensionless}
		measurement.Metrics[types.MetricKey(types.MetricSlumpScore, types.SideNone)] = types.MetricSample{Raw: slumpScore, Unit: types.UnitDimensionless}
		measurement.Metrics[types.MetricKey(types.MetricStrength, types.SideNone)] = types.MetricSample{Raw: strength, Unit: types.UnitDimensionless}

		out = append(out, measurement)

		if measurement.Symbol == types.Focus() {
			focusMeasurements = append(focusMeasurements, measurement)
		}
	}

	if len(focusMeasurements) > 0 {
		utils.Publish(signal.ui, datura.NewMap("measurements", focusMeasurements))
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
