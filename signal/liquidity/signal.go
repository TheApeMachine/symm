package liquidity

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
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/nomagique/probability"
	"gonum.org/v1/gonum/stat"
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
	symbol          string
	entity          *logic.Entity
	measurements    *ring.Ring
	warmupRemaining int
	transition      *probability.TransitionMatrix
	weights         numeric.ClassifierWeights
	tuner           *numeric.FeedbackTuner
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
) *Signal {
	capacity := viper.GetInt("signals.liquidity.measurements_capacity")

	if capacity <= 0 {
		capacity = 64
	}

	threshold := math.Min(
		math.Max(viper.GetFloat64("signals.liquidity.surprise_threshold"), 1.0),
		5.0,
	)
	alpha := math.Min(
		math.Max(viper.GetFloat64("signals.liquidity.alpha"), 0.1),
		1.0,
	)

	return &Signal{
		symbol:          symbol,
		entity:          entity,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		transition:      probability.NewTransitionMatrix(4, alpha),
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
			logic.SourceLiquidity,
			signal.symbol,
			logic.CategoryMedianDepth,
			3,
			signal.measurements,
			at,
			measurement,
			err,
		)
	case logic.EntityTick:
		measurement, err := signal.measureTick(at)
		return signalsupport.FinishMeasure(
			logic.SourceLiquidity,
			signal.symbol,
			logic.CategoryMedianDepth,
			3,
			signal.measurements,
			at,
			measurement,
			err,
		)
	case logic.EntityBook:
		measurement, err := signal.measureBook(at)
		return signalsupport.FinishMeasure(
			logic.SourceLiquidity,
			signal.symbol,
			logic.CategoryMedianDepth,
			3,
			signal.measurements,
			at,
			measurement,
			err,
		)
	default:
		return logic.Measurement{}, errnie.Error(
			fmt.Errorf("liquidity: unsupported entity %s", signal.entity.Type),
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
			err = fmt.Errorf("liquidity: expected trade update")
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

	_, _, ok := signalsupport.ResolvedChange(prices)

	if !ok {
		return logic.Measurement{}, nil
	}

	row, err := krakenmarket.SymbolRowFromPrices(signal.symbol, prices, quoteVol, 1, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	spread, spreadErr := signalsupport.TouchSpread(prices)

	if spreadErr != nil {
		return logic.Measurement{}, nil
	}

	return signal.fromCrossSection(row, spread, at)
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

	return signal.fromCrossSection(row, spread, at)
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	var (
		prices  []float64
		depths  []float64
		spreads []float64
		err     error
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		frame, ok := item.(*krakenmarket.BookUpdate)

		if !ok {
			err = fmt.Errorf("liquidity: expected book update")
			return
		}

		if len(frame.Bids) == 0 || len(frame.Asks) == 0 {
			return
		}

		mid := (frame.Bids[0].Price + frame.Asks[0].Price) / 2
		spread := frame.Asks[0].Price - frame.Bids[0].Price

		if spread <= 0 {
			return
		}

		depth := frame.Bids[0].Qty + frame.Asks[0].Qty

		prices = append(prices, mid)
		depths = append(depths, depth)
		spreads = append(spreads, spread)
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if len(prices) < 2 {
		return logic.Measurement{}, nil
	}

	price := prices[len(prices)-1]
	quoteVol := depths[len(depths)-1] * price
	spread := 0.0

	if len(spreads) > 0 {
		spread = spreads[len(spreads)-1]
	}

	if quoteVol <= 0 {
		return logic.Measurement{}, nil
	}

	_, _, ok := signalsupport.ResolvedChange([]float64{prices[0], price})

	if !ok {
		return logic.Measurement{}, nil
	}

	row, err := krakenmarket.SymbolRowFromPrices(signal.symbol, prices, quoteVol, 1, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	return signal.fromCrossSection(row, spread, at)
}

func (signal *Signal) fromCrossSection(
	row *krakenmarket.Symbol,
	spread float64,
	at time.Time,
) (logic.Measurement, error) {
	if err := crossSection.Observe(row); err != nil {
		return logic.Measurement{}, err
	}

	quoteVol := row.Volume
	price := row.Price

	peers := crossSection.Volumes()

	if len(peers) < 2 {
		return logic.Measurement{}, nil
	}

	lower, upper := signal.quartiles(peers)
	peakScarcity := signal.isPeakScarcity(quoteVol, peers)
	median := float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(peers...)...))

	if median <= 0 {
		return logic.Measurement{}, nil
	}

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

	probabilities, err := probability.SoftmaxScores([]float64{
		scarcityScore,
		depthScore,
		peakScore,
	})

	if err != nil {
		return logic.Measurement{}, err
	}

	categoryIndex := 0

	switch category {
	case logic.CategoryExtremeScarcity:
		categoryIndex = 1
	case logic.CategoryMedianDepth:
		categoryIndex = 2
	case logic.CategoryRobustLiquidity:
		categoryIndex = 3
	}

	surpriseVector := signal.transition.PadObserved(probabilities, 0)
	surprise, err := signal.transition.Surprise(surpriseVector)

	if err != nil {
		return logic.Measurement{}, err
	}

	signal.transition.Update(categoryIndex)

	confidence, err := probability.CategoryConfidence(probabilities, categoryIndex)

	if err != nil {
		return logic.Measurement{}, err
	}

	strength := scarcityScore

	if category == logic.CategoryRobustLiquidity {
		strength = depthScore
	}

	elapsed, err := signalsupport.ObservationElapsed(signal.measurements, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	if spread <= 0 {
		return logic.Measurement{}, nil
	}

	return logic.Measurement{
		Source:     logic.SourceLiquidity,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   strength,
		Volume:     quoteVol,
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

func (signal *Signal) quartiles(volumes []float64) (lower, upper float64) {
	return float64(statistic.NewQuantile(0.25, stat.LinInterp, nil).Observe(nomagique.Numbers(volumes...)...)), float64(statistic.NewQuantile(0.75, stat.LinInterp, nil).Observe(nomagique.Numbers(volumes...)...))
}

func (signal *Signal) isPeakScarcity(quoteVol float64, volumes []float64) bool {
	if len(volumes) == 0 {
		return false
	}

	minVolume := float64(statistic.NewQuantile(0, stat.LinInterp, nil).Observe(nomagique.Numbers(volumes...)...))

	return quoteVol <= minVolume
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
