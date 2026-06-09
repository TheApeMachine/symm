package toxicity

import (
	"container/ring"
	
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
)

const vacuumStrengthCap = 100.0

/*
Signal classifies book-quality into toxicity perspective categories.

Cancel-to-Fill Asymmetry : EMA of cancelled versus filled liquidity per side.
Toxic Level Detection    : Large young near-touch blocks that cancel rather than fill.
Directional BookFlow     : Which side is retreating under cancel/fill imbalance.

The "Bluffing" Story : Makers fake-bid near the touch; walls crumble on contact.
The "Vacuum" Story   : One side pulls away aggressively and price gets sucked through the void.
*/
type Signal struct {
	symbol       string
	entity       *logic.Entity
	measurements    *ring.Ring
	warmupRemaining int
	tracker      *Tracker
	transition   *numeric.TransitionMatrix
	weights      numeric.ClassifierWeights
	tuner        *numeric.FeedbackTuner
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
	capacity int,
	tracker *Tracker,
	threshold float64,
	alpha float64,
) *Signal {
	return &Signal{
		symbol:       symbol,
		entity:       entity,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		tracker:      tracker,
		transition:   numeric.NewTransitionMatrix(4, alpha),
		weights:      numeric.DefaultClassifierWeights(threshold),
		tuner:        numeric.NewFeedbackTuner(),
	}
}

func (signal *Signal) Measure(feedback *market.Feedback) (logic.Measurement, error) {
	if feedback != nil {
		_, err := signal.tuner.Apply(
			signal.symbol,
			feedback.Symbol,
			feedback.Samples,
			feedback.MSE,
			feedback.Scale,
			feedback.Bias,
			&signal.weights,
		)

		if err != nil {
			return logic.Measurement{}, errnie.Error(err)
		}
	}

	switch signal.entity.Type {
	case logic.EntityTrade:
		return signal.measureTrade()
	case logic.EntityTick:
		return signal.measureTick()
	case logic.EntityBook:
		return signal.measureBook()
	default:
		return logic.Measurement{}, errnie.Error(
			fmt.Errorf("toxicity: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade() (logic.Measurement, error) {
	return signal.fromQuality(time.Now())
}

func (signal *Signal) measureTick() (logic.Measurement, error) {
	return logic.Measurement{Symbol: signal.symbol}, nil
}

func (signal *Signal) measureBook() (logic.Measurement, error) {
	return signal.fromQuality(time.Now())
}

func (signal *Signal) fromQuality(at time.Time) (logic.Measurement, error) {
	snapshot, lastPrice, ok := signal.tracker.Snapshot(signal.symbol, at)

	if !ok || lastPrice <= 0 {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	category, strength, bluffScore, vacuumScore, supportScore := signal.classify(snapshot)

	if category == logic.CategoryTypeNone || strength <= 0 {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	if math.IsNaN(strength) || math.IsInf(strength, 0) {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	evidence := math.Max(bluffScore, math.Max(vacuumScore, supportScore))

	if evidence <= 0 {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	return signal.publish(category, lastPrice, strength, bluffScore, vacuumScore, supportScore)
}

func (signal *Signal) classify(
	snapshot bookQualitySnapshot,
) (
	logic.CategoryType,
	float64,
	float64,
	float64,
	float64,
) {
	if snapshot.toxicNear {
		bluffScore := toxicBluffEvidence(snapshot.toxicBluffStrength)

		return logic.CategoryToxicBluff, snapshot.toxicBluffStrength, bluffScore, 0, 0
	}

	bidRatio := cancelFillRatio(snapshot.cancelBid, snapshot.fillBid)
	askRatio := cancelFillRatio(snapshot.cancelAsk, snapshot.fillAsk)
	maxRatio := math.Max(bidRatio, askRatio)

	if snapshot.bidDepth > 0 && snapshot.askDepth > 0 && maxRatio == 0 {
		depthBalance := math.Min(snapshot.bidDepth, snapshot.askDepth) /
			math.Max(snapshot.bidDepth, snapshot.askDepth)
		supportScore := magnitudeMargin(depthBalance)

		return logic.CategoryHardSupport, depthBalance, 0, 0, supportScore
	}

	threshold := signal.tracker.fillToCancelThreshold()

	if threshold <= 0 {
		return logic.CategoryTypeNone, 0, 0, 0, 0
	}

	bidVacuum := bidRatio >= threshold && snapshot.fillBid > 0
	askVacuum := askRatio >= threshold && snapshot.fillAsk > 0

	if bidVacuum || askVacuum {
		margin := maxRatio - threshold
		vacuumScore := competitionMargin(margin, threshold)
		strength := math.Min(maxRatio/threshold, vacuumStrengthCap)

		return logic.CategoryLiquidityVacuum, strength, 0, vacuumScore, 0
	}

	if bidRatio > 0 && askRatio > 0 &&
		bidRatio < threshold/2 && askRatio < threshold/2 {
		half := threshold / 2
		margin := half - maxRatio
		supportScore := competitionMargin(margin, half)
		strength := supportScore

		return logic.CategoryHardSupport, strength, 0, 0, supportScore
	}

	return logic.CategoryTypeNone, 0, 0, 0, 0
}

func toxicBluffEvidence(churnRatio float64) float64 {
	if churnRatio <= 0 {
		return 0
	}

	if churnRatio <= flashChurnRatioThreshold {
		return magnitudeMargin(churnRatio)
	}

	margin := churnRatio - flashChurnRatioThreshold
	span := 1 - flashChurnRatioThreshold

	return competitionMargin(margin, span)
}

func (signal *Signal) publish(
	category logic.CategoryType,
	price, strength float64,
	bluffScore, vacuumScore, supportScore float64,
) (logic.Measurement, error) {
	probabilities := numeric.SoftmaxScores([]float64{
		bluffScore,
		vacuumScore,
		supportScore,
	})

	categoryIndex := signal.categoryIndex(category)

	surpriseVector := signal.transition.PadObserved(probabilities, 1e-6)
	surprise := signal.transition.Surprise(surpriseVector)

	signal.transition.Update(categoryIndex)

	confidence := 0.0

	if categoryIndex > 0 && categoryIndex-1 < len(probabilities) {
		confidence = probabilities[categoryIndex-1]
	}

	if category == logic.CategoryToxicBluff {
		confidence = math.Max(confidence, magnitudeMargin(strength))
	}

	return logic.Measurement{
		Source:     logic.SourceToxicity,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   strength,
		Volume:     0,
		Spread:     0,
		Elapsed:    0,
		Category:   category,
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   surprise,
	}, nil
}

func (signal *Signal) categoryIndex(category logic.CategoryType) int {
	switch category {
	case logic.CategoryToxicBluff:
		return 1
	case logic.CategoryLiquidityVacuum:
		return 2
	case logic.CategoryHardSupport:
		return 3
	default:
		return 0
	}
}

func (signal *Signal) Record(raw any) bool {
	warmed := false

	if signal.warmupRemaining > 0 {
		signal.warmupRemaining--
		warmed = true
	}

	signal.measurements.Value = raw
	signal.measurements = signal.measurements.Next()

	return warmed
}

func (signal *Signal) WarmupFilled() int {
	return signal.measurements.Len() - signal.warmupRemaining
}

