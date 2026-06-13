package fluid

import (
	"container/ring"
	"errors"
	"fmt"
	"math"
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
Signal applies order-book fluid dynamics per symbol from book, trades, and ticks.

Reynolds classifies laminar versus turbulent flow. Divergence is ∇·(ρv) at the
touch. Viscosity is replenishment resistance after consumption.
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

	return &Signal{
		symbol:          symbol,
		entity:          entity,
		system:          system,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		transition:      probability.NewTransitionMatrix(5, signalsupport.BoundedClassifierAlpha()),
		weights: learning.DefaultClassifierWeights(
			signalsupport.BoundedAdaptiveSurpriseThreshold(logic.SourceFluid),
		),
		tuner: learning.NewFeedbackTuner(),
	}
}

func (signal *Signal) RefreshSurpriseThreshold() {
	signalsupport.RefreshClassifierWeights(logic.SourceFluid, &signal.weights)
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
			fmt.Errorf("fluid: unsupported entity %s", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	return signal.measureFromSymbol(at)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	return signal.measureFromSymbol(at)
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	return signal.measureFromSymbol(at)
}

func (signal *Signal) measureFromSymbol(at time.Time) (logic.Measurement, error) {
	state := signal.system.loadSymbol(signal.symbol)

	if state == nil {
		return logic.Measurement{}, nil
	}

	reading, ok := state.Reading()

	if !ok {
		return logic.Measurement{}, nil
	}

	return signal.publish(reading, at)
}

func (signal *Signal) publish(reading fluidReading, at time.Time) (logic.Measurement, error) {
	category, laminarScore, turbulentScore, inertialScore, viscousScore := signal.classify(reading)

	scores := []float64{
		laminarScore,
		turbulentScore,
		inertialScore,
		viscousScore,
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

	state := signal.system.loadSymbol(reading.symbol)

	if state == nil {
		return logic.Measurement{}, fmt.Errorf("fluid: symbol state is missing")
	}

	changePct := state.changePct

	if changePct <= 0 && reading.spreadBPS > 0 {
		changePct = reading.spreadBPS / 10000
	}

	if changePct <= 0 {
		return logic.Measurement{}, nil
	}

	if state.volume <= 0 {
		return logic.Measurement{}, nil
	}

	if reading.spreadBPS <= 0 {
		return logic.Measurement{}, nil
	}

	elapsed, err := signalsupport.ObservationElapsed(signal.measurements, at)

	if err != nil {
		if errors.Is(err, signalsupport.ErrNoTimestampedSamples) {
			return logic.Measurement{}, nil
		}

		return logic.Measurement{}, errnie.Error(err)
	}

	row, err := krakenmarket.NewSymbolRow(
		reading.symbol,
		reading.price,
		changePct,
		state.volume,
		1,
		at,
	)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	strength := reading.reynolds

	if strength <= 0 || math.IsNaN(strength) || math.IsInf(strength, 0) {
		strength = math.Max(
			laminarScore,
			math.Max(turbulentScore, math.Max(inertialScore, viscousScore)),
		)
	}

	if strength <= 0 || math.IsNaN(strength) || math.IsInf(strength, 0) {
		return logic.Measurement{}, nil
	}

	return logic.Measurement{
		Source:     logic.SourceFluid,
		Symbol:     reading.symbol,
		Price:      reading.price,
		Strength:   strength,
		Volume:     state.volume,
		Spread:     reading.spreadBPS,
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
	laminarCeiling := reading.dynamics.laminarReynoldsCeiling(reynolds)
	turbulentFloor, turbulentReady := reading.dynamics.turbulentReynoldsFloor()
	divergenceEdge := reading.dynamics.laminarDivergenceEdge()

	if divergenceEdge <= 0 && divergence > 0 {
		divergenceEdge = divergence
	}

	laminarScore := 0.0

	if reynolds < laminarCeiling && divergenceEdge > 0 && divergence < divergenceEdge {
		laminarScore = viscosity * (1 - divergence/divergenceEdge)
	}

	turbulentScore := 0.0

	// recordReynolds only stores positive values, so turbulentFloor is positive when ready.
	if turbulentReady && reynolds >= turbulentFloor {
		turbulentScore = reynolds / turbulentFloor
	}

	inertialScore := divergence

	viscousScore := 0.0

	if viscosity > 0 {
		viscousScore = divergence / viscosity
	}

	icebergScore := reading.dynamics.icebergScore(reading.midAddRate, reading.midExecuteRate)

	if icebergScore > 0 {
		viscousScore = math.Max(viscousScore, icebergScore)
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

	if best <= 0 && reading.price > 0 && reynolds < laminarCeiling {
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

	state := signal.system.loadSymbol(signal.symbol)

	if state == nil {
		return warmed
	}

	eventAt := time.Now()

	switch event := raw.(type) {
	case *krakenmarket.TradeUpdate:
		if event == nil {
			break
		}

		eventAt := event.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		tradeAt := eventAt
		tradePrice := event.Price
		tradeQty := event.Qty
		tradeSide := event.Side

		signal.system.enqueue(signal.symbol, func(symbolState *FluidSymbol) {
			if err := symbolState.FeedTrade(tradeAt, tradePrice, tradeQty, tradeSide); err != nil {
				errnie.Error(err)
			}
		})
		eventAt = tradeAt
	case *krakenmarket.TickerUpdate:
		if event == nil {
			break
		}

		eventAt := event.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		tickerUpdate := *event
		tickerAt := eventAt

		signal.system.enqueue(signal.symbol, func(symbolState *FluidSymbol) {
			if err := symbolState.FeedTicker(tickerUpdate, tickerAt); err != nil {
				errnie.Error(err)
			}
		})
		eventAt = tickerAt
	case *krakenmarket.BookUpdate:
		if event == nil {
			break
		}

		eventAt := event.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		bookUpdate := *event
		bookAt := eventAt

		signal.system.enqueue(signal.symbol, func(symbolState *FluidSymbol) {
			if err := symbolState.FeedBook(bookUpdate, bookAt); err != nil {
				errnie.Error(err)
			}
		})
		eventAt = bookAt
	}

	if err := signal.system.publishFieldSnapshot(eventAt); errnie.Error(err) != nil {
		errnie.Error(err)
	}

	return warmed
}

func (signal *Signal) WarmupFilled() int {
	return signal.measurements.Len() - signal.warmupRemaining
}
