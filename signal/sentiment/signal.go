package sentiment

import (
	"container/ring"
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
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
	measurements    *ring.Ring
	warmupRemaining int
	crossSection      *crossSection
	transition        *numeric.TransitionMatrix
	weights           numeric.ClassifierWeights
	tuner             *numeric.FeedbackTuner
	baselineThreshold float64
	surgeBias         float64
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
		symbol:            symbol,
		entity:            entity,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		crossSection:      crossSection,
		transition:        numeric.NewTransitionMatrix(4, alpha),
		weights:           numeric.DefaultClassifierWeights(threshold),
		tuner:             numeric.NewFeedbackTuner(),
		baselineThreshold: threshold,
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

		signal.surgeBias = math.Min(
			math.Max(signal.weights.Threshold-signal.baselineThreshold, -0.25),
			0.25,
		)
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
			fmt.Errorf("sentiment: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade() (logic.Measurement, error) {
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
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	move, change := numeric.AnchorChange(prices[0], prices[len(prices)-1])

	return signal.fromCrossSection(
		prices[len(prices)-1],
		numeric.Sum(volumes),
		0,
		change,
		move,
	)
}

func (signal *Signal) measureTick() (logic.Measurement, error) {
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
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	spread := 0.0

	if len(spreads) > 0 {
		spread = spreads[len(spreads)-1]
	}

	price := ticker.Ask + ticker.Bid
	volume := ticker.AskQty + ticker.BidQty
	change := ticker.ChangePct

	return signal.fromCrossSection(price, volume, spread, change, change)
}

func (signal *Signal) measureBook() (logic.Measurement, error) {
	var (
		prices  []float64
		volumes []float64
		spreads []float64
		err     error
	)

	folded := krakenmarket.Book{}

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		frame, ok := item.(*krakenmarket.Book)

		if !ok {
			err = fmt.Errorf("sentiment: expected book update")
			return
		}

		folded.Fold(*frame, 0)

		mid, spread, depth, touchOK := folded.TouchQuote()

		if !touchOK {
			return
		}

		prices = append(prices, mid)
		volumes = append(volumes, depth)
		spreads = append(spreads, spread)
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if len(prices) == 0 {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	move, change := numeric.AnchorChange(prices[0], prices[len(prices)-1])
	spread := 0.0

	if len(spreads) > 0 {
		spread = spreads[len(spreads)-1]
	}

	return signal.fromCrossSection(
		prices[len(prices)-1],
		numeric.Sum(volumes),
		spread,
		change,
		move,
	)
}

func (signal *Signal) fromCrossSection(
	price, volume, spread, change, move float64,
) (logic.Measurement, error) {
	signal.crossSection.publishChange(signal.symbol, change)

	breadth := signal.crossSection.breadth()

	signal.crossSection.recordBreadth(breadth)

	surgeThreshold := signal.crossSection.majorityThreshold() + signal.surgeBias
	surgeThreshold = math.Min(math.Max(surgeThreshold, 0.5), 1)

	leader := signal.crossSection.isLeader(change)

	category := signal.classify(breadth, change, surgeThreshold, leader)

	leaderScore := 0.0

	if leader {
		leaderScore = 1
	}

	scores := []float64{
		breadth,
		math.Abs(change),
		leaderScore,
	}
	probabilities := numeric.SoftmaxScores(scores)

	categoryIndex := 0

	switch category {
	case logic.CategoryRiskOnSurge:
		categoryIndex = 1
	case logic.CategoryDivergentMove:
		categoryIndex = 2
	case logic.CategorySystemicSlump:
		categoryIndex = 3
	}

	surpriseVector := signal.transition.PadObserved(probabilities, 1e-6)
	surprise := signal.transition.Surprise(surpriseVector)

	signal.transition.Update(categoryIndex)

	confidence := 0.0

	if categoryIndex > 0 && categoryIndex-1 < len(probabilities) {
		confidence = probabilities[categoryIndex-1]
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

	return logic.Measurement{
		Source:     logic.SourceSentiment,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   strength,
		Volume:     volume,
		Spread:     spread,
		Elapsed:    0,
		Category:   category,
		Regime:     logic.RegimeTypeNone,
		Position:   position,
		Confidence: confidence,
		Surprise:   surprise,
	}, nil
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

