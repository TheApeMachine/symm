package hawkes

import (
	"container/ring"

	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
)

/*
Signal detects trade-cluster excitation via a bivariate self-exciting Hawkes
model from the executed trade tape.

| Category   | Spectral Radius   | Asymmetry    | Market "Feel"          |
|:-----------|:------------------|:-------------|:-----------------------|
| Frenzy     | Moderate          | High         | Aggressive/Directional |
| Saturation | High (→ 1.0)      | Low/Moderate | Contested/Unstable     |
| Organic    | Low               | Low          | Healthy/Quiet          |
| Exhaustion | Very Low          | Low          | Stalled/Dying          |
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
	capacity int,
	system *System,
	threshold float64,
	alpha float64,
) *Signal {
	return &Signal{
		symbol:          symbol,
		entity:          entity,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		system:          system,
		transition:      numeric.NewTransitionMatrix(5, alpha),
		weights:         numeric.DefaultClassifierWeights(threshold),
		tuner:           numeric.NewFeedbackTuner(),
	}
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
			fmt.Errorf("hawkes: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	var (
		trades []krakenmarket.TradeUpdate
		err    error
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		trade, ok := item.(*krakenmarket.TradeUpdate)

		if !ok {
			err = fmt.Errorf("hawkes: expected trade update")
			return
		}

		trades = append(trades, *trade)
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if len(trades) == 0 {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	state := signal.system.loadSymbol(signal.symbol)
	reading, ok := state.Measure(trades, at)

	if !ok {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	return signal.publish(reading)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	return logic.Measurement{Symbol: signal.symbol}, nil
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	return logic.Measurement{Symbol: signal.symbol}, nil
}

func (signal *Signal) publish(reading hawkesReading) (logic.Measurement, error) {
	probabilities := numeric.SoftmaxScores([]float64{
		reading.frenzy,
		reading.saturation,
		reading.organic,
		reading.exhaustion,
	})

	categoryIndex := signal.categoryIndex(reading.category)

	surpriseVector := signal.transition.PadObserved(probabilities, 1e-6)
	surprise := signal.transition.Surprise(surpriseVector)

	signal.transition.Update(categoryIndex)

	confidence := reading.confidence

	if categoryIndex > 0 && categoryIndex-1 < len(probabilities) {
		confidence = probabilities[categoryIndex-1]
	}

	price := 0.0

	if latest := signal.measurements.Value; latest != nil {
		if trade, ok := latest.(*krakenmarket.TradeUpdate); ok {
			price = trade.Price
		}
	}

	return logic.Measurement{
		Source:     logic.SourceHawkes,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   reading.strength,
		Volume:     0,
		Spread:     0,
		Elapsed:    0,
		Category:   reading.category,
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   surprise,
	}, nil
}

func (signal *Signal) categoryIndex(category logic.CategoryType) int {
	switch category {
	case logic.CategoryFrenzy:
		return 1
	case logic.CategorySaturation:
		return 2
	case logic.CategoryOrganic:
		return 3
	case logic.CategoryExhaustion:
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

	return warmed
}

func (signal *Signal) WarmupFilled() int {
	return signal.measurements.Len() - signal.warmupRemaining
}
