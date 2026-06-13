package depthflow

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
Signal measures distance-decayed book imbalance with trade-pressure confirmation.

Weighted Book Imbalance : Bid and ask depth weighted by exponential decay from mid over spread.
Touch vs Deep Skew      : Contradiction between deep-book and Level-1 imbalance flags spoof traps.
Trade Pressure          : Signed buy-minus-sell fraction from executed flow in the shared cross-section.

The "Wall" Story   : A loaded book side that price has not broken — resting liquidity is steering flow.
The "Trap" Story   : Deep liquidity on one side while the touch disagrees — a bluff before the sweep.

| Category          | Book Shape        | Trade Pressure | Market "Feel"        |
|-------------------|-------------------|----------------|----------------------|
| Loaded Imbalance  | Skewed, aligned   | Confirming     | Wall / Directional   |
| Spoof Trap        | Deep vs touch skew| Mixed          | Bluff / Fake Wall    |
| Book Thinning     | Touch depleted    | Any            | Hollow / Fragile     |
| Dense Neutrality  | Balanced depth    | Low            | Thick / Two-Sided    |
*/
type Signal struct {
	symbol          string
	entity          *logic.Entity
	measurements    *ring.Ring
	warmupRemaining int
	transition      *probability.TransitionMatrix
	weights         learning.ClassifierWeights
	tuner           *learning.FeedbackTuner
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
			signalsupport.BoundedAdaptiveSurpriseThreshold(logic.SourceDepthFlow),
		),
		tuner: learning.NewFeedbackTuner(),
	}
}

func (signal *Signal) RefreshSurpriseThreshold() {
	signalsupport.RefreshClassifierWeights(logic.SourceDepthFlow, &signal.weights)
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
			fmt.Errorf("depthflow: unsupported entity %s", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	var (
		buyVolume  float64
		sellVolume float64
		prices     []float64
		err        error
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		trade, ok := item.(*krakenmarket.TradeUpdate)

		if !ok {
			err = fmt.Errorf("depthflow: expected trade update")
			return
		}

		prices = append(prices, trade.Price)

		if trade.Side == "buy" {
			buyVolume += trade.Qty
		}

		if trade.Side == "sell" {
			sellVolume += trade.Qty
		}
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	gross := buyVolume + sellVolume

	if len(prices) < 2 || gross <= 0 {
		return logic.Measurement{}, nil
	}

	pressure := (buyVolume - sellVolume) / gross

	if pressure == 0 {
		return logic.Measurement{}, nil
	}

	quoteVol := gross * prices[len(prices)-1]

	_, _, ok := signalsupport.ResolvedChange(prices)

	if !ok {
		return logic.Measurement{}, nil
	}

	row, err := krakenmarket.SymbolRowFromPrices(signal.symbol, prices, quoteVol, pressure, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	if err := crossSection.Observe(row); err != nil {
		return logic.Measurement{}, nil
	}

	return logic.Measurement{}, nil
}

func (signal *Signal) measureTick(_ time.Time) (logic.Measurement, error) {
	return signal.bestEffort(
		time.Time{},
		"depthflow: tick measurement is not implemented",
	), nil
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	if !signalsupport.HasRecordedSamples(signal.measurements) {
		return logic.Measurement{}, nil
	}

	var (
		weightedHistory []float64
		level1History   []float64
		flatHistory     []float64
		err             error
	)

	var snapshot *krakenmarket.BookUpdate
	weighted := 0.0
	level1 := 0.0
	flat := 0.0
	weightedOK := false
	level1OK := false
	flatOK := false
	mid := 0.0
	spread := 0.0
	touchDepth := 0.0

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		frame, ok := item.(*krakenmarket.BookUpdate)

		if !ok {
			err = fmt.Errorf("depthflow: expected book update")
			return
		}

		if frame.Type == "snapshot" {
			snapshot = frame
		}

		if snapshot == nil {
			return
		}

		bids := frame.Bids
		asks := frame.Asks

		if len(bids) == 0 {
			bids = snapshot.Bids
		}

		if len(asks) == 0 {
			asks = snapshot.Asks
		}

		if len(bids) == 0 || len(asks) == 0 {
			return
		}

		touchMid := (bids[0].Price + asks[0].Price) / 2
		touchSpread := asks[0].Price - bids[0].Price

		frameWeighted, frameWeightedOK := signal.weightedImbalance(
			snapshot.Bids, snapshot.Asks, touchMid, touchSpread,
		)
		frameLevel1, frameLevel1OK := signal.level1Imbalance(bids, asks)
		frameFlat, frameFlatOK := signal.flatImbalance(snapshot.Bids, snapshot.Asks)

		if frameWeightedOK {
			weightedHistory = append(weightedHistory, math.Abs(frameWeighted))
		}

		if frameLevel1OK {
			level1History = append(level1History, math.Abs(frameLevel1))
		}

		if frameFlatOK {
			flatHistory = append(flatHistory, math.Abs(frameFlat))
		}

		weighted = frameWeighted
		weightedOK = frameWeightedOK
		level1 = frameLevel1
		level1OK = frameLevel1OK
		flat = frameFlat
		flatOK = frameFlatOK
		mid = touchMid
		spread = touchSpread
		touchDepth = bids[0].Qty + asks[0].Qty
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if !weightedOK || !level1OK || mid <= 0 {
		return signal.bestEffort(
			at,
			"depthflow: book history is not ready",
		), nil
	}

	measurement, err := signal.fromBook(
		weighted, level1, flat, flatOK, mid, spread, touchDepth,
		weightedHistory, level1History, flatHistory,
		at,
	)

	if err != nil {
		return logic.Measurement{}, err
	}

	if measurement.Category == logic.CategoryTypeNone {
		return signal.bestEffort(
			at,
			"depthflow: book measurement is not publishable",
		), nil
	}

	return measurement, nil
}

func (signal *Signal) fromBook(
	weighted, level1, flat float64,
	flatOK bool,
	mid, spread, touchDepth float64,
	weightedHistory, level1History, flatHistory []float64,
	at time.Time,
) (logic.Measurement, error) {
	weightedThreshold := float64(statistic.NewMedianAbsolute(nil).Observe(nomagique.Numbers(weightedHistory...)...))
	level1Threshold := float64(statistic.NewMedianAbsolute(nil).Observe(nomagique.Numbers(level1History...)...))
	tradePressure := crossSection.TradePressure(signal.symbol)

	spoofContrast := spoofContrastRatio(weightedHistory, level1History)
	depthGate := thinningDepthGate(weightedHistory, flatHistory)

	spoofed := signal.isSpoofSkew(weighted, level1, weightedThreshold, level1Threshold, spoofContrast)

	if flatOK {
		spoofed = spoofed || signal.isSpoofSkew(flat, level1, weightedThreshold, level1Threshold, spoofContrast)
	}

	thinning := signal.isBookThinning(weighted, flat, flatOK, depthGate)
	loaded := !spoofed && !thinning &&
		math.Abs(weighted) >= weightedThreshold &&
		weightedThreshold > 0

	category := signal.classify(spoofed, thinning, loaded)

	loadedScore := 0.0

	if loaded {
		loadedScore = math.Abs(weighted)

		pressureScale := loadedPressureScale(tradePressure, weightedThreshold)

		if pressureScale > 0 {
			loadedScore *= pressureScale
		}
	}

	spoofScore := 0.0

	if spoofed {
		spoofScore = math.Abs(weighted - level1)
	}

	thinScore := 0.0

	if thinning {
		thinScore = math.Abs(weighted) - math.Abs(flat)
	}

	neutralScore := 0.0

	if category == logic.CategoryDenseNeutrality {
		neutralScore = 1 - math.Abs(weighted)
	}

	strength := math.Abs(weighted)

	if category == logic.CategorySpoofTrap {
		strength = spoofScore
	}

	if category == logic.CategoryBookThinning {
		strength = math.Abs(thinScore)
	}

	scores := []float64{
		loadedScore,
		spoofScore,
		thinScore,
		neutralScore,
	}

	categoryIndex := signal.categoryIndex(category)

	selectedIndex := categoryIndex - 1

	if selectedIndex >= 0 && selectedIndex < len(scores) && scores[selectedIndex] <= 0 {
		scores[selectedIndex] = strength / (1 + strength)
	}

	probabilities, err := signalsupport.ClassifierProbabilities(scores)

	if err != nil {
		return logic.Measurement{}, err
	}

	surpriseVector := signal.transition.PadObserved(probabilities, 0)
	surprise, err := signal.transition.Surprise(surpriseVector)

	if err != nil {
		return logic.Measurement{}, err
	}

	signal.transition.Update(categoryIndex)

	confidence, err := probability.CategoryShareConfidence(scores, categoryIndex)

	if err != nil {
		return logic.Measurement{}, err
	}

	if strength <= 0 {
		return logic.Measurement{}, nil
	}

	elapsed, err := signalsupport.ObservationElapsed(signal.measurements, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	if spread <= 0 {
		return logic.Measurement{}, nil
	}

	quoteVol := mid * touchDepth

	if quoteVol <= 0 {
		return logic.Measurement{}, nil
	}

	value := math.Abs(weighted) / mid

	if value <= 0 {
		return logic.Measurement{}, nil
	}

	row, err := krakenmarket.NewSymbolRow(signal.symbol, mid, value, quoteVol, tradePressure, at)

	if err != nil {
		return logic.Measurement{}, nil
	}

	return logic.Measurement{
		Source:     logic.SourceDepthFlow,
		Symbol:     signal.symbol,
		Price:      mid,
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

func (signal *Signal) level1Imbalance(
	bids, asks []krakenmarket.BookLevel,
) (float64, bool) {
	if len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	total := bids[0].Qty + asks[0].Qty

	if total <= 0 {
		return 0, false
	}

	return (bids[0].Qty - asks[0].Qty) / total, true
}

func (signal *Signal) flatImbalance(
	bids, asks []krakenmarket.BookLevel,
) (float64, bool) {
	bidVolume := 0.0
	askVolume := 0.0

	for _, level := range bids {
		bidVolume += level.Qty
	}

	for _, level := range asks {
		askVolume += level.Qty
	}

	total := bidVolume + askVolume

	if total <= 0 {
		return 0, false
	}

	return (bidVolume - askVolume) / total, true
}

func (signal *Signal) weightedImbalance(
	bids, asks []krakenmarket.BookLevel,
	mid, spread float64,
) (float64, bool) {
	if mid <= 0 || spread <= 0 || len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	weightedBid := 0.0
	weightedAsk := 0.0

	for _, level := range bids {
		weight := math.Exp(-math.Abs(level.Price-mid) / spread)
		weightedBid += level.Qty * weight
	}

	for _, level := range asks {
		weight := math.Exp(-math.Abs(level.Price-mid) / spread)
		weightedAsk += level.Qty * weight
	}

	total := weightedBid + weightedAsk

	if total <= 0 {
		return 0, false
	}

	return (weightedBid - weightedAsk) / total, true
}

func (signal *Signal) isSpoofSkew(
	weighted, level1, weightedThreshold, level1Threshold, spoofContrast float64,
) bool {
	if math.Abs(weighted) < weightedThreshold {
		return false
	}

	if weighted*level1 >= 0 {
		return false
	}

	if spoofContrast <= 0 {
		return false
	}

	return math.Abs(level1) >= level1Threshold*spoofContrast
}

func (signal *Signal) isBookThinning(
	weighted, flat float64,
	flatOK bool,
	depthGate float64,
) bool {
	if !flatOK || math.Abs(weighted) <= 0 {
		return false
	}

	if depthGate <= 0 {
		return false
	}

	return math.Abs(flat) < depthGate*math.Abs(weighted)
}

func (signal *Signal) classify(spoofed, thinning, loaded bool) logic.CategoryType {
	if spoofed {
		return logic.CategorySpoofTrap
	}

	if thinning {
		return logic.CategoryBookThinning
	}

	if loaded {
		return logic.CategoryLoadedImbalance
	}

	return logic.CategoryDenseNeutrality
}

func (signal *Signal) categoryIndex(category logic.CategoryType) int {
	switch category {
	case logic.CategoryLoadedImbalance:
		return 1
	case logic.CategorySpoofTrap:
		return 2
	case logic.CategoryBookThinning:
		return 3
	case logic.CategoryDenseNeutrality:
		return 4
	default:
		return 0
	}
}

func (signal *Signal) bestEffort(at time.Time, reason string) logic.Measurement {
	return logic.Measurement{
		Source:     logic.SourceDepthFlow,
		Symbol:     signal.symbol,
		ObservedAt: at,
		BestEffort: true,
		GapReason:  reason,
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
