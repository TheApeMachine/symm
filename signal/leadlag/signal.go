package leadlag

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
	"github.com/theapemachine/symm/numeric/adaptive"
	signalsupport "github.com/theapemachine/symm/signal"
)

type moveBaseline struct {
	moments adaptive.EWMoments
	minObs  int
	alpha   float64
	minMove float64
}

type anchorMove struct {
	moved       bool
	stallMargin float64
	ready       bool
}

func newMoveBaseline() moveBaseline {
	return moveBaseline{
		minObs:  anchorMoveMinObs,
		alpha:   anchorMoveAlpha,
		minMove: anchorMoveMinLogRet,
	}
}

/*
Signal measures temporal correlation between the anchor pair and each follower.

Cross-Lag Correlation : Hayashi-Yoshida correlation at shifted anchor paths across bar lags.
Anchor Move Gate      : Adaptive EWMA baseline on anchor log-return over the lag search window.
Lag Fraction          : Best lag bars divided by the search window — catch-up vs synchronized beta.

The "Inefficiency" Story : Followers with high lag correlation but large lag fraction have not caught up yet.
The "Beta Drift" Story   : Tight synchronization with the anchor — no idiosyncratic lead of their own.

| Category              | Lag Correlation | Lag Fraction | Market "Feel"            |
|-----------------------|-----------------|--------------|--------------------------|
| Inefficient Lag       | High            | High         | Catch-up Opportunity     |
| Synchronized Drift    | High            | Low          | Systemic Beta            |
| Decoupled Move        | Low             | N/A          | Idiosyncratic Alpha      |
| Anchor Stall          | Low             | Low          | Leadership Exhaustion    |
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
	capacity := viper.GetInt("signals.leadlag.measurements_capacity")

	if capacity <= 0 {
		capacity = 64
	}

	threshold := math.Min(
		math.Max(viper.GetFloat64("signals.leadlag.surprise_threshold"), 1.0),
		5.0,
	)
	alpha := math.Min(
		math.Max(viper.GetFloat64("signals.leadlag.alpha"), 0.1),
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
		measurement, err := signal.measureTrade(at)
		return signalsupport.FinishMeasure(
			logic.SourceLeadLag,
			signal.symbol,
			logic.CategorySynchronizedDrift,
			4,
			signal.measurements,
			at,
			measurement,
			err,
		)
	case logic.EntityTick:
		measurement, err := signal.measureTick(at)
		return signalsupport.FinishMeasure(
			logic.SourceLeadLag,
			signal.symbol,
			logic.CategorySynchronizedDrift,
			4,
			signal.measurements,
			at,
			measurement,
			err,
		)
	case logic.EntityBook:
		measurement, err := signal.measureBook(at)
		return signalsupport.FinishMeasure(
			logic.SourceLeadLag,
			signal.symbol,
			logic.CategorySynchronizedDrift,
			4,
			signal.measurements,
			at,
			measurement,
			err,
		)
	default:
		return logic.Measurement{}, errnie.Error(
			fmt.Errorf("leadlag: unsupported entity %s", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(eventAt time.Time) (logic.Measurement, error) {
	trade, ok := signal.latest().(*krakenmarket.TradeUpdate)

	if !ok || trade.Price <= 0 {
		return logic.Measurement{}, nil
	}

	if eventAt.IsZero() {
		return logic.Measurement{}, errnie.Error(fmt.Errorf("leadlag: trade event time is zero"))
	}

	leadLagSection.observePrice(signal.symbol, trade.Price, eventAt)

	return signal.fromLag(eventAt)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	ticker, ok := signal.latest().(*krakenmarket.TickerUpdate)

	if !ok {
		return logic.Measurement{}, nil
	}

	price := ticker.Last

	if price <= 0 {
		price = (ticker.Ask + ticker.Bid) / 2
	}

	if price <= 0 {
		return logic.Measurement{}, nil
	}

	if at.IsZero() {
		return logic.Measurement{}, errnie.Error(fmt.Errorf("leadlag: ticker event time is zero"))
	}

	leadLagSection.observePrice(signal.symbol, price, at)

	return signal.fromLag(at)
}

func (signal *Signal) measureBook(_ time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, nil
}

func (signal *Signal) latest() any {
	var latest any

	signal.measurements.Do(func(item any) {
		if item != nil {
			latest = item
		}
	})

	return latest
}

func (signal *Signal) fromLag(at time.Time) (logic.Measurement, error) {
	move := leadLagSection.anchorMove()
	anchor := leadLagSection.anchorState()

	if anchor == nil {
		return logic.Measurement{}, nil
	}

	if signal.symbol == anchorSymbol() {
		return signal.fromAnchor(move, anchor.lastPrice(), at)
	}

	return signal.fromFollower(move, anchor, at)
}

func (signal *Signal) fromAnchor(move anchorMove, price float64, at time.Time) (logic.Measurement, error) {
	if !move.ready || move.moved || price <= 0 {
		return logic.Measurement{}, nil
	}

	return signal.publish(
		logic.CategoryAnchorStall,
		price,
		move.stallMargin,
		signal.componentMargin(move.stallMargin),
		0,
		0,
		at,
	)
}

func (signal *Signal) fromFollower(move anchorMove, anchor *symbolState, at time.Time) (logic.Measurement, error) {
	if !move.ready || !move.moved {
		return logic.Measurement{}, nil
	}

	follower := leadLagSection.ensure(signal.symbol)

	if follower == nil {
		return logic.Measurement{}, nil
	}

	price := follower.lastPrice()

	if price <= 0 {
		return logic.Measurement{}, nil
	}

	lagBars, corr, lagOK := follower.crossLag(anchor)

	if lagOK {
		return signal.publishLag(price, lagBars, corr, at)
	}

	contemporaneous, corrOK := follower.contemporaneous(anchor)

	if !corrOK {
		return logic.Measurement{}, nil
	}

	return signal.publishContemporaneous(price, contemporaneous, at)
}

func (signal *Signal) publishLag(price float64, lagBars int, corr float64, at time.Time) (logic.Measurement, error) {
	lagFraction := float64(lagBars) / float64(maxLagBars)
	threshold := minLagFraction()

	category := logic.CategorySynchronizedDrift
	inefficientScore := 0.0
	syncScore := 0.0

	if lagFraction >= threshold {
		category = logic.CategoryInefficientLag
		inefficientScore = lagFraction * corr
	}

	if category == logic.CategorySynchronizedDrift {
		syncScore = corr * (threshold - lagFraction)
	}

	return signal.publish(
		category,
		price,
		lagFraction,
		inefficientScore,
		syncScore,
		0,
		at,
	)
}

func (signal *Signal) publishContemporaneous(price, corr float64, at time.Time) (logic.Measurement, error) {
	sampleCount := minLagSamples
	significance := 1 / (2 * math.Sqrt(float64(sampleCount)))

	category := logic.CategoryDecoupledMove
	decoupledScore := math.Max(0, significance-corr)
	syncScore := 0.0

	if corr >= significance {
		category = logic.CategorySynchronizedDrift
		syncScore = corr
		decoupledScore = 0
	}

	return signal.publish(
		category,
		price,
		math.Max(0, corr),
		0,
		syncScore,
		decoupledScore,
		at,
	)
}

func (signal *Signal) publish(
	category logic.CategoryType,
	price, strength float64,
	inefficientScore, syncScore, decoupledScore float64,
	at time.Time,
) (logic.Measurement, error) {
	stallScore := 0.0

	if category == logic.CategoryAnchorStall {
		stallScore = strength
	}

	probabilities, err := numeric.SoftmaxScores([]float64{
		inefficientScore,
		syncScore,
		decoupledScore,
		stallScore,
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

	if category == logic.CategoryAnchorStall {
		margin := signal.componentMargin(strength)

		if margin > confidence {
			confidence = margin
		}
	}

	row, elapsed, volume, spread, err := signalsupport.RingMarketRow(signal.symbol, signal.measurements, at)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	if row == nil {
		return logic.Measurement{}, nil
	}

	return logic.Measurement{
		Source:     logic.SourceLeadLag,
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

func (signal *Signal) componentMargin(value float64) float64 {
	if value <= 0 {
		return 0
	}

	return value / (1 + value)
}

func (state *symbolState) recentPathMove(window time.Duration) (float64, bool) {
	var buffer [priceHistoryCap]numeric.PriceSample

	samples := state.priceSamplesInto(buffer[:0])

	if len(samples) < minLagSamples || window <= 0 {
		return 0, false
	}

	latest := samples[len(samples)-1]
	cutoff := latest.At.Add(-window)
	startIndex := -1

	for index, sample := range samples {
		if !sample.At.Before(cutoff) {
			startIndex = index

			break
		}
	}

	if startIndex < 0 {
		return 0, false
	}

	start := samples[startIndex]

	if start.Price <= 0 || latest.Price <= 0 {
		return 0, false
	}

	if latest.At.Sub(start.At) < window/2 {
		return 0, false
	}

	return math.Abs(math.Log(latest.Price / start.Price)), true
}

func (baseline *moveBaseline) evaluate(recentMove float64) (moved bool, stallMargin float64, ready bool) {
	if baseline.moments.Observations() < baseline.minObs {
		_ = baseline.moments.Update(recentMove, baseline.alpha)

		return false, 0, false
	}

	mean := baseline.moments.Mean()
	historicalVar := baseline.moments.VarianceEWMA()

	if historicalVar < 0 {
		historicalVar = 0
	}

	floorVar := baseline.minMove * baseline.minMove
	threshold := mean + math.Sqrt(historicalVar+floorVar)
	moved = recentMove > threshold

	if !moved && threshold > 0 {
		stallMargin = (threshold - recentMove) / threshold
	}

	_ = baseline.moments.Update(recentMove, baseline.alpha)

	return moved, stallMargin, true
}

func (crossSection *crossSection) anchorMove() anchorMove {
	anchor := crossSection.anchorState()

	if anchor == nil {
		return anchorMove{}
	}

	window := time.Duration(maxLagBars) * barInterval
	recentMove, ok := anchor.recentPathMove(window)

	if !ok {
		return anchorMove{}
	}

	moved, stallMargin, ready := crossSection.anchorBaseline.evaluate(recentMove)

	return anchorMove{moved: moved, stallMargin: stallMargin, ready: ready}
}

func (state *symbolState) crossLag(anchor *symbolState) (int, float64, bool) {
	var anchorBuffer [priceHistoryCap]numeric.PriceSample
	var stateBuffer [priceHistoryCap]numeric.PriceSample
	var shiftBuffer [priceHistoryCap]numeric.PriceSample

	anchorSeries := anchor.priceSamplesInto(anchorBuffer[:0])
	stateSeries := state.priceSamplesInto(stateBuffer[:0])

	if len(anchorSeries) < minLagSamples || len(stateSeries) < minLagSamples {
		return 0, 0, false
	}

	sampleCount := len(anchorSeries)

	if len(stateSeries) < sampleCount {
		sampleCount = len(stateSeries)
	}

	baseline := 0.0

	if corr, ok := numeric.HayashiYoshidaCorrelation(anchorSeries, stateSeries); ok {
		baseline = corr
	}

	bestCorr := 0.0
	bestLag := 0

	for lag := 1; lag <= maxLagBars; lag++ {
		shifted := numeric.ShiftPriceSamplesInto(
			shiftBuffer[:0], anchorSeries, time.Duration(lag)*barInterval,
		)
		corr, ok := numeric.HayashiYoshidaCorrelation(shifted, stateSeries)

		if ok && corr > bestCorr {
			bestCorr = corr
			bestLag = lag
		}
	}

	significance := 1 / (2 * math.Sqrt(float64(sampleCount)))

	if bestLag <= 0 || bestCorr <= significance {
		return 0, 0, false
	}

	floor := baseline

	if floor < 0 {
		floor = 0
	}

	margin := significance

	if relative := significance * math.Abs(baseline); relative > margin {
		margin = relative
	}

	if bestCorr <= floor+margin {
		return 0, 0, false
	}

	return bestLag, bestCorr, true
}

func (state *symbolState) contemporaneous(anchor *symbolState) (float64, bool) {
	var anchorBuffer [priceHistoryCap]numeric.PriceSample
	var stateBuffer [priceHistoryCap]numeric.PriceSample

	anchorSeries := anchor.priceSamplesInto(anchorBuffer[:0])
	stateSeries := state.priceSamplesInto(stateBuffer[:0])

	if len(anchorSeries) < minLagSamples || len(stateSeries) < minLagSamples {
		return 0, false
	}

	return numeric.HayashiYoshidaCorrelation(anchorSeries, stateSeries)
}

func minLagFraction() float64 {
	return math.Ceil(float64(maxLagBars)/2) / float64(maxLagBars)
}

func (signal *Signal) categoryIndex(category logic.CategoryType) int {
	switch category {
	case logic.CategoryInefficientLag:
		return 1
	case logic.CategorySynchronizedDrift:
		return 2
	case logic.CategoryDecoupledMove:
		return 3
	case logic.CategoryAnchorStall:
		return 4
	default:
		return 0
	}
}

// measureStall supports direct stall classification tests.
func (signal *Signal) measureStall(stallMargin float64, at time.Time) (logic.Measurement, error) {
	return signal.publish(
		logic.CategoryAnchorStall,
		1,
		stallMargin,
		0,
		0,
		0,
		at,
	)
}

// measureFollower supports follower classification tests against explicit states.
func (signal *Signal) measureFollower(anchor, follower *symbolState, at time.Time) (logic.Measurement, error) {
	move := leadLagSection.anchorMove()

	if !move.ready || !move.moved {
		return logic.Measurement{}, nil
	}

	price := follower.lastPrice()

	if price <= 0 {
		return logic.Measurement{}, nil
	}

	lagBars, corr, lagOK := follower.crossLag(anchor)

	if lagOK {
		return signal.publishLag(price, lagBars, corr, at)
	}

	contemporaneous, corrOK := follower.contemporaneous(anchor)

	if !corrOK {
		return logic.Measurement{}, nil
	}

	return signal.publishContemporaneous(price, contemporaneous, at)
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
