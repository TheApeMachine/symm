package liquidity

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/statistic"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	signalsupport "github.com/theapemachine/symm/signal"
)

func (signal *Signal) fromCrossSection(
	row *krakenmarket.Symbol,
	spread float64,
	at time.Time,
) (logic.Measurement, error) {
	if err := crossSection.Observe(row); err != nil {
		return logic.Measurement{}, err
	}

	quoteVol := row.Volume
	price := row.Price
	peers := crossSection.Volumes()

	if len(peers) < 2 {
		return signal.bestEffort(
			at,
			"liquidity: peer universe is not ready",
		), nil
	}

	relativeVolume, baselineReady, baselineErr := signal.observeVolumeBaseline(
		quoteVol,
		at,
	)

	if baselineErr != nil {
		return logic.Measurement{}, errnie.Error(baselineErr)
	}

	scaledQuoteVol, scaledPeers := signal.absoluteScaledVolumes(
		quoteVol,
		peers,
		relativeVolume,
		baselineReady,
	)

	lower, upper := signal.quartiles(scaledPeers)
	peakScarcity := signal.isPeakScarcity(scaledQuoteVol, scaledPeers)
	median := float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(scaledPeers...)...))

	if median <= 0 {
		return signal.bestEffort(
			at,
			fmt.Sprintf("liquidity: non-positive cross-section median %.4f", median),
		), nil
	}

	category := signal.classify(
		scaledQuoteVol,
		lower,
		upper,
		peakScarcity,
		signal.historicallyLiquid(relativeVolume, baselineReady, scaledPeers, scaledQuoteVol),
	)

	scarcityRaw := math.Max(0, (median-scaledQuoteVol)/median)
	depthRaw := math.Max(0, (scaledQuoteVol-median)/median)

	peakScore := 0.0

	if peakScarcity {
		peakScore = 1
	}

	competingScores := []float64{
		scarcityRaw,
		medianDepthEvidence(scaledQuoteVol, lower, upper),
		depthRaw,
	}

	if peakScarcity {
		competingScores[0] = math.Max(competingScores[0], peakScore)
	}

	probabilities, err := signalsupport.ClassifierProbabilities(competingScores)

	if err != nil {
		return logic.Measurement{}, err
	}

	return signal.publishCrossSection(
		row,
		category,
		probabilities,
		competingScores,
		scarcityRaw,
		depthRaw,
		spread,
		price,
		at,
	)
}

func (signal *Signal) publishCrossSection(
	row *krakenmarket.Symbol,
	category logic.CategoryType,
	probabilities []float64,
	competingScores []float64,
	scarcityRaw float64,
	depthRaw float64,
	spread float64,
	price float64,
	at time.Time,
) (logic.Measurement, error) {
	categoryIndex := signal.categoryIndex(category)
	surpriseVector := signal.transition.PadObserved(probabilities, 0)
	surprise, err := signal.transition.Surprise(surpriseVector)

	if err != nil {
		return logic.Measurement{}, err
	}

	signal.transition.Update(categoryIndex)

	confidence, err := probability.CategoryShareConfidence(competingScores, categoryIndex)

	if err != nil {
		return logic.Measurement{}, err
	}

	strength := scarcityRaw

	if category == logic.CategoryRobustLiquidity {
		strength = depthRaw
	}

	if category == logic.CategoryMedianDepth {
		strength = math.Max(competingScores[0], competingScores[1])
	}

	elapsed, err := signalsupport.ObservationElapsed(signal.measurements, at)

	if err != nil {
		return signal.bestEffort(
			at,
			fmt.Sprintf("liquidity: observation elapsed: %v", err),
		), nil
	}

	if spread <= 0 {
		return signal.bestEffort(
			at,
			fmt.Sprintf("liquidity: invalid spread %.4f", spread),
		), nil
	}

	return logic.Measurement{
		Source:     logic.SourceLiquidity,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   strength,
		Volume:     row.Volume,
		Spread:     spread,
		Elapsed:    elapsed,
		Category:   category,
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   surprise,
		ObservedAt: at,
		Market:     *row,
	}, nil
}

func (signal *Signal) categoryIndex(category logic.CategoryType) int {
	switch category {
	case logic.CategoryExtremeScarcity:
		return 1
	case logic.CategoryMedianDepth:
		return 2
	case logic.CategoryRobustLiquidity:
		return 3
	default:
		return 0
	}
}

func (signal *Signal) bestEffort(at time.Time, reason string) logic.Measurement {
	return logic.Measurement{
		Source:     logic.SourceLiquidity,
		Symbol:     signal.symbol,
		ObservedAt: at,
		BestEffort: true,
		GapReason:  reason,
	}
}
