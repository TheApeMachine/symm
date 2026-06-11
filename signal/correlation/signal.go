package correlation

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
	transition      *numeric.TransitionMatrix
	weights         numeric.ClassifierWeights
	tuner           *numeric.FeedbackTuner
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
) *Signal {
	capacity := viper.GetInt("signals.correlation.measurements_capacity")

	if capacity <= 0 {
		capacity = 64
	}

	threshold := math.Min(
		math.Max(viper.GetFloat64("signals.correlation.surprise_threshold"), 1.0),
		5.0,
	)
	alpha := math.Min(
		math.Max(viper.GetFloat64("signals.correlation.alpha"), 0.1),
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
		measurement, err := signal.measureTrade(at)
		return signalsupport.FinishMeasure(
			logic.SourceCorrelation,
			signal.symbol,
			logic.CategoryStochasticNoise,
			4,
			signal.measurements,
			at,
			measurement,
			err,
		)
	case logic.EntityTick:
		measurement, err := signal.measureTick(at)
		return signalsupport.FinishMeasure(
			logic.SourceCorrelation,
			signal.symbol,
			logic.CategoryStochasticNoise,
			4,
			signal.measurements,
			at,
			measurement,
			err,
		)
	case logic.EntityBook:
		measurement, err := signal.measureBook(at)
		return signalsupport.FinishMeasure(
			logic.SourceCorrelation,
			signal.symbol,
			logic.CategoryStochasticNoise,
			4,
			signal.measurements,
			at,
			measurement,
			err,
		)
	default:
		return logic.Measurement{}, errnie.Error(
			fmt.Errorf("correlation: unsupported entity %s", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	var (
		prices   []float64
		quoteVol float64
		err      error
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

		prices = append(prices, trade.Price)
		quoteVol += trade.Price * trade.Qty
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if len(prices) < 2 || quoteVol <= 0 {
		return logic.Measurement{}, nil
	}

	_, change := numeric.AnchorChange(prices[0], prices[len(prices)-1])

	if change == 0 {
		return logic.Measurement{}, nil
	}

	row, err := krakenmarket.SymbolRowFromPrices(signal.symbol, prices, quoteVol, 1, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	return signal.fromCrossSectionRow(row, at)
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
		return logic.Measurement{}, nil
	}

	return signal.fromCrossSection(ticker, at)
}

func (signal *Signal) measureBook(_ time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, nil
}

func (signal *Signal) fromCrossSection(ticker *krakenmarket.TickerUpdate, at time.Time) (logic.Measurement, error) {
	row, err := ticker.CompleteSymbol(1, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	return signal.fromCrossSectionRow(row, at)
}

func (signal *Signal) fromCrossSectionRow(row *krakenmarket.Symbol, at time.Time) (logic.Measurement, error) {
	if err := crossSection.Observe(row); err != nil {
		return logic.Measurement{}, err
	}

	price := row.Price

	window := crossSection.MinBarsRequired()
	symbolReturns := crossSection.SymbolReturns(signal.symbol, window)
	marketReturns := crossSection.MarketReturns(window, at)

	if len(symbolReturns) < window || len(marketReturns) < window {
		return logic.Measurement{}, nil
	}

	peerCorrelations := crossSection.PeerCorrelations(window, at)
	peerEnergies := crossSection.PeerEnergies(window, at)

	if len(peerCorrelations) < 2 || len(peerEnergies) < 2 {
		return logic.Measurement{}, nil
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

	probabilities, err := numeric.SoftmaxScores([]float64{
		herdScore,
		alphaScore,
		noiseScore,
		stressScore,
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

	strength := math.Abs(correlation)

	if category == logic.CategoryDecoupledAlpha {
		strength = alphaScore
	}

	if strength <= 0 {
		return logic.Measurement{}, nil
	}

	elapsed, err := signalsupport.ObservationElapsed(signal.measurements, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	spread := price * numeric.MedianAbsolute(symbolReturns)

	if spread <= 0 {
		return logic.Measurement{}, nil
	}

	return logic.Measurement{
		Source:     logic.SourceCorrelation,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   strength,
		Volume:     row.Volume,
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
	lowEnergy := energySpread > 0 && energy <= lowerEnergy

	if lowEnergy {
		return logic.CategoryStochasticNoise
	}

	correlationSpread := upperCorrelation - lowerCorrelation
	highPositiveCorrelation := correlation >= upperCorrelation

	if correlationSpread <= 0 {
		highPositiveCorrelation = correlation > 0
	}

	lowMagnitudeCorrelation := peerLowMagnitudeCorrelation(
		correlation,
		lowerCorrelation,
		correlationSpread,
		peerCorrelations,
	)

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
