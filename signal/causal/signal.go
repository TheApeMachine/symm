package causal

import (
	"container/ring"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/probability"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
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
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		system:          system,
		transition:      probability.NewTransitionMatrix(5, viper.GetFloat64("signals.causal.alpha")),
		weights:         learning.DefaultClassifierWeights(viper.GetFloat64("signals.causal.surprise_threshold")),
		tuner:           learning.NewFeedbackTuner(),
	}
}

func (signal *Signal) Symbol() string {
	return signal.symbol
}

func (signal *Signal) Measure(
	feedback *market.Feedback, at time.Time,
) (logic.Measurement, error) {
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
			errnie.Err(
				errnie.IO,
				"causal: unsupported entity",
				fmt.Errorf("causal: unsupported entity %s", signal.entity.Type),
			),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	if !signalsupport.HasRecordedSamples(signal.measurements) {
		return logic.Measurement{}, nil
	}

	if !signal.system.shouldPublish(at) {
		return logic.Measurement{}, nil
	}

	state, err := signal.system.loadSymbol(signal.symbol)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	var feedErr error

	signal.measurements.Do(func(item any) {
		if feedErr != nil {
			return
		}

		if item == nil {
			return
		}

		trade, ok := item.(*krakenmarket.TradeUpdate)

		if !ok {
			feedErr = fmt.Errorf("causal: expected trade update, got %T", item)
			return
		}

		feedErr = state.FeedTrade(*trade)
	})

	if feedErr != nil {
		return logic.Measurement{}, errnie.Error(feedErr)
	}

	return signal.fromSymbol(at)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	if !signalsupport.HasRecordedSamples(signal.measurements) {
		return logic.Measurement{}, nil
	}

	state, err := signal.system.loadSymbol(signal.symbol)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	var feedErr error

	signal.measurements.Do(func(item any) {
		if feedErr != nil {
			return
		}

		if item == nil {
			return
		}

		ticker, ok := item.(*krakenmarket.TickerUpdate)

		if !ok {
			feedErr = fmt.Errorf("causal: expected ticker update, got %T", item)
			return
		}

		state.FeedTicker(*ticker)
	})

	if feedErr != nil {
		return logic.Measurement{}, errnie.Error(feedErr)
	}

	return signal.fromSymbol(at)
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	if !signalsupport.HasRecordedSamples(signal.measurements) {
		return logic.Measurement{}, nil
	}

	state, err := signal.system.loadSymbol(signal.symbol)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	var feedErr error
	accepted := false

	signal.measurements.Do(func(item any) {
		if feedErr != nil {
			return
		}

		if item == nil {
			return
		}

		book, ok := item.(*krakenmarket.BookUpdate)

		if !ok {
			feedErr = fmt.Errorf("causal: expected book update, got %T", item)
			return
		}

		bookErr := state.FeedBook(*book)

		if errors.Is(bookErr, errBookTouchNotReady) {
			return
		}

		if bookErr != nil {
			feedErr = bookErr
			return
		}

		accepted = true
	})

	if feedErr != nil {
		return logic.Measurement{}, errnie.Error(feedErr)
	}

	if !accepted {
		return logic.Measurement{}, nil
	}

	return signal.fromSymbol(at)
}

func (signal *Signal) fromSymbol(now time.Time) (logic.Measurement, error) {
	state, err := signal.system.loadSymbol(signal.symbol)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	macroMomentum := crossSection.MacroMomentum(signal.symbol)
	contagion := signal.system.contagion()

	reading, err := state.Measure(macroMomentum, contagion, now)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if reading.Category == logic.CategoryTypeNone || reading.Strength <= 0 {
		return logic.Measurement{}, nil
	}

	reading.Symbol = signal.symbol
	reading.ObservedAt = now

	elapsed, err := signalsupport.ObservationElapsed(signal.measurements, now)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	reading.Elapsed = elapsed

	row, err := state.symbolRow(signal.symbol, macroMomentum, now)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
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

	scores := []float64{
		alphaScore,
		shockScore,
		betaScore,
		noiseScore,
	}
	probabilities, err := probability.SoftmaxScores(scores)

	if err != nil {
		return logic.Measurement{}, err
	}

	categoryIndex := signal.categoryIndex(reading.Category)

	surpriseVector := signal.transition.PadObserved(probabilities, 0)
	surprise, err := signal.transition.Surprise(surpriseVector)

	if err != nil {
		return logic.Measurement{}, err
	}

	signal.transition.Update(categoryIndex)

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
		Confidence: reading.Confidence,
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
