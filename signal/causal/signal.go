package causal

import (
	"errors"
	"fmt"
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
Causal is the engine’s "rationalist," moving beyond simple
correlations to identify the true structural drivers of
price using Judea Pearl’s "ladder of causation".

1. What it measures exactly (in isolation)

The Causal signal measures the structural relationship between
Macro Momentum, Liquidity, Local Flow, and Price Velocity. It
uses a Directed Acyclic Graph (DAG) to determine if a price move
is an independent event or just a symptom of broader market drift.

It isolates the following causal rungs and metrics:

Rung 1: Association: Measures simple observational correlation
($P(velocity | flow)$).

Rung 2: Intervention: Uses backdoor adjustment to calculate the
effect of "doing" a trade ($P(velocity | do(flow))$) while controlling
for macro and liquidity.

Rung 3: Counterfactual Uplift: Determines what the price move would
have been if the order flow were different than observed.

Structural Regimes: It dynamically switches roles based on market
health. In Normal conditions, "Flow" is the driver; in Panic conditions
(detected via cross-asset Contagion or collinearity), "Liquidity"
itself becomes the driver.

---

2. Semantically, what story does it tell?

The Causal signal tells the story of responsibility and origin.

The "Local vs. Global" Story: It asks: "Is this asset moving because
it's special right now, or because everything is moving?". It filters
out "Macro Drift" to find genuine local alpha.

The "Weaponized Liquidity" Story: It identifies a specific type of
market terror where makers pull quotes so aggressively that the sudden
void itself drives price, while trades merely lag into it.

1. Endogenous Alpha (The Leader)

The price is being driven by local, internal buying or selling pressure.
Indicators: High Counterfactual Uplift within the Normal (Flow) regime.
Semantic Meaning: The move is "authentic." The local order flow is the
primary cause of price velocity, independent of the rest of the market.

2. Systemic Beta (The Drifter)

The price is moving, but it has no internal driver; it is simply following the tide.
Indicators: High Association (Rung 1) but near-zero Intervention Effect (Rung 2).
Semantic Meaning: The asset is just a passenger. The "cause" is Macro Momentum,
and there is no unique structural reason to favor this specific symbol over the index.

3. Liquidity Shock (The Panic)

The internal mechanics have inverted; the absence of liquidity
is now the dominant force.
Indicators: Panic Regime roles active, triggered by a Contagion
spike toward 1.0 or an exploding Condition Number.
Semantic Meaning: The market is "hollow." Makers have pulled back, and the resulting void is sucking price in. This is a high-risk state where trade flow is a lagging indicator.

4. Causal Noise (The Equilibrium)

No single force—local or macro—has a clear grip on price movement.
Indicators: Low confidence across all causal rungs and high residuals
in the Non-Linear Model.
Semantic Meaning: The market is in a state of stochastic equilibrium.
Neither buyers, sellers, nor the broader macro environment are providing
a statistically significant "push."

# Summary of Causal Categories

| Category         | Active Regime | Dominant Factor       | Market "Feel"      |
|:-----------------|:--------------|:----------------------|:-------------------|
| Endogenous Alpha | Normal        | Counterfactual Uplift | Driven/Independent |
| Systemic Beta    | Normal        | Macro Momentum        | Drifting/Passive   |
| Liquidity Shock  | Panic         | Liquidity Void        | Fragile/Inverted   |
| Causal Noise     | Variable      | None                  | Stochastic/Unclear |

By combining this with the Fluid (mechanical health) and Hawkes (thermal excitation)
signals, the engine can distinguish between a move that is **excited and healthy
(Hawkes Frenzy + Fluid Laminar)** but **causally empty (Systemic Beta)**, versus
a move that is **structurally significant (Endogenous Alpha).**
*/
/*
Signal implements Judea Pearl's ladder of causation over live microstructure feeds.
See the struct comment block for category semantics.
*/
type Signal struct {
	symbol          string
	entity          *logic.Entity
	measurements    *signalsupport.SampleRing
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
	alpha := signalsupport.BoundedClassifierAlpha()

	return &Signal{
		symbol:          symbol,
		entity:          entity,
		system:          system,
		measurements:    signalsupport.NewSampleRing(capacity),
		warmupRemaining: capacity,
		transition:      probability.NewTransitionMatrix(5, alpha),
		weights: learning.DefaultClassifierWeights(
			signalsupport.BoundedAdaptiveSurpriseThreshold(logic.SourceCausal),
		),
		tuner: learning.NewFeedbackTuner(),
	}
}

func (signal *Signal) RefreshSurpriseThreshold() {
	signalsupport.RefreshClassifierWeights(logic.SourceCausal, &signal.weights)
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

	return signal.fromSymbol(state, at)
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

	return signal.fromSymbol(state, at)
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

	return signal.fromSymbol(state, at)
}

func (signal *Signal) fromSymbol(state *CausalSymbol, now time.Time) (logic.Measurement, error) {
	crossSection.Observe(signal.symbol, state.ChangePct())

	macroMomentum := crossSection.MacroMomentum(signal.symbol)
	contagion := signal.system.contagion(now)

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
	reading.Volume = state.volumeWindow.Sum()

	if reading.Volume <= 0 {
		reading.Volume = state.dailyQuoteVol
	}

	reading.Spread = state.spreadPrice()
	reading.Market = signal.symbol

	if reading.Spread <= 0 || reading.Volume <= 0 {
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
	probabilities, err := signalsupport.ClassifierProbabilities(scores)

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

	signal.measurements.Record(raw)

	return warmed
}

func (signal *Signal) WarmupFilled() int {
	return signal.measurements.Capacity() - signal.warmupRemaining
}
