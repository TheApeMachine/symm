package correlation

import (
	"container/ring"
	"fmt"
	"math"

	"time"

	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
)

/*
Signal measures how each symbol's return stream correlates with the cross-section median.

Rolling Correlation : Pearson correlation between a symbol's log-return window and the peer median return series.
Return Energy       : Median absolute log return — separates idle noise from symbols that are actually moving.
Peer Quartiles      : Correlation and energy ranks are scored against the live universe, not fixed constants.

The "Herd" Story      : A symbol moving in lockstep with the market — beta exposure, not alpha.
The "Lone Wolf" Story : High movement with low correlation — idiosyncratic flow the crowd is not sharing.

| Category          | Correlation | Energy  | Market "Feel"           |
|-------------------|-------------|---------|-------------------------|
| Systemic Herd     | High +      | High    | Beta / Crowded Move     |
| Decoupled Alpha   | Low         | High    | Idiosyncratic / Alpha   |
| Stochastic Noise  | Any         | Low     | Idle / Choppy           |
| Divergent Stress  | High -      | High    | Counter-Herd / Stress   |
*/
type Signal struct {
	symbol          string
	entity          *logic.Entity
	measurements    *ring.Ring
	warmupRemaining int
	crossSection    *crossSection
	transition      *numeric.TransitionMatrix
	weights         numeric.ClassifierWeights
	tuner           *numeric.FeedbackTuner
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
	capacity int,
	crossSection *crossSection,
	threshold float64,
	alpha float64,
) *Signal {
	return &Signal{
		symbol:          symbol,
		entity:          entity,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		crossSection:    crossSection,
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
			fmt.Errorf("correlation: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	var (
		price float64
		err   error
		seen  bool
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		trade, ok := item.(*krakenmarket.TradeUpdate)

		if !ok {
			err = fmt.Errorf("correlation: expected trade update")
			return
		}

		price = trade.Price
		seen = true
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if !seen || price <= 0 {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	return signal.fromCrossSection(price, at)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	var (
		ticker *krakenmarket.TickerUpdate
		err    error
		seen   bool
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		update, ok := item.(*krakenmarket.TickerUpdate)

		if !ok {
			err = fmt.Errorf("correlation: expected ticker update")
			return
		}

		ticker = update
		seen = true
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if !seen || ticker == nil {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	price := ticker.Last

	if price <= 0 {
		price = (ticker.Ask + ticker.Bid) / 2
	}

	if price <= 0 {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	return signal.fromCrossSection(price, at)
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	return logic.Measurement{Symbol: signal.symbol}, nil
}

func (signal *Signal) fromCrossSection(price float64, at time.Time) (logic.Measurement, error) {
	signal.crossSection.publishPrice(signal.symbol, price, at)

	window := signal.crossSection.minBarsRequired()
	symbolReturns := signal.crossSection.symbolReturns(signal.symbol, window)
	marketReturns := signal.crossSection.marketReturns(window, at)

	if len(symbolReturns) < window || len(marketReturns) < window {
		return logic.Measurement{Symbol: signal.symbol, ObservedAt: at}, nil
	}

	peerCorrelations := signal.crossSection.peerCorrelations(window, at)
	peerEnergies := signal.crossSection.peerEnergies(window, at)

	if len(peerCorrelations) < 2 || len(peerEnergies) < 2 {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	correlation := numeric.Pearson(symbolReturns, marketReturns)
	energy := numeric.MedianAbsolute(symbolReturns)

	category := signal.classify(correlation, energy, peerCorrelations, peerEnergies)

	herdScore := 0.0

	if category == logic.CategorySystemicHerd {
		herdScore = correlation * energy
	}

	alphaScore := 0.0

	if category == logic.CategoryDecoupledAlpha {
		alphaScore = energy * (1 - math.Abs(correlation))
	}

	noiseScore := 0.0

	if category == logic.CategoryStochasticNoise {
		noiseScore = 1 - energy
	}

	stressScore := 0.0

	if category == logic.CategoryDivergentStress {
		stressScore = math.Abs(correlation) * energy
	}

	probabilities := numeric.SoftmaxScores([]float64{
		herdScore,
		alphaScore,
		noiseScore,
		stressScore,
	})

	categoryIndex := signal.categoryIndex(category)

	surpriseVector := signal.transition.PadObserved(probabilities, 1e-6)
	surprise := signal.transition.Surprise(surpriseVector)

	signal.transition.Update(categoryIndex)

	confidence := 0.0

	if categoryIndex > 0 && categoryIndex-1 < len(probabilities) {
		confidence = probabilities[categoryIndex-1]
	}

	strength := math.Abs(correlation)

	if category == logic.CategoryDecoupledAlpha {
		strength = alphaScore
	}

	return logic.Measurement{
		Source:     logic.SourceCorrelation,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   strength,
		Volume:     0,
		Spread:     0,
		Elapsed:    0,
		Category:   category,
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   surprise,
	}, nil
}

func (signal *Signal) classify(
	correlation, energy float64,
	peerCorrelations, peerEnergies []float64,
) logic.CategoryType {
	sortedCorrelations := numeric.CopySorted(peerCorrelations)
	sortedEnergies := numeric.CopySorted(peerEnergies)

	upperCorrelation := numeric.PercentileSorted(sortedCorrelations, 0.75)
	lowerCorrelation := numeric.PercentileSorted(sortedCorrelations, 0.25)
	upperEnergy := numeric.PercentileSorted(sortedEnergies, 0.75)
	lowerEnergy := numeric.PercentileSorted(sortedEnergies, 0.25)
	medianEnergy := numeric.Median(peerEnergies)

	energySpread := upperEnergy - lowerEnergy
	lowEnergy := false

	if energySpread > 0 {
		lowEnergy = energy <= lowerEnergy
	}

	if !lowEnergy && medianEnergy > 0 {
		lowEnergy = energy <= medianEnergy/4
	}

	if lowEnergy {
		return logic.CategoryStochasticNoise
	}

	correlationSpread := upperCorrelation - lowerCorrelation
	highPositiveCorrelation := correlation >= upperCorrelation

	if correlationSpread <= 0 {
		highPositiveCorrelation = correlation > 0
	}

	lowMagnitudeCorrelation := math.Abs(correlation) <= lowerCorrelation

	if correlationSpread <= 0 {
		lowMagnitudeCorrelation = math.Abs(correlation) < 0.5
	}

	highEnergy := energy >= upperEnergy

	if energySpread <= 0 {
		highEnergy = energy >= medianEnergy
	}

	if correlation < 0 && highEnergy && math.Abs(correlation) >= math.Abs(lowerCorrelation) {
		return logic.CategoryDivergentStress
	}

	if highPositiveCorrelation && highEnergy {
		return logic.CategorySystemicHerd
	}

	if lowMagnitudeCorrelation && highEnergy {
		return logic.CategoryDecoupledAlpha
	}

	return logic.CategoryStochasticNoise
}

func (signal *Signal) categoryIndex(category logic.CategoryType) int {
	switch category {
	case logic.CategorySystemicHerd:
		return 1
	case logic.CategoryDecoupledAlpha:
		return 2
	case logic.CategoryStochasticNoise:
		return 3
	case logic.CategoryDivergentStress:
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
