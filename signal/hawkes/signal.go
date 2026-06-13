package hawkes

import (
	"container/ring"

	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/probability"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
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

	threshold := signalsupport.BoundedAdaptiveSurpriseThreshold(logic.SourceHawkes)
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
	signalsupport.RefreshClassifierWeights(logic.SourceHawkes, &signal.weights)
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
			fmt.Errorf("hawkes: unsupported entity %s", signal.entity.Type),
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

func (signal *Signal) measureTick(_ time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, nil
}

func (signal *Signal) measureBook(_ time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, nil
}

func (signal *Signal) publish(
	reading hawkesReading,
	trades []krakenmarket.TradeUpdate,
	at time.Time,
) (logic.Measurement, error) {
	if !hawkesDecisionEligible(reading) {
		return logic.Measurement{}, nil
	}

	scores := []float64{
		reading.frenzy,
		reading.saturation,
		reading.organic,
		reading.exhaustion,
	}
	probabilities, err := signalsupport.ClassifierProbabilities(scores)

	if err != nil {
		return logic.Measurement{}, err
	}

	categoryIndex := logic.CategoryIndexFor(logic.SourceHawkes, reading.category)

	if categoryIndex == logic.CategoryNoneIndex {
		return logic.Measurement{}, nil
	}

	surprise, err := signalsupport.TransitionNovelty(signal.transition, probabilities, 0)

	if err != nil {
		return logic.Measurement{}, err
	}

	signal.transition.Update(categoryIndex)

	confidence, err := signalsupport.ShareConfidence(scores, categoryIndex)

	if err != nil {
		return logic.Measurement{}, err
	}

	lastTrade := trades[len(trades)-1]
	prices := make([]float64, len(trades))

	for index, trade := range trades {
		prices[index] = trade.Price
	}

	_, change, ok := signalsupport.ResolvedChange(prices)

	if !ok {
		return logic.Measurement{}, nil
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
		Source:          logic.SourceHawkes,
		Symbol:          signal.symbol,
		Price:           lastTrade.Price,
		Strength:        reading.strength,
		Volume:          quoteVol,
		Spread:          spread,
		Elapsed:         elapsed,
		Category:        reading.category,
		Regime:          logic.RegimeTypeNone,
		Position:        logic.PositionTypeNone,
		Confidence:      confidence,
		Surprise:        surprise,
		NoveltySurprise: surprise,
		ObservedAt:      at,
		Market:          *row,
	}, nil
}

func hawkesDecisionEligible(reading hawkesReading) bool {
	if reading.eventCount < 4 {
		return false
	}

	switch reading.category {
	case logic.CategoryFrenzy, logic.CategorySaturation:
		if reading.branchingRatio >= 1 {
			return false
		}

		if reading.stationarityMargin <= 0 {
			return false
		}

		return reading.poissonImprovement > 0
	default:
		return true
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
