package toxicity

import (
	"container/ring"

	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
	signalsupport "github.com/theapemachine/symm/signal"
)

/*
Signal classifies book-quality into toxicity perspective categories.

Cancel-to-Fill Asymmetry : EMA of cancelled versus filled liquidity per side.
Toxic Level Detection    : Large young near-touch blocks that cancel rather than fill.
Directional BookFlow     : Which side is retreating under cancel/fill imbalance.

The "Bluffing" Story : Makers fake-bid near the touch; walls crumble on contact.
The "Vacuum" Story   : One side pulls away aggressively and price gets sucked through the void.
*/
type Signal struct {
	symbol          string
	entity          *logic.Entity
	measurements    *ring.Ring
	warmupRemaining int
	tracker         *Tracker
	transition      *numeric.TransitionMatrix
	weights         numeric.ClassifierWeights
	tuner           *numeric.FeedbackTuner
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
) *Signal {
	capacity := viper.GetInt("signals.toxicity.measurements_capacity")

	if capacity <= 0 {
		capacity = 64
	}

	threshold := math.Min(
		math.Max(viper.GetFloat64("signals.toxicity.surprise_threshold"), 1.0),
		5.0,
	)
	alpha := math.Min(
		math.Max(viper.GetFloat64("signals.toxicity.alpha"), 0.1),
		1.0,
	)

	return &Signal{
		symbol:          symbol,
		entity:          entity,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		tracker:         Default(),
		transition:      numeric.NewTransitionMatrix(4, alpha),
		weights:         numeric.DefaultClassifierWeights(threshold),
		tuner:           numeric.NewFeedbackTuner(),
	}
}

func (signal *Signal) Symbol() string {
	return signal.symbol
}

func (signal *Signal) Measure(feedback *market.Feedback, at time.Time) (logic.Measurement, error) {
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
		return signal.measureTrade(at)
	case logic.EntityTick:
		return signal.measureTick(at)
	case logic.EntityBook:
		return signal.measureBook(at)
	default:
		return logic.Measurement{}, errnie.Error(
			fmt.Errorf("toxicity: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	return signal.fromQuality(at)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, fmt.Errorf("toxicity: not ready")
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	return signal.fromQuality(at)
}

func (signal *Signal) fromQuality(at time.Time) (logic.Measurement, error) {
	snapshot, lastPrice, ok := signal.tracker.Snapshot(signal.symbol, at)

	if !ok || lastPrice <= 0 {
		return logic.Measurement{}, fmt.Errorf("toxicity: not ready")
	}

	category, strength, bluffScore, vacuumScore, supportScore := signal.classify(snapshot)

	if category == logic.CategoryTypeNone || strength <= 0 {
		return logic.Measurement{}, fmt.Errorf("toxicity: not ready")
	}

	if math.IsNaN(strength) || math.IsInf(strength, 0) {
		return logic.Measurement{}, fmt.Errorf("toxicity: not ready")
	}

	evidence := math.Max(bluffScore, math.Max(vacuumScore, supportScore))

	if evidence <= 0 {
		return logic.Measurement{}, fmt.Errorf("toxicity: not ready")
	}

	return signal.publish(category, lastPrice, strength, bluffScore, vacuumScore, supportScore, at)
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
		churnGate := signal.tracker.churnRatioGate(signal.symbol)
		bluffScore := toxicBluffEvidence(snapshot.toxicBluffStrength, churnGate)

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
		strengthCap := signal.tracker.vacuumStrengthLimit(signal.symbol, threshold, maxRatio)
		strength := math.Min(maxRatio/threshold, strengthCap)

		return logic.CategoryLiquidityVacuum, strength, 0, vacuumScore, 0
	}

	supportGate := threshold * signal.tracker.supportRatioGate(signal.symbol, threshold)

	if supportGate <= 0 && bidRatio > 0 && askRatio > 0 {
		supportGate = math.Min(bidRatio, askRatio)
	}

	if bidRatio > 0 && askRatio > 0 &&
		bidRatio < supportGate && askRatio < supportGate {
		half := supportGate
		margin := half - maxRatio
		supportScore := competitionMargin(margin, half)
		strength := supportScore

		return logic.CategoryHardSupport, strength, 0, 0, supportScore
	}

	return logic.CategoryTypeNone, 0, 0, 0, 0
}

func toxicBluffEvidence(churnRatio float64, churnGate float64) float64 {
	if churnRatio <= 0 {
		return 0
	}

	if churnGate <= 0 {
		return magnitudeMargin(churnRatio)
	}

	if churnRatio <= churnGate {
		return magnitudeMargin(churnRatio)
	}

	return competitionMargin(churnRatio-churnGate, churnGate)
}

func (signal *Signal) publish(
	category logic.CategoryType,
	price, strength float64,
	bluffScore, vacuumScore, supportScore float64,
	at time.Time,
) (logic.Measurement, error) {
	probabilities, err := numeric.SoftmaxScores([]float64{
		bluffScore,
		vacuumScore,
		supportScore,
	})

	if err != nil {
		return logic.Measurement{}, err
	}

	categoryIndex := signal.categoryIndex(category)

	surpriseVector := signal.transition.PadObserved(probabilities, 1e-6)
	surprise, err := signal.transition.Surprise(surpriseVector)

	if err != nil {
		return logic.Measurement{}, err
	}

	signal.transition.Update(categoryIndex)

	confidence, err := numeric.CategoryConfidence(probabilities, categoryIndex)

	if err != nil {
		return logic.Measurement{}, err
	}

	if category == logic.CategoryToxicBluff {
		confidence = math.Max(confidence, magnitudeMargin(strength))
	}

	row, elapsed, volume, spread, err := signalsupport.RingMarketRow(signal.symbol, signal.measurements, at)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	return logic.Measurement{
		Source:     logic.SourceToxicity,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   strength,
		Volume:     volume,
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
