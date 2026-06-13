package sentiment

import (
	"container/ring"
	"fmt"
	"math"

	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/statistic"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	signalsupport "github.com/theapemachine/symm/signal"
)

/*
Signal measures global market conviction by looking at the behavior of the entire universe simultaneously.

Market Breadth         : The ratio of symbols with a positive $changePct$ versus the total number of symbols.
Leadership Performance : Tracks the median performance of the "top" symbols to see if the leaders are actually leading.

The "Rising Tide" Story : It tells you if an asset's move is a solo effort or if it is being carried by a global "risk-on" regime where every asset is moving in unison.
The "Conviction" Story  : It distinguishes between a "fake" leader move (where only one asset is up) and a high-conviction market environment (breadth $> 0.55$).

| Category           | Breadth | Leader Strength | Market "Feel"                |
|--------------------|---------|-----------------|------------------------------|
| Risk-On Surge      | High    | Strong          | Rising Tide / Global Buy     |
| Divergent Move     | Low     | Strong          | Idiosyncratic Alpha          |
| Systemic Slump     | Low     | Weak            | Global Risk-Off              |
*/
type Signal struct {
	symbol            string
	entity            *logic.Entity
	measurements      *ring.Ring
	warmupRemaining   int
	transition        *probability.TransitionMatrix
	weights           learning.ClassifierWeights
	tuner             *learning.FeedbackTuner
	baselineThreshold float64
	surgeBias         float64
	lastCategory      logic.CategoryType
	lastCategoryAt    time.Time
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
) *Signal {
	capacity := market.MustSignalMeasurementCapacity()

	alpha := signalsupport.BoundedClassifierAlpha()
	threshold := signalsupport.BoundedAdaptiveSurpriseThreshold(logic.SourceSentiment)

	return &Signal{
		symbol:            symbol,
		entity:            entity,
		measurements:      ring.New(capacity),
		warmupRemaining:   capacity,
		transition:        probability.NewTransitionMatrix(4, alpha),
		weights:           learning.DefaultClassifierWeights(threshold),
		tuner:             learning.NewFeedbackTuner(),
		baselineThreshold: threshold,
	}
}

func (signal *Signal) RefreshSurpriseThreshold() {
	if signal == nil {
		return
	}

	threshold := signalsupport.BoundedAdaptiveSurpriseThreshold(logic.SourceSentiment)
	signal.baselineThreshold = threshold
	signalsupport.RefreshClassifierWeights(logic.SourceSentiment, &signal.weights)
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

		signal.surgeBias = math.Min(
			math.Max(signal.weights.Threshold-signal.baselineThreshold, -0.25),
			0.25,
		)
	}

	switch signal.entity.Type {
	case logic.EntityTrade:
		return signal.measureTrade(at)
	case logic.EntityTick:
		return signal.measureTick(at)
	case logic.EntityBook:
		return signal.measureBook(at)
	default:
		return logic.Measurement{}, nil
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	if !signalsupport.HasRecordedSamples(signal.measurements) {
		return logic.Measurement{}, nil
	}

	var (
		prices  []float64
		volumes []float64
		err     error
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		trade, ok := item.(*krakenmarket.TradeUpdate)

		if !ok {
			err = fmt.Errorf("sentiment: expected trade update")
			return
		}

		prices = append(prices, trade.Price)
		volumes = append(volumes, trade.Qty)
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if len(prices) == 0 {
		return logic.Measurement{}, nil
	}

	move, change, ok := signalsupport.ResolvedChange(prices)

	if !ok {
		return logic.Measurement{}, nil
	}

	quoteVol := float64(statistic.NewSum().Observe(nomagique.Numbers(volumes...)...))

	if quoteVol <= 0 {
		return logic.Measurement{}, nil
	}

	row, err := krakenmarket.NewSymbolRow(
		signal.symbol,
		prices[len(prices)-1],
		change,
		quoteVol,
		1,
		at,
	)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	spread, spreadErr := signalsupport.TouchSpread(prices)

	if spreadErr != nil {
		return logic.Measurement{}, nil
	}

	return signal.fromCrossSection(row, quoteVol, spread, change, move, at)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	if !signalsupport.HasRecordedSamples(signal.measurements) {
		return logic.Measurement{}, nil
	}

	var (
		ticker  *krakenmarket.TickerUpdate
		err     error
		seen    bool
		spreads []float64
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		update, ok := item.(*krakenmarket.TickerUpdate)

		if !ok {
			err = fmt.Errorf("sentiment: expected ticker update")
			return
		}

		ticker = update
		seen = true
		spreads = append(spreads, update.Ask-update.Bid)
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if !seen || ticker == nil {
		return logic.Measurement{}, nil
	}

	spread := 0.0

	if len(spreads) > 0 {
		spread = spreads[len(spreads)-1]
	}

	row, err := ticker.CompleteSymbol(1, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	return signal.fromCrossSection(row, row.Volume, spread, row.Value, row.Value, at)
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	if !signalsupport.HasRecordedSamples(signal.measurements) {
		return logic.Measurement{}, nil
	}

	var (
		prices  []float64
		volumes []float64
		spreads []float64
		err     error
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		frame, ok := item.(*krakenmarket.BookUpdate)

		if !ok {
			err = fmt.Errorf("sentiment: expected book update")
			return
		}

		if len(frame.Bids) == 0 || len(frame.Asks) == 0 {
			return
		}

		touchSpread := frame.Asks[0].Price - frame.Bids[0].Price

		if touchSpread <= 0 {
			return
		}

		spreads = append(spreads, touchSpread)

		for _, bid := range frame.Bids {
			if bid.Qty > 0 {
				prices = append(prices, bid.Price)
				volumes = append(volumes, bid.Qty)
			}
		}

		for _, ask := range frame.Asks {
			if ask.Qty > 0 {
				prices = append(prices, ask.Price)
				volumes = append(volumes, ask.Qty)
			}
		}
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if len(prices) == 0 {
		return logic.Measurement{}, nil
	}

	move, change, ok := signalsupport.ResolvedChange(prices)

	if !ok {
		return logic.Measurement{}, nil
	}

	spread := 0.0

	if len(spreads) > 0 {
		spread = spreads[len(spreads)-1]
	}

	row, err := krakenmarket.NewSymbolRow(
		signal.symbol,
		prices[len(prices)-1],
		change,
		float64(statistic.NewSum().Observe(nomagique.Numbers(volumes...)...)),
		1,
		at,
	)

	if err != nil {
		return logic.Measurement{}, nil
	}

	return signal.fromCrossSection(
		row,
		float64(statistic.NewSum().Observe(nomagique.Numbers(volumes...)...)),
		spread,
		change,
		move,
		at,
	)
}

func (signal *Signal) fromCrossSection(
	row *krakenmarket.Symbol,
	volume, spread, change, move float64,
	at time.Time,
) (logic.Measurement, error) {
	if volume > 0 {
		row.Volume = volume
	}

	if change != 0 {
		row.Value = change
	}

	row.Updated = at

	if err := row.Validate(); err != nil {
		return logic.Measurement{}, nil
	}

	if err := crossSection.Observe(row); err != nil {
		return logic.Measurement{}, nil
	}

	price := row.Price

	breadth := crossSection.Breadth(at)

	crossSection.RecordBreadth(breadth)

	surgeThreshold := crossSection.MajorityThreshold(at) + signal.surgeBias
	majorityFloor := crossSection.MajorityThreshold(at)

	if surgeThreshold < majorityFloor {
		surgeThreshold = majorityFloor
	}

	if surgeThreshold > 1 {
		surgeThreshold = 1
	}

	leader := crossSection.IsLeader(signal.symbol, change, at)

	category := signal.classifyWithHysteresis(breadth, change, surgeThreshold, leader, at)

	competingScores := []float64{
		finiteScore(breadth),
		finiteScore(math.Abs(change)),
		finiteScore(math.Max(0, surgeThreshold-breadth)),
	}

	probabilities, err := signalsupport.ClassifierProbabilities(competingScores)

	if err != nil {
		return logic.Measurement{}, nil
	}

	categoryIndex := 0

	switch category {
	case logic.CategoryRiskOnSurge:
		categoryIndex = 1
	case logic.CategoryDivergentMove:
		categoryIndex = 2
	case logic.CategorySystemicSlump:
		categoryIndex = 3
	}

	surpriseVector := signal.transition.PadObserved(probabilities, 0)
	surprise, err := signal.transition.Surprise(surpriseVector)

	if err != nil {
		return logic.Measurement{}, nil
	}

	signal.transition.Update(categoryIndex)

	confidence, err := probability.CategoryShareConfidence(competingScores, categoryIndex)

	if err != nil {
		return logic.Measurement{}, nil
	}

	strength := breadth

	if category == logic.CategoryDivergentMove {
		strength = math.Abs(change)
	}

	position := logic.PositionTypeNone

	if move > 0 {
		position = logic.PositionTypeLong
	}

	if move < 0 {
		position = logic.PositionTypeShort
	}

	elapsed, err := signalsupport.ObservationElapsed(signal.measurements, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	if spread <= 0 {
		return logic.Measurement{}, nil
	}

	return logic.Measurement{
		Source:     logic.SourceSentiment,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   strength,
		Volume:     volume,
		Spread:     spread,
		Elapsed:    elapsed,
		Category:   category,
		Regime:     logic.RegimeTypeNone,
		Position:   position,
		Confidence: confidence,
		Surprise:   surprise,
		ObservedAt: at,
		Market:     *row,
	}, nil
}

func finiteScore(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	return value
}

func (signal *Signal) classifyWithHysteresis(
	breadth, change, surgeThreshold float64,
	leader bool,
	at time.Time,
) logic.CategoryType {
	enterRiskOn := surgeThreshold
	exitRiskOn := surgeThreshold - 0.05

	if exitRiskOn < 0 {
		exitRiskOn = 0
	}

	proposed := signal.classify(breadth, change, enterRiskOn, leader)

	if signal.lastCategory == logic.CategoryTypeNone {
		signal.lastCategory = proposed
		signal.lastCategoryAt = at

		return proposed
	}

	if proposed == signal.lastCategory {
		signal.lastCategoryAt = at

		return proposed
	}

	if at.Sub(signal.lastCategoryAt) < time.Second {
		return signal.lastCategory
	}

	if signal.lastCategory == logic.CategoryRiskOnSurge && breadth >= exitRiskOn {
		return signal.lastCategory
	}

	signal.lastCategory = proposed
	signal.lastCategoryAt = at

	return proposed
}

func (signal *Signal) classify(
	breadth, change, surgeThreshold float64,
	leader bool,
) logic.CategoryType {
	if breadth >= surgeThreshold {
		return logic.CategoryRiskOnSurge
	}

	if leader && change != 0 {
		return logic.CategoryDivergentMove
	}

	return logic.CategorySystemicSlump
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
