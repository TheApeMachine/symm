package manifold

import (
	"container/ring"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/learning"
	mkernel "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/nomagique/probability"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	signalsupport "github.com/theapemachine/symm/signal"
)

/*
Signal classifies the 3D manifold state for one symbol.

PressureGradNorm captures cross-axis basis and beta dislocations.
CoherenceMag2 captures systemic herding / superfluid collapse.
GuidanceSpeed is the pilot-wave trend velocity from aligned Ψ.
ViscosityProxy inverts divergence — laminar when large, turbulent when small.
*/
type Signal struct {
	symbol          string
	entity          *logic.Entity
	measurements    *ring.Ring
	warmupRemaining int
	system          *System
	transition      *probability.TransitionMatrix
	weights         learning.ClassifierWeights
	tuner           *learning.FeedbackTuner
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
	system *System,
) *Signal {
	capacity := market.MustSignalMeasurementCapacity()

	threshold := signalsupport.BoundedAdaptiveSurpriseThreshold(logic.SourceManifold)
	alpha := signalsupport.BoundedClassifierAlpha()

	return &Signal{
		symbol:          symbol,
		entity:          entity,
		system:          system,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		transition:      probability.NewTransitionMatrix(5, alpha),
		weights:         learning.DefaultClassifierWeights(threshold),
		tuner:           learning.NewFeedbackTuner(),
	}
}

func (signal *Signal) RefreshSurpriseThreshold() {
	signalsupport.RefreshClassifierWeights(logic.SourceManifold, &signal.weights)
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

	return signal.measureFromField(at)
}

func (signal *Signal) measureFromField(at time.Time) (logic.Measurement, error) {
	reading, price, observedAt, ok := signal.system.field.Reading(signal.symbol)

	if !ok {
		return logic.Measurement{}, nil
	}

	if observedAt.IsZero() {
		observedAt = at
	}

	return signal.publish(reading, price, observedAt)
}

func (signal *Signal) publish(reading mkernel.Reading, price float64, at time.Time) (logic.Measurement, error) {
	if !reading.IsFinite() {
		return logic.Measurement{}, nil
	}

	category, herdScore, shockScore, driftScore, noiseScore := signal.classify(reading)

	scores := []float64{herdScore, shockScore, driftScore, noiseScore}

	if reading.CoherenceMag2 <= 0 ||
		reading.GuidanceSpeed <= 0 ||
		reading.ViscosityProxy <= 0 {
		return logic.Measurement{}, nil
	}

	probabilities, err := signalsupport.ClassifierProbabilities(scores)

	if err != nil {
		return logic.Measurement{}, err
	}

	categoryIndex := signal.categoryIndex(category)

	surpriseVector := signal.transition.PadObserved(probabilities, 0)
	surprise, err := signal.transition.Surprise(surpriseVector)

	if err != nil {
		return logic.Measurement{}, err
	}

	signal.transition.Update(categoryIndex)

	confidence, err := probability.CategoryShareConfidence(scores, categoryIndex)

	if err != nil {
		return logic.Measurement{}, err
	}

	strength := shockScore

	switch category {
	case logic.CategorySystemicHerd:
		strength = herdScore
	case logic.CategorySynchronizedDrift:
		strength = driftScore
	case logic.CategoryStochasticNoise:
		strength = noiseScore
	case logic.CategoryLiquidityShock:
		strength = shockScore
	}

	if strength <= 0 || math.IsNaN(strength) || math.IsInf(strength, 0) {
		return logic.Measurement{}, nil
	}

	row, elapsed, volume, spread, err := signalsupport.RingMarketRow(signal.symbol, signal.measurements, at)

	if err != nil || row == nil {
		return logic.Measurement{}, nil
	}

	if reading.ViscosityProxy > spread {
		spread = reading.ViscosityProxy
	}

	return logic.Measurement{
		Source:     logic.SourceManifold,
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

func (signal *Signal) classify(
	reading mkernel.Reading,
) (
	logic.CategoryType,
	float64,
	float64,
	float64,
	float64,
) {
	herdScore := reading.CoherenceMag2 * reading.GuidanceSpeed
	shockScore := reading.PressureGradNorm
	driftScore := reading.GuidanceSpeed * (1 / math.Max(reading.ViscosityProxy, 1e-9))
	noiseScore := reading.ViscosityProxy * (1 - reading.CoherenceMag2)

	best := noiseScore
	category := logic.CategoryStochasticNoise

	if herdScore > best && reading.CoherenceMag2 > 0 {
		best = herdScore
		category = logic.CategorySystemicHerd
	}

	if shockScore > best {
		best = shockScore
		category = logic.CategoryLiquidityShock
	}

	if driftScore > best {
		best = driftScore
		category = logic.CategorySynchronizedDrift
	}

	return category, herdScore, shockScore, driftScore, noiseScore
}

func (signal *Signal) categoryIndex(category logic.CategoryType) int {
	switch category {
	case logic.CategorySystemicHerd:
		return 1
	case logic.CategoryLiquidityShock:
		return 2
	case logic.CategorySynchronizedDrift:
		return 3
	case logic.CategoryStochasticNoise:
		return 4
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

	if signal.system == nil || signal.system.field == nil {
		return warmed
	}

	eventAt := time.Now()

	switch event := raw.(type) {
	case *krakenmarket.TradeUpdate:
		if event == nil {
			break
		}

		eventAt = event.Timestamp

		if eventAt.IsZero() {
			errnie.Debug(fmt.Sprintf(
				"manifold: trade %q missing timestamp, using synthetic time",
				event.Symbol,
			))
			eventAt = time.Now()
		}

		if feedErr := signal.system.field.enqueueTrade(event, eventAt); feedErr != nil {
			errnie.Error(manifoldFeedError(feedErr))
		}
	case *krakenmarket.TickerUpdate:
		if event == nil {
			break
		}

		eventAt = event.Timestamp

		if eventAt.IsZero() {
			errnie.Debug(fmt.Sprintf(
				"manifold: ticker %q missing timestamp, using synthetic time",
				event.Symbol,
			))
			eventAt = time.Now()
		}

		if feedErr := signal.system.field.enqueueTicker(*event, eventAt); feedErr != nil {
			errnie.Error(manifoldFeedError(feedErr))
		}
	case *krakenmarket.BookUpdate:
		if event == nil {
			break
		}

		eventAt = event.Timestamp

		if eventAt.IsZero() {
			errnie.Debug(fmt.Sprintf(
				"manifold: book %q missing timestamp, using synthetic time",
				event.Symbol,
			))
			eventAt = time.Now()
		}

		if feedErr := signal.system.field.enqueueBook(*event, eventAt); feedErr != nil {
			errnie.Error(manifoldFeedError(feedErr))
		}
	}

	return warmed
}

func (signal *Signal) WarmupFilled() int {
	return signal.measurements.Len() - signal.warmupRemaining
}

func manifoldFeedError(err error) error {
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), "non-finite") {
		return nil
	}

	return errnie.Error(err)
}
