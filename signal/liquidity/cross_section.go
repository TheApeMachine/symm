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

	lower, upper := signal.quartiles(peers)
	peakScarcity := signal.isPeakScarcity(quoteVol, peers)
	median := float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(peers...)...))
	relativeVolume, baselineReady, baselineErr := signal.observeVolumeBaseline(
		quoteVol,
		at,
	)

	if baselineErr != nil {
		return logic.Measurement{}, errnie.Error(baselineErr)
	}

	if median <= 0 {
		return signal.bestEffort(
			at,
			fmt.Sprintf("liquidity: non-positive cross-section median %.4f", median),
		), nil
	}

	category := signal.classify(
		quoteVol,
		lower,
		upper,
		peakScarcity,
		signal.historicallyLiquid(relativeVolume, baselineReady),
	)

	scarcityScore := math.Max(0, (median-quoteVol)/median)
	depthScore := math.Max(0, (quoteVol-median)/median)
	peakScore := 0.0

	if peakScarcity {
		peakScore = 1
	}

	probabilities, err := probability.SoftmaxScores([]float64{
		scarcityScore,
		depthScore,
		peakScore,
	})

	if err != nil {
		return logic.Measurement{}, err
	}

	return signal.publishCrossSection(
		row,
		category,
		probabilities,
		scarcityScore,
		depthScore,
		spread,
		price,
		at,
	)
}

func (signal *Signal) publishCrossSection(
	row *krakenmarket.Symbol,
	category logic.CategoryType,
	probabilities []float64,
	scarcityScore float64,
	depthScore float64,
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

	confidence, err := probability.CategoryConfidence(probabilities, categoryIndex)

	if err != nil {
		return logic.Measurement{}, err
	}

	strength := scarcityScore

	if category == logic.CategoryRobustLiquidity {
		strength = depthScore
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
