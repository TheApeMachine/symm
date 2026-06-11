package exhaust

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
	floatring "github.com/theapemachine/symm/ring"
	signalsupport "github.com/theapemachine/symm/signal"
)

/*
Signal classifies microstructure decay modes that advise when to close a position.

Book Thinning   : Trend of bid or ask depth — hollow moves as defensive walls disappear.
Pressure Fade   : Decay in smoothed trade-pressure EMA — aggressive flow running dry.
Spread Widening : Current spread vs its rolling median — mechanical friction rising.
Imbalance Flip  : Book weight crossing from support to resistance (or the reverse).

The "Party is Over" Story : Fresh liquidity stopped arriving; the trend is running on fumes.
The "Trap" Story          : Price still drifts while the bid wall thins — reversal risk is building.

| Category              | Primary Metric  | Urgency  | Market "Feel"                 |
|-----------------------|-----------------|----------|-------------------------------|
| Mechanical Collapse   | Book Thinning   | High     | Crumbling Walls / Flash-Risk  |
| Thermal Exhaustion    | Pressure Fade   | Moderate | Dying Momentum / Topping Out  |
| Fragile Expansion     | Spread Widen    | Moderate | Increasing Friction / Risky   |
| Active Reversal       | Imbalance Flip  | High     | Sentiment Flip / Counter-Move |
*/
type Signal struct {
	symbol          string
	entity          *logic.Entity
	measurements    *ring.Ring
	warmupRemaining int
	transition      *numeric.TransitionMatrix
	weights         numeric.ClassifierWeights
	tuner           *numeric.FeedbackTuner
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
) *Signal {
	capacity := viper.GetInt("signals.exhaust.measurements_capacity")

	if capacity <= 0 {
		capacity = 64
	}

	threshold := math.Min(
		math.Max(viper.GetFloat64("signals.exhaust.surprise_threshold"), 1.0),
		5.0,
	)
	alpha := math.Min(
		math.Max(viper.GetFloat64("signals.exhaust.alpha"), 0.1),
		1.0,
	)

	return &Signal{
		symbol:          symbol,
		entity:          entity,
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
			fmt.Errorf("exhaust: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	trade, ok := signal.latest().(*krakenmarket.TradeUpdate)

	if !ok {
		return logic.Measurement{}, fmt.Errorf("exhaust: not ready")
	}

	exhaustSection.observeTrade(signal.symbol, trade)

	return signal.fromFeatures(at)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	ticker, ok := signal.latest().(*krakenmarket.TickerUpdate)

	if !ok {
		return logic.Measurement{}, fmt.Errorf("exhaust: not ready")
	}

	exhaustSection.observeTick(signal.symbol, ticker)

	return signal.fromFeatures(at)
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	book, ok := signal.latest().(*krakenmarket.BookUpdate)

	if !ok {
		return logic.Measurement{}, fmt.Errorf("exhaust: not ready")
	}

	exhaustSection.observeBook(signal.symbol, book)

	return signal.fromFeatures(at)
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

func (signal *Signal) fromFeatures(at time.Time) (logic.Measurement, error) {
	history, ok := exhaustSection.snapshot(signal.symbol)

	if !ok {
		return logic.Measurement{}, fmt.Errorf("exhaust: not ready")
	}

	longUrgency, longCategory, longScores, longErr := signal.exitScore(history, 1)

	if longErr != nil {
		return logic.Measurement{}, longErr
	}

	shortUrgency, shortCategory, shortScores, shortErr := signal.exitScore(history, -1)

	if shortErr != nil {
		return logic.Measurement{}, shortErr
	}

	urgency := longUrgency
	category := longCategory
	scores := longScores

	if shortUrgency > urgency {
		urgency = shortUrgency
		category = shortCategory
		scores = shortScores
	}

	if urgency <= 0 || category == logic.CategoryTypeNone {
		return logic.Measurement{}, fmt.Errorf("exhaust: not ready")
	}

	probabilities, err := numeric.SoftmaxScores(scores)

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

	row, elapsed, volume, spread, err := signalsupport.RingMarketRow(signal.symbol, signal.measurements, at)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	return logic.Measurement{
		Source:     logic.SourceExhaustion,
		Symbol:     signal.symbol,
		Price:      history.lastPrice,
		Strength:   urgency,
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

func (signal *Signal) exitScore(
	history featureState,
	side int,
) (urgency float64, category logic.CategoryType, scores []float64, err error) {
	thinning := 0.0
	fade := 0.0
	flip := 0.0

	if side > 0 {
		thinning = signal.depthTrend(history.bidDepths)
		fade = signal.pressureFade(history.pressures, 1)
		flip = signal.imbalanceFlip(history.imbalances, 1)
	}

	if side < 0 {
		thinning = signal.depthTrend(history.askDepths)
		fade = signal.pressureFade(history.pressures, -1)
		flip = signal.imbalanceFlip(history.imbalances, -1)
	}

	widen := signal.spreadWiden(history.spreads)
	collapse := signal.depthTrend(history.densities)

	margins := []float64{
		signal.componentMargin(thinning),
		signal.componentMargin(widen),
		signal.componentMargin(fade),
		signal.componentMargin(flip),
		signal.componentMargin(collapse),
	}

	fusionWeights, err := numeric.SoftmaxScores(margins)

	if err != nil {
		return 0, logic.CategoryTypeNone, nil, err
	}

	for index, weight := range fusionWeights {
		urgency += weight * margins[index]
	}

	category = signal.classify(thinning, widen, fade, flip)
	scores = margins[:4]

	return urgency, category, scores, nil
}

func (signal *Signal) depthTrend(depths floatring.FloatRing) float64 {
	if depths.Len() < 4 {
		return 0
	}

	ordered := depths.Ordered()
	recent := numeric.Mean(ordered[len(ordered)-3:])
	prior := numeric.Mean(ordered[:len(ordered)-3])

	if prior <= 0 {
		return 0
	}

	return (prior - recent) / prior
}

func (signal *Signal) spreadWiden(spreads floatring.FloatRing) float64 {
	if spreads.Len() < 4 {
		return 0
	}

	ordered := spreads.Ordered()
	sorted := numeric.CopySorted(ordered)
	median := numeric.PercentileSorted(sorted, 0.5)
	current := ordered[len(ordered)-1]

	if median <= 0 || current <= median {
		return 0
	}

	return (current - median) / median
}

func (signal *Signal) pressureFade(pressures floatring.FloatRing, side int) float64 {
	if pressures.Len() < 3 {
		return 0
	}

	ordered := pressures.Ordered()
	recent := ordered[len(ordered)-1]
	priorPeak := numeric.Max(ordered[:len(ordered)-1])

	if side > 0 {
		if priorPeak <= 0 {
			return 0
		}

		if recent >= priorPeak {
			return 0
		}

		return (priorPeak - recent) / math.Max(math.Abs(priorPeak), 1e-9)
	}

	if priorPeak >= 0 {
		return 0
	}

	if recent <= priorPeak {
		return 0
	}

	return (recent - priorPeak) / math.Max(math.Abs(priorPeak), 1e-9)
}

func (signal *Signal) imbalanceFlip(imbalances floatring.FloatRing, side int) float64 {
	if imbalances.Len() < 2 {
		return 0
	}

	ordered := imbalances.Ordered()
	recent := ordered[len(ordered)-1]
	prior := numeric.Mean(ordered[:len(ordered)-1])

	if side > 0 && prior > 0 && recent < 0 {
		return signal.componentMargin(math.Abs(recent) / math.Max(prior, 1e-9))
	}

	if side < 0 && prior < 0 && recent > 0 {
		return signal.componentMargin(recent / math.Max(math.Abs(prior), 1e-9))
	}

	return 0
}

func (signal *Signal) componentMargin(value float64) float64 {
	if value <= 0 {
		return 0
	}

	return value / (1 + value)
}

func (signal *Signal) classify(thinning, widen, fade, flip float64) logic.CategoryType {
	best := thinning
	category := logic.CategoryMechanicalCollapse

	if widen > best {
		best = widen
		category = logic.CategoryFragileExpansion
	}

	if fade > best {
		best = fade
		category = logic.CategoryThermalExhaustion
	}

	if flip > best {
		category = logic.CategoryActiveReversal
	}

	if best <= 0 {
		return logic.CategoryTypeNone
	}

	return category
}

func (signal *Signal) categoryIndex(category logic.CategoryType) int {
	switch category {
	case logic.CategoryMechanicalCollapse:
		return 1
	case logic.CategoryFragileExpansion:
		return 2
	case logic.CategoryThermalExhaustion:
		return 3
	case logic.CategoryActiveReversal:
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
