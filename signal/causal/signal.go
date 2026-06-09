package causal

import (
	"container/ring"
	
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
)

/*
Signal scores Pearl's ladder — association, intervention (backdoor-adjusted),
counterfactual uplift — over a DAG of MacroMomentum → PriceVelocity ← LocalFlow
with Liquidity as a backdoor control, switching to a panic regime when
cross-asset contagion or collinearity spikes.

| Category         | Active Regime | Dominant Factor       | Market "Feel"      |
|:-----------------|:--------------|:----------------------|:-------------------|
| Endogenous Alpha | Normal        | Counterfactual Uplift | Driven/Independent |
| Systemic Beta    | Normal        | Macro Momentum        | Drifting/Passive   |
| Liquidity Shock  | Panic         | Liquidity Void        | Fragile/Inverted   |
| Causal Noise     | Variable      | None                  | Stochastic/Unclear |
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
			fmt.Errorf("causal: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade() (logic.Measurement, error) {
	if !signal.system.shouldPublish(time.Now()) {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	return signal.fromSymbol(time.Now())
}

func (signal *Signal) measureTick() (logic.Measurement, error) {
	return logic.Measurement{Symbol: signal.symbol}, nil
}

func (signal *Signal) measureBook() (logic.Measurement, error) {
	return signal.fromSymbol(time.Now())
}

func (signal *Signal) fromSymbol(now time.Time) (logic.Measurement, error) {
	state := signal.system.loadSymbol(signal.symbol)

	if state == nil {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	macroMomentum := signal.system.crossSection.macroMomentum(signal.symbol)
	contagion := signal.system.contagion()

	reading, err := state.Measure(macroMomentum, contagion, now)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if reading.Category == logic.CategoryTypeNone || reading.Strength <= 0 {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	reading.Symbol = signal.symbol

	return signal.publish(reading)
}

func (signal *Signal) publish(reading logic.Measurement) (logic.Measurement, error) {
	alphaScore := 0.0
	shockScore := 0.0
	betaScore := 0.0
	noiseScore := 0.0

	switch reading.Category {
	case logic.CategoryEndogenousAlpha:
		alphaScore = reading.Confidence
	case logic.CategoryLiquidityShock:
		shockScore = reading.Confidence
	case logic.CategorySystemicBeta:
		betaScore = reading.Confidence
	case logic.CategoryCausalNoise:
		noiseScore = reading.Confidence
	}

	if alphaScore <= 0 && shockScore <= 0 && betaScore <= 0 && noiseScore <= 0 {
		score := magnitudeMargin(reading.Strength)

		if score > 0 {
			alphaScore = score
		}
	}

	probabilities := numeric.SoftmaxScores([]float64{
		alphaScore,
		shockScore,
		betaScore,
		noiseScore,
	})

	categoryIndex := signal.categoryIndex(reading.Category)

	surpriseVector := signal.transition.PadObserved(probabilities, 1e-6)
	surprise := signal.transition.Surprise(surpriseVector)

	signal.transition.Update(categoryIndex)

	confidence := reading.Confidence

	if categoryIndex > 0 && categoryIndex-1 < len(probabilities) {
		confidence = math.Max(confidence, probabilities[categoryIndex-1])
	}

	return logic.Measurement{
		Source:     logic.SourceCausal,
		Symbol:     reading.Symbol,
		Price:      reading.Price,
		Strength:   reading.Strength,
		Volume:     0,
		Spread:     0,
		Elapsed:    0,
		Category:   reading.Category,
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   surprise,
	}, nil
}

func (signal *Signal) categoryIndex(category logic.CategoryType) int {
	switch category {
	case logic.CategoryEndogenousAlpha:
		return 1
	case logic.CategoryLiquidityShock:
		return 2
	case logic.CategorySystemicBeta:
		return 3
	case logic.CategoryCausalNoise:
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

