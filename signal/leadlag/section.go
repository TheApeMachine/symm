package leadlag

import (
	"math"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/statutil"
)

const (
	priceHistoryCap   = 256
	minLagSamples     = 16
	maxLagBars        = 12
	anchorMoveMinObs  = 12
	barInterval       = 5 * time.Minute
	ringSampleSpacing = 15 * time.Second
)

/*
priceSample is one observed price point on a symbol path.
*/
type priceSample struct {
	at    time.Time
	value float64
}

/*
Section tracks anchor-relative price paths for lead-lag scoring.
*/
type Section struct {
	universe     sync.Map
	anchorSymbol string
	moveHistory  []float64
}

/*
LagFeatures holds derived lag inputs for classification.
*/
type LagFeatures struct {
	IsAnchor    bool
	Price       float64
	MoveReady   bool
	MoveMoved   bool
	StallMargin float64
	LagOK       bool
	LagBars     int
	LagCorr     float64
	ContempOK   bool
	ContempCorr float64
	SampleCount int
	ObservedAt  time.Time
}

func NewSection(anchorSymbol string) *Section {
	resolved := anchorSymbol

	if resolved == "" {
		resolved = "BTC/USD"
	}

	return &Section{
		anchorSymbol: resolved,
		moveHistory:  make([]float64, 0, anchorMoveMinObs*2),
	}
}

func NewSectionFromConfig() (*Section, error) {
	anchor := strings.TrimSpace(viper.GetString("market.anchor_symbol"))

	return NewSection(anchor), nil
}

func (section *Section) AnchorSymbol() string {
	return section.anchorSymbol
}

/*
PriceSampleCount returns how many spaced price samples are buffered for symbol.
*/
func (section *Section) PriceSampleCount(symbol string) int {
	state := section.ensure(symbol)

	if state == nil {
		return 0
	}

	return len(state.prices)
}

type symbolState struct {
	last         float64
	lastSampleAt time.Time
	prices       []priceSample
}

func (section *Section) ensure(symbol string) *symbolState {
	raw, _ := section.universe.LoadOrStore(symbol, &symbolState{})

	state, ok := raw.(*symbolState)

	if !ok {
		return nil
	}

	return state
}

func (section *Section) ObservePrice(symbol string, price float64, at time.Time) {
	if symbol == "" || price <= 0 || at.IsZero() {
		return
	}

	state := section.ensure(symbol)

	if state == nil {
		return
	}

	state.last = price

	if !state.lastSampleAt.IsZero() && at.Sub(state.lastSampleAt) < ringSampleSpacing {
		return
	}

	state.lastSampleAt = at
	state.prices = append(state.prices, priceSample{at: at, value: price})

	if len(state.prices) > priceHistoryCap {
		state.prices = state.prices[len(state.prices)-priceHistoryCap:]
	}
}

func (section *Section) anchorState() *symbolState {
	return section.ensure(section.anchorSymbol)
}

func (section *Section) Features(scope string) LagFeatures {
	anchor := section.anchorState()
	follower := section.ensure(scope)

	if anchor == nil || follower == nil {
		return LagFeatures{}
	}

	price := follower.last

	if scope == section.anchorSymbol {
		price = anchor.last
	}

	move := section.anchorMove(anchor.prices)
	features := LagFeatures{
		IsAnchor:    scope == section.anchorSymbol,
		Price:       price,
		MoveReady:   move.ready,
		MoveMoved:   move.moved,
		StallMargin: move.stallMargin,
		ObservedAt:  follower.lastSampleAt,
	}

	if features.IsAnchor {
		return features
	}

	anchorSeries := append([]priceSample(nil), anchor.prices...)
	followerSeries := append([]priceSample(nil), follower.prices...)
	sampleCount := len(anchorSeries)

	if len(followerSeries) < sampleCount {
		sampleCount = len(followerSeries)
	}

	features.SampleCount = sampleCount

	contempCorr, contempOK := pairCorrelation(anchorSeries, followerSeries, 0)
	features.ContempOK = contempOK
	features.ContempCorr = contempCorr

	lagBars, lagCorr, lagOK := crossLagScore(anchorSeries, followerSeries, barInterval)
	features.LagOK = lagOK
	features.LagBars = lagBars
	features.LagCorr = lagCorr

	if features.ObservedAt.IsZero() {
		features.ObservedAt = anchor.lastSampleAt
	}

	return features
}

type anchorMove struct {
	moved       bool
	stallMargin float64
	ready       bool
}

func (section *Section) anchorMove(samples []priceSample) anchorMove {
	recentMove, ok := recentPathMove(samples, time.Duration(maxLagBars)*barInterval)

	if !ok {
		return anchorMove{}
	}

	threshold := section.moveThreshold(recentMove)
	ready := len(section.moveHistory) >= anchorMoveMinObs
	moved := ready && recentMove > threshold

	stallMargin := 0.0

	if threshold > 0 {
		stallMargin = math.Max(0, threshold-recentMove) / threshold
	}

	section.moveHistory = append(section.moveHistory, recentMove)

	if len(section.moveHistory) > priceHistoryCap {
		section.moveHistory = section.moveHistory[len(section.moveHistory)-priceHistoryCap:]
	}

	return anchorMove{
		moved:       moved,
		stallMargin: stallMargin,
		ready:       ready,
	}
}

func (section *Section) moveThreshold(sample float64) float64 {
	if len(section.moveHistory) < anchorMoveMinObs {
		return sample
	}

	median := statutil.Median(section.moveHistory)
	mean, std := meanStdDev(section.moveHistory)

	_ = mean

	return median + std
}

func recentPathMove(samples []priceSample, window time.Duration) (float64, bool) {
	if len(samples) < minLagSamples || window <= 0 {
		return 0, false
	}

	latest := samples[len(samples)-1]
	cutoff := latest.at.Add(-window)
	startIndex := -1

	for index, sample := range samples {
		if !sample.at.Before(cutoff) {
			startIndex = index

			break
		}
	}

	if startIndex < 0 {
		return 0, false
	}

	start := samples[startIndex]

	if start.value <= 0 || latest.value <= 0 {
		return 0, false
	}

	minimumSpan := ringSampleSpacing * time.Duration(minLagSamples-1)

	if latest.at.Sub(start.at) < minimumSpan {
		return 0, false
	}

	return math.Abs(math.Log(latest.value / start.value)), true
}

func pairCorrelation(left, right []priceSample, lag time.Duration) (float64, bool) {
	leftReturns, rightReturns := alignedReturns(left, right, lag)

	if len(leftReturns) < minLagSamples {
		return 0, false
	}

	correlation := pearson(leftReturns, rightReturns)

	if math.IsNaN(correlation) {
		return 0, false
	}

	return correlation, true
}

func crossLagScore(anchor, follower []priceSample, interval time.Duration) (int, float64, bool) {
	bestBars := 0
	bestCorr := 0.0
	found := false

	for bars := 1; bars <= maxLagBars; bars++ {
		lag := time.Duration(bars) * interval
		correlation, ok := pairCorrelation(anchor, follower, lag)

		if !ok {
			continue
		}

		if !found || math.Abs(correlation) > math.Abs(bestCorr) {
			bestBars = bars
			bestCorr = correlation
			found = true
		}
	}

	return bestBars, bestCorr, found
}

func alignedReturns(left, right []priceSample, lag time.Duration) ([]float64, []float64) {
	leftReturns := logReturns(left)
	rightReturns := logReturns(right)

	if lag == 0 {
		count := len(leftReturns)

		if len(rightReturns) < count {
			count = len(rightReturns)
		}

		if count < minLagSamples {
			return nil, nil
		}

		return leftReturns[len(leftReturns)-count:], rightReturns[len(rightReturns)-count:]
	}

	shift := int(lag / ringSampleSpacing)

	if shift <= 0 || shift >= len(leftReturns) || shift >= len(rightReturns) {
		return nil, nil
	}

	leftTail := leftReturns[:len(leftReturns)-shift]
	rightTail := rightReturns[shift:]
	count := len(leftTail)

	if len(rightTail) < count {
		count = len(rightTail)
	}

	if count < minLagSamples {
		return nil, nil
	}

	return leftTail[len(leftTail)-count:], rightTail[len(rightTail)-count:]
}

func logReturns(samples []priceSample) []float64 {
	if len(samples) < 2 {
		return nil
	}

	returns := make([]float64, 0, len(samples)-1)

	for index := 1; index < len(samples); index++ {
		previous := samples[index-1].value
		current := samples[index].value

		if previous <= 0 || current <= 0 {
			continue
		}

		returns = append(returns, math.Log(current/previous))
	}

	return returns
}

func pearson(left, right []float64) float64 {
	if len(left) != len(right) || len(left) == 0 {
		return math.NaN()
	}

	meanLeft, stdLeft := meanStdDev(left)
	meanRight, stdRight := meanStdDev(right)

	if stdLeft <= 0 || stdRight <= 0 {
		return math.NaN()
	}

	covariance := 0.0

	for index := range left {
		covariance += (left[index] - meanLeft) * (right[index] - meanRight)
	}

	covariance /= float64(len(left))

	return covariance / (stdLeft * stdRight)
}

func meanStdDev(values []float64) (mean float64, std float64) {
	if len(values) == 0 {
		return 0, 0
	}

	for _, value := range values {
		mean += value
	}

	mean /= float64(len(values))

	for _, value := range values {
		delta := value - mean
		std += delta * delta
	}

	std = math.Sqrt(std / float64(len(values)))

	return mean, std
}
