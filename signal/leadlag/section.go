package leadlag

import (
	"math"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/statutil"
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

type symbolState struct {
	last          float64
	lastSampleAt  time.Time
	prices        []priceSample
	observedCount int
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
		moveHistory:  make([]float64, 0, minLagSamplesFloor*2),
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

	if !state.lastSampleAt.IsZero() {
		spacing := seriesSampleSpacing(state.prices, nil)

		if spacing > 0 && at.Sub(state.lastSampleAt) < spacing {
			return
		}
	}

	state.lastSampleAt = at
	state.observedCount++
	state.prices = append(state.prices, priceSample{at: at, value: price})

	capacity := priceHistoryCapacity(state.observedCount)

	if len(state.prices) > capacity {
		state.prices = state.prices[len(state.prices)-capacity:]
	}

	if symbol == section.anchorSymbol {
		section.recordAnchorMove(state.prices)
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

	sampleSpacing := seriesSampleSpacing(anchorSeries, followerSeries)
	lagBars, lagCorr, lagOK := crossLagScore(
		anchorSeries,
		followerSeries,
		sampleSpacing,
		section.maxLagBars(sampleCount),
	)
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

func (section *Section) recordAnchorMove(samples []priceSample) {
	if len(samples) < 2 {
		return
	}

	window := samples[len(samples)-1].at.Sub(samples[0].at)
	recentMove, ok := recentPathMove(samples, window)

	if !ok {
		return
	}

	section.moveHistory = append(section.moveHistory, recentMove)

	moveCap := priceHistoryCapacity(len(section.moveHistory))

	if len(section.moveHistory) > moveCap {
		section.moveHistory = section.moveHistory[len(section.moveHistory)-moveCap:]
	}
}

func (section *Section) anchorMove(samples []priceSample) anchorMove {
	if len(samples) < 2 {
		return anchorMove{}
	}

	window := samples[len(samples)-1].at.Sub(samples[0].at)
	recentMove, ok := recentPathMove(samples, window)

	if !ok {
		return anchorMove{}
	}

	threshold := section.moveThreshold(recentMove)
	ready := len(section.moveHistory) >= minLagSamplesFloor
	moved := ready && recentMove > threshold

	stallMargin := 0.0

	if threshold > 0 {
		stallMargin = math.Max(0, threshold-recentMove) / threshold
	}

	return anchorMove{
		moved:       moved,
		stallMargin: stallMargin,
		ready:       ready,
	}
}

func (section *Section) moveThreshold(sample float64) float64 {
	minSamples := minCorrelationSamples(len(section.moveHistory) + 1)

	if len(section.moveHistory) < minSamples {
		return sample
	}

	median := statutil.Median(section.moveHistory)
	mean, std := meanStdDev(section.moveHistory)

	_ = mean

	return median + std
}

func (section *Section) maxLagBars(sampleCount int) int {
	return maxLagBarsForCount(sampleCount)
}
