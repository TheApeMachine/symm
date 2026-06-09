package fluid

import (
	"container/ring"
	
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
)

/*
Signal applies order-book fluid dynamics per symbol from book, trades, and ticks.

Reynolds Number : Maximum absolute field activity across divergence, vorticity, turbulence.
Divergence      : Trusted touch imbalance excluding toxic resting size.
Vorticity       : Aggressor pressure amplified by volume-clocked churn when flux is clean.
Viscosity       : Inverse spread — tight markets resist displacement.

| Category  | Visc (Spread) | Dominant Metric            | Market "Feel"     |
|:----------|:--------------|:---------------------------|:------------------|
| Laminar   | High (Tight)  | None (Low Activity)        | Smooth/Consistent |
| Turbulent | Variable      | Turbulence / Vorticity     | Shattered/Fragile |
| Inertial  | Moderate      | Reynolds / Divergence      | Direct/Heavy      |
| Viscous   | Low (Wide)    | Divergence (at walls)      | Resistant/Grinding|
*/
type Signal struct {
	symbol       string
	entity       *logic.Entity
	measurements    *ring.Ring
	warmupRemaining int
	system       *System
	transition   *numeric.TransitionMatrix
	weights      numeric.ClassifierWeights
	tuner        *numeric.FeedbackTuner
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
		symbol:       symbol,
		entity:       entity,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		system:       system,
		transition:   numeric.NewTransitionMatrix(5, alpha),
		weights:      numeric.DefaultClassifierWeights(threshold),
		tuner:        numeric.NewFeedbackTuner(),
	}
}

func (signal *Signal) Measure(feedback *market.Feedback) (logic.Measurement, error) {
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
		return signal.measureTrade()
	case logic.EntityTick:
		return signal.measureTick()
	case logic.EntityBook:
		return signal.measureBook()
	default:
		return logic.Measurement{}, errnie.Error(
			fmt.Errorf("fluid: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade() (logic.Measurement, error) {
	return logic.Measurement{Symbol: signal.symbol}, nil
}

func (signal *Signal) measureTick() (logic.Measurement, error) {
	return signal.MeasureFromSymbol()
}

func (signal *Signal) measureBook() (logic.Measurement, error) {
	return signal.MeasureFromSymbol()
}

func (signal *Signal) MeasureFromSymbol() (logic.Measurement, error) {
	state := signal.system.loadSymbol(signal.symbol)

	if state == nil {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	reading, ok := state.Reading()

	if !ok {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	return signal.publish(reading)
}

func (signal *Signal) publish(reading fluidReading) (logic.Measurement, error) {
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
	activity := math.Max(
		math.Abs(reading.divergence),
		math.Max(reading.turbulence, math.Max(reading.vorticity, reading.reynolds)),
	)

	laminarScore := 0.0

	if activity <= fluidDefaultBandEdges[0] {
		laminarScore = reading.viscosity * (1 - activity)
	}

	turbulentScore := math.Max(reading.turbulence, reading.vorticity)
	inertialScore := math.Max(reading.reynolds, math.Abs(reading.divergence))
	viscousScore := 0.0

	if reading.viscosity < 0.5 {
		viscousScore = (1 - reading.viscosity) * math.Abs(reading.divergence)
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

	if best <= 0 && reading.price > 0 {
		category = logic.CategoryLaminar
		laminarScore = reading.viscosity
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

