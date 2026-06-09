package liquidity

import (
	"container/ring"
	
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
	"time"
)

/*
Signal identifies opportunities in thin markets by ranking a symbol's quote volume against the broader market.

Cross-Section Ranking : Ranks the daily quote volume of all subscribed symbols.
Illiquidity Score     : Identifies symbols trading strictly below the cross-section median of their peers.
Peak Scarcity         : Uses a peak gate to find symbols that are currently the most illiquid in the universe.

The "Convexity" Story : It signals where a small amount of order flow will cause the largest price displacement — the thinnest pipes on the exchange.
The "Neglect" Story   : It identifies assets ignored by the broader market, prime for sudden volatility once flow arrives.

| Category         | Rank vs. Peers   | Volume   | Market "Feel"                |
|------------------|------------------|----------|------------------------------|
| Extreme Scarcity | Peak Illiquidity | Very Low | High Convexity / Fragile     |
| Median Depth     | Middle           | Normal   | Standard Efficiency          |
| Robust Liquidity | Bottom (Deep)    | High     | Efficient / Safe             |
*/
type Signal struct {
	symbol       string
	entity       *logic.Entity
	measurements    *ring.Ring
	warmupRemaining int
	crossSection *crossSection
	transition   *numeric.TransitionMatrix
	weights      numeric.ClassifierWeights
	tuner        *numeric.FeedbackTuner
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
		symbol:       symbol,
		entity:       entity,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		crossSection: crossSection,
		transition:   numeric.NewTransitionMatrix(4, alpha),
		weights:      numeric.DefaultClassifierWeights(threshold),
		tuner:        numeric.NewFeedbackTuner(),
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
			fmt.Errorf("liquidity: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	var (
		price    float64
		quoteVol float64
		err      error
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		trade, ok := item.(*krakenmarket.TradeUpdate)

		if !ok {
			err = fmt.Errorf("liquidity: expected trade update")
			return
		}

		price = trade.Price
		quoteVol += trade.Price * trade.Qty
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if quoteVol <= 0 {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	return signal.fromCrossSection(price, quoteVol, 0)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
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
			err = fmt.Errorf("liquidity: expected ticker update")
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

	price := ticker.Last

	if price <= 0 {
		price = (ticker.Ask + ticker.Bid) / 2
	}

	quoteVol := ticker.Volume * price

	return signal.fromCrossSection(price, quoteVol, spread)
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	var (
		prices  []float64
		depths  []float64
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
			err = fmt.Errorf("liquidity: expected book update")
			return
		}

		folded.Fold(*frame, 0)

		mid, spread, depth, touchOK := folded.TouchQuote()

		if !touchOK {
			return
		}

		prices = append(prices, mid)
		depths = append(depths, depth)
		spreads = append(spreads, spread)
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if len(prices) == 0 {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	price := prices[len(prices)-1] / 2
	quoteVol := depths[len(depths)-1] * price
	spread := 0.0

	if len(spreads) > 0 {
		spread = spreads[len(spreads)-1]
	}

	return signal.fromCrossSection(price, quoteVol, spread)
}

func (signal *Signal) fromCrossSection(
	price, quoteVol, spread float64,
) (logic.Measurement, error) {
	signal.crossSection.publishQuoteVol(signal.symbol, quoteVol)

	peers := signal.crossSection.snapshot()

	if len(peers) < 2 {
		return logic.Measurement{Symbol: signal.symbol}, nil
	}

	lower, upper := signal.quartiles(peers)
	peakScarcity := signal.isPeakScarcity(quoteVol, peers)
	median := numeric.Median(peers)

	category := signal.classify(quoteVol, lower, upper, peakScarcity)

	scarcityScore := 0.0

	if median > 0 {
		scarcityScore = math.Max(0, (median-quoteVol)/median)
	}

	depthScore := 0.0

	if median > 0 {
		depthScore = math.Max(0, (quoteVol-median)/median)
	}

	peakScore := 0.0

	if peakScarcity {
		peakScore = 1
	}

	probabilities := numeric.SoftmaxScores([]float64{
		scarcityScore,
		depthScore,
		peakScore,
	})

	categoryIndex := 0

	switch category {
	case logic.CategoryExtremeScarcity:
		categoryIndex = 1
	case logic.CategoryMedianDepth:
		categoryIndex = 2
	case logic.CategoryRobustLiquidity:
		categoryIndex = 3
	}

	surpriseVector := signal.transition.PadObserved(probabilities, 1e-6)
	surprise := signal.transition.Surprise(surpriseVector)

	signal.transition.Update(categoryIndex)

	confidence := 0.0

	if categoryIndex > 0 && categoryIndex-1 < len(probabilities) {
		confidence = probabilities[categoryIndex-1]
	}

	strength := scarcityScore

	if category == logic.CategoryRobustLiquidity {
		strength = depthScore
	}

	return logic.Measurement{
		Source:     logic.SourceLiquidity,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   strength,
		Volume:     quoteVol,
		Spread:     spread,
		Elapsed:    0,
		Category:   category,
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   surprise,
	}, nil
}

func (signal *Signal) quartiles(volumes []float64) (lower, upper float64) {
	sorted := numeric.CopySorted(volumes)

	return numeric.PercentileSorted(sorted, 0.25), numeric.PercentileSorted(sorted, 0.75)
}

func (signal *Signal) isPeakScarcity(quoteVol float64, volumes []float64) bool {
	if len(volumes) == 0 {
		return false
	}

	sorted := numeric.CopySorted(volumes)

	return quoteVol <= sorted[0]
}

func (signal *Signal) classify(
	quoteVol, lower, upper float64,
	peakScarcity bool,
) logic.CategoryType {
	if peakScarcity || quoteVol <= lower {
		return logic.CategoryExtremeScarcity
	}

	if quoteVol >= upper {
		return logic.CategoryRobustLiquidity
	}

	return logic.CategoryMedianDepth
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

