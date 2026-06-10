package manifold

import (
	"container/ring"
	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/physics"
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
	transition      *numeric.TransitionMatrix
	weights         numeric.ClassifierWeights
	tuner           *numeric.FeedbackTuner
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
	system *System,
) *Signal {
	capacity := viper.GetInt("signals.manifold.measurements_capacity")

	if capacity <= 0 {
		capacity = 64
	}

	threshold := math.Min(
		math.Max(viper.GetFloat64("signals.manifold.surprise_threshold"), 1.0),
		5.0,
	)
	alpha := math.Min(
		math.Max(viper.GetFloat64("signals.manifold.alpha"), 0.1),
		1.0,
	)

	return &Signal{
		symbol:          symbol,
		entity:          entity,
		system:          system,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		transition:      numeric.NewTransitionMatrix(5, alpha),
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
		return signal.measureFromField(at)
	case logic.EntityTick:
		return signal.measureFromField(at)
	case logic.EntityBook:
		return signal.measureFromField(at)
	default:
		return logic.Measurement{}, errnie.Error(
			fmt.Errorf("manifold: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureFromField(at time.Time) (logic.Measurement, error) {
	item := signal.latest()

	if item == nil {
		return logic.Measurement{Symbol: signal.symbol, ObservedAt: at}, nil
	}

	switch signal.entity.Type {
	case logic.EntityTrade:
		trade, ok := item.(*krakenmarket.TradeUpdate)

		if !ok || trade == nil {
			break
		}

		eventAt := trade.Timestamp

		if eventAt.IsZero() {
			eventAt = at
		}

		if err := signal.system.field.FeedTrade(trade, eventAt); err != nil {
			return logic.Measurement{}, errnie.Error(err)
		}
	case logic.EntityTick:
		ticker, ok := item.(*krakenmarket.TickerUpdate)

		if !ok || ticker == nil {
			break
		}

		eventAt := ticker.Timestamp

		if eventAt.IsZero() {
			eventAt = at
		}

		if err := signal.system.field.FeedTicker(*ticker, eventAt); err != nil {
			return logic.Measurement{}, errnie.Error(err)
		}
	case logic.EntityBook:
		book, ok := item.(*krakenmarket.BookUpdate)

		if !ok || book == nil {
			break
		}

		eventAt := book.Timestamp

		if eventAt.IsZero() {
			eventAt = at
		}

		if err := signal.system.field.FeedBook(*book, eventAt); err != nil {
			return logic.Measurement{}, errnie.Error(err)
		}
	}

	reading, price, observedAt, ok := signal.system.field.Reading(signal.symbol)

	if !ok {
		return logic.Measurement{Symbol: signal.symbol, ObservedAt: at}, nil
	}

	return signal.publish(reading, price, observedAt)
}

func (signal *Signal) publish(reading physics.Reading, price float64, at time.Time) (logic.Measurement, error) {
	category, herdScore, shockScore, driftScore, noiseScore := signal.classify(reading)

	probabilities := numeric.SoftmaxScores([]float64{
		herdScore,
		shockScore,
		driftScore,
		noiseScore,
	})

	categoryIndex := signal.categoryIndex(category)

	surpriseVector := signal.transition.PadObserved(probabilities, 1e-6)
	surprise := signal.transition.Surprise(surpriseVector)

	signal.transition.Update(categoryIndex)

	confidence := 0.0

	if categoryIndex > 0 && categoryIndex-1 < len(probabilities) {
		confidence = probabilities[categoryIndex-1]
	}

	strength := reading.PressureGradNorm

	if category == logic.CategorySynchronizedDrift {
		strength = reading.GuidanceSpeed
	}

	if category == logic.CategorySystemicHerd {
		strength = reading.CoherenceMag2
	}

	return logic.Measurement{
		Source:     logic.SourceManifold,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   strength,
		Volume:     0,
		Spread:     reading.ViscosityProxy,
		Elapsed:    0,
		Category:   category,
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   surprise,
		ObservedAt: at,
	}, nil
}

func (signal *Signal) classify(
	reading physics.Reading,
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

func (signal *Signal) latest() any {
	var latest any

	signal.measurements.Do(func(item any) {
		if item != nil {
			latest = item
		}
	})

	return latest
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
