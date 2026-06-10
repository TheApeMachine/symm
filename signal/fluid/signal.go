package fluid

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
)

var fluidDefaultBandEdges = []float64{0.2, 0.5, 1.5}

/*
Signal applies order-book fluid dynamics per symbol from book, trades, and ticks.

Reynolds classifies laminar versus turbulent flow. Divergence is ∇·v at the
touch. Viscosity is replenishment resistance after consumption.
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
	capacity := viper.GetInt("signals.fluid.measurements_capacity")

	return &Signal{
		symbol:          symbol,
		entity:          entity,
		system:          system,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		transition:      numeric.NewTransitionMatrix(5, viper.GetFloat64("signals.fluid.alpha")),
		weights:         numeric.DefaultClassifierWeights(viper.GetFloat64("signals.fluid.surprise_threshold")),
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
			fmt.Errorf("fluid: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	state := signal.system.loadSymbol(signal.symbol)
	trade, ok := signal.latest().(*krakenmarket.TradeUpdate)

	if !ok || trade == nil {
		return signal.measureFromSymbol(at)
	}

	eventAt := trade.Timestamp

	if eventAt.IsZero() {
		eventAt = at
	}

	if err := state.FeedTrade(eventAt, trade.Price, trade.Qty, trade.Side); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if err := signal.system.publishFieldSnapshot(eventAt); errnie.Error(err) != nil {
		return logic.Measurement{}, err
	}

	return signal.measureFromSymbol(at)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	state := signal.system.loadSymbol(signal.symbol)
	ticker, ok := signal.latest().(*krakenmarket.TickerUpdate)

	if !ok || ticker == nil {
		return signal.measureFromSymbol(at)
	}

	eventAt := ticker.Timestamp

	if eventAt.IsZero() {
		eventAt = at
	}

	if err := state.FeedTicker(*ticker, eventAt); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if err := signal.system.publishFieldSnapshot(eventAt); errnie.Error(err) != nil {
		return logic.Measurement{}, err
	}

	return signal.measureFromSymbol(at)
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	state := signal.system.loadSymbol(signal.symbol)
	book, ok := signal.latest().(*krakenmarket.BookUpdate)

	if !ok || book == nil {
		return signal.measureFromSymbol(at)
	}

	eventAt := book.Timestamp

	if eventAt.IsZero() {
		eventAt = at
	}

	if err := state.FeedBook(*book, eventAt); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if err := signal.system.publishFieldSnapshot(eventAt); errnie.Error(err) != nil {
		return logic.Measurement{}, err
	}

	return signal.measureFromSymbol(at)
}

func (signal *Signal) latest() any {
	if signal.measurements == nil {
		return nil
	}

	prev := signal.measurements.Prev()

	if prev == nil || prev.Value == nil {
		return nil
	}

	return prev.Value
}

func (signal *Signal) measureFromSymbol(at time.Time) (logic.Measurement, error) {
	state := signal.system.loadSymbol(signal.symbol)

	if state == nil {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	reading, ok := state.Reading()

	if !ok {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	return signal.publish(reading, at)
}

func (signal *Signal) publish(reading fluidReading, at time.Time) (logic.Measurement, error) {
	category, laminarScore, turbulentScore, inertialScore, viscousScore := signal.classify(reading)

	probabilities := numeric.SoftmaxScores([]float64{
		laminarScore,
		turbulentScore,
		inertialScore,
		viscousScore,
	})

	categoryIndex := signal.categoryIndex(category)

	surpriseVector := signal.transition.PadObserved(probabilities, 1e-6)
	surprise := signal.transition.Surprise(surpriseVector)

	signal.transition.Update(categoryIndex)

	confidence := 0.0

	if categoryIndex > 0 && categoryIndex-1 < len(probabilities) {
		confidence = probabilities[categoryIndex-1]
	}

	return logic.Measurement{
		Source:     logic.SourceFluid,
		Symbol:     reading.symbol,
		Price:      reading.price,
		Strength:   reading.reynolds,
		Volume:     0,
		Spread:     reading.spreadBPS,
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
	reading fluidReading,
) (
	logic.CategoryType,
	float64,
	float64,
	float64,
	float64,
) {
	reynolds := reading.reynolds
	divergence := math.Abs(reading.divergence)
	viscosity := reading.viscosity

	laminarScore := 0.0

	if reynolds < laminarReynoldsCeiling && divergence < fluidDefaultBandEdges[0] {
		laminarScore = viscosity * (1 - divergence/fluidDefaultBandEdges[0])
	}

	turbulentScore := 0.0

	if reynolds >= turbulentReynoldsFloor {
		turbulentScore = reynolds / turbulentReynoldsFloor
	}

	inertialScore := divergence

	viscousScore := 0.0

	if viscosity > 0 {
		viscousScore = divergence / viscosity
	}

	best := laminarScore
	category := logic.CategoryLaminar

	if turbulentScore > best {
		best = turbulentScore
		category = logic.CategoryTurbulent
	}

	if inertialScore > best {
		best = inertialScore
		category = logic.CategoryInertial
	}

	if viscousScore > best {
		category = logic.CategoryViscous
	}

	if best <= 0 && reading.price > 0 && reynolds < laminarReynoldsCeiling {
		category = logic.CategoryLaminar
		laminarScore = viscosity
	}

	return category, laminarScore, turbulentScore, inertialScore, viscousScore
}

func (signal *Signal) categoryIndex(category logic.CategoryType) int {
	switch category {
	case logic.CategoryLaminar:
		return 1
	case logic.CategoryTurbulent:
		return 2
	case logic.CategoryInertial:
		return 3
	case logic.CategoryViscous:
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
