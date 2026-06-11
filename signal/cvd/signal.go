package cvd

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
Signal measures cumulative volume delta (CVD) from executed trade flow.

Net Flow Fraction  : Signed buy-minus-sell volume over gross executed volume in the ring window.
Price Drift        : End-to-end price change across the same window.
Flow Divergence    : Large net flow with muted price response flags hidden absorption.

The "Iceberg" Story    : Aggressive buying that does not lift price — liquidity is being absorbed off-screen.
The "Steamroller" Story: One-sided aggression that price is actually following — directional conviction.

| Category            | Net Fraction | Price vs Flow | Market "Feel"           |
|---------------------|--------------|---------------|-------------------------|
| Hidden Absorption   | High         | Divergent     | Iceberg / Passive Depth |
| Aggressive Drive    | High         | Aligned       | Steamroller / Trend     |
| Stochastic Balance  | Low          | Mixed         | Two-Sided / Choppy      |
| Volume Starvation   | N/A          | Thin          | No Flow / Idle          |
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
	capacity := viper.GetInt("signals.cvd.measurements_capacity")

	if capacity <= 0 {
		capacity = 64
	}

	threshold := math.Min(
		math.Max(viper.GetFloat64("signals.cvd.surprise_threshold"), 1.0),
		5.0,
	)
	alpha := math.Min(
		math.Max(viper.GetFloat64("signals.cvd.alpha"), 0.1),
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
		return signal.measureTrade(at)
	case logic.EntityTick:
		return signal.measureTick(at)
	case logic.EntityBook:
		return signal.measureBook(at)
	default:
		return logic.Measurement{}, errnie.Error(
			fmt.Errorf("cvd: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	var (
		buyVolume  float64
		sellVolume float64
		prices     []float64
		tradeCount int
		err        error
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		trade, ok := item.(*krakenmarket.TradeUpdate)

		if !ok {
			err = fmt.Errorf("cvd: expected trade update")
			return
		}

		tradeCount++

		if trade.Side == "buy" {
			buyVolume += trade.Qty
		}

		if trade.Side == "sell" {
			sellVolume += trade.Qty
		}

		prices = append(prices, trade.Price)
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	return signal.fromSeries(buyVolume, sellVolume, prices, tradeCount, at)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, fmt.Errorf("cvd: not ready")
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, fmt.Errorf("cvd: not ready")
}

func (signal *Signal) fromSeries(
	buyVolume, sellVolume float64,
	prices []float64,
	tradeCount int,
	at time.Time,
) (logic.Measurement, error) {
	gross := buyVolume + sellVolume

	if gross <= 0 || tradeCount < 2 || len(prices) < 2 {
		return logic.Measurement{}, fmt.Errorf("cvd: insufficient trade window")
	}

	net := buyVolume - sellVolume
	netFraction := math.Abs(net) / gross
	priceDrift := prices[len(prices)-1] - prices[0]
	priceMoves := signal.priceMoves(prices)
	flatThreshold := numeric.MedianAbsolute(priceMoves)
	driveThreshold := signal.driveThreshold(tradeCount)

	highNet := netFraction >= driveThreshold
	flowAligned := (net > 0 && priceDrift > 0) || (net < 0 && priceDrift < 0)
	flatPrice := math.Abs(priceDrift) <= flatThreshold

	category := signal.classify(highNet, flowAligned, flatPrice)

	absorptionScore := 0.0

	if highNet && flatPrice {
		absorptionScore = netFraction
	}

	driveScore := 0.0

	if highNet && flowAligned && !flatPrice {
		driveScore = netFraction
	}

	balanceScore := 0.0

	if !highNet {
		balanceScore = 1 - netFraction
	}

	starvationScore := 0.0

	if category == logic.CategoryVolumeStarvation {
		starvationScore = 1
	}

	probabilities, err := numeric.SoftmaxScores([]float64{
		absorptionScore,
		driveScore,
		balanceScore,
		starvationScore,
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

	strength := netFraction

	if category == logic.CategoryStochasticBalance {
		strength = balanceScore
	}

	return signal.publish(
		category,
		prices,
		prices[len(prices)-1],
		strength,
		gross,
		confidence,
		surprise,
		at,
	)
}

func (signal *Signal) publish(
	category logic.CategoryType,
	prices []float64,
	price, strength, volume, confidence, surprise float64,
	at time.Time,
) (logic.Measurement, error) {
	elapsed, err := signalsupport.ObservationElapsed(signal.measurements, at)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	spread, err := signalsupport.TouchSpread(prices)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	row, err := krakenmarket.SymbolRowFromPrices(signal.symbol, prices, volume, 1, at)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	return logic.Measurement{
		Source:     logic.SourceCVD,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   strength,
		Volume:     volume,
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

func (signal *Signal) priceMoves(prices []float64) []float64 {
	moves := make([]float64, 0, len(prices)-1)

	for index := 1; index < len(prices); index++ {
		moves = append(moves, prices[index]-prices[index-1])
	}

	return moves
}

func (signal *Signal) driveThreshold(tradeCount int) float64 {
	if tradeCount <= 0 {
		return 1
	}

	return 1 / math.Sqrt(float64(tradeCount))
}

func (signal *Signal) classify(highNet, flowAligned, flatPrice bool) logic.CategoryType {
	if highNet && flatPrice {
		return logic.CategoryHiddenAbsorption
	}

	if highNet && flowAligned {
		return logic.CategoryAggressiveDrive
	}

	return logic.CategoryStochasticBalance
}

func (signal *Signal) categoryIndex(category logic.CategoryType) int {
	switch category {
	case logic.CategoryHiddenAbsorption:
		return 1
	case logic.CategoryAggressiveDrive:
		return 2
	case logic.CategoryStochasticBalance:
		return 3
	case logic.CategoryVolumeStarvation:
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
