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
	signalsupport "github.com/theapemachine/symm/signal"
)

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
		measurement, err := signal.measureTrade(at)
		return signalsupport.FinishMeasure(
			logic.SourceFluid,
			signal.symbol,
			logic.CategoryLaminar,
			4,
			signal.measurements,
			at,
			measurement,
			err,
		)
	case logic.EntityTick:
		measurement, err := signal.measureTick(at)
		return signalsupport.FinishMeasure(
			logic.SourceFluid,
			signal.symbol,
			logic.CategoryLaminar,
			4,
			signal.measurements,
			at,
			measurement,
			err,
		)
	case logic.EntityBook:
		measurement, err := signal.measureBook(at)
		return signalsupport.FinishMeasure(
			logic.SourceFluid,
			signal.symbol,
			logic.CategoryLaminar,
			4,
			signal.measurements,
			at,
			measurement,
			err,
		)
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

	probabilities, err := numeric.SoftmaxScores([]float64{
		laminarScore,
		turbulentScore,
		inertialScore,
		viscousScore,
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

	state := signal.system.loadSymbol(reading.symbol)

	if state == nil {
		return logic.Measurement{}, fmt.Errorf("fluid: symbol state is missing")
	}

	if state.changePct == 0 {
		return logic.Measurement{}, nil
	}

	row, err := krakenmarket.NewSymbolRow(
		reading.symbol,
		reading.price,
		state.changePct,
		state.volume,
		1,
		at,
	)

	if err != nil {
		return logic.Measurement{}, nil
	}

	elapsed, err := signalsupport.ObservationElapsed(signal.measurements, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	if reading.spreadBPS <= 0 {
		return logic.Measurement{}, nil
	}

	if state.volume <= 0 {
		return logic.Measurement{}, nil
	}

	return logic.Measurement{
		Source:     logic.SourceFluid,
		Symbol:     reading.symbol,
		Price:      reading.price,
		Strength:   reading.reynolds,
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
	turbulentFloor := reading.dynamics.turbulentReynoldsFloor(reynolds)
	divergenceEdge := reading.dynamics.laminarDivergenceEdge()

	if divergenceEdge <= 0 && divergence > 0 {
		divergenceEdge = divergence
	}

	laminarScore := 0.0

	if reynolds < laminarCeiling && divergenceEdge > 0 && divergence < divergenceEdge {
		laminarScore = viscosity * (1 - divergence/divergenceEdge)
	}

	turbulentScore := 0.0

	if turbulentFloor > 0 && reynolds >= turbulentFloor {
		turbulentScore = reynolds / turbulentFloor
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

		eventAt = event.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		if err := state.FeedTrade(eventAt, event.Price, event.Qty, event.Side); err != nil {
			errnie.Error(err)
		}
	case *krakenmarket.TickerUpdate:
		if event == nil {
			break
		}

		eventAt = event.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		if err := state.FeedTicker(*event, eventAt); err != nil {
			errnie.Error(err)
		}
	case *krakenmarket.BookUpdate:
		if event == nil {
			break
		}

		eventAt = event.Timestamp

		if eventAt.IsZero() {
			eventAt = time.Now()
		}

		if err := state.FeedBook(*event, eventAt); err != nil {
			errnie.Error(err)
		}
	}

	if err := signal.system.publishFieldSnapshot(eventAt); errnie.Error(err) != nil {
		errnie.Error(err)
	}

	return warmed
}

func (signal *Signal) WarmupFilled() int {
	return signal.measurements.Len() - signal.warmupRemaining
}
