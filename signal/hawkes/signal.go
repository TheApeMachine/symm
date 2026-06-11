package hawkes

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
	signalsupport "github.com/theapemachine/symm/signal"
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
	system *System,
) *Signal {
	capacity := viper.GetInt("signals.hawkes.measurements_capacity")

	if capacity <= 0 {
		capacity = 64
	}

	threshold := math.Min(
		math.Max(viper.GetFloat64("signals.hawkes.surprise_threshold"), 1.0),
		5.0,
	)
	alpha := math.Min(
		math.Max(viper.GetFloat64("signals.hawkes.alpha"), 0.1),
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
		return logic.Measurement{}, nil
	}

	state := signal.system.loadSymbol(signal.symbol)
	reading, ok := state.Measure(trades, at)

	if !ok {
		return logic.Measurement{}, nil
	}

	return signal.publish(reading, trades, at)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, nil
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, nil
}

func (signal *Signal) publish(
	reading hawkesReading,
	trades []krakenmarket.TradeUpdate,
	at time.Time,
) (logic.Measurement, error) {
	probabilities, err := numeric.SoftmaxScores([]float64{
		reading.frenzy,
		reading.saturation,
		reading.organic,
		reading.exhaustion,
	})

	if err != nil {
		return logic.Measurement{}, err
	}

	categoryIndex := signal.categoryIndex(reading.category)

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

	lastTrade := trades[len(trades)-1]
	_, change := numeric.AnchorChange(trades[0].Price, lastTrade.Price)

	if change == 0 {
		return logic.Measurement{}, nil
	}

	prices := make([]float64, len(trades))

	for index, trade := range trades {
		prices[index] = trade.Price
	}

	spread, err := signalsupport.TouchSpread(prices)

	if err != nil {
		return logic.Measurement{}, nil
	}

	quoteVol := 0.0

	for _, trade := range trades {
		quoteVol += trade.Price * trade.Qty
	}

	row, err := lastTrade.CompleteSymbol(change, 1, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	elapsed, err := signalsupport.ObservationElapsed(signal.measurements, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	return logic.Measurement{
		Source:     logic.SourceHawkes,
		Symbol:     signal.symbol,
		Price:      lastTrade.Price,
		Strength:   reading.strength,
		Volume:     quoteVol,
		Spread:     spread,
		Elapsed:    elapsed,
		Category:   reading.category,
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
