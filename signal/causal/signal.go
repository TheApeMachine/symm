package causal

import (
	"container/ring"
	"fmt"
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
Signal scores Pearl's ladder — association, intervention (backdoor-adjusted),
counterfactual uplift — over a DAG of MacroMomentum → PriceVelocity ← LocalFlow
with Liquidity as a backdoor control, switching to a panic regime when
cross-asset contagion or collinearity spikes.
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
	capacity := viper.GetInt("signals.causal.measurements_capacity")

	return &Signal{
		symbol:          symbol,
		entity:          entity,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		system:          system,
		transition:      numeric.NewTransitionMatrix(5, viper.GetFloat64("signals.causal.alpha")),
		weights:         numeric.DefaultClassifierWeights(viper.GetFloat64("signals.causal.surprise_threshold")),
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
			logic.SourceCausal,
			signal.symbol,
			logic.CategoryCausalNoise,
			4,
			signal.measurements,
			at,
			measurement,
			err,
		)
	case logic.EntityTick:
		measurement, err := signal.measureTick(at)
		return signalsupport.FinishMeasure(
			logic.SourceCausal,
			signal.symbol,
			logic.CategoryCausalNoise,
			4,
			signal.measurements,
			at,
			measurement,
			err,
		)
	case logic.EntityBook:
		measurement, err := signal.measureBook(at)
		return signalsupport.FinishMeasure(
			logic.SourceCausal,
			signal.symbol,
			logic.CategoryCausalNoise,
			4,
			signal.measurements,
			at,
			measurement,
			err,
		)
	default:
		return logic.Measurement{}, errnie.Error(
			fmt.Errorf("causal: unsupported entity %s", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	if !signal.system.shouldPublish(at) {
		return logic.Measurement{}, nil
	}

	state := signal.system.loadSymbol(signal.symbol)

	signal.measurements.Do(func(item any) {
		trade, ok := item.(*krakenmarket.TradeUpdate)
		if !ok {
			return
		}

		errnie.Error(state.FeedTrade(*trade))
	})

	return signal.fromSymbol(at)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	state := signal.system.loadSymbol(signal.symbol)

	signal.measurements.Do(func(item any) {
		ticker, ok := item.(*krakenmarket.TickerUpdate)

		if !ok {
			return
		}

		state.FeedTicker(*ticker)
	})

	return signal.fromSymbol(at)
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	state := signal.system.loadSymbol(signal.symbol)

	signal.measurements.Do(func(item any) {
		book, ok := item.(*krakenmarket.BookUpdate)
		if !ok {
			return
		}

		state.FeedBook(*book)
	})

	return signal.fromSymbol(at)
}

func (signal *Signal) fromSymbol(now time.Time) (logic.Measurement, error) {
	state := signal.system.loadSymbol(signal.symbol)

	macroMomentum := crossSection.MacroMomentum(signal.symbol)
	contagion := signal.system.contagion()

	reading, err := state.Measure(macroMomentum, contagion, now)

	if err != nil {
		return logic.Measurement{}, nil
	}

	if reading.Category == logic.CategoryTypeNone || reading.Strength <= 0 {
		return logic.Measurement{}, nil
	}

	reading.Symbol = signal.symbol
	reading.ObservedAt = now

	elapsed, err := signalsupport.ObservationElapsed(signal.measurements, now)

	if err != nil {
		return logic.Measurement{}, nil
	}

	reading.Elapsed = elapsed

	row, err := state.symbolRow(signal.symbol, macroMomentum, now)

	if err != nil {
		return logic.Measurement{}, nil
	}

	reading.Market = *row
	reading.Volume = row.Volume
	reading.Spread = state.spreadPrice()

	if reading.Spread <= 0 {
		return logic.Measurement{}, nil
	}

	return signal.publish(reading, now)
}

func (signal *Signal) publish(reading logic.Measurement, at time.Time) (logic.Measurement, error) {
	if reading.Symbol == "" ||
		reading.Price <= 0 ||
		reading.Strength <= 0 ||
		reading.Volume <= 0 ||
		reading.Spread <= 0 ||
		reading.Elapsed <= 0 ||
		reading.Confidence <= 0 {
		return logic.Measurement{}, nil
	}

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

	probabilities, err := numeric.SoftmaxScores([]float64{
		alphaScore,
		shockScore,
		betaScore,
		noiseScore,
	})

	if err != nil {
		return logic.Measurement{}, err
	}

	categoryIndex := signal.categoryIndex(reading.Category)

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

	return logic.Measurement{
		Source:     logic.SourceCausal,
		Symbol:     reading.Symbol,
		Price:      reading.Price,
		Strength:   reading.Strength,
		Volume:     reading.Volume,
		Spread:     reading.Spread,
		Elapsed:    reading.Elapsed,
		Category:   reading.Category,
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   surprise,
		ObservedAt: at,
		Market:     reading.Market,
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
