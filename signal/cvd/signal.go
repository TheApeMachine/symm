package cvd

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
	"gonum.org/v1/gonum/stat"
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
	transition      *probability.TransitionMatrix
	weights         learning.ClassifierWeights
	tuner           *learning.FeedbackTuner
	grossHistory    []float64
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
) *Signal {
	capacity := market.MustSignalMeasurementCapacity()

	alpha := signalsupport.BoundedClassifierAlpha()

	return &Signal{
		symbol:          symbol,
		entity:          entity,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		transition:      probability.NewTransitionMatrix(5, alpha),
		weights: learning.DefaultClassifierWeights(
			signalsupport.BoundedAdaptiveSurpriseThreshold(logic.SourceCVD),
		),
		tuner: learning.NewFeedbackTuner(),
	}
}

func (signal *Signal) RefreshSurpriseThreshold() {
	signalsupport.RefreshClassifierWeights(logic.SourceCVD, &signal.weights)
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
			fmt.Errorf("cvd: unsupported entity %s", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	var (
		buyNotional  float64
		sellNotional float64
		prices       []float64
		notionals    []float64
		tradeCount   int
		err          error
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
		notional := trade.Price * trade.Qty
		notionals = append(notionals, notional)

		if trade.Side == "buy" {
			buyNotional += notional
		}

		if trade.Side == "sell" {
			sellNotional += notional
		}

		prices = append(prices, trade.Price)
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	return signal.fromSeries(buyNotional, sellNotional, prices, notionals, tradeCount, at)
}

func (signal *Signal) measureTick(_ time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, nil
}

func (signal *Signal) measureBook(_ time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, nil
}

func (signal *Signal) fromSeries(
	buyNotional, sellNotional float64,
	prices []float64,
	notionals []float64,
	tradeCount int,
	at time.Time,
) (logic.Measurement, error) {
	gross := buyNotional + sellNotional

	if gross <= 0 || tradeCount < 2 || len(prices) < 2 {
		return logic.Measurement{}, nil
	}

	net := buyNotional - sellNotional
	netFraction := math.Abs(net) / gross
	firstPrice := prices[0]
	lastPrice := prices[len(prices)-1]

	if firstPrice <= 0 || lastPrice <= 0 {
		return logic.Measurement{}, nil
	}

	priceResponseBps := math.Abs(lastPrice/firstPrice-1) * 10000
	flowPressure := gross / math.Max(signal.medianNotional(notionals), 1e-9)
	impactEfficiency := priceResponseBps / math.Max(flowPressure, 1e-9)

	priceDrift := lastPrice - firstPrice
	priceMoves := signal.priceMoves(prices)
	flatThreshold := float64(statistic.NewMedianAbsolute(nil).Observe(nomagique.Numbers(priceMoves...)...))
	driveThreshold := signal.driveThreshold(tradeCount)
	grossFloor := signal.grossFloor()
	signal.recordGross(gross)

	highNet := netFraction >= driveThreshold
	flowAligned := (net > 0 && priceDrift > 0) || (net < 0 && priceDrift < 0)
	hiddenAbsorption := signal.hiddenAbsorption(
		highNet,
		flowPressure,
		impactEfficiency,
		priceDrift,
		flatThreshold,
		tradeCount,
	)
	flatPrice := hiddenAbsorption || signal.flatPrice(priceDrift, flatThreshold, tradeCount)

	category := signal.classify(
		highNet,
		flowAligned,
		flatPrice,
		gross,
		grossFloor,
		tradeCount,
		hiddenAbsorption,
	)

	absorptionScore := 0.0
	driveScore := 0.0
	balanceScore := math.Max(0, 1-netFraction)

	if hiddenAbsorption {
		absorptionScore = netFraction * (1 / (1 + impactEfficiency))
	}

	if highNet && flowAligned && !flatPrice {
		driveScore = netFraction
	}

	if highNet && !flatPrice && !flowAligned {
		absorptionScore = math.Max(absorptionScore, netFraction)
	}

	starvationScore := 0.0

	if category == logic.CategoryVolumeStarvation {
		if grossFloor > 0 && gross < grossFloor {
			starvationScore = math.Max(0, 1-gross/grossFloor)
		} else if tradeCount < 3 && !highNet {
			starvationScore = 1 - float64(tradeCount)/3
		}
	}

	scores := []float64{
		absorptionScore,
		driveScore,
		balanceScore,
		starvationScore,
	}
	probabilities, err := signalsupport.ClassifierProbabilities(scores)

	if err != nil {
		return logic.Measurement{}, err
	}

	categoryIndex := logic.CategoryIndexFor(logic.SourceCVD, category)

	if categoryIndex == logic.CategoryNoneIndex {
		return logic.Measurement{}, nil
	}

	surprise, err := signalsupport.TransitionNovelty(signal.transition, probabilities, 0)

	if err != nil {
		return logic.Measurement{}, err
	}

	signal.transition.Update(categoryIndex)

	confidence, err := signalsupport.CategoryConfidence(scores, categoryIndex)

	if err != nil {
		return logic.Measurement{}, nil
	}

	strength := netFraction

	if category == logic.CategoryStochasticBalance {
		strength = balanceScore
	}

	measurement, err := signal.publish(
		category,
		prices,
		lastPrice,
		strength,
		gross,
		confidence,
		surprise,
		net,
		at,
	)

	if err != nil {
		return logic.Measurement{}, err
	}

	measurement.NoveltySurprise = surprise

	return measurement, nil
}

func (signal *Signal) publish(
	category logic.CategoryType,
	prices []float64,
	price, strength, volume, confidence, surprise, net float64,
	at time.Time,
) (logic.Measurement, error) {
	elapsed, err := signalsupport.ObservationElapsed(signal.measurements, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	spread, err := signalsupport.TouchSpread(prices)

	if err != nil {
		return logic.Measurement{}, nil
	}

	_, _, ok := signalsupport.ResolvedChange(prices)

	if !ok {
		return logic.Measurement{}, nil
	}

	row, err := krakenmarket.SymbolRowFromPrices(signal.symbol, prices, volume, 1, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	position := logic.PositionTypeNone

	if net > 0 {
		position = logic.PositionTypeLong
	}

	if net < 0 {
		position = logic.PositionTypeShort
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
		Position:   position,
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

func (signal *Signal) flatPrice(priceDrift, stepThreshold float64, tradeCount int) bool {
	// A zero median |move| is the norm on tick tapes (consecutive trades
	// mostly print the same price). Treating that as "flat" classified every
	// high-flow window as absorption and made AggressiveDrive unreachable;
	// without a noise scale, only a literally unmoved price is flat.
	if stepThreshold <= 0 {
		return priceDrift == 0
	}

	windowThreshold := stepThreshold * math.Sqrt(float64(tradeCount))

	return math.Abs(priceDrift) <= windowThreshold
}

func (signal *Signal) classify(
	highNet, flowAligned, flatPrice bool,
	gross, grossFloor float64,
	tradeCount int,
	hiddenAbsorption bool,
) logic.CategoryType {
	if grossFloor > 0 && gross < grossFloor {
		return logic.CategoryVolumeStarvation
	}

	if tradeCount < 3 && !highNet {
		return logic.CategoryVolumeStarvation
	}

	if hiddenAbsorption {
		return logic.CategoryHiddenAbsorption
	}

	if highNet && flowAligned {
		return logic.CategoryAggressiveDrive
	}

	if highNet && !flatPrice && !flowAligned {
		return logic.CategoryHiddenAbsorption
	}

	return logic.CategoryStochasticBalance
}

func (signal *Signal) hiddenAbsorption(
	highNet bool,
	flowPressure float64,
	impactEfficiency float64,
	priceDrift float64,
	stepThreshold float64,
	tradeCount int,
) bool {
	if !highNet || flowPressure <= 0 {
		return false
	}

	if impactEfficiency <= 0 {
		return true
	}

	return flowPressure >= 1 && impactEfficiency <= 1 && signal.flatPrice(priceDrift, stepThreshold, tradeCount)
}

func (signal *Signal) medianNotional(notionals []float64) float64 {
	if len(notionals) == 0 {
		return 0
	}

	return float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(notionals...)...))
}

const grossHistoryCap = 64
const minGrossHistory = 8

func (signal *Signal) recordGross(gross float64) {
	if gross <= 0 {
		return
	}

	signal.grossHistory = appendRingFloat(signal.grossHistory, gross, grossHistoryCap)
}

func (signal *Signal) grossFloor() float64 {
	if len(signal.grossHistory) < minGrossHistory {
		return 0
	}

	return float64(
		statistic.NewQuantile(0.1, stat.LinInterp, nil).Observe(nomagique.Numbers(signal.grossHistory...)...),
	)
}

func appendRingFloat(values []float64, value float64, capacity int) []float64 {
	values = append(values, value)

	if len(values) <= capacity {
		return values
	}

	return values[len(values)-capacity:]
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
